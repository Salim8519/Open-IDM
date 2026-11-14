package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// EventType names a change in the manager.
type EventType string

const (
	EventAdded    EventType = "added"
	EventUpdated  EventType = "updated"  // status/metadata change
	EventProgress EventType = "progress" // periodic batch of active downloads
	EventRemoved  EventType = "removed"
	EventSettings EventType = "settings"
	// EventSchedule: the schedule window opened or closed, or the schedule changed.
	EventSchedule EventType = "schedule"
	// EventQueueDone: the queue ran empty after at least one download completed.
	EventQueueDone EventType = "queueDone"
)

// Event is delivered to subscribers. For EventProgress, Downloads holds the batch.
type Event struct {
	Type      EventType       `json:"type"`
	Download  *Download       `json:"download,omitempty"`
	Downloads []Download      `json:"downloads,omitempty"`
	ID        string          `json:"id,omitempty"`
	Settings  *Settings       `json:"settings,omitempty"`
	Schedule  *ScheduleStatus `json:"schedule,omitempty"`
}

// Manager owns all downloads, the queue and persistence. All methods are goroutine-safe.
type Manager struct {
	mu        sync.Mutex
	store     *Store
	settings  Settings
	downloads map[string]*Download
	tasks     map[string]*task
	speeds    map[string]*speedMeter
	client    *http.Client
	global    *rate.Limiter
	subs      map[int]func(Event)
	nextSub   int
	nextOrder int64
	closed    bool
	wg        sync.WaitGroup
	stopTick  chan struct{}

	protocols []Protocol // non-HTTP downloaders, newest registration first

	now       func() time.Time // clock; tests replace it before any call
	schedOpen bool             // last known schedule state
	completed bool             // a download completed since the queue was last empty
}

type speedMeter struct {
	last  int64
	at    time.Time
	speed float64
}

// NewManager loads state from store and starts any queued downloads.
func NewManager(store *Store) (*Manager, error) {
	return newManager(store, time.Now)
}

func newManager(store *Store, now func() time.Time) (*Manager, error) {
	st, err := store.LoadSettings()
	if err != nil {
		return nil, err
	}
	normalizeSettings(&st)
	if st.Schedule.validate() != nil {
		st.Schedule = defaultSchedule() // repair a bad stored value; never fail start-up
	}
	client, err := newClient(st.Proxy)
	if err != nil {
		client, _ = newClient("system")
	}
	m := &Manager{
		store:     store,
		settings:  st,
		downloads: map[string]*Download{},
		tasks:     map[string]*task{},
		speeds:    map[string]*speedMeter{},
		client:    client,
		global:    newLimiter(st.GlobalLimit),
		subs:      map[int]func(Event){},
		stopTick:  make(chan struct{}),
		now:       now,
	}
	m.schedOpen = st.Schedule.Status(m.now()).Open
	list, err := store.LoadDownloads()
	if err != nil {
		return nil, err
	}
	for _, d := range list {
		if d.Status == StatusDownloading {
			d.Status = StatusQueued // was interrupted (crash or quit): resume automatically
		}
		d.Speed, d.Active, d.ETA = 0, 0, -1
		m.downloads[d.ID] = d
		if d.Order >= m.nextOrder {
			m.nextOrder = d.Order + 1
		}
	}
	m.wg.Add(1)
	go m.ticker()
	m.mu.Lock()
	m.scheduleLocked()
	m.mu.Unlock()
	return m, nil
}

func normalizeSettings(s *Settings) {
	if s.MaxActive < 1 {
		s.MaxActive = 3
	}
	s.MaxActive = clamp(s.MaxActive, 1, 20)
	if s.Connections < 1 {
		s.Connections = 8
	}
	s.Connections = clamp(s.Connections, 1, MaxConnections)
	if s.MaxRetries < 0 {
		s.MaxRetries = 0
	}
	if s.DownloadDir == "" {
		s.DownloadDir = defaultDownloadDir()
	}
	if s.GlobalLimit < 0 {
		s.GlobalLimit = 0
	}
	normalizeSchedule(&s.Schedule)
	switch s.Theme {
	case "field", "field-day", "field-night", "system", "light", "dark":
	default:
		s.Theme = "field"
	}
	// Settings that come from the UI are current by definition, even if the client did
	// not send uiVersion; otherwise a later load would migrate a Classic choice away.
	s.UIVersion = currentUIVersion
}

