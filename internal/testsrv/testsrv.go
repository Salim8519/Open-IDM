// Package testsrv is an "evil" HTTP file server for tests. It serves deterministic
// content and can misbehave on purpose: drop connections, throttle, refuse ranges,
// lie about length, change the file, or expire links.
//
// Query parameters on /file:
//
//	size=N        file size in bytes (default 1 MiB)
//	seed=S        content seed (default 1); change it to "change the file"
//	norange=1     ignore Range, always 200 with full body
//	nolength=1    chunked response, no Content-Length (unknown size)
//	drop=N        close the connection after N body bytes per request
//	dropfirst=K   only drop on the first K requests (then behave)
//	throttle=B    limit each response to B bytes/s
//	expire=K      return 403 for requests after the first K
//	fail=K        return 503 for the first K requests
//	name=X        send Content-Disposition: attachment; filename="X"
//	etag=X        send ETag X (default derived from size+seed)
//	delay=MS      wait before sending headers
package testsrv

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Server wraps httptest.Server with request statistics.
type Server struct {
	*httptest.Server
	requests  atomic.Int64
	active    atomic.Int64
	maxActive atomic.Int64
	mu        sync.Mutex
	perKey    map[string]int
}

// New starts a server. Call Close when done.
func New() *Server {
	s := &Server{perKey: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/file", s.file)
	mux.HandleFunc("/file/", s.file) // allow /file/name.ext paths
	s.Server = httptest.NewServer(mux)
	return s
}

// Handler returns the raw handler (for running on a fixed port, e.g. e2e tests).
func Handler() (http.Handler, *Server) {
	s := &Server{perKey: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/file", s.file)
	mux.HandleFunc("/file/", s.file)
	return mux, s
}

// URL builds a /file URL with the given query, e.g. s.FileURL("size=1000&drop=100").
func (s *Server) FileURL(query string) string { return s.Server.URL + "/file?" + query }

// Requests returns how many requests were served.
func (s *Server) Requests() int64 { return s.requests.Load() }

// MaxActive returns the peak number of concurrent responses.
func (s *Server) MaxActive() int64 { return s.maxActive.Load() }

// Content returns the deterministic bytes for (size, seed).
func Content(size int64, seed uint64) []byte {
	b := make([]byte, size)
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	for i := range b {
		b[i] = byte(r.UintN(256))
	}
	return b
}

// SHA256 returns the hex SHA-256 of Content(size, seed).
func SHA256(size int64, seed uint64) string {
	h := sha256.Sum256(Content(size, seed))
	return hex.EncodeToString(h[:])
}

var cache sync.Map // key "size/seed" -> []byte

func cached(size int64, seed uint64) []byte {
	k := fmt.Sprintf("%d/%d", size, seed)
	if v, ok := cache.Load(k); ok {
		return v.([]byte)
	}
	b := Content(size, seed)
	cache.Store(k, b)
	return b
}

func qint(r *http.Request, k string, def int64) int64 {
	v := r.URL.Query().Get(k)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	n := s.requests.Add(1)
	a := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		m := s.maxActive.Load()
		if a <= m || s.maxActive.CompareAndSwap(m, a) {
			break
		}
	}
	q := r.URL.Query()
	// Per-URL request counter so expire/fail/dropfirst work per test.
	s.mu.Lock()
	s.perKey[r.URL.RawQuery]++
	nth := s.perKey[r.URL.RawQuery]
	s.mu.Unlock()
	_ = n

	if d := qint(r, "delay", 0); d > 0 {
		time.Sleep(time.Duration(d) * time.Millisecond)
	}
	if k := qint(r, "fail", 0); int64(nth) <= k {
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	if k := qint(r, "expire", -1); k >= 0 && int64(nth) > k {
		http.Error(w, "expired", http.StatusForbidden)
		return
	}

	size := qint(r, "size", 1<<20)
	seed := uint64(qint(r, "seed", 1))
	data := cached(size, seed)
	etag := q.Get("etag")
	if etag == "" {
		etag = fmt.Sprintf(`"%d-%d"`, size, seed)
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", time.Unix(1700000000+int64(seed), 0).UTC().Format(http.TimeFormat))
	w.Header().Set("Content-Type", "application/octet-stream")
	if name := q.Get("name"); name != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	}

	start, end := int64(0), size // [start,end)
	status := http.StatusOK
	noRange := q.Get("norange") == "1"
	if !noRange {
		w.Header().Set("Accept-Ranges", "bytes")
		if ir := r.Header.Get("If-Range"); ir != "" && ir != etag {
			// validator mismatch: send full body (RFC 9110)
		} else if rh := r.Header.Get("Range"); rh != "" {
			st, en, ok := parseRange(rh, size)
			if !ok {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			start, end, status = st, en, http.StatusPartialContent
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, size))
		}
	}
	body := data[start:end]
	if q.Get("nolength") != "1" {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	}
	w.WriteHeader(status)

	drop := qint(r, "drop", -1)
	if df := qint(r, "dropfirst", -1); df >= 0 && int64(nth) > df {
		drop = -1
	}
	throttle := qint(r, "throttle", 0)
	var out io.Writer = w
	flusher, _ := w.(http.Flusher)
	chunk := 32 << 10
	if throttle > 0 {
		chunk = int(max(1024, throttle/20))
	}
	sent := int64(0)
	for len(body) > 0 {
		c := min(chunk, len(body))
		if drop >= 0 && sent+int64(c) > drop {
			c = int(drop - sent)
			if c > 0 {
				_, _ = out.Write(body[:c])
			}
			if flusher != nil {
				flusher.Flush()
			}
			// Abort the connection mid-body.
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					conn.Close()
				}
			}
			panic(http.ErrAbortHandler)
		}
		if _, err := out.Write(body[:c]); err != nil {
			return
		}
		sent += int64(c)
		body = body[c:]
		if throttle > 0 {
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(time.Duration(float64(c) / float64(throttle) * float64(time.Second)))
		}
		if r.Context().Err() != nil {
			return
		}
	}
}

func parseRange(h string, size int64) (int64, int64, bool) {
	h = strings.TrimPrefix(h, "bytes=")
	if strings.Contains(h, ",") {
		return 0, 0, false
	}
	a, b, ok := strings.Cut(h, "-")
	if !ok {
		return 0, 0, false
	}
	var start, end int64
	var err error
	if a == "" { // suffix
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		start, end = max(0, size-n), size
	} else {
		start, err = strconv.ParseInt(a, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		end = size
		if b != "" {
			e, err := strconv.ParseInt(b, 10, 64)
			if err != nil {
				return 0, 0, false
			}
			end = min(e+1, size)
		}
	}
	if start >= size || start >= end {
		return 0, 0, false
	}
	return start, end, true
}
