// Package torrent downloads BitTorrent magnet links and .torrent files for the engine
// (kind "torrent"). It wraps github.com/anacrolix/torrent; see ADR 0014 for the defaults
// (no seeding after completion, capped upload, no UPnP, no uTP, quiet logs).
package torrent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	g "github.com/anacrolix/generics"
	at "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"

	"github.com/Salim8519/Peregrine/engine"
)

// Kind is the engine download kind handled by this package.
const Kind = "torrent"

// DefaultPort is the TCP/UDP port the client tries first (a random free port is used if it
// is taken).
const DefaultPort = 46881

// MaxTorrentFileSize caps a .torrent file fetched over HTTP.
const MaxTorrentFileSize = 20 << 20

// DefaultUploadLimit applies while downloading when there is no download speed limit.
const DefaultUploadLimit = 1 << 20 // 1 MiB/s

// progressInterval is how often a Run reports progress to the engine.
const progressInterval = time.Second

// Config configures the shared BitTorrent client.
type Config struct {
	// ListenPort for incoming peer connections (TCP) and the DHT (UDP). 0 = DefaultPort,
	// -1 = any free port. If the port is busy a random one is used.
	ListenPort int
	// DisableDHT turns off the distributed hash table. Magnet links then only work with
	// trackers or peers named in the link. Tests use it to stay off the internet.
	DisableDHT bool

	// Test hooks (unexported): listen only on this host, skip IPv6, and add these peers
	// ("host:port") to every torrent.
	listenHost  string
	disableIPv6 bool
	peers       []string
}

// Protocol is the engine.Protocol for BitTorrent. It owns one client shared by all
// torrent downloads; the host app must call Close on shutdown (after the Manager).
type Protocol struct {
	cfg Config

	mu     sync.Mutex
	client *at.Client
	closed bool
	down   *rate.Limiter // shared by all torrents (the library limits per client)
	up     *rate.Limiter
	limits map[*engine.Job]int64 // speed limit of each running job (0 = unlimited)
}

// New returns the protocol. The client starts on the first download.
func New(cfg Config) *Protocol {
	return &Protocol{
		cfg:    cfg,
		down:   rate.NewLimiter(rate.Inf, 1<<20),
		up:     rate.NewLimiter(rate.Limit(DefaultUploadLimit), 1<<20),
		limits: map[*engine.Job]int64{},
	}
}

// Kind implements engine.Protocol.
func (*Protocol) Kind() string { return Kind }

// Match implements engine.Protocol: magnet links with a BitTorrent v1 info hash,
// http(s) URLs whose path ends in .torrent, and local .torrent files (file:// URLs, see
// FileURL).
func (*Protocol) Match(rawURL string) bool {
	s := strings.TrimSpace(rawURL)
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "magnet:?") {
		return strings.Contains(low, "xt=urn:btih:")
	}
	if strings.HasPrefix(low, "file:") {
		_, ok := localPath(s)
		return ok
	}
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.HasSuffix(strings.ToLower(u.Path), ".torrent")
}

// Close stops the shared client: open connections, listeners and the DHT. Running
// downloads fail, so close the engine.Manager first (it pauses them).
func (p *Protocol) Close() error {
	p.mu.Lock()
	cl := p.client
	p.client, p.closed = nil, true
	p.mu.Unlock()
	if cl == nil {
		return nil
	}
	if errs := cl.Close(); len(errs) > 0 {
		return fmt.Errorf("close torrent client: %w", errors.Join(errs...))
	}
	return nil
}

// ListenPort returns the port the client listens on (0 if it is not running).
func (p *Protocol) ListenPort() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil {
		return 0
	}
	return p.client.LocalPort()
}

