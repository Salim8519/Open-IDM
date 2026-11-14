package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Salim8519/Peregrine/engine"
	ptorrent "github.com/Salim8519/Peregrine/engine/proto/torrent"
	"github.com/Salim8519/Peregrine/internal/api"
	"github.com/Salim8519/Peregrine/internal/protocols"
	"github.com/wailsapp/wails/v2/pkg/options"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is bound to the frontend. It owns the engine for the lifetime of the window.
type App struct {
	ctx            context.Context
	m              *engine.Manager
	svc            *api.Service
	srv            *api.Server
	unsub          func()
	initErr        error
	closeProtocols func() // releases torrent ports etc. after the engine stops
	life           *life
}

// NewApp creates the app; the engine starts in startup.
func NewApp() *App {
	a := &App{}
	a.life = newLife(a)
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	wruntime.OnFileDrop(ctx, a.onFileDrop)
	if err := a.initEngine(); err != nil {
		a.initErr = err
		log.Printf("peregrine: %v", err)
		return
	}
}

func (a *App) initEngine() error {
	dir, err := api.ConfigDir()
	if err != nil {
		return fmt.Errorf("config dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config dir: %w", err)
	}
	store, err := engine.OpenStore(filepath.Join(dir, "peregrine.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	m, err := engine.NewManager(store)
	if err != nil {
		_ = store.Close()
		return fmt.Errorf("start engine: %w", err)
	}
	a.m = m
	a.closeProtocols = protocols.Register(m, protocols.Options{})
	a.svc = api.NewService(m, api.Hooks{
		OpenPath:   openPath,
		RevealPath: revealPath,
		ShowWindow: a.showWindow,
		PickFolder: func(current string) (string, error) {
			return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
				DefaultDirectory:     current,
				Title:                "Choose download folder",
				CanCreateDirectories: true,
			})
		},
		Clipboard:       func() (string, error) { return wruntime.ClipboardGetText(a.ctx) },
		ExtensionDir:    bundledExtensionDir,
		OpenBrowserPage: openBrowserPage,
	})
	a.svc.SetBrowserStateFile(filepath.Join(dir, "browser.json"))
	a.unsub = a.svc.Subscribe(func(e api.Event) {
		if forwardToUI(e, a.life.uiHidden.Load()) {
			wruntime.EventsEmit(a.ctx, "peregrine:event", e)
		}
		a.life.event(e)
	})
	a.life.start(a.ctx)
	srv, err := api.Listen(a.svc, 0, api.NewToken(), nil)
	if err != nil {
		log.Printf("peregrine: local API disabled: %v", err)
		return nil
	}
	a.srv = srv
	if err := api.WriteIPCInfo(srv.Port(), srv.Token()); err != nil {
		log.Printf("peregrine: write ipc info: %v", err)
	}
	go registerBrowserHost()
	return nil
}

// registerBrowserHost (re-)registers the native messaging host that ships next to the app,
// so the browser extension can reach Peregrine. It is idempotent and runs on every start,
// which also repairs the registration after the app is moved or updated.
func registerBrowserHost() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	name := "peregrine-host"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	host := filepath.Join(filepath.Dir(exe), name)
	if _, err := os.Stat(host); err != nil {
		return // dev build without the host
	}
	cmd := exec.Command(host, "install")
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("peregrine: register browser host: %v: %s", err, out)
	}
}

// bundledExtensionDir finds the unpacked extension shipped with the app: next to the
// executable on Windows (installer and portable zip), in Contents/Resources on macOS.
func bundledExtensionDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	base := filepath.Dir(exe)
	for _, dir := range []string{
		filepath.Join(base, "extension"),
		filepath.Join(base, "..", "Resources", "extension"),
	} {
		if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
			return filepath.Clean(dir)
		}
	}
	return ""
}

func (a *App) domReady(ctx context.Context) {
	// A URL passed on the first launch (e.g. "Peregrine.exe https://...") opens the add dialog.
	if u := urlArg(os.Args[1:]); u != "" {
		go func() {
			time.Sleep(1500 * time.Millisecond) // let the UI subscribe to events first
			a.captureURL(u)
		}()
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.life.stop()
	if a.srv != nil {
		api.RemoveIPCInfo()
		_ = a.srv.Close()
	}
	if a.unsub != nil {
		a.unsub()
	}
	if a.svc != nil {
		a.svc.Close()
	}
	if a.m != nil {
		_ = a.m.Close()
	}
	if a.closeProtocols != nil {
		a.closeProtocols()
	}
}

func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	a.life.mu.Lock()
	a.life.setHiddenLocked(false)
	a.life.mu.Unlock()
	wruntime.WindowUnminimise(a.ctx)
	wruntime.WindowShow(a.ctx)
	if runtime.GOOS == "windows" {
		// Briefly topmost so the window lands in front of the browser instead of only
		// flashing in the taskbar (the native host grants the foreground right first).
		wruntime.WindowSetAlwaysOnTop(a.ctx, true)
		time.AfterFunc(400*time.Millisecond, func() { wruntime.WindowSetAlwaysOnTop(a.ctx, false) })
	}
	if runtime.GOOS == "darwin" {
		wruntime.Show(a.ctx) // unhide the app (the close button hides it)
	}
}

func (a *App) onSecondInstance(data options.SecondInstanceData) {
	a.showWindow()
	if u := urlArg(data.Args); u != "" {
		a.captureURL(u)
	}
}

func (a *App) captureURL(u string) {
	if a.svc == nil {
		return
	}
	params, _ := json.Marshal(api.Capture{Request: engine.Request{URL: u}, Source: "cli"})
	if _, err := a.svc.Call(context.Background(), "capture", params); err != nil {
		log.Printf("peregrine: capture: %v", err)
	}
}

// urlArg returns the first downloadable link among args: http(s), ftp(s) or a magnet link
// (the OS passes magnet links here once Peregrine is their handler), or an existing
// .torrent file (double-clicked once Peregrine opens .torrent files) as a file:// URL.
func urlArg(args []string) string {
	for _, s := range args {
		l := strings.ToLower(s)
		for _, p := range []string{"http://", "https://", "ftp://", "ftps://", "magnet:?"} {
			if strings.HasPrefix(l, p) {
				return s
			}
		}
		if u := torrentFileURL(s); u != "" {
			return u
		}
	}
	return ""
}

// torrentFileURL returns the file:// URL of path if it is an existing .torrent file.
func torrentFileURL(path string) string {
	if !strings.HasSuffix(strings.ToLower(path), ".torrent") {
		return ""
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return ptorrent.FileURL(path)
}

// onFileOpen handles .torrent files macOS hands to the app (double-click in Finder).
func (a *App) onFileOpen(path string) {
	a.showWindow()
	if u := torrentFileURL(path); u != "" {
		a.captureURL(u)
	}
}

// onFileDrop adds .torrent files dropped onto the window.
func (a *App) onFileDrop(_, _ int, paths []string) {
	for _, p := range paths {
		if u := torrentFileURL(p); u != "" {
			a.captureURL(u)
		}
	}
}

// onURLOpen handles links macOS hands to the app (magnet: links).
func (a *App) onURLOpen(u string) {
	a.showWindow()
	if v := urlArg([]string{u}); v != "" {
		a.captureURL(v)
	}
}

// Call is the single binding used by the frontend: JSON params in, JSON result out.
func (a *App) Call(method string, params string) (string, error) {
	if a.svc == nil {
		if a.initErr != nil {
			return "", a.initErr
		}
		return "", fmt.Errorf("engine is not ready")
	}
	res, err := a.svc.Call(a.ctx, method, json.RawMessage(params))
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
