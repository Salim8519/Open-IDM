package ftp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

// session is one FTP control connection plus its data connections.
type session struct {
	p      *Protocol
	t      *target
	tlsCfg *tls.Config

	ctrl     net.Conn // current control connection (a *tls.Conn once encrypted)
	tp       *textproto.Conn
	remoteIP net.IP
	secure   bool // control channel is TLS: data channels use PROT P
	noEPSV   bool

	mu     sync.Mutex
	conns  []net.Conn // raw sockets, closed on cancel
	closed bool
}

func newSession(p *Protocol, t *target) *session {
	return &session{p: p, t: t}
}

// track registers a raw socket so closeAll can unblock it.
func (s *session) track(c net.Conn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		_ = c.Close()
		return net.ErrClosed
	}
	s.conns = append(s.conns, c)
	return nil
}

// closeAll closes every socket for good (cancel or end of attempt).
func (s *session) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, c := range s.conns {
		_ = c.Close()
	}
	s.conns = nil
}

// dropConns closes the sockets but keeps the session usable for a new connect.
func (s *session) dropConns() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		_ = c.Close()
	}
	s.conns = nil
	s.secure = false
}

func (s *session) dial(ctx context.Context, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if err := s.track(c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *session) setCtrl(c net.Conn) {
	s.ctrl = c
	s.tp = textproto.NewConn(c)
}

// tlsError is a failed TLS handshake. After one, credentials are never sent in plain text.
type tlsError struct {
	afterAuth bool // explicit TLS: the server accepted AUTH TLS, then the handshake failed
	err       error
}

func (e *tlsError) Error() string {
	if e.afterAuth {
		return fmt.Sprintf("the server offers encryption (AUTH TLS) but the secure connection failed, so the password was not sent: %v", e.err)
	}
	return fmt.Sprintf("secure connection (FTPS) failed: %v", e.err)
}
func (e *tlsError) Unwrap() error { return e.err }

// permanent: a refused certificate or protocol mismatch will not fix itself; a dropped
// connection mid-handshake might.
func (e *tlsError) permanent() bool {
	var ne net.Error
	return !errors.As(e.err, &ne) && !errors.Is(e.err, io.EOF) && !errors.Is(e.err, io.ErrUnexpectedEOF)
}

func (s *session) handshake(ctx context.Context, raw net.Conn) (*tls.Conn, error) {
	if s.tlsCfg == nil {
		s.tlsCfg = s.p.tlsConfigFor(s.t.host)
	}
	tc := tls.Client(raw, s.tlsCfg)
	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	if err := tc.HandshakeContext(hctx); err != nil {
		return nil, err
	}
	return tc, nil
}

// connect opens the control connection and reads the greeting. With tryTLS, a plain ftp://
// connection is upgraded with AUTH TLS when the server accepts it.
func (s *session) connect(ctx context.Context, tryTLS bool) error {
	raw, err := s.dial(ctx, s.t.addr)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", s.t.addr, err)
	}
	if a, ok := raw.RemoteAddr().(*net.TCPAddr); ok {
		s.remoteIP = a.IP
	}
	s.setCtrl(raw)
	if s.t.implicitTLS {
		tc, err := s.handshake(ctx, raw)
		if err != nil {
			return &tlsError{err: err}
		}
		s.setCtrl(tc)
		s.secure = true
	}
	code, msg, err := s.read()
	if err != nil {
		return fmt.Errorf("read server greeting: %w", err)
	}
	if code != 220 {
		return s.replyErr("connect", code, msg)
	}
	if s.t.implicitTLS || !tryTLS {
		return nil
	}
	code, _, err = s.cmd("AUTH TLS")
	if err != nil {
		return err
	}
	if code != 234 {
		return nil // no TLS on offer: plain FTP, as the ftp:// address says
	}
	tc, err := s.handshake(ctx, raw)
	if err != nil {
		return &tlsError{afterAuth: true, err: err}
	}
	s.setCtrl(tc)
	s.secure = true
	return nil
}

// read reads one (possibly multi-line) reply.
func (s *session) read() (int, string, error) {
	_ = s.ctrl.SetDeadline(time.Now().Add(replyTimeout))
	code, msg, err := s.tp.ReadResponse(0)
	if err != nil {
		var pe textproto.ProtocolError
		if errors.As(err, &pe) {
			return 0, "", errors.New("malformed reply from the FTP server")
		}
		return 0, "", err
	}
	return code, msg, nil
}