// getClient starts the shared client on first use.
func (p *Protocol) getClient() (*at.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, errors.New("torrent client is shut down")
	}
	if p.client != nil {
		return p.client, nil
	}
	port := p.cfg.ListenPort
	switch {
	case port == 0:
		port = DefaultPort
	case port < 0:
		port = 0 // the OS picks
	}
	cl, err := at.NewClient(p.clientConfig(port))
	if err != nil {
		// Most likely the port is taken (another client, a second instance): use any port.
		var err2 error
		cl, err2 = at.NewClient(p.clientConfig(0))
		if err2 != nil {
			return nil, fmt.Errorf("start torrent client: %w", errors.Join(err, err2))
		}
	}
	p.client = cl
	return cl, nil
}

func (p *Protocol) clientConfig(port int) *at.ClientConfig {
	c := at.NewDefaultClientConfig()
	c.ListenPort = port
	if h := p.cfg.listenHost; h != "" {
		c.ListenHost = func(string) string { return h }
	}
	c.DisableIPv6 = p.cfg.disableIPv6
	c.NoDHT = p.cfg.DisableDHT
	c.DisablePEX = false
	c.DisableUTP = true              // keep one transport; behaves the same with and without cgo
	c.NoDefaultPortForwarding = true // no UPnP/NAT-PMP: never open ports on the user's router
	c.DisableWebtorrent = true       // no WebRTC or websocket trackers
	c.Seed = false                   // and Run drops the torrent once it is complete
	c.HeaderObfuscationPolicy = at.HeaderObfuscationPolicy{Preferred: true, RequirePreferred: false}
	c.DownloadRateLimiter = p.down
	c.UploadRateLimiter = p.up
	c.DefaultStorage = storage.NewFileOpts(storage.NewFileClientOpts{
		// Never used (every Run opens its own storage); set so the library doesn't create
		// its default piece database in the working directory.
		ClientBaseDir:   os.TempDir(),
		PieceCompletion: storage.NewMapPieceCompletion(),
		UsePartFiles:    g.Some(false),
		DisableMmap:     true,
	})
	// The library logs tracker URLs and peer details; keep all of it out of our output.
	c.Slogger = slog.New(slog.DiscardHandler)
	return c
}

// state is saved with the download so a resume needs neither the .torrent URL nor the DHT.
type state struct {
	InfoHash  string     `json:"infoHash"`
	InfoBytes []byte     `json:"info"`
	Trackers  [][]string `json:"trackers,omitempty"`
	Webseeds  []string   `json:"webseeds,omitempty"`
}

// Run implements engine.Protocol.
func (p *Protocol) Run(ctx context.Context, j *engine.Job) error {
	spec, err := p.spec(ctx, j)
	if err != nil {
		return err
	}
	cl, err := p.getClient()
	if err != nil {
		return err
	}
	p.setJobLimit(j, j.SpeedLimit())
	defer p.removeJob(j)

	opts := spec.AddTorrentOpts
	opts.DeferStorageOpen = true // open storage once we know where each file must go
	t, isNew := cl.AddTorrentOpt(opts)
	if !isNew {
		return errors.New("this torrent is already being downloaded")
	}
	var st storage.ClientImplCloser
	defer func() {
		t.Drop() // closes the files; data stays on disk
		if st != nil {
			_ = st.Close()
		}
	}()
	merge := *spec
	merge.AddTorrentOpts = at.AddTorrentOpts{}
	merge.PeerAddrs = append(merge.PeerAddrs, p.cfg.peers...)
	if err := t.MergeSpec(&merge); err != nil {
		return fmt.Errorf("invalid torrent: %w", err)
	}

	// Magnet links: wait for the metadata from peers.
	if err := waitInfo(ctx, j, t); err != nil {
		return err
	}
	info := t.Info()
	if info == nil {
		return errors.New("torrent has no metadata")
	}
	if j.Fresh {
		if err := j.SetName(info.BestName()); err != nil {
			return err
		}
	}
	multi := info.IsDir()
	if multi {
		if err := j.SetDirectory(); err != nil {
			return fmt.Errorf("create folder: %w", err)
		}
	}
	st = newStorage(j.PartPath, multi)
	if err := t.OpenStorage(ctx, st); err != nil {
		return fmt.Errorf("open torrent files: %w", err)
	}
	total := info.TotalLength()
	j.SetSize(total)
	j.SetResumable(true)
	if err := j.SaveState(state{
		InfoHash:  t.InfoHash().HexString(),
		InfoBytes: t.Metainfo().InfoBytes,
		Trackers:  spec.Trackers,
		Webseeds:  spec.Webseeds,
	}); err != nil {
		return err
	}
	t.DownloadAll()

	tick := time.NewTicker(progressInterval)
	defer tick.Stop()
	report := func() {
		j.SetDownloaded(t.BytesCompleted())
		j.SetActive(t.Stats().ActivePeers)
		p.setJobLimit(j, j.SpeedLimit())
	}
	report()
	for {
		select {
		case <-t.Complete().On():
			// Done: stop here, so nothing is seeded after completion (ADR 0014).
			j.SetDownloaded(total)
			j.SetActive(0)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-t.Closed():
			return errors.New("torrent client stopped")
		case <-tick.C:
			report()
		}
	}
}

