package video

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

// fakeBin is the fake yt-dlp built by TestMain from ./internal/fakeytdlp.
var fakeBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeytdlp-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	name := "yt-dlp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fakeBin = filepath.Join(dir, name)
	goBin, err := exec.LookPath("go")
	if err != nil {
		goBin = filepath.Join(os.Getenv("GOROOT"), "bin", "go")
	}
	build := exec.Command(goBin, "build", "-o", fakeBin, "./internal/fakeytdlp")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fake yt-dlp: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// fakeJob records what the protocol reports, like engine.Job would.
type fakeJob struct {
	mu         sync.Mutex
	dir, name  string
	size       int64
	downloaded []int64
	state      json.RawMessage
	resumable  bool
	limit      int64
}

func (f *fakeJob) SetName(n string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.name = engine.SanitizeFilename(n)
	return nil
}
func (f *fakeJob) SetSize(n int64) { f.mu.Lock(); f.size = n; f.mu.Unlock() }
func (f *fakeJob) SetDownloaded(n int64) {
	f.mu.Lock()
	f.downloaded = append(f.downloaded, n)
	f.mu.Unlock()
}
func (f *fakeJob) SetResumable(ok bool) { f.mu.Lock(); f.resumable = ok; f.mu.Unlock() }
func (f *fakeJob) SetActive(int)        {}
func (f *fakeJob) SpeedLimit() int64    { return f.limit }
func (f *fakeJob) SaveState(v any) error {
	b, err := json.Marshal(v)
	f.mu.Lock()
	f.state = b
	f.mu.Unlock()
	return err
}
func (f *fakeJob) partPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return filepath.Join(f.dir, f.name+".peregrine.part")
}
func (f *fakeJob) lastDownloaded() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.downloaded) == 0 {
		return 0
	}
	return f.downloaded[len(f.downloaded)-1]
}

func newFakeJob(t *testing.T) *fakeJob {
	return &fakeJob{dir: t.TempDir(), name: "watch"}
}

func (f *fakeJob) job(rawURL string, headers map[string]string, fresh bool) *job {
	f.mu.Lock()
	st := f.state
	f.mu.Unlock()
	return &job{api: f, url: rawURL, headers: headers, state: st, fresh: fresh, partPath: f.partPath}
}

// testProto returns the protocol wired to the fake yt-dlp and a log of its invocations.
func testProto(t *testing.T) (*Protocol, string) {
	log := filepath.Join(t.TempDir(), "calls.jsonl")
	p := New(Options{YtDlpPath: fakeBin, FFmpegPath: fakeBin})
	p.env = []string{"FAKEYTDLP_LOG=" + log}
	return p, log
}

type call struct {
	Args       []string `json:"args"`
	Cookies    string   `json:"cookies"`
	CookieFile string   `json:"cookieFile"`
	ResumeFrom int64    `json:"resumeFrom"`
}

