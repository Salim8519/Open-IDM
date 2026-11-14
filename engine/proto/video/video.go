// Package video downloads from video sites (YouTube, Vimeo, X, TikTok, Instagram, Twitch,
// SoundCloud and the other sites yt-dlp supports) by running yt-dlp as a separate program.
// See docs/decisions/0013-video-sites-yt-dlp.md.
//
// The engine owns the queue, pause, the .peregrine.part file and its rename; this package
// only turns a page URL into one media file at Job.PartPath. yt-dlp works in a folder next
// to it (<PartPath>.d) whose own .part files make pause/resume continue where it stopped.
package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

// Kind is the engine download kind handled by this package.
const Kind = "video"

// Options configures the protocol. Empty paths mean "find it": next to the running
// executable (or in the macOS app's Resources), then on PATH.
type Options struct {
	YtDlpPath  string
	FFmpegPath string
	// Playlist downloads a whole playlist when the URL names one; by default only the
	// video is fetched (--no-playlist). A playlist still ends up as a single file today.
	Playlist bool
	// RestrictFilenames keeps the file name to ASCII without spaces (--restrict-filenames).
	RestrictFilenames bool
}

// Protocol is the "video" engine.Protocol. It is safe for concurrent use.
type Protocol struct {
	opts   Options
	exeDir func() (string, error) // where bundled tools live; replaced in tests
	env    []string               // extra environment for yt-dlp; set in tests
}

// New returns the protocol. Tools are looked up on every Run, not here.
func New(opts Options) *Protocol {
	return &Protocol{opts: opts, exeDir: executableDir}
}

// Kind implements engine.Protocol.
func (*Protocol) Kind() string { return Kind }

// jobAPI is the part of *engine.Job the protocol calls; tests supply a fake.
type jobAPI interface {
	SetName(name string) error
	SetSize(n int64)
	SetDownloaded(n int64)
	SetResumable(ok bool)
	SetActive(n int)
	SpeedLimit() int64
	SaveState(v any) error
}

// job is one Run's input.
type job struct {
	api       jobAPI
	url       string
	headers   map[string]string
	userAgent string
	state     json.RawMessage
	fresh     bool
	partPath  func() string // changes after SetName
}

// state is saved with the download between runs.
type state struct {
	Named bool `json:"named"` // the title was fetched and the name set
}

// Run implements engine.Protocol.
func (p *Protocol) Run(ctx context.Context, j *engine.Job) error {
	return p.run(ctx, &job{
		api: j, url: j.URL, headers: j.Headers, userAgent: j.UserAgent,
		state: j.State, fresh: j.Fresh, partPath: func() string { return j.PartPath },
	})
}

func (p *Protocol) run(ctx context.Context, j *job) error {
	ytdlp, ffmpeg, err := p.tools()
	if err != nil {
		return err
	}
	var st state
	if len(j.state) > 0 {
		_ = json.Unmarshal(j.state, &st)
	}
	j.api.SetResumable(true)
	a := p.argsFor(j, ffmpeg)

	if j.fresh && !st.Named {
		name, err := p.probe(ctx, ytdlp, a, j)
		if err != nil {
			return err
		}
		if name != "" {
			if err := j.api.SetName(name); err != nil {
				return fmt.Errorf("set name: %w", err)
			}
		}
		st.Named = true
		if err := j.api.SaveState(st); err != nil {
			return err
		}
	}

	part := j.partPath()
	work := part + ".d"
	if _, err := os.Stat(work); errors.Is(err, os.ErrNotExist) {
		if info, err := os.Stat(part); err == nil && !info.IsDir() {
			return nil // moved into place by a run that stopped before reporting success
		}
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return fmt.Errorf("create work folder: %w", err)
	}
	removeStaleCookies(work)
	if a.cookies, err = cookieFor(work, j); err != nil {
		return err
	}
	if a.cookies != "" {
		defer os.Remove(a.cookies)
	}

	j.api.SetActive(1)
	defer j.api.SetActive(0)
	result, err := p.download(ctx, ytdlp, a, j, work)
	if a.cookies != "" {
		_ = os.Remove(a.cookies)
	}
	if err != nil {
		return err
	}
	if err := os.Remove(part); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace part file: %w", err)
	}
	if err := os.Rename(result, part); err != nil {
		return fmt.Errorf("move video into place: %w", err)
	}
	if info, err := os.Stat(part); err == nil {
		j.api.SetSize(info.Size())
		j.api.SetDownloaded(info.Size())
	}
	if err := os.RemoveAll(work); err != nil {
		return fmt.Errorf("remove work folder: %w", err)
	}
	return nil
}

func cookieFor(dir string, j *job) (string, error) {
	c := header(j.headers, "Cookie")
	if c == "" {
		return "", nil
	}
	return writeCookieFile(dir, j.url, c)
}

// args holds the yt-dlp options shared by the probe and the download.
type args struct {
	ffmpeg     string
	restrict   bool
	noPlaylist bool
	limit      int64
	cookies    string
	referer    string
	userAgent  string
}

func (p *Protocol) argsFor(j *job, ffmpeg string) args {
	ua := j.userAgent
	if ua == "" {
		ua = header(j.headers, "User-Agent")
	}
	return args{
		ffmpeg:     ffmpeg,
		restrict:   p.opts.RestrictFilenames,
		noPlaylist: !p.opts.Playlist,
		limit:      j.api.SpeedLimit(),
		referer:    header(j.headers, "Referer"),
		userAgent:  ua,
	}
}

