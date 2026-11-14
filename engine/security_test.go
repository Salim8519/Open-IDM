package engine

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCookiesNotForwardedAcrossHosts (threat T5): a download that redirects to another host
// must not carry the browser's cookies or auth header there.
func TestCookiesNotForwardedAcrossHosts(t *testing.T) {
	var mu sync.Mutex
	var leaked []string
	content := bytes.Repeat([]byte("x"), 100000)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if c := r.Header.Get("Cookie"); c != "" {
			leaked = append(leaked, "Cookie: "+c)
		}
		if a := r.Header.Get("Authorization"); a != "" {
			leaked = append(leaked, "Authorization: "+a)
		}
		mu.Unlock()
		http.ServeContent(w, r, "f.bin", time.Unix(1700000000, 0), bytes.NewReader(content))
	}))
	defer target.Close()
	// Same port, different host name => a different origin for the redirect check.
	other := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other+"/f.bin", http.StatusFound)
	}))
	defer origin.Close()

	m := newTestManager(t, ":memory:", nil)
	d, err := m.Add(Request{URL: origin.URL + "/start", Headers: map[string]string{"Cookie": "session=SECRET", "Authorization": "Bearer SECRET"}})
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, d.ID, StatusCompleted, 15*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(leaked) > 0 {
		t.Fatalf("secrets reached another host: %v", leaked)
	}
}

// TestFilenameCannotEscapeFolder (threat T1): a hostile Content-Disposition name lands
// inside the download folder.
func TestFilenameCannotEscapeFolder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="../../../../evil.bat"`)
		http.ServeContent(w, r, "x", time.Unix(1700000000, 0), strings.NewReader("echo pwned"))
	}))
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	dir := m.Settings().DownloadDir
	d, _ := m.Add(Request{URL: srv.URL + "/dl"})
	d = waitStatus(t, m, d.ID, StatusCompleted, 10*time.Second)
	if d.Dir != dir || d.Filename != "evil.bat" {
		t.Fatalf("file escaped: dir=%q name=%q (want dir %q)", d.Dir, d.Filename, dir)
	}
}

// TestURLPasswordNeverReturned: a password in a link (ftp://user:secret@host/f) is kept for
// the download itself but never shown to the UI, the API or the browser extension.
func TestURLPasswordNeverReturned(t *testing.T) {
	m := newTestManager(t, ":memory:", nil)
	m.RegisterProtocol(&fakeProto{})
	var events []Event
	var mu sync.Mutex
	unsub := m.Subscribe(func(e Event) { mu.Lock(); events = append(events, e); mu.Unlock() })
	defer unsub()
	d, err := m.Add(Request{URL: "fake://alice:s3cret@files.example.com/movie.bin", Paused: true})
	if err != nil {
		t.Fatal(err)
	}
	check := func(where, u string) {
		t.Helper()
		if strings.Contains(u, "s3cret") {
			t.Fatalf("%s leaks the password: %s", where, u)
		}
		if !strings.Contains(u, "alice") || !strings.Contains(u, "files.example.com/movie.bin") {
			t.Fatalf("%s lost the rest of the link: %s", where, u)
		}
	}
	check("Add", d.URL)
	got, _ := m.Get(d.ID)
	check("Get", got.URL)
	check("List", m.List()[0].URL)
	mu.Lock()
	for _, e := range events {
		if e.Download != nil {
			check("event", e.Download.URL)
		}
	}
	mu.Unlock()
	// The download itself still has the full link.
	if full := m.snapshot(d.ID); full == nil || !strings.Contains(full.URL, "s3cret") {
		t.Fatal("internal copy lost the password")
	}
}
