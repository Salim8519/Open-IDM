package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/internal/update"
)

// updateState remembers the last update check (ADR 0020).
type updateState struct {
	mu   sync.Mutex
	url  string // release API URL; "" = GitHub (tests point it at a local server)
	last *update.Result
}

// SetUpdateURL points the update check at another release API (tests only). Call before use.
func (s *Service) SetUpdateURL(u string) { s.upd.url = u }

// CheckUpdate asks the release server for the latest version, using the user's proxy
// setting, remembers the result and sends it to UI clients as an "update" event.
func (s *Service) CheckUpdate(ctx context.Context) (*update.Result, error) {
	client, err := engine.NewHTTPClient(s.M.Settings().Proxy)
	if err != nil {
		return nil, fmt.Errorf("update check: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	s.upd.mu.Lock()
	u := s.upd.url
	s.upd.mu.Unlock()
	r, err := update.Checker{Client: client, URL: u, Current: engine.Version}.Check(ctx)
	if err != nil {
		return nil, err
	}
	s.upd.mu.Lock()
	s.upd.last = &r
	s.upd.mu.Unlock()
	s.emit(Event{Event: engine.Event{Type: "update"}, Update: &r})
	return &r, nil
}

// updateStatus returns the last check's result, or nil if there was none yet.
func (s *Service) updateStatus() *update.Result {
	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	return s.upd.last
}

var errNoUpdate = errors.New("no newer version is known")

// openUpdatePage opens the release page of the newer version found by the last check.
// The URL is the one the checker built from the version; callers cannot pass their own.
func (s *Service) openUpdatePage() (bool, error) {
	if s.Hooks.OpenURL == nil {
		return false, errUnsupported
	}
	r := s.updateStatus()
	if r == nil || !r.Newer {
		return false, errNoUpdate
	}
	return true, s.Hooks.OpenURL(r.URL)
}