func readCalls(t *testing.T, path string) []call {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []call
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var c call
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

func isProbe(c call) bool { return slices.Contains(c.Args, "filename") }

func TestDownloadNamesProgressesAndMoves(t *testing.T) {
	p, log := testProto(t)
	fj := newFakeJob(t)
	u := "https://video.test/watch?title=My+Clip&size=95000&chunk=10000"
	if err := p.run(context.Background(), fj.job(u, nil, true)); err != nil {
		t.Fatal(err)
	}
	if fj.name != "My Clip.mp4" {
		t.Fatalf("name = %q, want from --print", fj.name)
	}
	info, err := os.Stat(fj.partPath())
	if err != nil || info.Size() != 95000 {
		t.Fatalf("part file: %v %v", info, err)
	}
	if _, err := os.Stat(fj.partPath() + ".d"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("work folder left behind: %v", err)
	}
	if !fj.resumable || fj.size != 95000 {
		t.Fatalf("resumable=%v size=%d", fj.resumable, fj.size)
	}
	// Progress came from the template lines: many increasing steps ending at the size.
	if len(fj.downloaded) < 9 || !slices.IsSorted(fj.downloaded) || fj.lastDownloaded() != 95000 {
		t.Fatalf("progress = %v", fj.downloaded)
	}
	calls := readCalls(t, log)
	if len(calls) != 2 || !isProbe(calls[0]) || isProbe(calls[1]) {
		t.Fatalf("want probe then download, got %d calls", len(calls))
	}
	for _, want := range []string{"--no-playlist", "--no-mtime", "--newline", "--merge-output-format", "--ffmpeg-location"} {
		if !slices.Contains(calls[1].Args, want) {
			t.Errorf("download args lack %s: %q", want, calls[1].Args)
		}
	}
}

func TestPauseKillsAndResumeReusesWorkFolder(t *testing.T) {
	p, log := testProto(t)
	fj := newFakeJob(t)
	u := "https://video.test/watch?title=Long&size=300000&chunk=10000&delay=20"

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- p.run(ctx, fj.job(u, nil, true)) }()
	deadline := time.Now().Add(10 * time.Second)
	for fj.lastDownloaded() < 50000 {
		if time.Now().After(deadline) {
			t.Fatal("no progress")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("paused run returned %v, want context.Canceled", err)
	}
	partial := filepath.Join(fj.partPath()+".d", "media.mp4.part")
	before, err := os.Stat(partial)
	if err != nil {
		t.Fatalf("yt-dlp part file missing after pause: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if after, _ := os.Stat(partial); after.Size() != before.Size() {
		t.Fatal("yt-dlp still writing after cancel: process not killed")
	}
	if before.Size() >= 300000 {
		t.Fatal("finished before the pause")
	}

	// Resume: no second probe, and yt-dlp continues the part file in the same folder.
	if err := p.run(context.Background(), fj.job(u, nil, false)); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(fj.partPath()); err != nil || info.Size() != 300000 {
		t.Fatalf("final: %v %v", info, err)
	}
	calls := readCalls(t, log)
	probes := 0
	for _, c := range calls {
		if isProbe(c) {
			probes++
		}
	}
	last := calls[len(calls)-1]
	if probes != 1 || last.ResumeFrom < before.Size() || last.ResumeFrom == 0 {
		t.Fatalf("probes=%d resumeFrom=%d (paused at %d)", probes, last.ResumeFrom, before.Size())
	}
}

func TestMissingYtDlp(t *testing.T) {
	fj := newFakeJob(t)
	p := New(Options{YtDlpPath: filepath.Join(t.TempDir(), "nope", "yt-dlp")})
	err := p.run(context.Background(), fj.job("https://youtu.be/x", nil, true))
	if err == nil || !strings.Contains(err.Error(), "yt-dlp not found") {
		t.Fatalf("explicit missing path: %v", err)
	}

	empty := t.TempDir()
	t.Setenv("PATH", empty)
	p = New(Options{})
	p.exeDir = func() (string, error) { return empty, nil }
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/opt/homebrew/bin/yt-dlp"); err == nil {
			t.Skip("Homebrew yt-dlp installed")
		}
		if _, err := os.Stat("/usr/local/bin/yt-dlp"); err == nil {
			t.Skip("yt-dlp installed in /usr/local/bin")
		}
	}
	err = p.run(context.Background(), fj.job("https://youtu.be/x", nil, true))
	if !errors.Is(err, ErrNoYtDlp) {
		t.Fatalf("not found anywhere: %v", err)
	}
}

func TestFindsBundledToolsNextToExecutable(t *testing.T) {
	dir := t.TempDir()
	name := "yt-dlp"
	ff := "ffmpeg"
	if runtime.GOOS == "windows" {
		name, ff = name+".exe", ff+".exe"
	}
	for _, n := range []string{name, ff} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p := New(Options{})
	p.exeDir = func() (string, error) { return dir, nil }
	y, f, err := p.tools()
	if err != nil || y != filepath.Join(dir, name) || f != filepath.Join(dir, ff) {
		t.Fatalf("tools() = %q %q %v", y, f, err)
	}
}

