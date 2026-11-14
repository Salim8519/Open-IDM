// Package api exposes the engine through one method-based RPC surface, used by:
//   - the desktop app (Wails binding calls Service.Call directly),
//   - the local HTTP server (browser-mode UI, e2e tests, the native messaging host),
//   - the CLI's remote commands.
//
// See docs/API.md for the method list.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/internal/ipc"
)

// Hooks lets the host app provide platform actions. Any field may be nil.
type Hooks struct {
	OpenPath   func(path string) error // open a file with its default app
	RevealPath func(path string) error // show a file in Explorer/Finder
	ShowWindow func()                  // bring the app window to front
	PickFolder func(current string) (string, error)
	Clipboard  func() (string, error)
	// ExtensionDir returns the folder of the bundled unpacked extension ("" if not shipped).
	ExtensionDir func() string
	// OpenBrowserPage opens page (an extensions page) in the given browser key.
	OpenBrowserPage func(browser, page string) error
}

// Capture is a download offered by the browser extension (or clipboard).
type Capture struct {
	engine.Request
	Source   string `json:"source"` // "extension" | "clipboard" | "cli"
	PageURL  string `json:"pageUrl,omitempty"`
	Size     int64  `json:"size,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// Event is what UI clients receive: engine events plus "capture".
type Event struct {
	engine.Event
	Capture *Capture       `json:"capture,omitempty"`
	Browser *BrowserStatus `json:"browser,omitempty"`
}

// Service dispatches RPC calls to the manager.
type Service struct {
	M     *engine.Manager
	Hooks Hooks

	mu      sync.Mutex
	subs    map[int]func(Event)
	nextSub int
	unsub   func()
	browser browserState
}

// SetBrowserStateFile makes the extension check-in time survive restarts. Call before use.
func (s *Service) SetBrowserStateFile(path string) { s.browser.path = path }

// NewService wires a service to a manager.
func NewService(m *engine.Manager, hooks Hooks) *Service {
	s := &Service{M: m, Hooks: hooks, subs: map[int]func(Event){}}
	s.unsub = m.Subscribe(func(e engine.Event) { s.emit(Event{Event: e}) })
	return s
}

// Close detaches from the manager.
func (s *Service) Close() { s.unsub() }

// Subscribe receives all UI events. fn must not block.
func (s *Service) Subscribe(fn func(Event)) func() {
	s.mu.Lock()
	id := s.nextSub
	s.nextSub++
	s.subs[id] = fn
	s.mu.Unlock()
	return func() { s.mu.Lock(); delete(s.subs, id); s.mu.Unlock() }
}

func (s *Service) emit(e Event) {
	s.mu.Lock()
	fns := make([]func(Event), 0, len(s.subs))
	for _, fn := range s.subs {
		fns = append(fns, fn)
	}
	s.mu.Unlock()
	for _, fn := range fns {
		fn(e)
	}
}

// RPCError is returned to clients as {"error": {"code","message"}} (defined in internal/ipc).
type RPCError = ipc.RPCError

func toRPCError(err error) *RPCError {
	code := "error"
	switch {
	case errors.Is(err, engine.ErrNotFound):
		code = "not_found"
	case errors.Is(err, errBadParams):
		code = "bad_params"
	case errors.Is(err, errUnknownMethod):
		code = "unknown_method"
	case errors.Is(err, errUnsupported):
		code = "unsupported"
	}
	return &RPCError{Code: code, Message: err.Error()}
}

var (
	errBadParams     = errors.New("bad parameters")
	errUnknownMethod = errors.New("unknown method")
	errUnsupported   = errors.New("not supported in this mode")
)

type idParams struct {
	ID         string            `json:"id"`
	DeleteFile bool              `json:"deleteFile,omitempty"`
	URL        string            `json:"url,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Bps        int64             `json:"bps,omitempty"`
	Delta      int               `json:"delta,omitempty"`
}

func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%w: %v", errBadParams, err)
	}
	return nil
}

