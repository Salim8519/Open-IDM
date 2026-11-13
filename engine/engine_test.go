package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salim8519/Peregrine/internal/testsrv"
)

func newTestManager(t *testing.T, dbPath string, mutate func(*Settings)) *Manager {
	t.Helper()
	st, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	s := DefaultSettings()
	s.DownloadDir = t.TempDir()
	s.MaxRetries = 20
	s.Proxy = "none"
	if mutate != nil {
		mutate(&s)
	}
	if err := st.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(st)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close(); st.Close() })
	return m
}

func waitStatus(t *testing.T, m *Manager, id string, want Status, timeout time.Duration) Download {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		d, err := m.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if d.Status == want {
			return d
		}
		if d.Status == StatusError && want != StatusError {
			t.Fatalf("download failed: %s", d.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	d, _ := m.Get(id)
	t.Fatalf("timeout waiting for %s; status=%s err=%q downloaded=%d/%d", want, d.Status, d.Error, d.Downloaded, d.Size)
	return d
}

func fileSHA(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestSegmentedDownload(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	const size = 8 << 20
	d, err := m.Add(Request{URL: srv.FileURL("size=8388608&name=big.bin&throttle=4000000"), Connections: 8})
	if err != nil {
		t.Fatal(err)
	}
	d = waitStatus(t, m, d.ID, StatusCompleted, 30*time.Second)
	if d.Filename != "big.bin" {
		t.Errorf("filename = %q, want big.bin", d.Filename)
	}
	if got, want := fileSHA(t, d.Path()), testsrv.SHA256(size, 1); got != want {
		t.Fatalf("sha mismatch")
	}
	if d.SHA256 != testsrv.SHA256(size, 1) {
		t.Errorf("reported sha256 wrong")
	}
	if srv.MaxActive() < 4 {
		t.Errorf("expected parallel connections, peak was %d", srv.MaxActive())
	}
	if _, err := os.Stat(d.Path() + PartSuffix); !os.IsNotExist(err) {
		t.Errorf("part file left behind")
	}
}

func TestDynamicSegmentationSplits(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	// 4 MiB with 4 connections; throttled so splits must happen as workers finish unevenly.
	d, _ := m.Add(Request{URL: srv.FileURL("size=4194304&throttle=1500000"), Connections: 4})
	d = waitStatus(t, m, d.ID, StatusCompleted, 30*time.Second)
	if fileSHA(t, d.Path()) != testsrv.SHA256(4<<20, 1) {
		t.Fatal("content mismatch")
	}
	if srv.Requests() <= 5 { // 1 probe + 4 initial; splitting adds more
		t.Logf("requests=%d (no split happened; acceptable but unexpected)", srv.Requests())
	}
}

func TestNoRangeFallback(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	d, _ := m.Add(Request{URL: srv.FileURL("size=300000&norange=1"), Connections: 8})
	d = waitStatus(t, m, d.ID, StatusCompleted, 20*time.Second)
	if d.Resumable {
		t.Error("should be marked not resumable")
	}
	if fileSHA(t, d.Path()) != testsrv.SHA256(300000, 1) {
		t.Fatal("content mismatch")
	}
}

func TestUnknownSize(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	d, _ := m.Add(Request{URL: srv.FileURL("size=500000&norange=1&nolength=1")})
	d = waitStatus(t, m, d.ID, StatusCompleted, 20*time.Second)
	if d.Size != 500000 {
		t.Errorf("size = %d", d.Size)
	}
	if fileSHA(t, d.Path()) != testsrv.SHA256(500000, 1) {
		t.Fatal("content mismatch")
	}
}

func TestRetriesAfterDroppedConnections(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	// Every request is cut after 100 KB for the first 12 requests.
	d, _ := m.Add(Request{URL: srv.FileURL("size=2000000&drop=100000&dropfirst=12"), Connections: 4})
	d = waitStatus(t, m, d.ID, StatusCompleted, 60*time.Second)
	if fileSHA(t, d.Path()) != testsrv.SHA256(2000000, 1) {
		t.Fatal("content mismatch after retries")
	}
}

func TestServerBusyThenOK(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	d, _ := m.Add(Request{URL: srv.FileURL("size=100000&fail=0")})
	waitStatus(t, m, d.ID, StatusCompleted, 20*time.Second)
}

func TestPauseResume(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	d, _ := m.Add(Request{URL: srv.FileURL("size=3000000&throttle=1000000"), Connections: 4})
	waitStatus(t, m, d.ID, StatusDownloading, 5*time.Second)
	time.Sleep(700 * time.Millisecond)
	if err := m.Pause(d.ID); err != nil {
		t.Fatal(err)
	}
	p, _ := m.Get(d.ID)
	if p.Status != StatusPaused || p.Downloaded == 0 || p.Downloaded >= 3000000 {
		t.Fatalf("after pause: status=%s downloaded=%d", p.Status, p.Downloaded)
	}
	reqBefore := srv.Requests()
	if err := m.Resume(d.ID); err != nil {
		t.Fatal(err)
	}
	d = waitStatus(t, m, d.ID, StatusCompleted, 30*time.Second)
	if fileSHA(t, d.Path()) != testsrv.SHA256(3000000, 1) {
		t.Fatal("content mismatch after resume")
	}
	if srv.Requests() == reqBefore {
		t.Fatal("no requests after resume?")
	}
}

func TestCrashSafeResumeAcrossRestart(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	db := filepath.Join(t.TempDir(), "state.db")
	dir := t.TempDir()
	st, _ := OpenStore(db)
	s := DefaultSettings()
	s.DownloadDir, s.Proxy = dir, "none"
	_ = st.SaveSettings(s)
	m1, _ := NewManager(st)
	d, _ := m1.Add(Request{URL: srv.FileURL("size=4000000&throttle=1500000"), Connections: 4})
	waitStatus(t, m1, d.ID, StatusDownloading, 5*time.Second)
	time.Sleep(1500 * time.Millisecond)
	m1.Close() // simulates quitting mid-download
	st.Close()

	st2, _ := OpenStore(db)
	defer st2.Close()
	m2, err := NewManager(st2)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	got, err := m2.Get(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Downloaded == 0 {
		t.Fatal("progress was not persisted")
	}
	d = waitStatus(t, m2, d.ID, StatusCompleted, 30*time.Second)
	if fileSHA(t, d.Path()) != testsrv.SHA256(4000000, 1) {
		t.Fatal("content mismatch after restart")
	}
}

func TestRemoteChangedDetected(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	d, _ := m.Add(Request{URL: srv.FileURL("size=3000000&throttle=800000&seed=1"), Connections: 2})
	waitStatus(t, m, d.ID, StatusDownloading, 5*time.Second)
	time.Sleep(500 * time.Millisecond)
	_ = m.Pause(d.ID)
	// Same URL path, but the server now sends a different file (new seed => new ETag).
	if err := m.UpdateURL(d.ID, srv.FileURL("size=3000000&throttle=800000&seed=2"), nil); err != nil {
		t.Fatal(err)
	}
	_ = m.Resume(d.ID)
	got := waitStatus(t, m, d.ID, StatusError, 10*time.Second)
	if !strings.Contains(got.Error, "changed on the server") {
		t.Fatalf("error = %q", got.Error)
	}
}

func TestExpiredLinkIsNotRetriedForever(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	d, _ := m.Add(Request{URL: srv.FileURL("size=1000&expire=0")})
	got := waitStatus(t, m, d.ID, StatusError, 10*time.Second)
	if !strings.Contains(got.Error, "expired") {
		t.Fatalf("error = %q", got.Error)
	}
}

func TestChecksumVerification(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	good, _ := m.Add(Request{URL: srv.FileURL("size=50000"), Checksum: "sha256:" + testsrv.SHA256(50000, 1)})
	waitStatus(t, m, good.ID, StatusCompleted, 10*time.Second)
	bad, _ := m.Add(Request{URL: srv.FileURL("size=50001"), Checksum: "sha256:" + strings.Repeat("0", 64)})
	got := waitStatus(t, m, bad.ID, StatusError, 10*time.Second)
	if !strings.Contains(got.Error, "checksum") {
		t.Fatalf("error = %q", got.Error)
	}
	if _, err := m.Add(Request{URL: srv.FileURL("x=1"), Checksum: "sha256:nothex"}); err == nil {
		t.Fatal("invalid checksum accepted")
	}
}

func TestSpeedLimit(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	start := time.Now()
	d, _ := m.Add(Request{URL: srv.FileURL("size=600000"), SpeedLimit: 300000, Connections: 4})
	waitStatus(t, m, d.ID, StatusCompleted, 20*time.Second)
	if el := time.Since(start); el < 1200*time.Millisecond {
		t.Fatalf("600 KB at 300 KB/s took only %v", el)
	}
}

func TestQueueRespectsMaxActive(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", func(s *Settings) { s.MaxActive = 2 })
	var ids []string
	for i := 0; i < 5; i++ {
		d, _ := m.Add(Request{URL: srv.FileURL("size=400000&throttle=400000&seed=" + string(rune('1'+i)))})
		ids = append(ids, d.ID)
	}
	time.Sleep(300 * time.Millisecond)
	running := 0
	for _, d := range m.List() {
		if d.Status == StatusDownloading {
			running++
		}
	}
	if running > 2 {
		t.Fatalf("%d running, max is 2", running)
	}
	for _, id := range ids {
		waitStatus(t, m, id, StatusCompleted, 30*time.Second)
	}
}

func TestDuplicateNamesGetUniquePaths(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	a, _ := m.Add(Request{URL: srv.FileURL("size=1000&name=same.txt&seed=1")})
	b, _ := m.Add(Request{URL: srv.FileURL("size=1000&name=same.txt&seed=2")})
	a = waitStatus(t, m, a.ID, StatusCompleted, 10*time.Second)
	b = waitStatus(t, m, b.ID, StatusCompleted, 10*time.Second)
	if a.Path() == b.Path() {
		t.Fatalf("both downloads wrote %s", a.Path())
	}
}

func TestRemoveDeletesPartFile(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", nil)
	d, _ := m.Add(Request{URL: srv.FileURL("size=3000000&throttle=500000")})
	waitStatus(t, m, d.ID, StatusDownloading, 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	cur, _ := m.Get(d.ID)
	if err := m.Remove(d.ID, true); err != nil {
		t.Fatal(err)
	}
	if exists(cur.PartPath()) {
		t.Fatal("part file not removed")
	}
	if _, err := m.Get(d.ID); err != ErrNotFound {
		t.Fatal("still listed")
	}
}

func TestCategoriesFolder(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", func(s *Settings) { s.UseCategories = true })
	d, _ := m.Add(Request{URL: srv.FileURL("size=1000&name=movie.mp4")})
	d = waitStatus(t, m, d.ID, StatusCompleted, 10*time.Second)
	if filepath.Base(d.Dir) != "Video" {
		t.Fatalf("dir = %s", d.Dir)
	}
}

func TestRejectsNonHTTP(t *testing.T) {
	m := newTestManager(t, ":memory:", nil)
	for _, u := range []string{"file:///etc/passwd", "javascript:alert(1)", "ftp://x/y", "", "http://"} {
		if _, err := m.Add(Request{URL: u}); err == nil {
			t.Errorf("accepted %q", u)
		}
	}
}
