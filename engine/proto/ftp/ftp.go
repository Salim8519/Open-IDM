// Package ftp downloads files over FTP and FTPS (RFC 959, RFC 2428 EPSV, RFC 3659 SIZE and
// REST, RFC 4217 TLS) as an engine.Protocol of kind "ftp".
//
//   - ftp://host/path  — plain FTP; upgraded to TLS with AUTH TLS when the server offers it.
//   - ftps://host/path — implicit TLS (port 990 by default).
//
// Credentials come from the URL (ftp://user:pass@host/path), else anonymous. Passwords never
// appear in errors. Transfers use passive mode only and always connect the data channel to
// the control connection's IP address, ignoring the address in a PASV reply.
package ftp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

// Kind is the engine download kind handled by this package.
const Kind = "ftp"

// Timeouts, modelled on the engine's HTTP client.
const (
	dialTimeout      = 30 * time.Second // like the HTTP dialer
	handshakeTimeout = 20 * time.Second // like TLSHandshakeTimeout
	replyTimeout     = 60 * time.Second // like ResponseHeaderTimeout: one control reply
	idleTimeout      = 60 * time.Second // no data at all for this long = stalled
	quitTimeout      = 5 * time.Second
)

// Retry policy for transient failures (dropped connections, 4xx replies). The engine does
// not retry protocol downloads itself.
const (
	maxAttempts  = 5
	retryBackoff = time.Second
	maxBackoff   = 30 * time.Second
)

const (
	anonymousUser = "anonymous"
	anonymousPass = "anonymous@" // the customary "e-mail address" for anonymous FTP
)

// Protocol is the FTP/FTPS engine.Protocol. The zero value is not usable; call New.
type Protocol struct {
	tlsConfig *tls.Config // base TLS config (nil = system roots, normal verification)
	backoff   time.Duration
}

// Option configures a Protocol.
type Option func(*Protocol)

// WithTLSConfig sets the base TLS configuration, e.g. custom root CAs. ServerName is filled
// in per connection. Production code should not need this; tests use it to trust a
// self-signed certificate.
func WithTLSConfig(c *tls.Config) Option {
	return func(p *Protocol) { p.tlsConfig = c }
}

