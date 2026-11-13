package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/internal/testsrv"
)

func setup(t *testing.T) (*Server, *Client, *testsrv.Server) {
	t.Helper()
	st, err := engine.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	s := engine.DefaultSettings()
	s.DownloadDir, s.Proxy = t.TempDir(), "none"
	_ = st.SaveSettings(s)
	m, err := engine.NewManager(st)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(m, Hooks{})
	srv, err := Listen(svc, 0, NewToken(), nil)
	if err != nil {
		t.Fatal(err)
	}
	files := testsrv.New()
	t.Cleanup(func() { srv.Close(); svc.Close(); m.Close(); st.Close(); files.Close() })
	c := &Client{Base: "http://127.0.0.1:" + itoa(srv.Port()), Token: srv.Token(), HTTP: &http.Client{Timeout: 10 * time.Second}}
	return srv, c, files
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestRPCAddAndComplete(t *testing.T) {
	_, c, files := setup(t)
	raw, err := c.Call("add", map[string]any{"url": files.FileURL("size=100000&name=a.bin")})
	if err != nil {
		t.Fatal(err)
	}
	var d engine.Download
	_ = json.Unmarshal(raw, &d)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := c.Call("get", map[string]any{"id": d.ID})
		if err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(raw, &d)
		if d.Status == engine.StatusCompleted {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("not completed: %+v", d)
}

func TestRPCErrors(t *testing.T) {
	_, c, _ := setup(t)
	if _, err := c.Call("nope", nil); err == nil || !strings.Contains(err.Error(), "unknown method") {
		t.Fatalf("err = %v", err)
	}
	if _, err := c.Call("pause", map[string]any{"id": "missing"}); err == nil {
		t.Fatal("expected not found")
	}
	if _, err := c.Call("openFile", map[string]any{"id": "x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestHeadersNeverReturned(t *testing.T) {
	_, c, files := setup(t)
	_, err := c.Call("add", map[string]any{"url": files.FileURL("size=10"), "headers": map[string]string{"Cookie": "session=SECRET"}, "paused": true})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := c.Call("list", nil)
	if strings.Contains(string(raw), "SECRET") {
		t.Fatal("cookie leaked to client")
	}
}

func TestAuthAndHostGuard(t *testing.T) {
	srv, c, _ := setup(t)
	body := `{"method":"version"}`
	cases := []struct {
		name string
		mod  func(*http.Request)
		want int
	}{
		{"no token", func(r *http.Request) { r.Header.Del("Authorization") }, 401},
		{"wrong token", func(r *http.Request) { r.Header.Set("Authorization", "Bearer x") }, 401},
		{"rebinding host", func(r *http.Request) { r.Host = "evil.example.com" }, 403},
		{"cross-site origin", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example.com") }, 403},
		{"ok", func(r *http.Request) {}, 200},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest("POST", c.Base+"/rpc", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+srv.Token())
		tc.mod(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, resp.StatusCode, tc.want)
		}
	}
}

func TestCaptureEmitsEventWhenNotAutoStart(t *testing.T) {
	_, c, files := setup(t)
	raw, err := c.Call("capture", map[string]any{"url": files.FileURL("size=10"), "source": "extension"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "prompted") {
		t.Fatalf("result = %s", raw)
	}
	if _, err := c.Call("updateSettings", map[string]any{"autoStartExt": true}); err != nil {
		t.Fatal(err)
	}
	raw, _ = c.Call("capture", map[string]any{"url": files.FileURL("size=10"), "source": "extension"})
	if !strings.Contains(string(raw), `"queued":true`) {
		t.Fatalf("result = %s", raw)
	}
}
