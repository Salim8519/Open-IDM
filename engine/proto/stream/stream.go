// Package stream downloads HLS (.m3u8) and DASH (.mpd) streams. It provides two
// engine.Protocol implementations, kinds "hls" and "dash" (see ADR 0012).
//
// Segments are fetched in parallel and written in order into the job's part file (and,
// for a second track, into a temp folder next to it). Resume state is the next segment
// and the bytes written per track. When ffmpeg is available the result is remuxed into
// an .mp4 without re-encoding; DASH (and HLS with a separate audio rendition) needs it to
// combine video and audio.
package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

// Options configure both protocols.
type Options struct {
	// FFmpeg is the path to the ffmpeg executable. Empty = look next to the running
	// executable, then on PATH. A set path that does not exist means "no ffmpeg".
	FFmpeg string
	// Parallel is the number of segments fetched at once per track (default 4).
	Parallel int
}

// Protocol is the HLS or DASH downloader.
type Protocol struct {
	kind string
	ext  string // URL path suffix it matches
	opt  Options

	// Test hooks.
	ffmpeg  func() string // finds ffmpeg; "" = none
	poll    time.Duration // live reload interval override
	backoff time.Duration // first retry delay
}

// NewHLS returns the "hls" protocol for .m3u8 playlists.
func NewHLS(opt Options) *Protocol { return newProtocol("hls", ".m3u8", opt) }

// NewDASH returns the "dash" protocol for .mpd manifests.
func NewDASH(opt Options) *Protocol { return newProtocol("dash", ".mpd", opt) }

func newProtocol(kind, ext string, opt Options) *Protocol {
	if opt.Parallel <= 0 {
		opt.Parallel = 4
	}
	opt.Parallel = min(opt.Parallel, 16)
	p := &Protocol{kind: kind, ext: ext, opt: opt, backoff: 500 * time.Millisecond}
	p.ffmpeg = func() string { return findFFmpeg(p.opt.FFmpeg) }
	return p
}

// Kind implements engine.Protocol.
func (p *Protocol) Kind() string { return p.kind }

// Match implements engine.Protocol: http(s) URLs whose path ends in .m3u8 / .mpd.
func (p *Protocol) Match(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return strings.HasSuffix(strings.ToLower(u.Path), p.ext)
}

// Limits for live recordings, so a dead or frozen stream cannot keep us busy forever.
const (
	maxReloadFailures = 5  // consecutive playlist reload failures
	maxStaleReloads   = 12 // consecutive reloads without a new segment
)

// trackState is the resume state of one track (video, or audio).
type trackState struct {
	Next  int64  `json:"next"`           // sequence number of the next segment; -1 = from the start
	Bytes int64  `json:"bytes"`          // bytes written to the track's file
	Init  string `json:"init,omitempty"` // init segment last written (so it is written once)
}

// state is saved with Job.SaveState.
type state struct {
	Sources  []string     `json:"sources,omitempty"` // HLS: chosen media playlists; DASH: representation ids
	Tracks   []trackState `json:"tracks"`
	Remux    bool         `json:"remux,omitempty"`    // finish with ffmpeg into .mp4
	Live     bool         `json:"live,omitempty"`     // a live recording
	Stopped  bool         `json:"stopped,omitempty"`  // live recording ended by pause/stop
	Complete bool         `json:"complete,omitempty"` // every segment is on disk
	Final    bool         `json:"final,omitempty"`    // the part file holds the final output
}

// list is one load of a track's segments.
type list struct {
	segs   []segment
	ended  bool
	target time.Duration
}

// source is one track to download.
type source struct {
	name     string // "video", "audio" (for messages)
	live     bool
	first    *list
	reload   func(ctx context.Context) (*list, error)
	estimate int64 // bytes, from bandwidth × duration; -1 unknown
}

// plan is what a protocol found in the playlist / manifest.
type plan struct {
	sources    []*source
	ids        []string
	ext        string // output extension when not remuxing
	needFFmpeg bool   // separate tracks must be muxed
	live       bool
}

// Run implements engine.Protocol.
func (p *Protocol) Run(ctx context.Context, j *engine.Job) error {
	var st state
	if !j.Fresh && len(j.State) > 0 {
		if err := json.Unmarshal(j.State, &st); err != nil {
			st = state{} // unreadable: start over
		}
	}
	origin := ""
	if u, err := url.Parse(j.URL); err == nil {
		origin = strings.ToLower(u.Hostname())
	}
	r := &runner{p: p, j: j, st: &st, cache: map[string]*cacheEntry{}}
	r.c = &client{hc: j.HTTPClient(), headers: j.Headers, ua: j.UserAgent, origin: origin,
		count: j.Received, backoff: p.backoff}
	return r.run(ctx)
}

