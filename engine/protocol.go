package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Protocol downloads one kind of source besides plain HTTP(S): FTP, HLS/DASH streams,
// video pages (yt-dlp), BitTorrent. The Manager keeps everything around it — queue,
// schedule, pause/resume, persisted progress, the .peregrine.part file and its atomic
// rename, checksum, Mark-of-the-Web and speed limits. A Protocol only moves bytes.
//
// Protocols live in their own packages (engine/proto/...) so the core keeps few
// dependencies; the host app registers the ones it ships with Manager.RegisterProtocol.
type Protocol interface {
	// Kind is the stable id stored in Download.Kind, e.g. "ftp", "hls", "video", "torrent".
	Kind() string
	// Match reports whether this protocol handles a URL the user added without naming a
	// kind (by scheme, file extension or host). It must be cheap and never do I/O.
	Match(rawURL string) bool
	// Run downloads into j.PartPath (a file, or a directory after j.SetDirectory) and
	// returns nil when complete. It must stop promptly when ctx is cancelled (pause,
	// shutdown, schedule) and return ctx's error; the next Run gets j.State back.
	Run(ctx context.Context, j *Job) error
}

// Job is what a Protocol sees of one download.
type Job struct {
	URL       string
	Headers   map[string]string // from the browser: Cookie, Referer, User-Agent
	UserAgent string            // settings override ("" = protocol default)
	Name      string            // current file name
	PartPath  string            // write here: <dir>/<name>.peregrine.part
	State     json.RawMessage   // protocol resume state saved by the previous Run (nil at first)
	Fresh     bool              // no bytes on disk yet: the name may still change

	t *task
}

// HTTPClient returns the engine's HTTP client (proxy and timeouts from settings).
func (j *Job) HTTPClient() *http.Client {
	j.t.m.mu.Lock()
	defer j.t.m.mu.Unlock()
	return j.t.m.client
}

// SetName proposes the real file (or folder) name, e.g. from a torrent's metadata. It is
// sanitized and made unique, and updates j.Name and j.PartPath. Only allowed while Fresh.
func (j *Job) SetName(name string) error {
	if !j.Fresh {
		return nil // the name is fixed once data is on disk
	}
	m := j.t.m
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.downloads[j.t.id]
	if !ok {
		return ErrNotFound
	}
	d.Filename = SanitizeFilename(name)
	d.Category = Category(d.Filename)
	m.resolveDirLocked(d)
	p := uniquePath(filepath.Join(d.Dir, d.Filename), func(p string) bool { return m.takenLocked(d.ID, p) })
	d.Dir, d.Filename = filepath.Dir(p), filepath.Base(p)
	d.AutoName, d.AutoDir = false, false
	j.Name, j.PartPath = d.Filename, d.PartPath()
	_ = m.store.SaveDownload(d)
	snap := m.snapshotLocked(d)
	m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	return nil
}

// SetDirectory marks the output as a folder (multi-file torrents): PartPath is created as
// a directory and renamed as a whole when done; no single-file SHA-256 is computed.
func (j *Job) SetDirectory() error {
	m := j.t.m
	m.mu.Lock()
	if d, ok := m.downloads[j.t.id]; ok && !d.IsDir {
		d.IsDir = true
		_ = m.store.SaveDownload(d)
	}
	m.mu.Unlock()
	return os.MkdirAll(j.PartPath, 0o755)
}

// SetSize reports the total size in bytes (-1 if unknown).
func (j *Job) SetSize(n int64) {
	m := j.t.m
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.downloads[j.t.id]; ok && d.Size != n {
		d.Size = n
		snap := m.snapshotLocked(d)
		m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	}
}

// SetResumable tells the UI whether a pause keeps progress.
func (j *Job) SetResumable(ok bool) {
	j.t.resumable = ok
	m := j.t.m
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, found := m.downloads[j.t.id]; found {
		d.Resumable = ok
	}
}

// SetActive reports live connections or peers (shown as "active connections").
func (j *Job) SetActive(n int) { j.t.active.Store(int32(n)) }

// Downloaded returns the byte count so far (restored from the previous run).
func (j *Job) Downloaded() int64 { return j.t.downloaded.Load() }

// SetDownloaded sets the byte count (protocols that track progress themselves).
func (j *Job) SetDownloaded(n int64) { j.t.downloaded.Store(n) }

// Received counts n new bytes and waits as needed for the global and per-download speed
// limits. Protocols that move bytes themselves should call it for every chunk.
func (j *Job) Received(ctx context.Context, n int) error {
	for n > 0 {
		c := min(n, bufSize)
		if err := j.t.limiter.WaitN(ctx, c); err != nil {
			return err
		}
		if err := j.t.m.globalLimiter().WaitN(ctx, c); err != nil {
			return err
		}
		j.t.downloaded.Add(int64(c))
		n -= c
	}
	return nil
}