// New returns the FTP protocol, ready for engine.Manager.RegisterProtocol.
func New(opts ...Option) *Protocol {
	p := &Protocol{backoff: retryBackoff}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Kind implements engine.Protocol.
func (*Protocol) Kind() string { return Kind }

// Match implements engine.Protocol: ftp:// and ftps:// URLs.
func (*Protocol) Match(rawURL string) bool {
	s := strings.TrimSpace(rawURL)
	return hasPrefixFold(s, "ftp://") || hasPrefixFold(s, "ftps://")
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// state is the resume state saved with the download.
type state struct {
	// Size is the remote size seen by the previous run (-1 unknown, 0 not yet seen).
	// A different size on resume means the file changed: start again from zero.
	Size int64 `json:"size"`
}

// Run implements engine.Protocol.
func (p *Protocol) Run(ctx context.Context, j *engine.Job) error {
	t, err := parseTarget(j.URL)
	if err != nil {
		return err
	}
	var st state
	if len(j.State) > 0 {
		_ = json.Unmarshal(j.State, &st) // unreadable state = fresh start, still safe
	}
	failures := 0
	for {
		progressed, err := p.attempt(ctx, j, t, &st)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !retryable(err) {
			return err
		}
		if progressed {
			failures = 0
		}
		failures++
		if failures >= maxAttempts {
			return fmt.Errorf("giving up after %d attempts: %w", failures, err)
		}
		delay := min(p.backoff<<(failures-1), maxBackoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// attempt runs one FTP session: connect, log in, transfer what is missing. progressed
// reports whether new bytes reached the part file.
func (p *Protocol) attempt(ctx context.Context, j *engine.Job, t *target, st *state) (progressed bool, err error) {
	s := newSession(p, t)
	stop := context.AfterFunc(ctx, s.closeAll) // cancel = close every socket, unblocking I/O
	defer stop()
	defer s.closeAll()

	if err := s.connect(ctx, true); err != nil {
		var te *tlsError
		if !errors.As(err, &te) || t.implicitTLS || !t.anonymous || !te.afterAuth {
			return false, err
		}
		// The server offered AUTH TLS but the handshake failed. Without a password at stake
		// it is safe to carry on in plain FTP, which is what ftp:// asked for anyway.
		s.dropConns()
		if err := s.connect(ctx, false); err != nil {
			return false, err
		}
	}
	if err := s.login(); err != nil {
		return false, err
	}
	size, sizeKnown := s.size()
	if !sizeKnown {
		size = -1
	}
	j.SetSize(size)
	j.SetResumable(true)
	changed := st.Size != 0 && st.Size != size
	st.Size = size
	if err := j.SaveState(*st); err != nil {
		return false, err
	}

	f, err := os.OpenFile(j.PartPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return false, permanent(fmt.Errorf("open part file: %w", err))
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, permanent(fmt.Errorf("open part file: %w", err))
	}
	off := info.Size()
	if changed || (sizeKnown && off > size) {
		off = 0 // the remote file changed: the bytes on disk belong to another version
	}
	if err := f.Truncate(off); err != nil {
		return false, permanent(fmt.Errorf("truncate part file: %w", err))
	}
	j.SetDownloaded(off)
	if sizeKnown && off == size {
		s.quit()
		return false, nil // already complete
	}

	j.SetActive(1)
	defer j.SetActive(0)
	start := off
	end, err := s.retrieve(ctx, j, f, off, size)
	progressed = end > start
	if err != nil {
		return progressed, err
	}
	if err := f.Close(); err != nil {
		return progressed, permanent(fmt.Errorf("write part file: %w", err))
	}
	s.quit()
	return progressed, nil
}

// target is a parsed FTP URL.
type target struct {
	implicitTLS bool
	host        string // for TLS verification
	addr        string // host:port
	user, pass  string
	anonymous   bool
	path        string // as sent in SIZE/RETR
}

// errBadURL never includes the URL itself: it may carry a password.
var errBadURL = permanent(errors.New("invalid FTP address"))

// parseTarget reads an ftp:// or ftps:// URL. Errors never include the URL, which may hold
// credentials (url.Parse errors quote the whole input, so they are not wrapped).
func parseTarget(raw string) (*target, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, errBadURL
	}
	t := &target{}
	port := "21"
	switch strings.ToLower(u.Scheme) {
	case "ftp":
	case "ftps":
		t.implicitTLS, port = true, "990"
	default:
		return nil, permanent(errors.New("not an ftp:// or ftps:// address"))
	}
	t.host = u.Hostname()
	if t.host == "" {
		return nil, permanent(errors.New("FTP address has no host"))
	}
	if p := u.Port(); p != "" {
		port = p
	}
	t.addr = net.JoinHostPort(t.host, port)

	t.user, t.pass, t.anonymous = anonymousUser, anonymousPass, true
	if u.User != nil && u.User.Username() != "" {
		t.user, t.anonymous = u.User.Username(), false
		t.pass, _ = u.User.Password()
	}

	// RFC 1738: the path is relative to the login directory; "%2F" makes it absolute
	// (ftp://host/%2Fetc/file → "/etc/file"). A ";type=i" suffix is dropped: we always
	// transfer in binary.
	p := u.Path
	if i := strings.LastIndex(p, ";type="); i >= 0 && len(p)-i == len(";type=")+1 {
		p = p[:i]
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" || strings.HasSuffix(p, "/") {
		return nil, permanent(errors.New("the FTP address must name a file, not a folder"))
	}
	t.path = p
	if strings.ContainsAny(t.path+t.user+t.pass, "\r\n\x00") {
		return nil, permanent(errors.New("FTP address contains control characters"))
	}
	return t, nil
}

// tlsConfig returns a per-session TLS config: the base config with the server name and a
// session cache, so the data channel can resume the control channel's TLS session (many
// servers require that).
func (p *Protocol) tlsConfigFor(host string) *tls.Config {
	var c *tls.Config
	if p.tlsConfig != nil {
		c = p.tlsConfig.Clone()
	} else {
		c = &tls.Config{}
	}
	if c.ServerName == "" {
		c.ServerName = host
	}
	if c.MinVersion < tls.VersionTLS12 {
		c.MinVersion = tls.VersionTLS12
	}
	if c.ClientSessionCache == nil {
		c.ClientSessionCache = tls.NewLRUClientSessionCache(4)
	}
	return c
}