type runner struct {
	p  *Protocol
	j  *engine.Job
	c  *client
	mu sync.Mutex // guards st, est, total, first, lastSize
	st *state

	est      []int64 // measured size estimate per track (-1 unknown)
	base     []int64 // bandwidth estimate per track
	total    []int   // segments per track (VOD)
	first    []int64 // first sequence number per track
	lastSize int64

	keyMu  sync.Mutex
	cache  map[string]*cacheEntry
	active atomic.Int32
}

func (r *runner) tmpDir() string {
	return strings.TrimSuffix(r.j.PartPath, engine.PartSuffix) + ".peregrine.tmp"
}

func (r *runner) trackPath(i int) string {
	if i == 0 {
		return r.j.PartPath
	}
	return filepath.Join(r.tmpDir(), fmt.Sprintf("track-%d", i))
}

func (r *runner) save() {
	_ = r.j.SaveState(r.st) // caller holds r.mu or runs alone
}

func (r *runner) run(ctx context.Context) error {
	r.j.SetResumable(true)
	st := r.st
	if st.Final {
		return nil
	}
	if st.Complete || st.Stopped {
		return r.finish(ctx)
	}
	ffmpeg := r.p.ffmpeg()
	fresh := len(st.Tracks) == 0
	if fresh {
		st.Sources = nil
	}
	pl, err := r.p.plan(ctx, r, st.Sources, ffmpeg != "")
	if err != nil {
		return err
	}
	if len(pl.sources) == 0 {
		return errors.New("no tracks to download")
	}
	if fresh {
		if pl.needFFmpeg && ffmpeg == "" {
			return ErrFFmpegMissing
		}
		st.Remux, st.Live, st.Sources = ffmpeg != "", pl.live, pl.ids
		st.Tracks = make([]trackState, len(pl.sources))
		for i := range st.Tracks {
			st.Tracks[i].Next = -1
		}
		ext := pl.ext
		if st.Remux {
			ext = ".mp4"
		}
		if r.j.Fresh {
			if err := r.j.SetName(stem(r.j.Name, r.j.URL) + ext); err != nil {
				return fmt.Errorf("set name: %w", err)
			}
		}
		_ = os.RemoveAll(r.tmpDir())
	} else if len(pl.sources) != len(st.Tracks) {
		return errors.New("the stream changed since the download started; restart it")
	}
	if st.Remux && ffmpeg == "" {
		return ErrFFmpegMissing
	}

	n := len(pl.sources)
	r.est, r.base, r.total, r.first = make([]int64, n), make([]int64, n), make([]int, n), make([]int64, n)
	for i, s := range pl.sources {
		r.est[i], r.base[i] = -1, s.estimate
	}
	r.lastSize = -2
	files := make([]*os.File, n)
	defer func() {
		for _, f := range files {
			if f != nil {
				_ = f.Close()
			}
		}
	}()
	var have int64
	for i := range pl.sources {
		f, err := r.openTrack(i)
		if err != nil {
			return err
		}
		files[i] = f
		have += st.Tracks[i].Bytes
	}
	r.j.SetDownloaded(have)
	r.save()
	if st.Live {
		r.j.SetSize(-1)
	} else {
		r.reportSize() // bandwidth × duration until segments give a better estimate
	}

	tctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i, s := range pl.sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if errs[i] = r.track(tctx, i, s, files[i]); errs[i] != nil {
				cancel()
			}
		}()
	}
	wg.Wait()
	r.j.SetActive(0)
	if ctx.Err() != nil {
		r.mu.Lock()
		if st.Live && r.written() > 0 { // a pause or stop ends a live recording
			st.Stopped = true
			r.save()
		}
		r.mu.Unlock()
		return ctx.Err()
	}
	for _, err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	for _, f := range files {
		if err := f.Close(); err != nil {
			return fmt.Errorf("write: %w", err)
		}
	}
	files = nil
	r.mu.Lock()
	st.Complete = true
	r.save()
	r.mu.Unlock()
	return r.finish(ctx)
}

func (r *runner) written() int64 {
	var n int64
	for _, t := range r.st.Tracks {
		n += t.Bytes
	}
	return n
}

