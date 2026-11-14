package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultUserAgent is sent when neither the request nor settings provide one.
var DefaultUserAgent = "Mozilla/5.0 (compatible; Peregrine/" + Version + "; +https://github.com/Salim8519/Peregrine)"

// Version is the engine version, overridden at build time with -ldflags.
var Version = "0.4.1"

// NewHTTPClient returns a client that uses the given proxy setting ("", "system" or a URL),
// for the app's own small requests (the update check).
func NewHTTPClient(proxy string) (*http.Client, error) { return newClient(proxy) }

// newClient builds an HTTP client for segmented downloads.
//
// HTTP/2 is deliberately disabled: it multiplexes every segment over one TCP connection,
// which removes the speed gain of parallel connections (see ADR 0006).
func newClient(proxy string) (*http.Client, error) {
	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{}, // HTTP/1.1 only
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   MaxConnections,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    true, // we need raw byte ranges
	}
	switch strings.TrimSpace(proxy) {
	case "", "none":
		tr.Proxy = nil
	case "system":
		tr.Proxy = http.ProxyFromEnvironment
	default:
		u, err := url.Parse(proxy)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("invalid proxy %q", proxy)
		}
		switch u.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Keep the caller's headers (cookies, referer) on same-host redirects only.
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				req.Header.Del("Cookie")
				req.Header.Del("Authorization")
			}
			return nil
		},
	}, nil
}

// probeResult is what we learn about a remote file before downloading.
type probeResult struct {
	FinalURL  string
	Size      int64 // -1 unknown
	Ranges    bool
	ETag      string
	LastMod   string
	Filename  string
	StatusErr error
}

func setHeaders(req *http.Request, headers map[string]string, ua string) {
	for k, v := range headers {
		if strings.EqualFold(k, "Range") || strings.EqualFold(k, "Accept-Encoding") || strings.EqualFold(k, "Host") {
			continue
		}
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		if ua == "" {
			ua = DefaultUserAgent
		}
		req.Header.Set("User-Agent", ua)
	}
	req.Header.Set("Accept-Encoding", "identity")
}

// probe asks for the first byte with a Range request: this tells us the size, whether
// ranges work, and the validators, in one round trip that works even where HEAD is broken.
func probe(ctx context.Context, c *http.Client, rawURL string, headers map[string]string, ua string) (*probeResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("bad URL: %w", err)
	}
	setHeaders(req, headers, ua)
	req.Header.Set("Range", "bytes=0-0")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 1<<16)

	if err := statusError(resp.StatusCode); err != nil {
		return nil, err
	}
	pr := &probeResult{
		FinalURL: resp.Request.URL.String(),
		Size:     -1,
		ETag:     resp.Header.Get("ETag"),
		LastMod:  resp.Header.Get("Last-Modified"),
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		if total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range")); ok {
			pr.Size = total
			pr.Ranges = total > 0
		}
	case http.StatusOK:
		if resp.ContentLength >= 0 {
			pr.Size = resp.ContentLength
		}
	case http.StatusRequestedRangeNotSatisfiable:
		// Empty file.
		if total, ok := parseContentRangeTotal(resp.Header.Get("Content-Range")); ok && total == 0 {
			pr.Size = 0
		}
	}
	pr.Filename = filenameFrom(pr.FinalURL, resp.Header.Get("Content-Disposition"), resp.Header.Get("Content-Type"))
	return pr, nil
}

func statusError(code int) error {
	switch {
	case code == http.StatusForbidden || code == http.StatusGone || code == http.StatusUnauthorized:
		return fmt.Errorf("HTTP %d: %w", code, ErrLinkExpired)
	case code == http.StatusRequestedRangeNotSatisfiable:
		return nil
	case code >= 400:
		return &httpStatusError{code}
	}
	return nil
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d %s", e.code, http.StatusText(e.code))
}

// retryable reports whether the status is worth retrying.
func (e *httpStatusError) retryable() bool {
	return e.code == http.StatusTooManyRequests || e.code >= 500
}

// parseContentRangeTotal parses "bytes 0-0/12345".
func parseContentRangeTotal(v string) (int64, bool) {
	i := strings.LastIndexByte(v, '/')
	if i < 0 || i == len(v)-1 {
		return 0, false
	}
	t := v[i+1:]
	if t == "*" {
		return 0, false
	}
	n, err := strconv.ParseInt(t, 10, 64)
	return n, err == nil && n >= 0
}

// parseContentRangeStart parses the start of "bytes 100-199/1000".
func parseContentRangeStart(v string) (int64, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "bytes ")
	i := strings.IndexByte(v, '-')
	if i <= 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(v[:i], 10, 64)
	return n, err == nil
}