func waitInfo(ctx context.Context, j *engine.Job, t *at.Torrent) error {
	tick := time.NewTicker(progressInterval)
	defer tick.Stop()
	for {
		select {
		case <-t.GotInfo():
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-t.Closed():
			return errors.New("torrent client stopped")
		case <-tick.C:
			j.SetActive(t.Stats().ActivePeers)
		}
	}
}

// spec builds the torrent spec from saved state, a magnet link or a .torrent URL.
func (p *Protocol) spec(ctx context.Context, j *engine.Job) (*at.TorrentSpec, error) {
	if len(j.State) > 0 {
		var s state
		if err := json.Unmarshal(j.State, &s); err == nil && len(s.InfoBytes) > 0 {
			return specFromMetaInfo(&metainfo.MetaInfo{InfoBytes: s.InfoBytes, AnnounceList: s.Trackers, UrlList: s.Webseeds})
		}
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(j.URL)), "magnet:") {
		spec, err := at.TorrentSpecFromMagnetUri(strings.TrimSpace(j.URL))
		if err != nil {
			// The parser's message may quote the link (trackers, passkeys): don't wrap it.
			return nil, errors.New("invalid magnet link")
		}
		if spec.InfoHash.IsZero() {
			return nil, errors.New("magnet link has no BitTorrent info hash")
		}
		return spec, nil
	}
	if path, ok := localPath(j.URL); ok {
		return readTorrentFile(path)
	}
	mi, err := p.fetchTorrentFile(ctx, j)
	if err != nil {
		return nil, err
	}
	return specFromMetaInfo(mi)
}

func specFromMetaInfo(mi *metainfo.MetaInfo) (*at.TorrentSpec, error) {
	spec, err := at.TorrentSpecFromMetaInfoErr(mi)
	if err != nil {
		return nil, fmt.Errorf("invalid torrent file: %w", err)
	}
	if spec.InfoHash.IsZero() && !spec.InfoHashV2.Ok {
		return nil, errors.New("invalid torrent file: no info hash")
	}
	return spec, nil
}

// FileURL turns a local .torrent path into the file:// URL the protocol accepts.
func FileURL(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows: C:/x -> /C:/x
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// localPath returns the local path of a file:// URL naming a .torrent file. Remote
// (host) file URLs are refused so a link can never make us open a network share.
func localPath(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "file") || (u.Host != "" && !strings.EqualFold(u.Host, "localhost")) {
		return "", false
	}
	p := u.Path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' { // /C:/x -> C:/x
		p = p[1:]
	}
	if p == "" || !strings.HasSuffix(strings.ToLower(p), ".torrent") || strings.HasPrefix(p, "//") {
		return "", false
	}
	return filepath.FromSlash(p), true
}

// readTorrentFile loads a .torrent file from disk, with the same size cap as over HTTP.
func readTorrentFile(path string) (*at.TorrentSpec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open .torrent file: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxTorrentFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read .torrent file: %w", err)
	}
	if len(b) > MaxTorrentFileSize {
		return nil, errors.New("torrent file too large")
	}
	mi, err := metainfo.Load(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("invalid torrent file: %w", err)
	}
	return specFromMetaInfo(mi)
}

