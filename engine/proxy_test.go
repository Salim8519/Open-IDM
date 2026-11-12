package engine

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Salim8519/Open-IDM/internal/testsrv"
)

// TestHTTPProxy routes a segmented download through a forward proxy.
func TestHTTPProxy(t *testing.T) {
	files := testsrv.New()
	defer files.Close()
	var hits atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		out, _ := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), nil)
		out.Header = r.Header.Clone()
		resp, err := http.DefaultTransport.RoundTrip(out)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer proxy.Close()

	m := newTestManager(t, ":memory:", func(s *Settings) { s.Proxy = proxy.URL })
	d, err := m.Add(Request{URL: files.FileURL("size=1500000"), Connections: 4})
	if err != nil {
		t.Fatal(err)
	}
	d = waitStatus(t, m, d.ID, StatusCompleted, 20*time.Second)
	if fileSHA(t, d.Path()) != testsrv.SHA256(1500000, 1) {
		t.Fatal("content mismatch through proxy")
	}
	if hits.Load() < 2 {
		t.Fatalf("proxy saw %d requests", hits.Load())
	}
}

func TestInvalidProxyRejected(t *testing.T) {
	m := newTestManager(t, ":memory:", nil)
	s := m.Settings()
	s.Proxy = "ftp://nope:1"
	if _, err := m.UpdateSettings(s); err == nil {
		t.Fatal("invalid proxy accepted")
	}
}