func TestCookiesFileCreatedAndRemoved(t *testing.T) {
	p, log := testProto(t)
	fj := newFakeJob(t)
	fj.limit = 123456
	headers := map[string]string{
		"cookie":     "SID=abc; pref=x=1; bad\tname=1",
		"Referer":    "https://www.youtube.com/feed",
		"User-Agent": "TestBrowser/1.0",
	}
	if err := p.run(context.Background(), fj.job("https://www.youtube.com/watch?v=x&size=20000", headers, true)); err != nil {
		t.Fatal(err)
	}
	calls := readCalls(t, log)
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
	for i, c := range calls {
		if c.CookieFile == "" {
			t.Fatalf("call %d had no --cookies", i)
		}
		if _, err := os.Stat(c.CookieFile); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cookie file %s not removed: %v", c.CookieFile, err)
		}
		for _, want := range []string{
			"# Netscape HTTP Cookie File",
			".youtube.com\tTRUE\t/\tFALSE\t",
			"\tSID\tabc\n",
			"\tpref\tx=1\n",
		} {
			if !strings.Contains(c.Cookies, want) {
				t.Errorf("call %d cookies lack %q:\n%s", i, want, c.Cookies)
			}
		}
		if strings.Contains(c.Cookies, "bad") {
			t.Errorf("cookie with a tab was written:\n%s", c.Cookies)
		}
		if !hasPair(c.Args, "--referer", "https://www.youtube.com/feed") ||
			!hasPair(c.Args, "--user-agent", "TestBrowser/1.0") ||
			!hasPair(c.Args, "--limit-rate", "123456") {
			t.Errorf("call %d args: %q", i, c.Args)
		}
	}
	if !strings.HasPrefix(calls[1].CookieFile, fj.partPath()+".d") {
		t.Errorf("download cookie file %s not in the work folder", calls[1].CookieFile)
	}
	// Nothing else left in the download folder (probe folder, cookies).
	entries, _ := os.ReadDir(fj.dir)
	if len(entries) != 1 {
		t.Fatalf("download folder holds %d entries, want only the part file", len(entries))
	}
}

func TestCookieFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	path, err := writeCookieFile(t.TempDir(), "https://vimeo.com/1", "a=b")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cookie file mode: %v %v", info.Mode(), err)
	}
}

func hasPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}

func TestErrorsAreClearAndRedacted(t *testing.T) {
	p, _ := testProto(t)
	fj := newFakeJob(t)
	err := p.run(context.Background(), fj.job("https://video.test/w?fail=probe", nil, true))
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	for _, bad := range []string{"ERROR:", "token=secret", "pw@", "[fake]", "#x"} {
		if strings.Contains(msg, bad) {
			t.Errorf("message %q contains %q", msg, bad)
		}
	}
	if !strings.Contains(msg, "This video is private") || !strings.Contains(msg, "https://example.com/help for details") {
		t.Errorf("message = %q", msg)
	}

	err = p.run(context.Background(), fj.job("https://video.test/w?fail=download&size=40000", nil, true))
	if err == nil || !strings.Contains(err.Error(), "HTTP Error 403") {
		t.Fatalf("download failure: %v", err)
	}
}

func TestURLAlwaysAfterDoubleDash(t *testing.T) {
	evil := "--exec=calc.exe"
	for _, a := range []args{{}, {ffmpeg: "ff", cookies: "c", referer: "r", userAgent: "u", limit: 5, restrict: true}} {
		for _, argv := range [][]string{a.probeArgs(evil), a.downloadArgs("/w", evil)} {
			n := len(argv)
			if n < 2 || argv[n-2] != "--" || argv[n-1] != evil {
				t.Fatalf("URL not last after --: %q", argv)
			}
			if slices.Contains(argv[:n-2], evil) || slices.Contains(argv[:n-2], "--") {
				t.Fatalf("URL or -- appears early: %q", argv)
			}
			if !slices.Contains(argv, "--no-config") {
				t.Fatalf("--no-config missing: %q", argv)
			}
		}
	}
	// The fake also rejects a URL before --, so every real run above checked it too.
	noFF := args{}.common()
	if !hasPair(noFF, "-f", "b") || slices.Contains(noFF, "--merge-output-format") {
		t.Fatalf("without ffmpeg: %q", noFF)
	}
	if got := (args{}).downloadArgs(`C:\100%`, "u"); !slices.Contains(got, filepath.Join(`C:\100%%`, "media.%(ext)s")) {
		t.Fatalf("%% in folder not escaped: %q", got)
	}
}

