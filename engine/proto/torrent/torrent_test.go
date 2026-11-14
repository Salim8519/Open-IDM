package torrent

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	g "github.com/anacrolix/generics"
	at "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"

	"github.com/Salim8519/Peregrine/engine"
)

// Everything here runs on 127.0.0.1: no DHT, no trackers, no internet.

const pieceLen = 32 << 10

func writeRandom(t *testing.T, path string, size int) []byte {
	t.Helper()
	b := make([]byte, size)
	_, _ = rand.Read(b)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

// buildTorrent makes metainfo for a file or folder; mutate may change the info (e.g. paths).
func buildTorrent(t *testing.T, root string, mutate func(*metainfo.Info)) *metainfo.MetaInfo {
	t.Helper()
	info := metainfo.Info{PieceLength: pieceLen}
	if err := info.BuildFromFilePath(root); err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(&info)
	}
	ib, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return &metainfo.MetaInfo{InfoBytes: ib}
}

type seeder struct {
	cl   *at.Client
	addr string
}

// startSeeder serves mi from baseDir (default layout: baseDir/<name>/<path...>) on loopback.
func startSeeder(t *testing.T, mi *metainfo.MetaInfo, baseDir string, upLimit rate.Limit, pathMaker storage.FilePathMaker) *seeder {
	t.Helper()
	c := at.NewDefaultClientConfig()
	c.ListenHost = func(string) string { return "127.0.0.1" }
	c.ListenPort = 0
	c.NoDHT = true
	c.DisableIPv6 = true
	c.DisableUTP = true
	c.DisableWebtorrent = true
	c.NoDefaultPortForwarding = true
	c.Seed = true
	c.Slogger = slog.New(slog.DiscardHandler)
	if upLimit > 0 {
		c.UploadRateLimiter = rate.NewLimiter(upLimit, 256<<10)
	}
	c.DefaultStorage = storage.NewFileOpts(storage.NewFileClientOpts{
		ClientBaseDir:   baseDir,
		FilePathMaker:   pathMaker,
		PieceCompletion: storage.NewMapPieceCompletion(),
		UsePartFiles:    g.Some(false),
		DisableMmap:     true,
	})
	cl, err := at.NewClient(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	tor, err := cl.AddTorrent(mi)
	if err != nil {
		t.Fatal(err)
	}
	if err := tor.VerifyDataContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	select { // the flag can trail VerifyData slightly
	case <-tor.Complete().On():
	case <-time.After(10 * time.Second):
		t.Fatalf("seeder has %d of %d bytes", tor.BytesCompleted(), tor.Length())
	}
	return &seeder{cl: cl, addr: fmt.Sprintf("127.0.0.1:%d", cl.LocalPort())}
}

func newProto(t *testing.T, peers ...string) *Protocol {
	t.Helper()
	p := New(Config{ListenPort: -1, DisableDHT: true})
	p.cfg.listenHost = "127.0.0.1"
	p.cfg.disableIPv6 = true
	p.cfg.peers = peers
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	return p
}

func newManager(t *testing.T, p *Protocol) (*engine.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := engine.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	s := engine.DefaultSettings()
	s.DownloadDir = dir
	s.Proxy = "none"
	if err := st.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	m, err := engine.NewManager(st)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close(); st.Close() })
	m.RegisterProtocol(p)
	return m, dir
}

func waitStatus(t *testing.T, m *engine.Manager, id string, want engine.Status, timeout time.Duration) engine.Download {
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
		if d.Status == engine.StatusError {
			t.Fatalf("download failed: %s", d.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	d, _ := m.Get(id)
	t.Fatalf("status %s after %v, want %s (downloaded %d)", d.Status, timeout, want, d.Downloaded)
	return d
}

func serveTorrent(t *testing.T, mi *metainfo.MetaInfo) string {
	t.Helper()
	var buf bytes.Buffer
	if err := mi.Write(&buf); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/files/release.torrent?passkey=secret"
}

// magnetFor returns a magnet link with no trackers (peers come from the test hook).
func magnetFor(t *testing.T, mi *metainfo.MetaInfo) string {
	t.Helper()
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatal(err)
	}
	return metainfo.Magnet{InfoHash: mi.HashInfoBytes(), DisplayName: info.Name}.String()
}

func assertFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: content differs (%d vs %d bytes)", path, len(got), len(want))
	}
}

func TestMatch(t *testing.T) {
	p := New(Config{})
	yes := []string{
		"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=x",
		"MAGNET:?dn=x&xt=urn:btih:0123456789abcdef0123456789abcdef01234567",
		"https://example.com/a/b.torrent",
		"http://example.com/b.TORRENT?passkey=1",
	}
	no := []string{
		"magnet:?xt=urn:ed2k:abc",
		"https://example.com/b.torrent.html",
		"https://example.com/file.zip",
		"ftp://example.com/x.torrent",
		"file:///C:/x.torrent",
	}
	for _, u := range yes {
		if !p.Match(u) {
			t.Errorf("Match(%q) = false", u)
		}
	}
	for _, u := range no {
		if p.Match(u) {
			t.Errorf("Match(%q) = true", u)
		}
	}
}

