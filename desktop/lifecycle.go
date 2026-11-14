package main

import (
	"context"
	"log"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Salim8519/Peregrine/desktop/autostart"
	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/internal/api"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// hiddenFlag is passed by the launch-at-login entry: start in the tray, without a window.
const hiddenFlag = "--hidden"

// powerCountdown is how long the user has to cancel a when-done action.
const powerCountdown = 60 * time.Second

func launchedHidden() bool { return slices.Contains(os.Args[1:], hiddenFlag) }

// life holds the desktop-only behaviour around the window: tray, notifications,
// launch at login and the scheduler's when-done action.
type life struct {
	app  *App
	tray *tray

	mu          sync.Mutex
	status      map[string]engine.Status // last known status, to notify on transitions
	focused     bool
	hidden      bool
	hiddenSince time.Time // when hidden last became true (memory trimming)
	// uiHidden mirrors hidden without l.mu, for the engine event path: engine subscribers run
	// under the engine lock, and l.mu is sometimes held while calling the engine.
	uiHidden     atomic.Bool
	toldHidden   bool // "still running in the tray" shown once per run
	quitting     bool
	notifyOK     bool
	powerTimer   *time.Timer
	powerAction  string
	autostartSet bool
	autostartOn  bool

	// Engine events are queued and handled on one goroutine, in order. The engine calls
	// subscribers while holding its lock, so handling them inline must never call back into
	// the engine (that froze the whole app in v0.2.0).
	qmu     sync.Mutex
	queue   []api.Event
	wake    chan struct{}
	stopped chan struct{}
}

// PowerEvent is sent to the UI as "peregrine:power": a when-done action is pending
// (Seconds > 0) or was cancelled/finished (Action "").
type PowerEvent struct {
	Action  string `json:"action"`
	Seconds int    `json:"seconds"`
}

func newLife(a *App) *life {
	l := &life{
		app: a, tray: newTray(a), status: map[string]engine.Status{}, focused: true,
		wake: make(chan struct{}, 1), stopped: make(chan struct{}),
	}
	go l.handleEvents()
	return l
}

// event receives every API event, from engine goroutines that may hold the engine lock.
// It only queues the event; it must not block or call into the engine.
func (l *life) event(e api.Event) {
	l.qmu.Lock()
	l.queue = append(l.queue, e)
	l.qmu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *life) handleEvents() {
	for {
		select {
		case <-l.stopped:
			return
		case <-l.wake:
		}
		for {
			l.qmu.Lock()
			batch := l.queue
			l.queue = nil
			l.qmu.Unlock()
			if len(batch) == 0 {
				break
			}
			for _, e := range batch {
				l.handle(e)
			}
		}
	}
}

func (l *life) stopEvents() {
	select {
	case <-l.stopped:
	default:
		close(l.stopped)
	}
}

// start runs after the engine is up.
func (l *life) start(ctx context.Context) {
	st := l.app.m.Settings()
	l.mu.Lock()
	for _, d := range l.app.m.List() {
		l.status[d.ID] = d.Status
	}
	l.setHiddenLocked(launchedHidden())
	l.mu.Unlock()

	l.tray.start(st.Language)
	go l.trimLoop()
	l.syncAutostart(st.LaunchAtLogin)

	if err := wruntime.InitializeNotifications(ctx); err == nil && wruntime.IsNotificationAvailable(ctx) {
		if runtime.GOOS == "darwin" {
			if ok, _ := wruntime.CheckNotificationAuthorization(ctx); !ok {
				ok, _ = wruntime.RequestNotificationAuthorization(ctx)
				l.setNotifyOK(ok)
			} else {
				l.setNotifyOK(true)
			}
		} else {
			l.setNotifyOK(true)
		}
		wruntime.OnNotificationResponse(ctx, func(wruntime.NotificationResult) { l.app.showWindow() })
	} else if err != nil {
		log.Printf("peregrine: notifications unavailable: %v", err)
	}

	// Started hidden but no tray appeared (e.g. a Linux desktop without a tray host):
	// show the window so the app is never invisible.
	if launchedHidden() {
		go func() {
			time.Sleep(5 * time.Second)
			if !l.tray.ready.Load() {
				l.app.showWindow()
			}
		}()
	}
}

func (l *life) setNotifyOK(ok bool) { l.mu.Lock(); l.notifyOK = ok; l.mu.Unlock() }

// setHiddenLocked records whether the window is hidden; l.mu must be held.
func (l *life) setHiddenLocked(hidden bool) {
	if hidden && !l.hidden {
		l.hiddenSince = time.Now()
	}
	l.hidden = hidden
	l.uiHidden.Store(hidden)
}

func (l *life) stop() {
	l.stopEvents()
	l.cancelPower()
	l.tray.stop()
	if l.app.ctx != nil {
		wruntime.CleanupNotifications(l.app.ctx)
	}
}

// handle runs on the event goroutine, outside the engine lock.
func (l *life) handle(e api.Event) {
	switch e.Type {
	case engine.EventProgress:
		l.tray.progress(e.Downloads)
	case engine.EventUpdated, engine.EventAdded:
		if e.Download != nil {
			l.downloadChanged(*e.Download)
		}
	case engine.EventRemoved:
		l.mu.Lock()
		delete(l.status, e.ID)
		l.mu.Unlock()
	case engine.EventSettings:
		if e.Settings != nil {
			l.tray.settingsChanged(*e.Settings)
			go l.syncAutostart(e.Settings.LaunchAtLogin)
		}
	case engine.EventQueueDone:
		go l.queueDone()
	}
}

func (l *life) downloadChanged(d engine.Download) {
	l.mu.Lock()
	prev, known := l.status[d.ID]
	l.status[d.ID] = d.Status
	l.mu.Unlock()
	if !known || prev == d.Status {
		if d.Status != engine.StatusDownloading {
			go l.tray.progress(l.app.m.List())
		}
		return
	}
	switch d.Status {
	case engine.StatusCompleted:
		l.notify("done-"+d.ID, func(s uiStrings) string { return s.get("completed") }, d.Filename)
	case engine.StatusError:
		l.notify("err-"+d.ID, func(s uiStrings) string { return s.get("failed") }, d.Filename+"\n"+d.Error)
	}
	if d.Status != engine.StatusDownloading {
		go l.tray.progress(l.app.m.List())
	}
}

// notify shows a system notification when the user is not looking at the window.
func (l *life) notify(id string, title func(uiStrings) string, body string) {
	st := l.app.m.Settings()
	l.mu.Lock()
	ok := l.notifyOK && st.Notify && (l.hidden || !l.focused)
	l.mu.Unlock()
	if !ok || l.app.ctx == nil {
		return
	}
	go func() {
		err := wruntime.SendNotification(l.app.ctx, wruntime.NotificationOptions{
			ID: id, Title: title(stringsFor(st.Language)), Body: body,
		})
		if err != nil {
			log.Printf("peregrine: notification: %v", err)
		}
	}()
}

// SetFocused is called by the UI on window focus/blur (bound to the frontend).
func (a *App) SetFocused(focused bool) {
	a.life.mu.Lock()
	a.life.focused = focused
	if focused {
		a.life.setHiddenLocked(false)
	}
	a.life.mu.Unlock()
}

// beforeClose decides what the window close button does. Returning true keeps the app
// running. On macOS the close button only hides the app (native behaviour, see main.go),
// so this is reached only for Quit (Cmd+Q, menu) and must not block it.
func (l *life) beforeClose(ctx context.Context) bool {
	l.mu.Lock()
	quitting := l.quitting
	l.mu.Unlock()
	if quitting || runtime.GOOS == "darwin" || l.app.m == nil {
		return false
	}
	if !l.app.m.Settings().CloseToTray || !l.tray.ready.Load() {
		return false
	}
	wruntime.WindowHide(ctx)
	l.mu.Lock()
	l.setHiddenLocked(true)
	first := !l.toldHidden
	l.toldHidden = true
	notifyOK := l.notifyOK
	l.mu.Unlock()
	if first && notifyOK {
		s := stringsFor(l.app.m.Settings().Language)
		go func() {
			_ = wruntime.SendNotification(ctx, wruntime.NotificationOptions{ID: "tray-hint", Title: "Peregrine", Body: s.get("stillRunning")})
		}()
	}
	return true
}

// quit really exits (tray menu, when-done action).
func (a *App) quit() {
	a.life.mu.Lock()
	a.life.quitting = true
	a.life.mu.Unlock()
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}

// syncAutostart makes the login entry match the setting. Enabling rewrites the entry every
// start, so it keeps pointing at the current executable after an update or a move.
func (l *life) syncAutostart(on bool) {
	l.mu.Lock()
	if l.autostartSet && l.autostartOn == on {
		l.mu.Unlock()
		return
	}
	l.autostartSet, l.autostartOn = true, on
	l.mu.Unlock()
	exe, err := os.Executable()
	if err != nil {
		log.Printf("peregrine: launch at login: %v", err)
		return
	}
	if err := autostart.Set(autostartEntry(exe), on); err != nil {
		log.Printf("peregrine: launch at login: %v", err)
	}
}

func autostartEntry(exe string) autostart.Entry {
	name := "Peregrine"
	switch runtime.GOOS {
	case "darwin":
		name = "com.peregrine.app"
	case "linux":
		name = "peregrine"
	}
	return autostart.Entry{Name: name, Exe: exe, Args: []string{hiddenFlag}}
}

// queueDone runs the scheduler's when-done action after a cancellable countdown.
func (l *life) queueDone() {
	st := l.app.m.Settings()
	action := st.Schedule.WhenDone
	if !st.Schedule.Enabled || action == engine.WhenDoneNone || action == "" {
		return
	}
	l.mu.Lock()
	if l.powerTimer != nil {
		l.mu.Unlock()
		return
	}
	l.powerAction = action
	l.powerTimer = time.AfterFunc(powerCountdown, func() { l.runPower(action) })
	notifyOK := l.notifyOK
	l.mu.Unlock()

	secs := int(powerCountdown / time.Second)
	wruntime.EventsEmit(l.app.ctx, "peregrine:power", PowerEvent{Action: action, Seconds: secs})
	l.app.showWindow()
	if notifyOK && st.Notify {
		s := stringsFor(st.Language)
		_ = wruntime.SendNotification(l.app.ctx, wruntime.NotificationOptions{
			ID: "power", Title: s.get("queueDone"), Body: s.get("powerIn", "action", s.get("action."+action), "secs", secs),
		})
	}
}

func (l *life) runPower(action string) {
	l.mu.Lock()
	if l.powerTimer == nil || l.powerAction != action {
		l.mu.Unlock()
		return // cancelled
	}
	l.powerTimer = nil
	l.mu.Unlock()
	wruntime.EventsEmit(l.app.ctx, "peregrine:power", PowerEvent{})
	var err error
	switch action {
	case engine.WhenDoneQuit:
		l.app.quit()
	case engine.WhenDoneSleep:
		err = sleepNow()
	case engine.WhenDoneShutdown:
		err = shutdownNow()
	}
	if err != nil {
		log.Printf("peregrine: %s: %v", action, err)
	}
}

func (l *life) cancelPower() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.powerTimer == nil {
		return false
	}
	l.powerTimer.Stop()
	l.powerTimer = nil
	return true
}

// CancelPowerAction stops a pending when-done action (bound to the frontend).
func (a *App) CancelPowerAction() {
	if a.life.cancelPower() && a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "peregrine:power", PowerEvent{})
	}
}

// Tray actions.
func (a *App) pauseAll() {
	if a.m != nil {
		a.m.PauseAll()
	}
}

func (a *App) resumeAll() {
	if a.m != nil {
		a.m.ResumeAll()
	}
}

func (a *App) scheduleEnabled() bool { return a.m != nil && a.m.Settings().Schedule.Enabled }

func (a *App) toggleSchedule() {
	if a.m == nil {
		return
	}
	st := a.m.Settings()
	st.Schedule.Enabled = !st.Schedule.Enabled
	if _, err := a.m.UpdateSettings(st); err != nil {
		log.Printf("peregrine: toggle schedule: %v", err)
	}
}