// common returns the options for every yt-dlp call. --no-config keeps a user or portable
// config file from adding options (such as --exec) to ours.
func (a args) common() []string {
	out := []string{"--no-config", "--encoding", "utf-8", "--no-mtime"}
	if a.noPlaylist {
		out = append(out, "--no-playlist")
	}
	if a.ffmpeg != "" {
		// Best video + best audio merged; mkv when the streams do not fit in mp4.
		out = append(out, "-f", "bv*+ba/b", "--merge-output-format", "mp4/mkv", "--ffmpeg-location", a.ffmpeg)
	} else {
		out = append(out, "-f", "b") // merging needs ffmpeg: take the best single file
	}
	if a.restrict {
		out = append(out, "--restrict-filenames")
	}
	if a.limit > 0 {
		out = append(out, "--limit-rate", strconv.FormatInt(a.limit, 10))
	}
	if a.cookies != "" {
		out = append(out, "--cookies", a.cookies)
	}
	if a.referer != "" {
		out = append(out, "--referer", a.referer)
	}
	if a.userAgent != "" {
		out = append(out, "--user-agent", a.userAgent)
	}
	return out
}

// probeArgs asks for the final file name (title + extension after merging) only.
func (a args) probeArgs(rawURL string) []string {
	out := append(a.common(), "--simulate", "--print", "filename", "-o", "%(title).200B.%(ext)s")
	return append(out, "--", rawURL)
}

// downloadArgs downloads into work, printing machine-readable progress and the final path.
func (a args) downloadArgs(work, rawURL string) []string {
	tmpl := filepath.Join(strings.ReplaceAll(work, "%", "%%"), "media.%(ext)s")
	out := append(a.common(),
		"--no-simulate", "--continue", "--newline", "--progress",
		"--progress-template", progressTemplate,
		"--print", "after_move:"+fileMarker+"%(filepath)s",
		"-o", tmpl,
	)
	return append(out, "--", rawURL)
}

// probe returns the file name yt-dlp would use, or "" if it printed none.
func (p *Protocol) probe(ctx context.Context, ytdlp string, a args, j *job) (string, error) {
	pc := a
	if pc.cookies == "" {
		if c := header(j.headers, "Cookie"); c != "" {
			dir, err := os.MkdirTemp(filepath.Dir(j.partPath()), ".peregrine-probe-*")
			if err != nil {
				return "", fmt.Errorf("create probe folder: %w", err)
			}
			defer os.RemoveAll(dir)
			if pc.cookies, err = writeCookieFile(dir, j.url, c); err != nil {
				return "", err
			}
		}
	}
	var name string
	err := p.exec(ctx, ytdlp, pc.probeArgs(j.url), func(stdout bool, line string) {
		if s := strings.TrimSpace(line); stdout && s != "" {
			name = s
		}
	})
	if err != nil {
		return "", err
	}
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return name, nil
}

// download runs yt-dlp in work and returns the path of the finished file.
func (p *Protocol) download(ctx context.Context, ytdlp string, a args, j *job, work string) (string, error) {
	var (
		mu     sync.Mutex
		prog   progress
		result string
	)
	err := p.exec(ctx, ytdlp, a.downloadArgs(work, j.url), func(stdout bool, line string) {
		mu.Lock()
		defer mu.Unlock()
		if done, size, ok := prog.parse(line); ok {
			j.api.SetDownloaded(done)
			if size > 0 {
				j.api.SetSize(size)
			}
			return
		}
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), strings.TrimSpace(fileMarker)); ok && stdout {
			result = strings.TrimSpace(rest)
		}
	})
	if err != nil {
		return "", err
	}
	if inside(work, result) {
		if info, err := os.Stat(result); err == nil && !info.IsDir() {
			return result, nil
		}
	}
	return findResult(work)
}

// inside reports whether path is a file within dir.
func inside(dir, path string) bool {
	if path == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

// findResult is the fallback when yt-dlp did not print the final path: the one finished
// file in the work folder.
func findResult(work string) (string, error) {
	entries, err := os.ReadDir(work)
	if err != nil {
		return "", fmt.Errorf("read work folder: %w", err)
	}
	var found []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || strings.HasSuffix(n, ".part") || strings.HasSuffix(n, ".ytdl") ||
			strings.Contains(n, ".temp.") || strings.HasPrefix(n, "cookies-") {
			continue
		}
		found = append(found, filepath.Join(work, n))
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", errors.New("yt-dlp finished but produced no file")
	default:
		return "", fmt.Errorf("yt-dlp produced %d files instead of one (is ffmpeg missing?)", len(found))
	}
}

// exec runs yt-dlp (never through a shell) and feeds each output line to onLine. A
// cancelled ctx kills yt-dlp and its children, and exec returns ctx's error.
func (p *Protocol) exec(ctx context.Context, bin string, argv []string, onLine func(stdout bool, line string)) error {
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	cmd.Env = append(cmd.Env, p.env...)
	cmd.WaitDelay = 5 * time.Second // do not hang on pipes a stray child keeps open
	tree := newProcTree(cmd)
	defer tree.close()

	var mu sync.Mutex
	var tail []string // last stderr lines, for the error message
	out := &lineWriter{fn: func(s string) { onLine(true, s) }}
	errw := &lineWriter{fn: func(s string) {
		onLine(false, s)
		if !strings.HasPrefix(strings.TrimSpace(s), progressMarker) {
			mu.Lock()
			tail = append(tail, s)
			if len(tail) > 50 {
				tail = tail[1:]
			}
			mu.Unlock()
		}
	}}
	cmd.Stdout, cmd.Stderr = out, errw
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start yt-dlp: %w", err)
	}
	tree.started(cmd)
	err := cmd.Wait()
	out.flush()
	errw.flush()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return nil
	}
	mu.Lock()
	msg := errorMessage(tail)
	mu.Unlock()
	if msg != "" {
		return fmt.Errorf("yt-dlp: %s", msg)
	}
	return fmt.Errorf("yt-dlp failed: %w", err)
}