func TestMatch(t *testing.T) {
	p := New(Options{})
	yes := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://m.youtube.com/shorts/abc",
		"https://music.youtube.com/watch?v=x",
		"https://vimeo.com/123",
		"https://x.com/user/status/1",
		"https://twitter.com/user/status/1",
		"https://www.tiktok.com/@u/video/1",
		"https://www.instagram.com/reel/abc/",
		"https://www.facebook.com/watch?v=1",
		"https://fb.watch/abc/",
		"https://www.twitch.tv/videos/1",
		"https://clips.twitch.tv/abc",
		"https://www.dailymotion.com/video/x1",
		"https://soundcloud.com/a/b",
		"https://www.bilibili.com/video/BV1",
		"https://www.reddit.com/r/videos/comments/abc/x/",
		"https://v.redd.it/abc",
		"https://streamable.com/abc",
		"HTTPS://WWW.YOUTUBE.COM./watch?v=x",
	}
	no := []string{
		"https://example.com/video.mp4",
		"https://notyoutube.com/watch",
		"https://box.com/file", // must not match x.com
		"https://youtube.com.evil.example/watch",
		"https://www.reddit.com/",   // not a post
		"https://i.redd.it/abc.jpg", // image host
		"ftp://youtube.com/x",
		"javascript:alert(1)",
		"",
		"://bad",
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

func TestProgressParsing(t *testing.T) {
	var pr progress
	steps := []struct {
		line       string
		done, size int64
		ok         bool
	}{
		{"[download] Destination: x", 0, 0, false},
		{progressMarker + " downloading 100 NA 1000.0", 100, 1000, true},
		{progressMarker + " downloading 600 1000 NA", 600, 1000, true},
		{progressMarker + " finished NA 1000 NA", 1000, 1000, true},
		// Audio stream of a merged download starts at zero.
		{progressMarker + " downloading 50 200 NA", 1050, 1200, true},
		{progressMarker + " downloading 200 200 NA", 1200, 1200, true},
		// A third stream without a "finished" line in between.
		{progressMarker + " downloading 10 NA NA", 1210, 1210, true},
		{progressMarker + " garbage", 0, 0, false},
	}
	for i, s := range steps {
		done, size, ok := pr.parse(s.line)
		if ok != s.ok || (ok && (done != s.done || size != s.size)) {
			t.Fatalf("step %d %q: got %d/%d %v, want %d/%d %v", i, s.line, done, size, ok, s.done, s.size, s.ok)
		}
	}
}

func TestLineWriter(t *testing.T) {
	var got []string
	w := &lineWriter{fn: func(s string) { got = append(got, s) }}
	_, _ = w.Write([]byte("a\r\nb\rc"))
	_, _ = w.Write([]byte("d\n"))
	_, _ = w.Write([]byte("tail"))
	w.flush()
	if want := []string{"a", "b", "cd", "tail"}; !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// TestManagerEndToEnd registers the protocol on a real engine.Manager: a YouTube URL is
// matched as "video", named from yt-dlp, moved into place and checksummed by the engine.
func TestManagerEndToEnd(t *testing.T) {
	m := newManager(t)
	p, _ := testProto(t)
	m.RegisterProtocol(p)

	d, err := m.Add(engine.Request{URL: "https://www.youtube.com/watch?v=abc&title=Hello+World&size=64000"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Kind {
		t.Fatalf("kind = %q", d.Kind)
	}
	// A page on an unknown site works when the user picks "video".
	d2, err := m.Add(engine.Request{URL: "https://unknown.example/page?title=Other&size=1000", Kind: Kind})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{d.ID, d2.ID} {
		done := waitDone(t, m, id)
		info, err := os.Stat(done.Path())
		if err != nil || info.Size() != done.Size || done.SHA256 == "" {
			t.Fatalf("%s: %v %v sha=%q", done.Filename, info, err, done.SHA256)
		}
		if _, err := os.Stat(done.PartPath() + ".d"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("work folder left: %v", err)
		}
	}
	if got, _ := m.Get(d.ID); got.Filename != "Hello World.mp4" || got.Size != 64000 {
		t.Fatalf("completed: %q size %d", got.Filename, got.Size)
	}
}

func newManager(t *testing.T) *engine.Manager {
	t.Helper()
	st, err := engine.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	s := engine.DefaultSettings()
	// Not t.TempDir: on Windows, a virus scanner may still hold the fresh video (with its
	// Mark-of-the-Web stream) for a moment, so removal is retried instead of failing.
	dir, err := os.MkdirTemp("", "video-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for range 50 {
			if os.RemoveAll(dir) == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
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
	return m
}

func waitDone(t *testing.T, m *engine.Manager, id string) engine.Download {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		d, err := m.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		switch d.Status {
		case engine.StatusCompleted:
			return d
		case engine.StatusError:
			t.Fatalf("download failed: %s", d.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", id)
	return engine.Download{}
}
