package stream

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

// Limits that keep a hostile server from exhausting memory or looping us forever.
const (
	maxPlaylistBytes = 10 << 20  // playlists and manifests
	maxSegmentBytes  = 256 << 20 // one media segment (held in memory to keep order)
	maxKeyBytes      = 16        // AES-128 key
	maxSegments      = 200_000   // per playlist / representation
	maxAttempts      = 5         // per request
	rangeChunk       = 8 << 20   // a single-file DASH representation is fetched in chunks of this size
)

// resource is a URL plus an optional byte range (length < 0 means "whole resource").
type resource struct {
	URL    string
	Offset int64
	Length int64
}

func (r resource) key() string { return fmt.Sprintf("%s|%d|%d", r.URL, r.Offset, r.Length) }

// resolve resolves ref against base and allows only http(s) results.
func resolve(base *url.URL, ref string) (*url.URL, error) {
	ref = strings.TrimSpace(ref)
	u, err := url.Parse(ref)
	if err != nil {
		return nil, fmt.Errorf("bad URI in playlist: %w", err)
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URI scheme %q in playlist (only http and https)", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("URI without host in playlist")
	}
	return u, nil
}

// statusErr is a non-2xx HTTP answer.
type statusErr struct{ code int }

func (e *statusErr) Error() string { return fmt.Sprintf("HTTP %d %s", e.code, http.StatusText(e.code)) }

func retryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var se *statusErr
	if errors.As(err, &se) {
		return se.code >= 500 || se.code == http.StatusRequestTimeout || se.code == http.StatusTooManyRequests
	}
	var tooBig *tooBigErr
	return !errors.As(err, &tooBig)
}

type tooBigErr struct{ what string }

func (e *tooBigErr) Error() string { return e.what + " is too large" }

// client wraps the engine's HTTP client with the browser headers and byte accounting.
type client struct {
	hc      *http.Client
	headers map[string]string
	ua      string
	origin  string                                 // host of the URL the user added
	count   func(ctx context.Context, n int) error // Job.Received (nil for manifests)
	backoff time.Duration
}

// sendSecrets reports whether Cookie/Authorization may go to u: the same host as the
// original URL or one of its subdomains. A CDN on an unrelated host never gets them.
func (c *client) sendSecrets(u *url.URL) bool {
	h := strings.ToLower(u.Hostname())
	return h == c.origin || strings.HasSuffix(h, "."+c.origin)
}

func (c *client) request(ctx context.Context, u *url.URL, rng string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	secrets := c.sendSecrets(u)
	for k, v := range c.headers {
		switch strings.ToLower(k) {
		case "range", "accept-encoding", "host":
			continue
		case "cookie", "authorization":
			if !secrets {
				continue
			}
		}
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		ua := c.ua
		if ua == "" {
			ua = engine.DefaultUserAgent
		}
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Set("Accept-Encoding", "identity")
	if rng != "" {
		req.Header.Set("Range", rng)
	}
	return req, nil
}

// get fetches a resource fully into memory (at most limit bytes), retrying transient
// failures. counted bytes go through c.count so speed limits apply.
func (c *client) get(ctx context.Context, r resource, limit int64, what string, counted bool) ([]byte, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, fmt.Errorf("%s: bad URL: %w", what, err)
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			wait := c.backoff << (attempt - 1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(min(wait, 8*time.Second)):
			}
		}
		b, err := c.getOnce(ctx, u, r, limit, what, counted)
		if err == nil {
			return b, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = err
		if !retryable(err) {
			break
		}
	}
	return nil, fmt.Errorf("%s: %w", what, lastErr)
}

