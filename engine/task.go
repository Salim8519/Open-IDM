package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const bufSize = 64 << 10

var bufPool = sync.Pool{New: func() any { b := make([]byte, bufSize); return &b }}

var errPaused = errors.New("paused")

// seg is a live segment. A worker holds mu while writing so that a splitter always
// sees an exact Done and can never shrink End below bytes already written.
type seg struct {
	mu      sync.Mutex
	start   int64
	end     int64 // exclusive; -1 = unknown (until EOF)
	done    int64
	running bool
}

func (s *seg) remaining() int64 {
	if s.end < 0 {
		return -1
	}
	return s.end - s.start - s.done
}

// task runs one download. It is created by the Manager and lives until the download
// stops (pause, error, completion).
type task struct {
	m      *Manager
	id     string
	ctx    context.Context
	cancel context.CancelCauseFunc
	done   chan struct{}

	mu        sync.Mutex // guards segs and failErr
	segs      []*seg
	failErr   error
	resumable bool

	protoFields // resume state of non-HTTP protocols

	file       *os.File
	downloaded atomic.Int64
	active     atomic.Int32
	limiter    *rate.Limiter
}

func newLimiter(bps int64) *rate.Limiter {
	if bps <= 0 {
		return rate.NewLimiter(rate.Inf, bufSize)
	}
	burst := max(bufSize, int(bps/8))
	return rate.NewLimiter(rate.Limit(bps), burst)
}

func setLimiter(l *rate.Limiter, bps int64) {
	if bps <= 0 {
		l.SetLimit(rate.Inf)
		l.SetBurst(bufSize)
		return
	}
	l.SetLimit(rate.Limit(bps))
	l.SetBurst(max(bufSize, int(bps/8)))
}