// cmd sends a command and reads its reply. Arguments were checked for CR/LF in parseTarget.
func (s *session) cmd(format string, args ...any) (int, string, error) {
	_ = s.ctrl.SetDeadline(time.Now().Add(replyTimeout))
	if _, err := s.tp.Cmd(format, args...); err != nil {
		return 0, "", err
	}
	return s.read()
}

// login sends USER/PASS and the settings for a binary transfer.
func (s *session) login() error {
	code, msg, err := s.cmd("USER %s", s.t.user)
	if err != nil {
		return err
	}
	if code == 331 {
		code, msg, err = s.cmd("PASS %s", s.t.pass)
		if err != nil {
			return err
		}
	}
	if code != 230 && code != 202 {
		re := s.replyErr("", code, msg)
		if code/100 == 4 {
			return re // e.g. 421 too many users: worth a retry
		}
		if s.t.anonymous {
			return permanent(fmt.Errorf("anonymous login refused; this server needs a user name and password (ftp://user:password@host/path): %w", re))
		}
		return permanent(fmt.Errorf("login failed; check the user name and password: %w", re))
	}
	if s.secure {
		// RFC 4217: protect the data channel too.
		if code, msg, err := s.cmd("PBSZ 0"); err != nil {
			return err
		} else if code/100 != 2 {
			return s.replyErr("PBSZ", code, msg)
		}
		if code, msg, err := s.cmd("PROT P"); err != nil {
			return err
		} else if code/100 != 2 {
			return s.replyErr("PROT P", code, msg)
		}
	}
	if code, msg, err := s.cmd("TYPE I"); err != nil {
		return err
	} else if code/100 != 2 {
		return s.replyErr("TYPE I", code, msg)
	}
	_, _, _ = s.cmd("OPTS UTF8 ON") // non-ASCII names on servers that want it; optional
	return nil
}