// Close pauses running downloads (keeping them queued for next start) and saves state.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	tasks := make([]*task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	m.mu.Unlock()
	for _, t := range tasks {
		t.cancel(errShutdown)
		<-t.done
	}
	close(m.stopTick)
	m.wg.Wait()
	return nil
}

var errShutdown = errors.New("shutting down")

// Subscribe registers fn for events and returns an unsubscribe function.
// fn is called from manager goroutines and must not block.
func (m *Manager) Subscribe(fn func(Event)) func() {
	m.mu.Lock()
	id := m.nextSub
	m.nextSub++
	m.subs[id] = fn
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.subs, id)
		m.mu.Unlock()
	}
}

func (m *Manager) emitLocked(e Event) {
	for _, fn := range m.subs {
		fn(e)
	}
}

// snapshotLocked returns a copy of d merged with live progress. m.mu must be held.
func (m *Manager) snapshotLocked(d *Download) Download {
	c := *d
	c.Headers = nil             // never send cookies/auth to the UI
	c.URL = HidePassword(c.URL) // nor a password inside the link (ftp://user:pass@host)
	c.ProtoState = nil          // protocol internals (may hold tracker passkeys)
	if t, ok := m.tasks[d.ID]; ok {
		c.Downloaded = t.downloaded.Load()
		c.Active = int(t.active.Load())
	}
	if sm, ok := m.speeds[d.ID]; ok && c.Status == StatusDownloading {
		c.Speed = int64(sm.speed)
	} else {
		c.Speed = 0
	}
	c.ETA = -1
	if c.Speed > 0 && c.Size > 0 {
		c.ETA = (c.Size - c.Downloaded) / c.Speed
	}
	c.Segments = append([]Segment(nil), d.Segments...)
	return c
}

// snapshot returns an internal copy including headers (for the task), or nil.
func (m *Manager) snapshot(id string) *Download {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.downloads[id]
	if !ok {
		return nil
	}
	c := *d
	c.Headers = make(map[string]string, len(d.Headers))
	for k, v := range d.Headers {
		c.Headers[k] = v
	}
	return &c
}

// List returns all downloads in queue order.
func (m *Manager) List() []Download {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Download, 0, len(m.downloads))
	for _, d := range m.downloads {
		out = append(out, m.snapshotLocked(d))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// Get returns one download.
func (m *Manager) Get(id string) (Download, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.downloads[id]
	if !ok {
		return Download{}, ErrNotFound
	}
	return m.snapshotLocked(d), nil
}

// Settings returns the current settings.
func (m *Manager) Settings() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings
}

// UpdateSettings validates, applies and saves settings.
func (m *Manager) UpdateSettings(s Settings) (Settings, error) {
	normalizeSettings(&s)
	if err := s.Schedule.validate(); err != nil {
		return Settings{}, err
	}
	if s.Proxy != m.Settings().Proxy {
		c, err := newClient(s.Proxy)
		if err != nil {
			return Settings{}, err
		}
		m.mu.Lock()
		m.client = c
		m.mu.Unlock()
	}
	if err := m.store.SaveSettings(s); err != nil {
		return Settings{}, err
	}
	m.mu.Lock()
	m.settings = s
	setLimiter(m.global, s.GlobalLimit)
	m.emitLocked(Event{Type: EventSettings, Settings: &s})
	stop := m.checkScheduleLocked(true)
	m.scheduleLocked()
	m.mu.Unlock()
	stopTasks(stop)
	return s, nil
}

// ScheduleStatus reports whether the schedule lets downloads run now.
func (m *Manager) ScheduleStatus() ScheduleStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings.Schedule.Status(m.now())
}

var errScheduleStop = errors.New("schedule window closed")

// checkScheduleLocked notices the window opening or closing. When it closes it returns the
// running tasks, which the caller must stop (outside the lock) with stopTasks; they go
// back to the queue. force emits the status even without a change.
func (m *Manager) checkScheduleLocked(force bool) []*task {
	st := m.settings.Schedule.Status(m.now())
	changed := st.Open != m.schedOpen
	m.schedOpen = st.Open
	if changed || force {
		m.emitLocked(Event{Type: EventSchedule, Schedule: &st})
	}
	if !changed {
		return nil
	}
	if st.Open {
		m.scheduleLocked()
		return nil
	}
	stop := make([]*task, 0, len(m.tasks))
	for _, t := range m.tasks {
		stop = append(stop, t)
	}
	return stop
}

func stopTasks(ts []*task) {
	for _, t := range ts {
		t.cancel(errScheduleStop)
	}
}