func TestSafeRelPath(t *testing.T) {
	sep := string(filepath.Separator)
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"a", "b.txt"}, "a" + sep + "b.txt"},
		{[]string{"..", "evil"}, "evil"},
		{[]string{"a", "..", "..", "evil"}, "a" + sep + "evil"},
		{[]string{"/etc/passwd"}, "passwd"},
		{[]string{`C:\Windows\x.dll`}, "x.dll"},
		{[]string{"C:"}, "C_"},
		{[]string{"CON", "aux.txt"}, "_CON" + sep + "_aux.txt"},
		{[]string{"", ".", ".."}, "download"},
		{nil, "download"},
	}
	for _, c := range cases {
		got := SafeRelPath(c.in)
		if got != c.want {
			t.Errorf("SafeRelPath(%q) = %q, want %q", c.in, got, c.want)
		}
		if filepath.IsAbs(got) || strings.HasPrefix(got, "..") {
			t.Errorf("SafeRelPath(%q) escapes: %q", c.in, got)
		}
	}
}

func TestLimits(t *testing.T) {
	a, b := &engine.Job{}, &engine.Job{}
	if d, u := limitsFor(map[*engine.Job]int64{}); d != 0 || u != DefaultUploadLimit {
		t.Fatalf("none: %d %d", d, u)
	}
	if d, u := limitsFor(map[*engine.Job]int64{a: 100_000, b: 50_000}); d != 150_000 || u != 300_000 {
		t.Fatalf("limited: %d %d", d, u)
	}
	if d, u := limitsFor(map[*engine.Job]int64{a: 100_000, b: 0}); d != 0 || u != DefaultUploadLimit {
		t.Fatalf("one unlimited: %d %d", d, u)
	}
}

// A single-file .torrent fetched over HTTP lands exactly at the final name.
func TestSingleFileFromTorrentURL(t *testing.T) {
	src := t.TempDir()
	data := writeRandom(t, filepath.Join(src, "movie.mkv"), 300_000)
	mi := buildTorrent(t, filepath.Join(src, "movie.mkv"), nil)
	sd := startSeeder(t, mi, src, 0, nil)
	p := newProto(t, sd.addr)
	m, dir := newManager(t, p)

	d, err := m.Add(engine.Request{URL: serveTorrent(t, mi)})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Kind {
		t.Fatalf("kind = %q", d.Kind)
	}
	done := waitStatus(t, m, d.ID, engine.StatusCompleted, 30*time.Second)
	if done.Filename != "movie.mkv" || done.IsDir || done.Downloaded != int64(len(data)) || done.Size != int64(len(data)) {
		t.Fatalf("completed: %+v", done)
	}
	assertFile(t, filepath.Join(dir, "movie.mkv"), data)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("download folder has %d entries, want only the file", len(entries))
	}
	if p.ListenPort() == 0 {
		t.Fatal("client not running")
	}
}

// A multi-file torrent becomes a folder (built inside the .peregrine.part folder).
func TestMultiFileFolder(t *testing.T) {
	src := t.TempDir()
	root := filepath.Join(src, "Album")
	a := writeRandom(t, filepath.Join(root, "01.flac"), 100_000)
	b := writeRandom(t, filepath.Join(root, "covers", "front.jpg"), 70_001)
	_ = writeRandom(t, filepath.Join(root, "empty.txt"), 0)
	mi := buildTorrent(t, root, nil)
	sd := startSeeder(t, mi, src, 0, nil)
	p := newProto(t, sd.addr)
	m, dir := newManager(t, p)

	d, err := m.Add(engine.Request{URL: serveTorrent(t, mi)})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, d.ID, engine.StatusCompleted, 30*time.Second)
	if !done.IsDir || done.Filename != "Album" {
		t.Fatalf("completed: %+v", done)
	}
	assertFile(t, filepath.Join(dir, "Album", "01.flac"), a)
	assertFile(t, filepath.Join(dir, "Album", "covers", "front.jpg"), b)
	assertFile(t, filepath.Join(dir, "Album", "empty.txt"), nil)
	if _, err := os.Stat(filepath.Join(dir, "Album.peregrine.part")); !os.IsNotExist(err) {
		t.Fatalf("part folder left behind: %v", err)
	}
}