// fetchTorrentFile downloads and parses a .torrent file with the engine's HTTP client.
func (p *Protocol) fetchTorrentFile(ctx context.Context, j *engine.Job) (*metainfo.MetaInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.URL, nil)
	if err != nil {
		return nil, errors.New("invalid .torrent URL")
	}
	for k, v := range j.Headers {
		req.Header.Set(k, v)
	}
	if j.UserAgent != "" {
		req.Header.Set("User-Agent", j.UserAgent)
	}
	resp, err := j.HTTPClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("fetch .torrent file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch .torrent file: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxTorrentFileSize {
		return nil, fmt.Errorf("torrent file too large (%d bytes)", resp.ContentLength)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxTorrentFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("fetch .torrent file: %w", err)
	}
	if len(b) > MaxTorrentFileSize {
		return nil, errors.New("torrent file too large")
	}
	mi, err := metainfo.Load(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("invalid torrent file: %w", err)
	}
	return mi, nil
}

// newStorage maps the torrent's files onto the job's part path: a single file is the part
// path itself; a multi-file torrent goes inside the part folder, each path element
// sanitized so nothing can land outside it.
func newStorage(partPath string, multi bool) storage.ClientImplCloser {
	opts := storage.NewFileClientOpts{
		PieceCompletion: storage.NewMapPieceCompletion(), // existing data is re-verified on start
		UsePartFiles:    g.Some(false),                   // the engine already uses .peregrine.part
		DisableMmap:     true,                            // plain I/O: files close cleanly before the rename
		Logger:          slog.New(slog.DiscardHandler),
	}
	if multi {
		opts.ClientBaseDir = partPath
		opts.FilePathMaker = func(o storage.FilePathMakerOpts) string { return SafeRelPath(o.File.BestPath()) }
	} else {
		opts.ClientBaseDir = filepath.Dir(partPath)
		base := filepath.Base(partPath)
		opts.FilePathMaker = func(storage.FilePathMakerOpts) string { return base }
	}
	return storage.NewFileOpts(opts)
}

// SafeRelPath turns a torrent file path (a list of untrusted elements) into a relative path
// that stays inside the download folder: empty, "." and ".." elements are dropped, and each
// element goes through engine.SanitizeFilename (no separators, drive letters, control or
// bidi characters, reserved Windows names).
func SafeRelPath(elems []string) string {
	out := make([]string, 0, len(elems))
	for _, e := range elems {
		e = strings.TrimSpace(e)
		if e == "" || e == "." || e == ".." {
			continue
		}
		out = append(out, engine.SanitizeFilename(e))
	}
	if len(out) == 0 {
		return "download"
	}
	return filepath.Join(out...)
}

// setJobLimit records a job's download limit and updates the shared limiters: download is
// the sum of the jobs' limits (unlimited if any job is), upload is twice that, or
// DefaultUploadLimit when downloads are unlimited.
func (p *Protocol) setJobLimit(j *engine.Job, bps int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limits[j] = bps
	p.applyLimitsLocked()
}

func (p *Protocol) removeJob(j *engine.Job) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.limits, j)
	p.applyLimitsLocked()
}

func (p *Protocol) applyLimitsLocked() {
	down, up := limitsFor(p.limits)
	if down == 0 {
		p.down.SetLimit(rate.Inf)
	} else {
		p.down.SetLimit(rate.Limit(down))
	}
	p.up.SetLimit(rate.Limit(up))
}

func limitsFor(jobs map[*engine.Job]int64) (down, up int64) {
	for _, l := range jobs {
		if l <= 0 {
			down = 0
			break
		}
		down += l
	}
	if len(jobs) == 0 {
		down = 0
	}
	if down == 0 {
		return 0, DefaultUploadLimit
	}
	return down, 2 * down
}
