package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Salim8519/Peregrine/internal/api"
)

const devID = "bcnjpgddmjlffmcfgnfjaadkdjphdhfd"

func frame(b []byte) []byte {
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(b)))
	return append(hdr[:], b...)
}

func TestFramingRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	msgs := []string{`{"id":1,"method":"ping"}`, `{"x":"ünïcødé ✓"}`, strings.Repeat("a", 70000)}
	for _, m := range msgs {
		if err := writeFrame(&buf, []byte(m)); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range msgs {
		got, err := readFrame(&buf)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != m {
			t.Fatalf("got %q want %q", truncate(string(got), 30), truncate(m, 30))
		}
	}
	if _, err := readFrame(&buf); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestFramingBadFrames(t *testing.T) {
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], maxInMessage+1)
	cases := map[string][]byte{
		"oversized":        append(hdr[:], 'x'),
		"huge length":      {0xff, 0xff, 0xff, 0xff},
		"zero length":      {0, 0, 0, 0},
		"truncated header": {5, 0},
		"truncated body":   append([]byte{10, 0, 0, 0}, "abc"...),
	}
	for name, in := range cases {
		if _, err := readFrame(bytes.NewReader(in)); err == nil || errors.Is(err, io.EOF) {
			t.Errorf("%s: want error, got %v", name, err)
		}
	}
	if err := writeFrame(io.Discard, make([]byte, maxOutMessage+1)); !errors.Is(err, errTooLarge) {
		t.Errorf("oversized write: %v", err)
	}
}

// fakeApp records calls and answers like the real app.
type fakeApp struct {
	mu    sync.Mutex
	calls []string
	last  map[string]any
}

func (f *fakeApp) Call(method string, params any) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method)
	if params != nil {
		b, _ := json.Marshal(params)
		f.last = map[string]any{}
		_ = json.Unmarshal(b, &f.last)
	}
	switch method {
	case "version":
		return json.RawMessage(`{"version":"1.2.3"}`), nil
	case "getSettings":
		return json.RawMessage(`{"minExtSize":5,"downloadDir":"C:\\x","proxy":"http://u:p@h","autoStartExt":true,"language":"ar"}`), nil
	case "list":
		return json.RawMessage(`[{"id":"a","url":"https://x/y?tok=1","dir":"C:\\d","filename":"y.zip","status":"queued","size":10}]`), nil
	case "capture":
		return json.RawMessage(`{"queued":false,"prompted":true}`), nil
	case "add":
		return nil, &api.RPCError{Code: "bad_params", Message: "nope"}
	}
	return nil, &api.RPCError{Code: "unknown_method", Message: method}
}

func newTestHost(app *fakeApp, running bool) (*host, *int) {
	launches := 0
	h := &host{
		log:        log.New(io.Discard, "", 0),
		launchWait: 300 * time.Millisecond,
		pollEvery:  10 * time.Millisecond,
	}
	h.dial = func() (caller, error) {
		if running {
			return app, nil
		}
		return nil, errors.New("not running")
	}
	h.launch = func() error { launches++; return errAppMissing }
	return h, &launches
}