// openTrack opens track i's file at its saved length (what is beyond it is a segment
// that was not finished). A file shorter than the state restarts that track.
func (r *runner) openTrack(i int) (*os.File, error) {
	p := r.trackPath(i)
	if i > 0 {
		if err := os.MkdirAll(r.tmpDir(), 0o755); err != nil {
			return nil, fmt.Errorf("create temp folder: %w", err)
		}
	}
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open output: %w", err)
	}
	ts := &r.st.Tracks[i]
	if info, err := f.Stat(); err != nil || info.Size() < ts.Bytes {
		*ts = trackState{Next: -1}
	}
	if err := f.Truncate(ts.Bytes); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("truncate output: %w", err)
	}
	if _, err := f.Seek(ts.Bytes, 0); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("seek output: %w", err)
	}
	return f, nil
}

// finish remuxes (when chosen) into the part file and removes the temp folder.
func (r *runner) finish(ctx context.Context) error {
	st := r.st
	if st.Remux {
		ffmpeg := r.p.ffmpeg()
		if ffmpeg == "" {
			return ErrFFmpegMissing
		}
		if err := os.MkdirAll(r.tmpDir(), 0o755); err != nil {
			return fmt.Errorf("create temp folder: %w", err)
		}
		inputs := []string{r.j.PartPath}
		for i := 1; i < len(st.Tracks); i++ {
			inputs = append(inputs, r.trackPath(i))
		}
		for i, in := range inputs {
			abs, err := filepath.Abs(in)
			if err != nil {
				return fmt.Errorf("output path: %w", err)
			}
			inputs[i] = abs
		}
		out, err := filepath.Abs(filepath.Join(r.tmpDir(), "out.mp4"))
		if err != nil {
			return fmt.Errorf("output path: %w", err)
		}
		if err := remux(ctx, ffmpeg, inputs, out); err != nil {
			return err
		}
		if err := os.Rename(out, r.j.PartPath); err != nil {
			return fmt.Errorf("move remuxed file: %w", err)
		}
	}
	st.Final = true
	r.save()
	_ = os.RemoveAll(r.tmpDir())
	if info, err := os.Stat(r.j.PartPath); err == nil {
		r.j.SetSize(info.Size())
		r.j.SetDownloaded(info.Size())
	}
	return nil
}

// track downloads one track: once for VOD, repeatedly for a live playlist.
func (r *runner) track(ctx context.Context, i int, src *source, f *os.File) error {
	l := src.first
	stale, failures := 0, 0
	for {
		if l == nil {
			nl, err := src.reload(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				failures++
				if failures >= maxReloadFailures {
					r.mu.Lock()
					got := r.st.Tracks[i].Bytes
					r.mu.Unlock()
					if got > 0 {
						return nil // the stream went away: end the recording with what we have
					}
					return err
				}
				if err := sleep(ctx, r.p.reloadEvery(time.Second)); err != nil {
					return err
				}
				continue
			}
			failures, l = 0, nl
		}
		r.mu.Lock()
		next := r.st.Tracks[i].Next
		if !src.live && len(l.segs) > 0 {
			r.total[i], r.first[i] = len(l.segs), l.segs[0].Seq
		}
		r.mu.Unlock()
		start := 0
		if next >= 0 {
			start = len(l.segs)
			for k, s := range l.segs {
				if s.Seq >= next { // a live window that moved past next is joined at its start
					start = k
					break
				}
			}
		}
		pending := l.segs[start:]
		if len(pending) > 0 {
			if err := r.download(ctx, i, src, f, pending); err != nil {
				return err
			}
			stale = 0
		} else {
			stale++
		}
		if !src.live || l.ended || stale >= maxStaleReloads {
			return nil
		}
		wait := l.target
		if wait <= 0 {
			wait = 5 * time.Second
		}
		if len(pending) == 0 {
			wait /= 2
		}
		if err := sleep(ctx, r.p.reloadEvery(wait)); err != nil {
			return err
		}
		l = nil
	}
}