// size asks for the file size (RFC 3659). Servers without SIZE give ok=false.
func (s *session) size() (int64, bool) {
	code, msg, err := s.cmd("SIZE %s", s.t.path)
	if err != nil || code != 213 {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(msg), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// openData opens a passive data connection: EPSV, else PASV. The data connection always
// goes to the control connection's IP, never to an address chosen by the server (blocks
// FTP bounce tricks and fixes servers behind NAT that report private addresses).
func (s *session) openData(ctx context.Context) (net.Conn, error) {
	port := -1
	if !s.noEPSV {
		code, msg, err := s.cmd("EPSV")
		if err != nil {
			return nil, err
		}
		if code == 229 {
			port = parseEPSV(msg)
		}
		if port < 0 {
			s.noEPSV = true
		}
	}
	if port < 0 {
		code, msg, err := s.cmd("PASV")
		if err != nil {
			return nil, err
		}
		if code != 227 {
			return nil, s.replyErr("passive mode", code, msg)
		}
		if port = parsePASV(msg); port < 0 {
			return nil, errors.New("malformed passive mode reply from the FTP server")
		}
	}
	addr := net.JoinHostPort(s.remoteIP.String(), strconv.Itoa(port))
	raw, err := s.dial(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("open data connection: %w", err)
	}
	if s.secure {
		// The handshake runs on the first Read, after RETR: some servers only start TLS
		// on the data channel once the transfer command arrived.
		return tls.Client(raw, s.tlsCfg), nil
	}
	return raw, nil
}

// parseEPSV reads "229 Entering Extended Passive Mode (|||6446|)".
func parseEPSV(msg string) int {
	i, k := strings.IndexByte(msg, '('), strings.LastIndexByte(msg, ')')
	if i < 0 || k < i+5 {
		return -1
	}
	inner := msg[i+1 : k] // "|||6446|"
	d := inner[0]
	parts := strings.Split(inner, string(d))
	if len(parts) != 5 {
		return -1
	}
	p, err := strconv.Atoi(parts[3])
	if err != nil || p <= 0 || p > 65535 {
		return -1
	}
	return p
}

// parsePASV reads "227 Entering Passive Mode (h1,h2,h3,h4,p1,p2)"; the host part is ignored.
func parsePASV(msg string) int {
	i := strings.IndexByte(msg, '(')
	k := strings.IndexByte(msg, ')')
	var inner string
	if i >= 0 && k > i {
		inner = msg[i+1 : k]
	} else { // some servers omit the parentheses
		f := strings.Fields(msg)
		if len(f) == 0 {
			return -1
		}
		inner = f[len(f)-1]
	}
	parts := strings.Split(inner, ",")
	if len(parts) != 6 {
		return -1
	}
	hi, err1 := strconv.Atoi(strings.TrimSpace(parts[4]))
	lo, err2 := strconv.Atoi(strings.TrimSpace(parts[5]))
	if err1 != nil || err2 != nil || hi < 0 || hi > 255 || lo < 0 || lo > 255 {
		return -1
	}
	p := hi<<8 | lo
	if p == 0 {
		return -1
	}
	return p
}

// errShort is a transfer that ended before the announced size.
var errShort = errors.New("the server closed the transfer early")

// retrieve downloads from offset off (REST) into f and returns the end position.
func (s *session) retrieve(ctx context.Context, j *engine.Job, f *os.File, off, size int64) (int64, error) {
	data, err := s.openData(ctx)
	if err != nil {
		return off, err
	}
	defer data.Close()
	if off > 0 {
		code, _, err := s.cmd("REST %d", off)
		if err != nil {
			return off, err
		}
		if code != 350 { // no resume on this server: start again from zero
			if err := f.Truncate(0); err != nil {
				return off, permanent(fmt.Errorf("truncate part file: %w", err))
			}
			off = 0
			j.SetDownloaded(0)
		}
	}
	code, msg, err := s.cmd("RETR %s", s.t.path)
	if err != nil {
		return off, err
	}
	if code != 125 && code != 150 {
		re := s.replyErr("", code, msg)
		if code == 550 {
			return off, permanent(fmt.Errorf("file not found or not readable on the server: %w", re))
		}
		return off, fmt.Errorf("download %q: %w", s.t.path, re)
	}

	pos := off
	buf := make([]byte, 64<<10)
	for {
		_ = data.SetReadDeadline(time.Now().Add(idleTimeout))
		n, rerr := data.Read(buf)
		if n > 0 {
			if _, werr := f.WriteAt(buf[:n], pos); werr != nil {
				return pos, permanent(fmt.Errorf("write part file: %w", werr))
			}
			pos += int64(n)
			if err := j.Received(ctx, n); err != nil {
				return pos, err
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			var te *tls.CertificateVerificationError
			if errors.As(rerr, &te) {
				return pos, &tlsError{err: rerr}
			}
			return pos, fmt.Errorf("transfer: %w", rerr)
		}
		if size >= 0 && pos > size {
			return pos, errors.New("the server sent more data than the file size it reported")
		}
	}
	_ = data.Close()

	complete := size < 0 || pos == size
	code, msg, err = s.read() // 226 Transfer complete
	switch {
	case err != nil && complete && size >= 0:
		// Every byte arrived; a control connection dropped by a NAT during a long transfer
		// does not make the file wrong.
	case err != nil:
		return pos, fmt.Errorf("transfer: %w", err)
	case code/100 != 2:
		return pos, s.replyErr("transfer", code, msg)
	}
	if !complete {
		return pos, errShort
	}
	return pos, nil
}

// quit says goodbye politely; failures do not matter.
func (s *session) quit() {
	_ = s.ctrl.SetDeadline(time.Now().Add(quitTimeout))
	if _, err := s.tp.Cmd("QUIT"); err == nil {
		_, _, _ = s.tp.ReadResponse(0)
	}
}

// replyError is an unexpected FTP reply. Its message has the password scrubbed out.
type replyError struct {
	Code int
	Msg  string
}

func (e *replyError) Error() string { return fmt.Sprintf("server replied %d %s", e.Code, e.Msg) }

// permanent: 5xx replies will not change on retry; 4xx are transient (RFC 959).
func (e *replyError) permanent() bool { return e.Code >= 500 }

func (s *session) replyErr(op string, code int, msg string) error {
	re := &replyError{Code: code, Msg: s.scrub(msg)}
	if op == "" {
		return re
	}
	return fmt.Errorf("%s: %w", op, re)
}

// scrub removes the password and control characters from server text and keeps it short.
func (s *session) scrub(msg string) string {
	if !s.t.anonymous && len(s.t.pass) >= 3 {
		msg = strings.ReplaceAll(msg, s.t.pass, "***")
	}
	msg = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, msg)
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	return msg
}

// permanentError marks failures that a retry cannot fix.
type permanentError struct{ err error }

func (e *permanentError) Error() string   { return e.err.Error() }
func (e *permanentError) Unwrap() error   { return e.err }
func (e *permanentError) permanent() bool { return true }

func permanent(err error) error { return &permanentError{err: err} }

// retryable reports whether another attempt may succeed. The outermost error that knows
// decides; anything else (network trouble, early close) is worth a retry.
func retryable(err error) bool {
	type perm interface{ permanent() bool }
	var p perm
	if errors.As(err, &p) {
		return !p.permanent()
	}
	return true
}