// A magnet link gets its metadata from the (local) peer, then the data.
func TestMagnet(t *testing.T) {
	src := t.TempDir()
	data := writeRandom(t, filepath.Join(src, "linux.iso"), 200_000)
	mi := buildTorrent(t, filepath.Join(src, "linux.iso"), nil)
	sd := startSeeder(t, mi, src, 0, nil)
	p := newProto(t, sd.addr)
	m, dir := newManager(t, p)

	magnet := magnetFor(t, mi)
	if !p.Match(magnet) {
		t.Fatalf("magnet not matched: %s", magnet)
	}
	d, err := m.Add(engine.Request{URL: magnet})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, d.ID, engine.StatusCompleted, 30*time.Second)
	if done.Filename != "linux.iso" {
		t.Fatalf("name = %q", done.Filename)
	}
	assertFile(t, filepath.Join(dir, "linux.iso"), data)
}

// Pausing halfway and resuming keeps the verified pieces: the seeder uploads the file
// about once, not twice, and the second run needs no metadata from the network.
func TestPauseResume(t *testing.T) {
	src := t.TempDir()
	const size = 3 << 20
	data := writeRandom(t, filepath.Join(src, "big.bin"), size)
	mi := buildTorrent(t, filepath.Join(src, "big.bin"), nil)
	sd := startSeeder(t, mi, src, 1<<20, nil) // 1 MiB/s
	p := newProto(t, sd.addr)
	m, dir := newManager(t, p)

	d, err := m.Add(engine.Request{URL: magnetFor(t, mi)})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		got, _ := m.Get(d.ID)
		if got.Downloaded >= size/3 {
			break
		}
		if got.Status == engine.StatusError {
			t.Fatal(got.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("no progress: %+v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := m.Pause(d.ID); err != nil {
		t.Fatal(err)
	}
	paused := waitStatus(t, m, d.ID, engine.StatusPaused, 10*time.Second)
	state, _ := m.ResumeState(d.ID) // resume state is internal (not in snapshots)
	if !paused.Resumable || len(state) == 0 || paused.Downloaded == 0 || paused.Downloaded >= size {
		t.Fatalf("after pause: resumable=%v state=%d downloaded=%d", paused.Resumable, len(state), paused.Downloaded)
	}
	if err := m.Resume(d.ID); err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, d.ID, engine.StatusCompleted, 60*time.Second)
	if done.Filename != "big.bin" {
		t.Fatalf("name = %q", done.Filename)
	}
	assertFile(t, filepath.Join(dir, "big.bin"), data)
	stats := sd.cl.Stats()
	uploaded := stats.BytesWrittenData.Int64()
	if uploaded > size*3/2 {
		t.Fatalf("seeder uploaded %d bytes for a %d-byte file: resume re-fetched data", uploaded, size)
	}
}

// A torrent whose file paths try to climb out of the folder is sanitized: everything is
// written inside the download's own folder.
func TestMaliciousPaths(t *testing.T) {
	src := t.TempDir()
	root := filepath.Join(src, "pack")
	evil := writeRandom(t, filepath.Join(root, "evil.txt"), 40_000)
	ok := writeRandom(t, filepath.Join(root, "ok.txt"), 40_000)
	mi := buildTorrent(t, root, func(info *metainfo.Info) {
		for i := range info.Files {
			if info.Files[i].Path[0] == "evil.txt" {
				info.Files[i].Path = []string{"..", "..", "evil.txt"}
			}
		}
	})
	// The seeder reads each file by its last path element.
	sd := startSeeder(t, mi, root, 0, func(o storage.FilePathMakerOpts) string {
		p := o.File.BestPath()
		return p[len(p)-1]
	})
	p := newProto(t, sd.addr)
	m, dir := newManager(t, p)

	d, err := m.Add(engine.Request{URL: serveTorrent(t, mi)})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, d.ID, engine.StatusCompleted, 30*time.Second)
	assertFile(t, filepath.Join(done.Path(), "evil.txt"), evil)
	assertFile(t, filepath.Join(done.Path(), "ok.txt"), ok)
	for _, outside := range []string{filepath.Join(dir, "evil.txt"), filepath.Join(filepath.Dir(dir), "evil.txt")} {
		if _, err := os.Stat(outside); !os.IsNotExist(err) {
			t.Fatalf("file written outside the download folder: %s", outside)
		}
	}
}

// Bad input fails with an error that doesn't echo the link (it may hold passkeys).
func TestBadSources(t *testing.T) {
	p := newProto(t)
	m, _ := newManager(t, p)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not a torrent</html>"))
	}))
	defer srv.Close()
	d, err := m.Add(engine.Request{URL: srv.URL + "/x.torrent?passkey=secret"})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitStatus(t, m, d.ID, engine.StatusError, 10*time.Second)
	if strings.Contains(failed.Error, "secret") {
		t.Fatalf("error leaks the passkey: %s", failed.Error)
	}
}