func (m *Manager) globalLimiter() *rate.Limiter { return m.global }

// ProbeInfo is shown in the add dialog before a download starts.
type ProbeInfo struct {
	URL       string `json:"url"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	Resumable bool   `json:"resumable"`
	Category  string `json:"category"`
	Kind      string `json:"kind,omitempty"` // downloader that will handle it ("" = http)
}

// Probe fetches name, size and resumability of a URL without downloading it.
func (m *Manager) Probe(ctx context.Context, rawURL string, headers map[string]string) (ProbeInfo, error) {
	rawURL = strings.TrimSpace(rawURL)
	m.mu.Lock()
	kind, err := m.kindForLocked(rawURL, "")
	c, ua := m.client, m.settings.UserAgent
	m.mu.Unlock()
	if err != nil {
		return ProbeInfo{}, err
	}
	if kind != KindHTTP {
		// Other protocols learn name and size when they start (e.g. torrent metadata).
		name := SanitizeFilename(filenameFrom(rawURL, "", ""))
		return ProbeInfo{URL: rawURL, Filename: name, Size: -1, Category: Category(name), Kind: kind}, nil
	}
	pr, err := probe(ctx, c, rawURL, headers, ua)
	if err != nil {
		return ProbeInfo{}, err
	}
	return ProbeInfo{URL: rawURL, Filename: pr.Filename, Size: pr.Size, Resumable: pr.Ranges && pr.Size > 0, Category: Category(pr.Filename)}, nil
}

func validateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http and https links are supported (got %q)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid URL: missing host")
	}
	return nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Add queues a new download.
func (m *Manager) Add(r Request) (Download, error) {
	r.URL = strings.TrimSpace(r.URL)
	if !ValidChecksum(r.Checksum) {
		return Download{}, fmt.Errorf("invalid checksum %q (use e.g. sha256:<64 hex chars>)", r.Checksum)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Download{}, errShutdown
	}
	kind, err := m.kindForLocked(r.URL, r.Kind)
	if err != nil {
		return Download{}, err
	}
	name := r.Filename
	if name == "" {
		name = filenameFrom(r.URL, "", "")
	}
	name = SanitizeFilename(name)
	conns := r.Connections
	if conns <= 0 {
		conns = m.settings.Connections
	}
	d := &Download{
		ID:          newID(),
		URL:         r.URL,
		Dir:         r.Dir,
		Filename:    name,
		Size:        -1,
		Status:      StatusQueued,
		Connections: clamp(conns, 1, MaxConnections),
		Headers:     r.Headers,
		Checksum:    r.Checksum,
		SpeedLimit:  max(0, r.SpeedLimit),
		Category:    Category(name),
		CreatedAt:   time.Now().UTC(),
		Order:       m.nextOrder,
		ETA:         -1,
		AutoName:    r.Filename == "",
		AutoDir:     r.Dir == "",
	}
	if kind != KindHTTP {
		d.Kind = kind
	}
	m.nextOrder++
	m.resolveDirLocked(d)
	if r.Paused {
		d.Status = StatusPaused
	}
	if err := m.store.SaveDownload(d); err != nil {
		return Download{}, fmt.Errorf("save: %w", err)
	}
	m.downloads[d.ID] = d
	snap := m.snapshotLocked(d)
	m.emitLocked(Event{Type: EventAdded, Download: &snap})
	m.scheduleLocked()
	return m.snapshotLocked(d), nil
}

func (m *Manager) resolveDirLocked(d *Download) {
	if !d.AutoDir {
		return
	}
	d.Dir = m.settings.DownloadDir
	if m.settings.UseCategories {
		d.Dir = filepath.Join(d.Dir, d.Category)
	}
}

// takenLocked reports whether another unfinished download already targets path.
func (m *Manager) takenLocked(self, path string) bool {
	for id, o := range m.downloads {
		if id != self && o.Status != StatusCompleted && strings.EqualFold(o.Path(), path) {
			return true
		}
	}
	return false
}

// prepareFresh applies probe results before the first byte is written.
func (m *Manager) prepareFresh(id string, pr *probeResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.downloads[id]
	if !ok {
		return ErrNotFound
	}
	if d.AutoName && pr.Filename != "" {
		d.Filename = pr.Filename
		d.Category = Category(d.Filename)
	}
	m.resolveDirLocked(d)
	p := uniquePath(filepath.Join(d.Dir, d.Filename), func(p string) bool { return m.takenLocked(id, p) })
	d.Dir, d.Filename = filepath.Dir(p), filepath.Base(p)
	d.AutoName, d.AutoDir = false, false // name is fixed once the file exists
	d.Size = pr.Size
	d.ETag = pr.ETag
	d.LastMod = pr.LastMod
	d.Resumable = pr.Ranges && pr.Size > 0
	d.Downloaded = 0
	d.Segments = nil
	d.Error = ""
	_ = m.store.SaveDownload(d)
	snap := m.snapshotLocked(d)
	m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	return nil
}

func (m *Manager) setRunning(id string, resumable bool, conns int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.downloads[id]; ok {
		d.Resumable = resumable
		snap := m.snapshotLocked(d)
		m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	}
}

func (m *Manager) persistProgress(t *task) {
	segs := t.segmentsSnapshot()
	m.mu.Lock()
	d, ok := m.downloads[t.id]
	if !ok {
		m.mu.Unlock()
		return
	}
	d.Segments = segs
	d.Downloaded = t.downloaded.Load()
	c := *d
	m.mu.Unlock()
	_ = m.store.SaveDownload(&c)
}

// scheduleLocked starts queued downloads while slots are free.
func (m *Manager) scheduleLocked() {
	if m.closed || !m.schedOpen {
		return
	}
	active := len(m.tasks)
	if active >= m.settings.MaxActive {
		return
	}
	var queued []*Download
	for _, d := range m.downloads {
		if d.Status == StatusQueued {
			if _, running := m.tasks[d.ID]; !running {
				queued = append(queued, d)
			}
		}
	}
	sort.Slice(queued, func(i, j int) bool { return queued[i].Order < queued[j].Order })
	for _, d := range queued {
		if active >= m.settings.MaxActive {
			break
		}
		m.startLocked(d)
		active++
	}
}

func (m *Manager) startLocked(d *Download) {
	ctx, cancel := context.WithCancelCause(context.Background())
	t := &task{m: m, id: d.ID, ctx: ctx, cancel: cancel, done: make(chan struct{}), limiter: newLimiter(d.SpeedLimit)}
	t.downloaded.Store(d.Downloaded)
	m.tasks[d.ID] = t
	m.speeds[d.ID] = &speedMeter{last: d.Downloaded, at: time.Now()}
	d.Status = StatusDownloading
	d.Error = ""
	_ = m.store.SaveDownload(d)
	snap := m.snapshotLocked(d)
	m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	m.wg.Add(1)
	var p Protocol
	if d.Kind != "" && d.Kind != KindHTTP {
		p = m.protocolLocked(d.Kind)
	}
	go func() {
		defer m.wg.Done()
		var err error
		switch {
		case d.Kind != "" && d.Kind != KindHTTP && p == nil:
			err = fmt.Errorf("download type %q is not available in this build", d.Kind)
		case p != nil:
			err = t.runProtocol(p)
		default:
			err = t.run()
		}
		m.finish(t, err)
	}()
}

// finish records the outcome of a task and schedules the next one.
func (m *Manager) finish(t *task, err error) {
	if cause := context.Cause(t.ctx); cause != nil {
		err = cause // a pause/remove/shutdown may surface as a plain network error
	}
	var sum string
	var hashErr error
	var finalPath string
	d := m.snapshot(t.id)
	if err == nil && d != nil {
		// Verify and move into place outside the lock: hashing large files takes time.
		if d.IsDir {
			sum, hashErr = "", nil // a folder has no single checksum
		} else {
			sum, hashErr = hashFile(d.PartPath(), d.Checksum)
		}
		if hashErr == nil {
			finalPath = d.Path()
			if exists(finalPath) { // someone created a file with our name meanwhile
				finalPath = uniquePath(finalPath, func(string) bool { return false })
			}
			if rerr := os.Rename(d.PartPath(), finalPath); rerr != nil {
				hashErr = fmt.Errorf("move into place: %w", rerr)
			} else {
				markTree(finalPath, d.URL, d.Headers["Referer"])
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, t.id)
	delete(m.speeds, t.id)
	defer close(t.done)
	dd, ok := m.downloads[t.id]
	if !ok { // removed while running
		m.scheduleLocked()
		return
	}
	dd.Downloaded = t.downloaded.Load()
	dd.Speed, dd.Active, dd.ETA = 0, 0, -1
	switch {
	case err == nil && hashErr == nil:
		dd.Status = StatusCompleted
		dd.SHA256 = sum
		dd.Dir, dd.Filename = filepath.Dir(finalPath), filepath.Base(finalPath)
		if dd.Size < 0 {
			dd.Size = dd.Downloaded
		}
		now := time.Now().UTC()
		dd.CompletedAt = &now
		dd.Segments = nil
		dd.Error = ""
	case err == nil:
		dd.Status = StatusError
		dd.Error = hashErr.Error()
	case errors.Is(err, errPaused):
		dd.Status = StatusPaused
	case errors.Is(err, errShutdown), errors.Is(err, errScheduleStop):
		dd.Status = StatusQueued // resume on next start / next schedule window
	case errors.Is(err, errRemoved):
		// handled by Remove
	default:
		dd.Status = StatusError
		dd.Error = redact(err.Error())
	}
	_ = m.store.SaveDownload(dd)
	snap := m.snapshotLocked(dd)
	m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	if dd.Status == StatusCompleted {
		m.completed = true
	}
	m.scheduleLocked()
	m.checkQueueDoneLocked()
}

// checkQueueDoneLocked emits EventQueueDone once the queue is empty after a completion.
func (m *Manager) checkQueueDoneLocked() {
	if !m.completed || len(m.tasks) > 0 {
		return
	}
	for _, d := range m.downloads {
		if d.Status == StatusQueued {
			return
		}
	}
	m.completed = false
	m.emitLocked(Event{Type: EventQueueDone})
}

var errRemoved = errors.New("removed")

// Pause stops a download, keeping its progress.
func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	d, ok := m.downloads[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	t := m.tasks[id]
	if t == nil {
		if d.Status == StatusQueued || d.Status == StatusError {
			d.Status = StatusPaused
			_ = m.store.SaveDownload(d)
			snap := m.snapshotLocked(d)
			m.emitLocked(Event{Type: EventUpdated, Download: &snap})
		}
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	t.cancel(errPaused)
	<-t.done
	return nil
}

// Resume queues a paused or failed download; it starts when a slot is free.
func (m *Manager) Resume(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.downloads[id]
	if !ok {
		return ErrNotFound
	}
	if d.Status == StatusPaused || d.Status == StatusError {
		d.Status = StatusQueued
		d.Error = ""
		_ = m.store.SaveDownload(d)
		snap := m.snapshotLocked(d)
		m.emitLocked(Event{Type: EventUpdated, Download: &snap})
		m.scheduleLocked()
	}
	return nil
}

// Restart discards progress and downloads again from zero.
func (m *Manager) Restart(id string) error {
	if err := m.Pause(id); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.downloads[id]
	if !ok {
		return ErrNotFound
	}
	wasCompleted := d.Status == StatusCompleted
	if !wasCompleted {
		_ = removePart(d)
	}
	d.Segments, d.Downloaded, d.ETag, d.LastMod, d.Size = nil, 0, "", "", -1
	d.ProtoState = nil
	d.CompletedAt, d.SHA256, d.Error = nil, "", ""
	d.Status = StatusQueued
	if wasCompleted {
		d.Filename = filepath.Base(uniquePath(d.Path(), func(string) bool { return false }))
	}
	_ = m.store.SaveDownload(d)
	snap := m.snapshotLocked(d)
	m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	m.scheduleLocked()
	return nil
}

// Remove deletes a download from the list. If deleteFile is true the file (or part
// file) is deleted from disk too.
func (m *Manager) Remove(id string, deleteFile bool) error {
	m.mu.Lock()
	d, ok := m.downloads[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	t := m.tasks[id]
	m.mu.Unlock()
	if t != nil {
		t.cancel(errRemoved)
		<-t.done
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.downloads, id)
	_ = m.store.DeleteDownload(id)
	if d.Status != StatusCompleted {
		_ = removePart(d) // an unfinished part file is useless without its state
	} else if deleteFile {
		if d.IsDir {
			_ = os.RemoveAll(d.Path())
		} else {
			_ = os.Remove(d.Path())
		}
	}
	m.emitLocked(Event{Type: EventRemoved, ID: id})
	m.scheduleLocked()
	return nil
}

// UpdateURL replaces an expired link, keeping progress (the server must send the same file).
func (m *Manager) UpdateURL(id, newURL string, headers map[string]string) error {
	if err := validateURL(newURL); err != nil {
		return err
	}
	m.mu.Lock()
	d, ok := m.downloads[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	running := m.tasks[id] != nil
	m.mu.Unlock()
	if running {
		_ = m.Pause(id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d.URL = strings.TrimSpace(newURL)
	if headers != nil {
		d.Headers = headers
	}
	_ = m.store.SaveDownload(d)
	snap := m.snapshotLocked(d)
	m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	return nil
}

// SetSpeedLimit changes a download's own limit (bytes/s, 0 = unlimited).
func (m *Manager) SetSpeedLimit(id string, bps int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.downloads[id]
	if !ok {
		return ErrNotFound
	}
	d.SpeedLimit = max(0, bps)
	if t := m.tasks[id]; t != nil {
		setLimiter(t.limiter, d.SpeedLimit)
	}
	_ = m.store.SaveDownload(d)
	snap := m.snapshotLocked(d)
	m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	return nil
}

// Move changes queue order: delta -1 moves up, +1 down.
func (m *Manager) Move(id string, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.downloads[id]; !ok {
		return ErrNotFound
	}
	list := make([]*Download, 0, len(m.downloads))
	for _, d := range m.downloads {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Order < list[j].Order })
	idx := -1
	for i, d := range list {
		if d.ID == id {
			idx = i
		}
	}
	j := clamp(idx+delta, 0, len(list)-1)
	if j == idx {
		return nil
	}
	list[idx], list[j] = list[j], list[idx]
	for i, d := range list {
		if d.Order != int64(i) {
			d.Order = int64(i)
			_ = m.store.SaveDownload(d)
		}
	}
	m.nextOrder = int64(len(list))
	for _, d := range list {
		snap := m.snapshotLocked(d)
		m.emitLocked(Event{Type: EventUpdated, Download: &snap})
	}
	return nil
}

// PauseAll pauses every active or queued download.
func (m *Manager) PauseAll() {
	for _, d := range m.List() {
		if d.Status == StatusDownloading || d.Status == StatusQueued {
			_ = m.Pause(d.ID)
		}
	}
}

// ResumeAll resumes every paused or failed download.
func (m *Manager) ResumeAll() {
	for _, d := range m.List() {
		if d.Status == StatusPaused || d.Status == StatusError {
			_ = m.Resume(d.ID)
		}
	}
}

// ClearCompleted removes finished downloads from the list (files stay on disk).
func (m *Manager) ClearCompleted() {
	for _, d := range m.List() {
		if d.Status == StatusCompleted {
			_ = m.Remove(d.ID, false)
		}
	}
}

// ticker updates speeds and emits progress batches twice a second.
func (m *Manager) ticker() {
	defer m.wg.Done()
	tk := time.NewTicker(500 * time.Millisecond)
	defer tk.Stop()
	for {
		select {
		case <-m.stopTick:
			return
		case now := <-tk.C:
			m.mu.Lock()
			stop := m.checkScheduleLocked(false)
			var batch []Download
			for id, t := range m.tasks {
				sm := m.speeds[id]
				cur := t.downloaded.Load()
				dt := now.Sub(sm.at).Seconds()
				if dt > 0 {
					inst := float64(cur-sm.last) / dt
					if inst < 0 {
						inst = 0
					}
					if sm.speed == 0 {
						sm.speed = inst
					} else {
						sm.speed = 0.7*sm.speed + 0.3*inst
					}
				}
				sm.last, sm.at = cur, now
				if d, ok := m.downloads[id]; ok {
					batch = append(batch, m.snapshotLocked(d))
				}
			}
			if len(batch) > 0 {
				m.emitLocked(Event{Type: EventProgress, Downloads: batch})
			}
			m.mu.Unlock()
			stopTasks(stop)
		}
	}
}

// redact removes query strings and user:password from URLs inside error messages (they
// often hold tokens; ftp:// URLs carry passwords).
func redact(s string) string {
	fields := strings.Fields(s)
	for i, f := range fields {
		trimmed := strings.Trim(f, `"':,`)
		if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") ||
			strings.HasPrefix(trimmed, "ftp://") || strings.HasPrefix(trimmed, "ftps://") {
			if u, err := url.Parse(trimmed); err == nil {
				u.RawQuery, u.User, u.Fragment = "", nil, ""
				fields[i] = strings.Replace(f, trimmed, u.String(), 1)
			}
		}
	}
	return strings.Join(fields, " ")
}

// HidePassword removes the password from a URL's userinfo, keeping the user name.
func HidePassword(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, has := u.User.Password(); !has {
		return raw
	}
	u.User = url.User(u.User.Username())
	return u.String()
}