// Call runs one method. params is JSON (may be empty). The result is JSON-serializable.
func (s *Service) Call(ctx context.Context, method string, params json.RawMessage) (any, error) {
	var p idParams
	switch method {
	case "version":
		return map[string]string{"version": engine.Version}, nil
	case "list":
		return s.M.List(), nil
	case "get":
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		return s.M.Get(p.ID)
	case "add":
		var r engine.Request
		if err := decode(params, &r); err != nil {
			return nil, err
		}
		return s.M.Add(r)
	case "probe":
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return s.M.Probe(ctx, p.URL, p.Headers)
	case "pause", "resume", "restart", "remove", "updateURL", "setSpeedLimit", "move", "openFile", "showInFolder":
		if err := decode(params, &p); err != nil {
			return nil, err
		}
		if p.ID == "" {
			return nil, fmt.Errorf("%w: id is required", errBadParams)
		}
		return s.byID(method, p)
	case "pauseAll":
		s.M.PauseAll()
		return true, nil
	case "resumeAll":
		s.M.ResumeAll()
		return true, nil
	case "clearCompleted":
		s.M.ClearCompleted()
		return true, nil
	case "getSettings":
		return s.M.Settings(), nil
	case "scheduleStatus":
		return s.M.ScheduleStatus(), nil
	case "updateSettings":
		st := s.M.Settings()
		if err := decode(params, &st); err != nil {
			return nil, err
		}
		return s.M.UpdateSettings(st)
	case "capture":
		var c Capture
		if err := decode(params, &c); err != nil {
			return nil, err
		}
		return s.capture(c)
	case "pickFolder":
		if s.Hooks.PickFolder == nil {
			return nil, errUnsupported
		}
		var q struct{ Current string }
		_ = decode(params, &q)
		dir, err := s.Hooks.PickFolder(q.Current)
		return map[string]string{"dir": dir}, err
	case "browserStatus":
		return s.browserStatus(), nil
	case "extensionHello":
		return s.extensionHello(), nil
	case "openExtensionsPage":
		var q struct {
			Browser string `json:"browser"`
		}
		if err := decode(params, &q); err != nil {
			return nil, err
		}
		return s.openExtensionsPage(q.Browser)
	case "showExtensionFolder":
		return s.showExtensionFolder()
	case "readClipboard":
		if s.Hooks.Clipboard == nil {
			return nil, errUnsupported
		}
		txt, err := s.Hooks.Clipboard()
		return map[string]string{"text": txt}, err
	}
	return nil, fmt.Errorf("%w: %s", errUnknownMethod, method)
}

func (s *Service) byID(method string, p idParams) (any, error) {
	var err error
	switch method {
	case "pause":
		err = s.M.Pause(p.ID)
	case "resume":
		err = s.M.Resume(p.ID)
	case "restart":
		err = s.M.Restart(p.ID)
	case "remove":
		err = s.M.Remove(p.ID, p.DeleteFile)
	case "updateURL":
		err = s.M.UpdateURL(p.ID, p.URL, p.Headers)
	case "setSpeedLimit":
		err = s.M.SetSpeedLimit(p.ID, p.Bps)
	case "move":
		err = s.M.Move(p.ID, p.Delta)
	case "openFile", "showInFolder":
		d, gerr := s.M.Get(p.ID)
		if gerr != nil {
			return nil, gerr
		}
		path := d.Path()
		if d.Status != engine.StatusCompleted {
			path = d.PartPath()
		}
		hook := s.Hooks.OpenPath
		if method == "showInFolder" {
			hook = s.Hooks.RevealPath
		}
		if hook == nil {
			return nil, errUnsupported
		}
		err = hook(path)
	}
	if err != nil {
		return nil, err
	}
	return true, nil
}

// capture handles a download offered by the extension: start it directly if the user
// enabled that, otherwise ask the UI to show the add dialog.
func (s *Service) capture(c Capture) (any, error) {
	if c.URL == "" {
		return nil, fmt.Errorf("%w: url is required", errBadParams)
	}
	if s.M.Settings().AutoStartExt {
		d, err := s.M.Add(c.Request)
		if err != nil {
			return nil, err
		}
		return map[string]any{"queued": true, "id": d.ID}, nil
	}
	if s.Hooks.ShowWindow != nil {
		s.Hooks.ShowWindow()
	}
	s.emit(Event{Event: engine.Event{Type: "capture"}, Capture: &c})
	return map[string]any{"queued": false, "prompted": true}, nil
}