func (c *client) getOnce(ctx context.Context, u *url.URL, r resource, limit int64, what string, counted bool) ([]byte, error) {
	rng := ""
	if r.Length >= 0 {
		if r.Length > limit {
			return nil, &tooBigErr{what}
		}
		rng = fmt.Sprintf("bytes=%d-%d", r.Offset, r.Offset+r.Length-1)
	}
	req, err := c.request(ctx, u, rng)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, scrub(err)
	}
	defer resp.Body.Close()
	skip := int64(0)
	switch {
	case resp.StatusCode == http.StatusPartialContent && rng != "":
	case resp.StatusCode == http.StatusOK:
		if rng != "" { // server ignored Range: cut our part out of the full body
			skip = r.Offset
		}
	default:
		return nil, &statusErr{resp.StatusCode}
	}
	want := limit
	if r.Length >= 0 {
		want = r.Length
	}
	var buf bytes.Buffer
	body := io.Reader(resp.Body)
	if skip > 0 {
		if skip > maxSegmentBytes {
			return nil, &tooBigErr{what}
		}
		if _, err := io.CopyN(io.Discard, body, skip); err != nil {
			return nil, fmt.Errorf("read: %w", scrub(err))
		}
	}
	chunk := make([]byte, 32<<10)
	for {
		n, rerr := body.Read(chunk)
		if n > 0 {
			if int64(buf.Len()+n) > want {
				if r.Length < 0 {
					return nil, &tooBigErr{what}
				}
				n = int(want - int64(buf.Len())) // ranged: keep only what we asked for
			}
			buf.Write(chunk[:n])
			if counted && c.count != nil {
				if err := c.count(ctx, n); err != nil {
					return nil, err
				}
			}
			if r.Length >= 0 && int64(buf.Len()) == want {
				break
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, fmt.Errorf("read: %w", scrub(rerr))
		}
	}
	if r.Length >= 0 && int64(buf.Len()) != r.Length {
		return nil, fmt.Errorf("short read: got %d of %d bytes", buf.Len(), r.Length)
	}
	return buf.Bytes(), nil
}

// size learns a resource's length with a one-byte range request. ok is false when the
// server does not support ranges.
func (c *client) size(ctx context.Context, rawURL string) (n int64, ok bool, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, false, fmt.Errorf("bad URL: %w", err)
	}
	req, err := c.request(ctx, u, "bytes=0-0")
	if err != nil {
		return 0, false, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, false, scrub(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1))
	if resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode == http.StatusOK {
			return resp.ContentLength, false, nil
		}
		return 0, false, &statusErr{resp.StatusCode}
	}
	cr := resp.Header.Get("Content-Range")
	_, total, found := strings.Cut(cr, "/")
	if !found {
		return 0, false, nil
	}
	n, perr := strconv.ParseInt(strings.TrimSpace(total), 10, 64)
	if perr != nil || n <= 0 {
		return 0, false, nil
	}
	return n, true, nil
}

// scrub removes the URL (with its query string, often a token) from net/http errors.
func scrub(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		var ne net.Error
		if errors.As(ue.Err, &ne) && ne.Timeout() {
			return fmt.Errorf("%s: timeout", ue.Op)
		}
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

// decryptAES128 decrypts an AES-128-CBC segment with PKCS#7 padding (HLS METHOD=AES-128).
func decryptAES128(data, key, iv []byte) ([]byte, error) {
	if len(key) != 16 || len(iv) != 16 {
		return nil, errors.New("AES-128: bad key or IV length")
	}
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, errors.New("AES-128: encrypted segment is not a whole number of blocks")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES-128: %w", err)
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	pad := int(out[len(out)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(out) {
		return nil, errors.New("AES-128: bad padding (wrong key?)")
	}
	for _, b := range out[len(out)-pad:] {
		if int(b) != pad {
			return nil, errors.New("AES-128: bad padding (wrong key?)")
		}
	}
	return out[:len(out)-pad], nil
}

// ivFromSequence is the default HLS IV: the media sequence number as a 128-bit big-endian.
func ivFromSequence(seq int64) []byte {
	iv := make([]byte, 16)
	binary.BigEndian.PutUint64(iv[8:], uint64(seq))
	return iv
}