func call(t *testing.T, h *host, msg string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(h.handle([]byte(msg)))
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func errCode(m map[string]any) string {
	if e, ok := m["error"].(map[string]any); ok {
		return e["code"].(string)
	}
	return ""
}

func TestMethodAllowlist(t *testing.T) {
	app := &fakeApp{}
	h, _ := newTestHost(app, true)
	for _, m := range []string{"remove", "updateSettings", "openFile", "showInFolder", "pickFolder", "readClipboard", "pauseAll", "extensionHello", "browserStatus", "openExtensionsPage", "showExtensionFolder", "", "PING", "__proto__"} {
		out := call(t, h, `{"id":7,"method":"`+m+`"}`)
		if errCode(out) != codeForbidden {
			t.Errorf("%q: want %s, got %v", m, codeForbidden, out)
		}
		if out["id"] != float64(7) {
			t.Errorf("%q: id not echoed: %v", m, out["id"])
		}
	}
	if len(app.calls) != 0 {
		t.Fatalf("refused methods reached the app: %v", app.calls)
	}
	for _, m := range []string{"version", "getSettings", "list"} {
		if out := call(t, h, `{"id":1,"method":"`+m+`"}`); out["error"] != nil {
			t.Errorf("%s: %v", m, out)
		}
	}
}

func TestBadRequests(t *testing.T) {
	h, _ := newTestHost(&fakeApp{}, true)
	for _, msg := range []string{`not json`, `[1,2]`, `{"id":{"a":1},"method":"ping"}`, `{"id":[1],"method":"ping"}`} {
		if c := errCode(call(t, h, msg)); c != codeBadRequest {
			t.Errorf("%s: got %q", msg, c)
		}
	}
	for _, msg := range []string{
		`{"id":1,"method":"capture"}`,
		`{"id":1,"method":"capture","params":{"url":"javascript:alert(1)"}}`,
		`{"id":1,"method":"capture","params":{"url":"file:///C:/Windows/win.ini"}}`,
		`{"id":1,"method":"capture","params":{"url":"https://"}}`,
		`{"id":1,"method":"add","params":{"url":"data:text/plain,hi"}}`,
		`{"id":1,"method":"capture","params":"https://x/y"}`,
	} {
		if c := errCode(call(t, h, msg)); c != codeBadParams {
			t.Errorf("%s: got %q", msg, c)
		}
	}
}

func TestPing(t *testing.T) {
	app := &fakeApp{}
	h, launches := newTestHost(app, true)
	out := call(t, h, `{"id":"abc","method":"ping"}`)
	res := out["result"].(map[string]any)
	if res["app"] != true || res["host"] != hostVersion || res["appVersion"] != "1.2.3" || out["id"] != "abc" {
		t.Fatalf("ping: %v", out)
	}
	// The app learns the extension is installed from the host, not from the extension.
	if strings.Join(app.calls, ",") != "version,extensionHello" {
		t.Fatalf("ping should tell the app the extension checked in, calls: %v", app.calls)
	}
	h2, _ := newTestHost(app, false)
	res = call(t, h2, `{"id":1,"method":"ping"}`)["result"].(map[string]any)
	if res["app"] != false {
		t.Fatalf("ping with app down: %v", res)
	}
	if *launches != 0 {
		t.Fatal("ping must not launch the app")
	}
}

func TestCaptureSanitized(t *testing.T) {
	app := &fakeApp{}
	h, _ := newTestHost(app, true)
	out := call(t, h, `{"id":1,"method":"capture","params":{
		"url":"https://example.com/a.zip?sig=1","filename":"a.zip","dir":"C:\\Windows",
		"source":"evil","pageUrl":"https://example.com/page","size":123,"mimeType":"application/zip",
		"headers":{"cookie":"s=1","Referer":"https://example.com/page","User-Agent":"UA","Authorization":"Bearer x","X-Evil":"1","Accept-Language":"a\r\nInjected: 1"}}}`)
	if out["error"] != nil {
		t.Fatal(out)
	}
	got := app.last
	if got["url"] != "https://example.com/a.zip?sig=1" || got["source"] != "extension" || got["dir"] != nil ||
		got["pageUrl"] != "https://example.com/page" || got["size"] != float64(123) || got["filename"] != "a.zip" {
		t.Fatalf("forwarded capture: %v", got)
	}
	hd := got["headers"].(map[string]any)
	want := map[string]any{"Cookie": "s=1", "Referer": "https://example.com/page", "User-Agent": "UA"}
	if len(hd) != len(want) {
		t.Fatalf("headers: %v", hd)
	}
	for k, v := range want {
		if hd[k] != v {
			t.Fatalf("headers: %v", hd)
		}
	}
}

func TestFilteredReplies(t *testing.T) {
	h, _ := newTestHost(&fakeApp{}, true)
	s := call(t, h, `{"id":1,"method":"getSettings"}`)["result"].(map[string]any)
	if s["minExtSize"] != float64(5) || s["downloadDir"] != nil || s["proxy"] != nil || s["language"] != "ar" {
		t.Fatalf("settings leaked or wrong: %v", s)
	}
	l := call(t, h, `{"id":1,"method":"list"}`)["result"].([]any)
	d := l[0].(map[string]any)
	if d["url"] != nil || d["dir"] != nil || d["filename"] != "y.zip" {
		t.Fatalf("list leaked or wrong: %v", d)
	}
	// App errors pass through with their code.
	if c := errCode(call(t, h, `{"id":1,"method":"add","params":{"url":"https://x.test/f"}}`)); c != "bad_params" {
		t.Fatalf("add error code %q", c)
	}
}

func TestAppUnavailableAndLaunch(t *testing.T) {
	app := &fakeApp{}
	h, launches := newTestHost(app, false)
	for _, m := range []string{"getSettings", "list"} {
		if c := errCode(call(t, h, `{"id":1,"method":"`+m+`"}`)); c != codeAppUnavailable {
			t.Fatalf("%s: %q", m, c)
		}
	}
	if *launches != 0 {
		t.Fatal("getSettings/list must not launch the app")
	}
	start := time.Now()
	if c := errCode(call(t, h, `{"id":1,"method":"capture","params":{"url":"https://x.test/f.zip"}}`)); c != codeAppUnavailable {
		t.Fatalf("capture: %q", c)
	}
	if *launches != 1 || time.Since(start) > 200*time.Millisecond {
		t.Fatal("missing executable should fail fast after one launch attempt")
	}

	// Launch succeeds and the app comes up after a few polls.
	up := false
	polls := 0
	h.launch = func() error { up = true; return nil }
	h.dial = func() (caller, error) {
		if up {
			polls++
			if polls > 3 {
				return app, nil
			}
		}
		return nil, errors.New("down")
	}
	if out := call(t, h, `{"id":1,"method":"version"}`); out["error"] != nil {
		t.Fatalf("version after launch: %v", out)
	}

	// Launch "succeeds" but the app never answers: time out.
	h.dial = func() (caller, error) { return nil, errors.New("down") }
	if c := errCode(call(t, h, `{"id":1,"method":"version"}`)); c != codeAppUnavailable {
		t.Fatalf("timeout: %q", c)
	}
}

func TestServeLoop(t *testing.T) {
	h, _ := newTestHost(&fakeApp{}, true)
	var in bytes.Buffer
	in.Write(frame([]byte(`{"id":1,"method":"ping"}`)))
	in.Write(frame([]byte(`garbage`)))
	in.Write(frame([]byte(`{"id":3,"method":"version"}`)))
	var out bytes.Buffer
	if err := h.serve(&in, &out); err != nil {
		t.Fatal(err)
	}
	var ids []any
	for {
		b, err := readFrame(&out)
		if err != nil {
			break
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("stdout carried a non-JSON frame: %q", b)
		}
		ids = append(ids, m["id"])
	}
	if len(ids) != 3 || ids[0] != float64(1) || ids[1] != nil || ids[2] != float64(3) {
		t.Fatalf("ids: %v", ids)
	}

	// An oversized frame stops the loop with an error reply.
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], maxInMessage+10)
	out.Reset()
	if err := h.serve(bytes.NewReader(hdr[:]), &out); err == nil {
		t.Fatal("want error on oversized frame")
	}
	b, err := readFrame(&out)
	if err != nil || !strings.Contains(string(b), codeBadRequest) {
		t.Fatalf("oversized reply: %q %v", b, err)
	}
}