// SpeedLimit returns the effective limit in bytes/s for protocols that throttle
// themselves (0 = unlimited): the smaller of the global and per-download limits.
func (j *Job) SpeedLimit() int64 {
	m := j.t.m
	m.mu.Lock()
	defer m.mu.Unlock()
	lim := m.settings.GlobalLimit
	if d, ok := m.downloads[j.t.id]; ok && d.SpeedLimit > 0 && (lim == 0 || d.SpeedLimit < lim) {
		lim = d.SpeedLimit
	}
	return lim
}

// SaveState stores resume state; it is persisted with the download about once a second
// and handed back in Job.State on the next Run.
func (j *Job) SaveState(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	j.t.protoMu.Lock()
	j.t.protoState = b
	j.t.protoMu.Unlock()
	return nil
}

// RegisterProtocol adds a protocol. Call before adding downloads of that kind; later
// registrations take precedence in Match.
func (m *Manager) RegisterProtocol(p Protocol) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.protocols = append([]Protocol{p}, m.protocols...)
	m.scheduleLocked() // downloads of this kind may be waiting since start-up
}

// Kinds lists the registered protocol kinds besides "http".
func (m *Manager) Kinds() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.protocols))
	for _, p := range m.protocols {
		out = append(out, p.Kind())
	}
	return out
}

func (m *Manager) protocolLocked(kind string) Protocol {
	for _, p := range m.protocols {
		if p.Kind() == kind {
			return p
		}
	}
	return nil
}

// kindFor picks the protocol for a new download: an explicit kind must be registered;
// otherwise the first protocol that matches, else plain HTTP(S).
func (m *Manager) kindForLocked(rawURL, explicit string) (string, error) {
	if explicit != "" && explicit != KindHTTP {
		if m.protocolLocked(explicit) == nil {
			return "", fmt.Errorf("download type %q is not available", explicit)
		}
		return explicit, nil
	}
	if explicit == "" {
		for _, p := range m.protocols {
			if p.Match(rawURL) {
				return p.Kind(), nil
			}
		}
	}
	if err := validateURL(rawURL); err != nil {
		return "", err
	}
	return KindHTTP, nil
}

// KindHTTP is the built-in segmented HTTP(S) downloader (Download.Kind "" means the same).
const KindHTTP = "http"

// runProtocol is task.run for non-HTTP kinds.
func (t *task) runProtocol(p Protocol) error {
	m := t.m
	d := m.snapshot(t.id)
	if d == nil {
		return ErrNotFound
	}
	settings := m.Settings()
	if err := os.MkdirAll(d.Dir, 0o755); err != nil {
		return fmt.Errorf("create folder: %w", err)
	}
	j := &Job{
		URL: d.URL, Headers: d.Headers, UserAgent: settings.UserAgent,
		Name: d.Filename, PartPath: d.PartPath(), State: d.ProtoState,
		Fresh: d.Downloaded == 0 && len(d.ProtoState) == 0 && !exists(d.PartPath()), t: t,
	}
	t.downloaded.Store(d.Downloaded)
	t.resumable = d.Resumable
	m.setRunning(t.id, d.Resumable, 1)

	stop := make(chan struct{})
	saved := make(chan struct{})
	go func() {
		defer close(saved)
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		for {
			select {
			case <-tk.C:
				m.persistProto(t)
			case <-stop:
				return
			}
		}
	}()
	err := p.Run(t.ctx, j)
	close(stop)
	<-saved
	m.persistProto(t)
	if cause := context.Cause(t.ctx); cause != nil {
		return cause
	}
	return err
}

func (m *Manager) persistProto(t *task) {
	t.protoMu.Lock()
	state := t.protoState
	t.protoMu.Unlock()
	m.mu.Lock()
	d, ok := m.downloads[t.id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if state != nil {
		d.ProtoState = state
	}
	d.Downloaded = t.downloaded.Load()
	c := *d
	m.mu.Unlock()
	_ = m.store.SaveDownload(&c)
}

// protoMu/protoState live on task (see task.go); declared here to keep protocol code together.
type protoFields struct {
	protoMu    sync.Mutex
	protoState json.RawMessage
}

// removePart deletes an unfinished part file or folder.
func removePart(d *Download) error {
	if d.IsDir {
		return os.RemoveAll(d.PartPath())
	}
	return os.Remove(d.PartPath())
}

// markTree sets Mark-of-the-Web / quarantine on a file, or on every file in a folder.
func markTree(path, url, referrer string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if !info.IsDir() {
		markFromInternet(path, url, referrer)
		return
	}
	_ = filepath.WalkDir(path, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			markFromInternet(p, url, referrer)
		}
		return nil
	})
}
