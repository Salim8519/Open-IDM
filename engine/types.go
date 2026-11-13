// Package engine is Peregrine's download engine: segmented, resumable, crash-safe HTTP
// downloads with a queue, speed limits and persistent state. It has no UI dependencies.
package engine

import (
	"encoding/json"
	"errors"
	"time"
)

// Status is the lifecycle state of a download.
type Status string

const (
	StatusQueued      Status = "queued"      // waiting for a free slot
	StatusDownloading Status = "downloading" // actively transferring
	StatusPaused      Status = "paused"      // stopped by the user, resumable
	StatusError       Status = "error"       // stopped by a failure; see Download.Error
	StatusCompleted   Status = "completed"   // finished and verified
)

// Segment is a byte range [Start, End) of the target file. Done bytes from Start are on disk.
// End == -1 means "until EOF" (unknown size, single connection).
type Segment struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`
}

// Remaining returns the bytes still to fetch, or -1 if unknown.
func (s Segment) Remaining() int64 {
	if s.End < 0 {
		return -1
	}
	return s.End - s.Start - s.Done
}

// Request describes a download to add.
type Request struct {
	URL         string            `json:"url"`
	Dir         string            `json:"dir,omitempty"`      // target folder; default from settings
	Filename    string            `json:"filename,omitempty"` // override; sanitized
	Connections int               `json:"connections,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`  // e.g. Cookie, Referer, User-Agent
	Checksum    string            `json:"checksum,omitempty"` // "sha256:<hex>", "md5:<hex>", ...
	SpeedLimit  int64             `json:"speedLimit,omitempty"`
	Paused      bool              `json:"paused,omitempty"` // add without starting
	// Kind forces a downloader ("http", "ftp", "hls", "dash", "video", "torrent"); empty =
	// detect from the URL (see Protocol.Match).
	Kind string `json:"kind,omitempty"`
}

// Download is a snapshot of one download's state, safe to serialize to the UI.
type Download struct {
	ID          string            `json:"id"`
	URL         string            `json:"url"`
	Dir         string            `json:"dir"`
	Filename    string            `json:"filename"`
	Size        int64             `json:"size"` // -1 if unknown
	Downloaded  int64             `json:"downloaded"`
	Status      Status            `json:"status"`
	Error       string            `json:"error,omitempty"`
	Resumable   bool              `json:"resumable"`
	Connections int               `json:"connections"`
	Active      int               `json:"active"` // live connections right now
	Speed       int64             `json:"speed"`  // bytes/s (smoothed)
	ETA         int64             `json:"eta"`    // seconds, -1 if unknown
	Headers     map[string]string `json:"headers,omitempty"`
	ETag        string            `json:"etag,omitempty"`
	LastMod     string            `json:"lastModified,omitempty"`
	Checksum    string            `json:"checksum,omitempty"`
	SHA256      string            `json:"sha256,omitempty"`
	SpeedLimit  int64             `json:"speedLimit,omitempty"`
	Category    string            `json:"category"`
	Segments    []Segment         `json:"segments,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	CompletedAt *time.Time        `json:"completedAt,omitempty"`
	Order       int64             `json:"order"`
	AutoName    bool              `json:"autoName,omitempty"` // name may still change after probing
	AutoDir     bool              `json:"autoDir,omitempty"`  // folder follows settings/category
	Kind        string            `json:"kind,omitempty"`     // downloader; "" = http
	ProtoState  json.RawMessage   `json:"protoState,omitempty"`
	IsDir       bool              `json:"isDir,omitempty"` // output is a folder (multi-file torrent)
}

// Path returns the final path of the file.
func (d *Download) Path() string { return joinPath(d.Dir, d.Filename) }

// PartPath returns the in-progress path of the file.
func (d *Download) PartPath() string { return d.Path() + PartSuffix }

// PartSuffix is appended to files while they download.
const PartSuffix = ".peregrine.part"

// Settings are user-adjustable engine options.
type Settings struct {
	DownloadDir    string `json:"downloadDir"`
	UseCategories  bool   `json:"useCategories"` // put files into <dir>/<Category>/
	MaxActive      int    `json:"maxActive"`     // concurrent downloads
	Connections    int    `json:"connections"`   // default connections per download
	GlobalLimit    int64  `json:"globalLimit"`   // bytes/s, 0 = unlimited
	Proxy          string `json:"proxy"`         // "", "system", or URL (http://, socks5://)
	UserAgent      string `json:"userAgent"`
	MaxRetries     int    `json:"maxRetries"`
	ClipboardWatch bool   `json:"clipboardWatch"`
	AutoStartExt   bool   `json:"autoStartExt"` // start extension downloads without asking
	Theme          string `json:"theme"`        // "system" | "light" | "dark"
	Language       string `json:"language"`     // UI language code, e.g. "en", "ar", "zh"
	MinExtSize     int64  `json:"minExtSize"`   // extension: only capture files >= this size

	Schedule Schedule `json:"schedule"` // when the queue may run (Q3)

	// Desktop-app options; the engine only stores them.
	CloseToTray   bool `json:"closeToTray"`   // closing the window keeps the app in the tray
	LaunchAtLogin bool `json:"launchAtLogin"` // start (hidden) when the user signs in
	Notify        bool `json:"notify"`        // system notifications for finished/failed downloads
}

// DefaultSettings returns sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		DownloadDir: defaultDownloadDir(),
		MaxActive:   3,
		Connections: 8,
		MaxRetries:  10,
		Proxy:       "system",
		Theme:       "system",
		Language:    "en",
		MinExtSize:  1 << 20,
		Schedule:    defaultSchedule(),
		CloseToTray: true,
		Notify:      true,
	}
}

// Limits.
const (
	MaxConnections = 32
	minSplitSize   = 256 << 10 // never create segments smaller than this
)

// Errors.
var (
	ErrNotFound      = errors.New("download not found")
	ErrRemoteChanged = errors.New("the file changed on the server since the download started")
	ErrChecksum      = errors.New("checksum mismatch")
	ErrLinkExpired   = errors.New("the link expired or access was denied; refresh the address")
)