func (p *Protocol) reloadEvery(d time.Duration) time.Duration {
	if p.poll > 0 {
		return p.poll
	}
	return d
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// download fetches segs with up to Parallel requests in flight and writes them in order.
func (r *runner) download(ctx context.Context, i int, src *source, f *os.File, segs []segment) error {
	par := r.p.opt.Parallel
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	type result struct {
		data []byte
		err  error
	}
	slots := make([]chan result, len(segs))
	launch := func(k int) {
		ch := make(chan result, 1)
		slots[k] = ch
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.j.SetActive(int(r.active.Add(1)))
			data, err := r.fetchSegment(ctx, src, segs[k])
			r.j.SetActive(int(r.active.Add(-1)))
			ch <- result{data, err}
		}()
	}
	for k := 0; k < min(par, len(segs)); k++ {
		launch(k)
	}
	for k := range segs {
		var res result
		select {
		case res = <-slots[k]:
		case <-ctx.Done():
			return ctx.Err()
		}
		if res.err != nil {
			return res.err
		}
		if k+par < len(segs) {
			launch(k + par)
		}
		if err := r.write(ctx, i, src, f, segs[k], res.data); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) fetchSegment(ctx context.Context, src *source, s segment) ([]byte, error) {
	what := fmt.Sprintf("%s segment %d", src.name, s.Seq)
	data, err := r.c.get(ctx, s.Res, maxSegmentBytes, what, true)
	if err != nil {
		return nil, err
	}
	if s.Key == nil {
		return data, nil
	}
	key, err := r.key(ctx, s.Key.URL)
	if err != nil {
		return nil, err
	}
	iv := s.Key.IV
	if iv == nil {
		iv = ivFromSequence(s.Seq)
	}
	out, err := decryptAES128(data, key, iv)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return out, nil
}

// cached fetches a small shared resource (key, init segment) once, even when several
// segment workers ask for it at the same time. Failures are not cached.
func (r *runner) cached(ctx context.Context, id string, fetch func() ([]byte, error)) ([]byte, error) {
	for {
		r.keyMu.Lock()
		e, ok := r.cache[id]
		if !ok {
			e = &cacheEntry{done: make(chan struct{})}
			r.cache[id] = e
			r.keyMu.Unlock()
			e.data, e.err = fetch()
			if e.err != nil {
				r.keyMu.Lock()
				delete(r.cache, id)
				r.keyMu.Unlock()
			}
			close(e.done)
			return e.data, e.err
		}
		r.keyMu.Unlock()
		select {
		case <-e.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if e.err == nil {
			return e.data, nil
		}
		// the other fetch failed: try ourselves (its error may have been its own cancel)
	}
}

type cacheEntry struct {
	done chan struct{}
	data []byte
	err  error
}

func (r *runner) key(ctx context.Context, keyURL string) ([]byte, error) {
	return r.cached(ctx, "key|"+keyURL, func() ([]byte, error) {
		k, err := r.c.get(ctx, resource{URL: keyURL, Length: -1}, maxKeyBytes, "decryption key", false)
		if err != nil {
			return nil, err
		}
		if len(k) != 16 {
			return nil, fmt.Errorf("decryption key has %d bytes, want 16", len(k))
		}
		return k, nil
	})
}

func (r *runner) initData(ctx context.Context, src *source, res resource) ([]byte, error) {
	return r.cached(ctx, "init|"+res.key(), func() ([]byte, error) {
		return r.c.get(ctx, res, maxSegmentBytes, src.name+" init segment", true)
	})
}

// write appends one segment (preceded by its init segment when that changed) and saves
// the resume state.
func (r *runner) write(ctx context.Context, i int, src *source, f *os.File, s segment, data []byte) error {
	r.mu.Lock()
	lastInit := r.st.Tracks[i].Init
	r.mu.Unlock()
	initKey := lastInit
	n := int64(0)
	if s.Init != nil && s.Init.key() != lastInit {
		b, err := r.initData(ctx, src, *s.Init)
		if err != nil {
			return err
		}
		if _, err := f.Write(b); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		initKey, n = s.Init.key(), int64(len(b))
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	n += int64(len(data))
	r.mu.Lock()
	defer r.mu.Unlock()
	ts := &r.st.Tracks[i]
	ts.Bytes += n
	ts.Next = s.Seq + 1
	ts.Init = initKey
	r.save()
	if !src.live && r.total[i] > 0 {
		if done := ts.Next - r.first[i]; done > 0 {
			r.est[i] = ts.Bytes * int64(r.total[i]) / done
		}
		r.reportSize()
	}
	return nil
}

// reportSize sends the estimated total when it moved by more than 1%. Caller holds r.mu.
func (r *runner) reportSize() {
	var sum int64
	for k := range r.est {
		switch {
		case r.est[k] >= 0:
			sum += r.est[k]
		case r.base[k] >= 0:
			sum += r.base[k]
		default:
			return
		}
	}
	if d := sum - r.lastSize; r.lastSize < 0 || d > r.lastSize/100 || -d > r.lastSize/100 {
		r.lastSize = sum
		r.j.SetSize(sum)
	}
}

// stem is the output name without extension: the job's name (from the URL or the user)
// minus a playlist/media extension.
func stem(name, rawURL string) string {
	if name == "" {
		if u, err := url.Parse(rawURL); err == nil {
			name = path.Base(u.Path)
		}
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u8", ".m3u", ".mpd", ".ts", ".mp4", ".m4a", ".webm", ".mkv":
		name = name[:len(name)-len(filepath.Ext(name))]
	}
	if name == "" || name == "." || name == "/" {
		name = "video"
	}
	return name
}