// run executes the download and returns nil on completion, errPaused on pause, or an error.
func (t *task) run() error {
	m := t.m
	d := m.snapshot(t.id)
	if d == nil {
		return ErrNotFound
	}
	settings := m.Settings()

	fresh := len(d.Segments) == 0 || d.Downloaded == 0
	pr, err := probe(t.ctx, m.client, d.URL, d.Headers, settings.UserAgent)
	if err != nil {
		return err
	}

	if !fresh {
		changed := (d.Size >= 0 && pr.Size >= 0 && d.Size != pr.Size) ||
			(d.ETag != "" && pr.ETag != "" && d.ETag != pr.ETag) ||
			(d.LastMod != "" && pr.LastMod != "" && d.LastMod != pr.LastMod)
		if changed {
			return ErrRemoteChanged
		}
		if !pr.Ranges || !exists(d.PartPath()) {
			fresh = true // cannot resume: start over
		}
	}

	t.resumable = pr.Ranges && pr.Size > 0
	if fresh {
		if err := m.prepareFresh(t.id, pr); err != nil {
			return err
		}
		d = m.snapshot(t.id)
	}

	if err := os.MkdirAll(d.Dir, 0o755); err != nil {
		return fmt.Errorf("create folder: %w", err)
	}
	flags := os.O_RDWR | os.O_CREATE
	if fresh {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(d.PartPath(), flags, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	t.file = f
	defer f.Close()

	// Build the live segment plan.
	conns := clamp(d.Connections, 1, MaxConnections)
	if fresh {
		if d.Size > 0 {
			_ = f.Truncate(d.Size) // pre-allocate (sparse where supported)
		}
		t.segs = planSegments(d.Size, conns, t.resumable)
	} else {
		for _, s := range d.Segments {
			t.segs = append(t.segs, &seg{start: s.Start, end: s.End, done: s.Done})
		}
	}
	var total int64
	for _, s := range t.segs {
		total += s.done
	}
	t.downloaded.Store(total)
	if !t.resumable {
		conns = 1
	}
	m.setRunning(t.id, t.resumable, conns)

	// Persist progress periodically so a crash loses at most ~1s.
	stopSaver := make(chan struct{})
	saverDone := make(chan struct{})
	go func() {
		defer close(saverDone)
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		for {
			select {
			case <-tk.C:
				m.persistProgress(t)
			case <-stopSaver:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < conns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t.worker(settings.MaxRetries)
		}()
	}
	wg.Wait()
	close(stopSaver)
	<-saverDone

	if cause := context.Cause(t.ctx); cause != nil {
		_ = f.Sync()
		m.persistProgress(t)
		return cause
	}
	t.mu.Lock()
	ferr := t.failErr
	t.mu.Unlock()
	if ferr != nil {
		_ = f.Sync()
		m.persistProgress(t)
		return ferr
	}
	for _, s := range t.segs {
		if r := s.remaining(); r > 0 {
			m.persistProgress(t)
			return fmt.Errorf("download incomplete: %d bytes missing", r)
		}
	}
	// Unknown-size downloads: trim to what we actually received.
	if d.Size < 0 {
		_ = f.Truncate(t.downloaded.Load())
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("flush to disk: %w", err)
	}
	m.persistProgress(t)
	return nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// planSegments splits [0,size) into up to n equal segments of at least minSplitSize.
func planSegments(size int64, n int, resumable bool) []*seg {
	if !resumable || size <= 0 {
		end := size
		if size < 0 {
			end = -1
		}
		return []*seg{{start: 0, end: end}}
	}
	n = int(min(int64(n), max(1, size/minSplitSize)))
	segs := make([]*seg, 0, n)
	chunk := size / int64(n)
	var start int64
	for i := 0; i < n; i++ {
		end := start + chunk
		if i == n-1 {
			end = size
		}
		segs = append(segs, &seg{start: start, end: end})
		start = end
	}
	return segs
}

// next hands a worker an idle segment, or splits the largest running one (dynamic
// segmentation). It returns nil when there is nothing left worth doing.
func (t *task) next() *seg {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.segs {
		s.mu.Lock()
		r := s.remaining()
		if !s.running && r != 0 {
			s.running = true
			s.mu.Unlock()
			return s
		}
		s.mu.Unlock()
	}
	if !t.resumable {
		return nil
	}
	var victim *seg
	var best int64
	for _, s := range t.segs {
		s.mu.Lock()
		if s.running {
			if r := s.remaining(); r > best {
				best, victim = r, s
			}
		}
		s.mu.Unlock()
	}
	if victim == nil || best < 2*minSplitSize {
		return nil
	}
	victim.mu.Lock()
	r := victim.remaining()
	if r < 2*minSplitSize {
		victim.mu.Unlock()
		return nil
	}
	mid := victim.start + victim.done + r/2
	mid -= mid % 4096 // keep writes page-aligned
	ns := &seg{start: mid, end: victim.end, running: true}
	victim.end = mid
	victim.mu.Unlock()
	t.segs = append(t.segs, ns)
	return ns
}

func (t *task) fail(err error) {
	t.mu.Lock()
	if t.failErr == nil {
		t.failErr = err
	}
	t.mu.Unlock()
	t.cancel(nil)
}

func (t *task) worker(maxRetries int) {
	for {
		if t.ctx.Err() != nil {
			return
		}
		s := t.next()
		if s == nil {
			return
		}
		t.active.Add(1)
		err := t.fetch(s, maxRetries)
		t.active.Add(-1)
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		if err != nil {
			if t.ctx.Err() == nil {
				t.fail(err)
			}
			return
		}
	}
}

// fetch downloads one segment, retrying with backoff on transient errors.
func (t *task) fetch(s *seg, maxRetries int) error {
	failures := 0
	for {
		progressed, err := t.fetchOnce(s)
		if err == nil {
			return nil
		}
		if t.ctx.Err() != nil {
			return t.ctx.Err()
		}
		if !isRetryable(err) {
			return err
		}
		if progressed {
			failures = 0
		}
		failures++
		if failures > maxRetries {
			return fmt.Errorf("giving up after %d retries: %w", maxRetries, err)
		}
		// Exponential backoff with jitter, capped at 60s.
		backoff := min(time.Duration(1<<min(failures-1, 6))*time.Second, 60*time.Second)
		backoff = backoff/2 + rand.N(backoff/2+1)
		select {
		case <-time.After(backoff):
		case <-t.ctx.Done():
			return t.ctx.Err()
		}
	}
}

type fatalError struct{ err error }

func (e *fatalError) Error() string { return e.err.Error() }
func (e *fatalError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var fe *fatalError
	if errors.As(err, &fe) {
		return false
	}
	if errors.Is(err, ErrLinkExpired) {
		return false
	}
	var he *httpStatusError
	if errors.As(err, &he) {
		return he.retryable()
	}
	return true // network errors, early EOF, timeouts
}

func (t *task) fetchOnce(s *seg) (progressed bool, err error) {
	s.mu.Lock()
	pos := s.start + s.done
	end := s.end
	if !t.resumable && s.done > 0 {
		// Server cannot resume: start this (only) segment over.
		s.done = 0
		t.downloaded.Store(0)
		pos = 0
	}
	s.mu.Unlock()
	if end >= 0 && pos >= end {
		return false, nil
	}

	d := t.m.snapshot(t.id)
	if d == nil {
		return false, &fatalError{ErrNotFound}
	}
	req, err := http.NewRequestWithContext(t.ctx, http.MethodGet, d.URL, nil)
	if err != nil {
		return false, &fatalError{err}
	}
	setHeaders(req, d.Headers, t.m.Settings().UserAgent)
	if t.resumable {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(pos, 10)+"-"+strconv.FormatInt(end-1, 10))
		if d.ETag != "" && !isWeakETag(d.ETag) {
			req.Header.Set("If-Range", d.ETag)
		}
	}
	resp, err := t.m.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if err := statusError(resp.StatusCode); err != nil {
		return false, err
	}
	if t.resumable {
		if resp.StatusCode != http.StatusPartialContent {
			// If-Range mismatch or the server dropped range support mid-way.
			return false, &fatalError{ErrRemoteChanged}
		}
		if got, ok := parseContentRangeStart(resp.Header.Get("Content-Range")); !ok || got != pos {
			return false, fmt.Errorf("server returned the wrong range (asked %d, got %d)", pos, got)
		}
	}

	bp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bp)
	buf := *bp
	global := t.m.globalLimiter()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if err := global.WaitN(t.ctx, n); err != nil {
				return progressed, err
			}
			if err := t.limiter.WaitN(t.ctx, n); err != nil {
				return progressed, err
			}
			finished, werr := t.write(s, buf[:n])
			if werr != nil {
				return progressed, &fatalError{fmt.Errorf("write to disk: %w", werr)}
			}
			progressed = true
			if finished {
				return progressed, nil
			}
		}
		if rerr == io.EOF {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.end < 0 {
				s.end = s.start + s.done // unknown size: EOF is the end
				return progressed, nil
			}
			if s.remaining() > 0 {
				return progressed, io.ErrUnexpectedEOF
			}
			return progressed, nil
		}
		if rerr != nil {
			return progressed, rerr
		}
	}
}

func isWeakETag(e string) bool { return len(e) > 2 && e[:2] == "W/" }

// write stores p at the segment's current position, clamped to its (possibly shrunk) end.
func (t *task) write(s *seg, p []byte) (finished bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := s.start + s.done
	if s.end >= 0 {
		allowed := s.end - pos
		if allowed <= 0 {
			return true, nil
		}
		if int64(len(p)) > allowed {
			p = p[:allowed]
		}
	}
	n, err := t.file.WriteAt(p, pos)
	s.done += int64(n)
	t.downloaded.Add(int64(n))
	if err != nil {
		return false, err
	}
	return s.end >= 0 && s.start+s.done >= s.end, nil
}

// segmentsSnapshot copies the live segments for persistence.
func (t *task) segmentsSnapshot() []Segment {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Segment, len(t.segs))
	for i, s := range t.segs {
		s.mu.Lock()
		out[i] = Segment{Start: s.start, End: s.end, Done: s.done}
		s.mu.Unlock()
	}
	return out
}