func TestOriginCheck(t *testing.T) {
	dir := t.TempDir()
	ids := allowedIDs(dir)
	ok := []string{"chrome-extension://" + devID + "/", "chrome-extension://" + devID}
	bad := []string{"", "chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/", "chrome-extension://" + devID + "x/",
		"chrome-extension://" + devID + "/../", "moz-extension://" + devID + "/", "https://evil.example/", devID,
		"chrome-extension://" + strings.ToUpper(devID) + "/"}
	for _, o := range ok {
		if !originAllowed(o, ids) {
			t.Errorf("should allow %q", o)
		}
	}
	for _, o := range bad {
		if originAllowed(o, ids) {
			t.Errorf("should reject %q", o)
		}
	}
	if got := callerOrigin([]string{"chrome-extension://" + devID + "/", "--parent-window=0"}); got != ok[0] {
		t.Errorf("callerOrigin windows: %q", got)
	}
	if got := callerOrigin(nil); got != "" {
		t.Errorf("callerOrigin empty: %q", got)
	}

	// Extra IDs from the config file (array form and object form); invalid ones ignored.
	extra := "cccccccccccccccccccccccccccccccc"
	_ = os.WriteFile(filepath.Join(dir, extraIDsFile), []byte(`["`+extra+`","NOT-AN-ID","`+devID+`"]`), 0o600)
	ids = allowedIDs(dir)
	if len(ids) != 2 || !originAllowed("chrome-extension://"+extra+"/", ids) {
		t.Fatalf("extra ids (array): %v", ids)
	}
	_ = os.WriteFile(filepath.Join(dir, extraIDsFile), []byte(`{"ids":["`+extra+`"]}`), 0o600)
	if ids = allowedIDs(dir); len(ids) != 2 {
		t.Fatalf("extra ids (object): %v", ids)
	}
	_ = os.WriteFile(filepath.Join(dir, extraIDsFile), []byte(`garbage`), 0o600)
	if ids = allowedIDs(dir); len(ids) != 1 {
		t.Fatalf("garbage file: %v", ids)
	}
}

func TestRejectedOriginWritesNothing(t *testing.T) {
	t.Setenv("PEREGRINE_HOME", t.TempDir())
	var out bytes.Buffer
	in := bytes.NewReader(frame([]byte(`{"id":1,"method":"ping"}`)))
	if code := run([]string{"chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/"}, in, &out, io.Discard); code == 0 {
		t.Fatal("want non-zero exit")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout must stay empty, got %q", out.Bytes())
	}
}

