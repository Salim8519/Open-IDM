package api

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

// ExtensionID is the fixed ID of the Peregrine Chromium extension (from the "key" in
// extension/manifest.json). The native messaging host accepts only this ID.
const ExtensionID = "bcnjpgddmjlffmcfgnfjaadkdjphdhfd"

// Browsers whose extensions page the app can open. The value is the page URL; the hook
// maps the key to a program, so no caller-supplied string ever reaches a command line.
var extensionPages = map[string]string{
	"chrome": "chrome://extensions",
	"edge":   "edge://extensions",
	"brave":  "brave://extensions",
}

// BrowserStatus tells the UI whether the browser extension has ever reached the app.
type BrowserStatus struct {
	Connected    bool   `json:"connected"`              // the extension has checked in at least once
	LastSeen     int64  `json:"lastSeen,omitempty"`     // unix milliseconds
	ExtensionDir string `json:"extensionDir,omitempty"` // bundled unpacked extension, if shipped
	ExtensionID  string `json:"extensionId"`
}

// browserState keeps the last check-in time, persisted to a small JSON file so the
// first-run prompt does not come back after the extension is installed.
type browserState struct {
	mu       sync.Mutex
	path     string // "" = memory only
	lastSeen int64
	loaded   bool
}

func (b *browserState) load() {
	if b.loaded {
		return
	}
	b.loaded = true
	if b.path == "" {
		return
	}
	raw, err := os.ReadFile(b.path)
	if err != nil {
		return
	}
	var v struct {
		LastSeen int64 `json:"lastSeen"`
	}
	if json.Unmarshal(raw, &v) == nil && v.LastSeen > 0 {
		b.lastSeen = v.LastSeen
	}
}

func (b *browserState) seen(now time.Time) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.load()
	b.lastSeen = now.UnixMilli()
	if b.path != "" {
		raw, _ := json.Marshal(map[string]int64{"lastSeen": b.lastSeen})
		tmp := b.path + ".tmp"
		if os.WriteFile(tmp, raw, 0o600) == nil {
			_ = os.Rename(tmp, b.path)
		}
	}
	return b.lastSeen
}

func (b *browserState) get() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.load()
	return b.lastSeen
}

func (s *Service) browserStatus() BrowserStatus {
	last := s.browser.get()
	st := BrowserStatus{Connected: last > 0, LastSeen: last, ExtensionID: ExtensionID}
	if s.Hooks.ExtensionDir != nil {
		st.ExtensionDir = s.Hooks.ExtensionDir()
	}
	return st
}

// extensionHello is called by the native messaging host whenever the extension talks to
// it while the app is running. It only records a timestamp; it carries no data.
func (s *Service) extensionHello() BrowserStatus {
	s.browser.seen(time.Now())
	st := s.browserStatus()
	s.emit(Event{Event: engine.Event{Type: "browser"}, Browser: &st})
	return st
}

func (s *Service) openExtensionsPage(browser string) (any, error) {
	page, ok := extensionPages[browser]
	if !ok {
		return nil, fmt.Errorf("%w: unknown browser %q", errBadParams, browser)
	}
	if s.Hooks.OpenBrowserPage == nil {
		return nil, errUnsupported
	}
	if err := s.Hooks.OpenBrowserPage(browser, page); err != nil {
		return nil, err
	}
	return true, nil
}

func (s *Service) showExtensionFolder() (any, error) {
	if s.Hooks.ExtensionDir == nil || s.Hooks.OpenPath == nil {
		return nil, errUnsupported
	}
	dir := s.Hooks.ExtensionDir()
	if dir == "" {
		return nil, fmt.Errorf("the extension folder is not bundled with this build")
	}
	return true, s.Hooks.OpenPath(dir)
}