func TestManifest(t *testing.T) {
	abs, _ := filepath.Abs("peregrine-host")
	b, err := buildManifest(abs, []string{devID})
	if err != nil {
		t.Fatal(err)
	}
	var m hostManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "com.peregrine.host" || m.Type != "stdio" || m.Path != abs || len(m.AllowedOrigins) != 1 ||
		m.AllowedOrigins[0] != "chrome-extension://"+devID+"/" || m.Description == "" {
		t.Fatalf("manifest: %+v", m)
	}
	if _, err := buildManifest("relative/host", nil); err == nil {
		t.Fatal("relative path must be rejected")
	}
}

func TestTargets(t *testing.T) {
	ts := targetsFor("windows", "", "", browsers)
	if len(ts) != 5 || ts[0].RegKey != `Software\Google\Chrome\NativeMessagingHosts\com.peregrine.host` ||
		ts[1].RegKey != `Software\Microsoft\Edge\NativeMessagingHosts\com.peregrine.host` {
		t.Fatalf("windows: %+v", ts)
	}
	ts = targetsFor("windows", "", `Software\PeregrineTest\x\`, browsers[:1])
	if ts[0].RegKey != `Software\PeregrineTest\x\Google\Chrome\NativeMessagingHosts\com.peregrine.host` {
		t.Fatalf("windows root: %+v", ts)
	}
	ts = targetsFor("darwin", "/Users/u", "", browsers)
	if ts[2].Dir != filepath.Join("/Users/u", "Library", "Application Support", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts") {
		t.Fatalf("darwin: %+v", ts[2])
	}
	ts = targetsFor("linux", "/home/u", "/tmp/r", browsers)
	if ts[0].Dir != filepath.Join("/tmp/r", ".config", "google-chrome", "NativeMessagingHosts") {
		t.Fatalf("linux: %+v", ts[0])
	}
	if _, err := selectBrowsers("chrome,opera"); err == nil {
		t.Fatal("opera must be rejected (it reads Chrome's registration)")
	}
	if bs, err := selectBrowsers(" Edge , brave"); err != nil || len(bs) != 2 {
		t.Fatalf("select: %v %v", bs, err)
	}
}

func TestInstallUninstallDirs(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(root, "cfg")
	hostPath := filepath.Join(root, "bin", "peregrine-host")
	for _, goos := range []string{"darwin", "linux"} {
		ts := targetsFor(goos, "", filepath.Join(root, goos), browsers)
		in := &installer{configDir: cfg, hostPath: hostPath, ids: []string{devID},
			setReg: setRegistry, delReg: deleteRegistry, logf: t.Logf}
		if err := in.install(ts); err != nil {
			t.Fatal(err)
		}
		for _, tg := range ts {
			b, err := os.ReadFile(filepath.Join(tg.Dir, manifestFileName))
			if err != nil {
				t.Fatal(err)
			}
			var m hostManifest
			if json.Unmarshal(b, &m) != nil || m.Path != hostPath {
				t.Fatalf("%s manifest: %s", tg.Browser, b)
			}
		}
		if err := in.uninstall(ts, true); err != nil {
			t.Fatal(err)
		}
		for _, tg := range ts {
			if _, err := os.Stat(filepath.Join(tg.Dir, manifestFileName)); !os.IsNotExist(err) {
				t.Fatalf("%s: manifest not removed", tg.Browser)
			}
		}
		if err := in.uninstall(ts, true); err != nil {
			t.Fatalf("second uninstall must be a no-op: %v", err)
		}
	}
}

// TestDialRealClient wires the host to a real api.Client via ipc.json discovery.
func TestDialRealClient(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PEREGRINE_HOME", home)
	token := api.NewToken()
	var got []string
	var mu sync.Mutex
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct{ Method string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		got = append(got, req.Method)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"result":{"version":"7.0.0"}}`))
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	if err := api.WriteIPCInfo(ln.Addr().(*net.TCPAddr).Port, token); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	in := bytes.NewReader(frame([]byte(`{"id":1,"method":"ping"}`)))
	if code := run([]string{"chrome-extension://" + devID + "/", "--parent-window=0"}, in, &out, io.Discard); code != 0 {
		t.Fatalf("exit %d", code)
	}
	b, err := readFrame(&out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"appVersion":"7.0.0"`) || !strings.Contains(string(b), `"app":true`) {
		t.Fatalf("reply: %s", b)
	}
	if _, err := os.Stat(filepath.Join(home, logFileName)); err != nil {
		t.Fatalf("log file: %v", err)
	}
}

func TestRedact(t *testing.T) {
	if got := redactURL("https://user:pw@example.com/a/b.zip?token=SECRET#frag"); strings.Contains(got, "SECRET") || strings.Contains(got, "pw") || got != "https://example.com/a/b.zip?REDACTED" {
		t.Fatalf("redactURL: %q", got)
	}
	if got := redactText(`Post "http://127.0.0.1:5/rpc?token=abc": refused`); strings.Contains(got, "abc") {
		t.Fatalf("redactText: %q", got)
	}
}
