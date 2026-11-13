package main

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
	"github.com/Salim8519/Peregrine/engine"
)

// tray is the system tray (Windows, Linux) / menu-bar (macOS) icon and its menu.
type tray struct {
	app   *App
	ready atomic.Bool

	mu                                   sync.Mutex
	open, pause, resume, scheduled, quit *systray.MenuItem
	lang                                 string
	lastTip                              string
	lastTipAt                            time.Time
}

func newTray(a *App) *tray { return &tray{app: a} }

// start shows the icon. It never blocks; the menu appears once the platform is ready.
func (t *tray) start(lang string) {
	t.mu.Lock()
	t.lang = lang
	t.mu.Unlock()
	runTray(t.onReady)
}

func (t *tray) stop() {
	if t.ready.Load() {
		systray.Quit()
	}
}

func (t *tray) onReady() {
	setTrayIcon()
	systray.SetTooltip("Peregrine")
	if runtime.GOOS != "darwin" {
		systray.SetOnTapped(t.app.showWindow) // left click opens the window; right click the menu
	}

	t.mu.Lock()
	s := stringsFor(t.lang)
	t.open = systray.AddMenuItem(s.get("open"), "")
	systray.AddSeparator()
	t.pause = systray.AddMenuItem(s.get("pauseAll"), "")
	t.resume = systray.AddMenuItem(s.get("resumeAll"), "")
	t.scheduled = systray.AddMenuItemCheckbox(s.get("scheduled"), "", t.app.scheduleEnabled())
	systray.AddSeparator()
	t.quit = systray.AddMenuItem(s.get("quit"), "")
	open, pause, resume, scheduled, quit := t.open, t.pause, t.resume, t.scheduled, t.quit
	t.mu.Unlock()
	t.ready.Store(true)

	go func() {
		for {
			select {
			case <-open.ClickedCh:
				t.app.showWindow()
			case <-pause.ClickedCh:
				go t.app.pauseAll()
			case <-resume.ClickedCh:
				go t.app.resumeAll()
			case <-scheduled.ClickedCh:
				go t.app.toggleSchedule()
			case <-quit.ClickedCh:
				t.app.quit()
				return
			}
		}
	}()
}

// settingsChanged updates the language and the schedule checkbox.
func (t *tray) settingsChanged(st engine.Settings) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready.Load() {
		t.lang = st.Language
		return
	}
	if st.Language != t.lang {
		t.lang = st.Language
		s := stringsFor(t.lang)
		t.open.SetTitle(s.get("open"))
		t.pause.SetTitle(s.get("pauseAll"))
		t.resume.SetTitle(s.get("resumeAll"))
		t.scheduled.SetTitle(s.get("scheduled"))
		t.quit.SetTitle(s.get("quit"))
		t.lastTip = ""
	}
	if st.Schedule.Enabled != t.scheduled.Checked() {
		if st.Schedule.Enabled {
			t.scheduled.Check()
		} else {
			t.scheduled.Uncheck()
		}
	}
}

// progress updates the tooltip with the number of active downloads and total speed,
// at most once a second.
func (t *tray) progress(list []engine.Download) {
	if !t.ready.Load() {
		return
	}
	var n int
	var speed int64
	for _, d := range list {
		if d.Status == engine.StatusDownloading {
			n++
			speed += d.Speed
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s := stringsFor(t.lang)
	tip := "Peregrine\n" + s.get("idle")
	if n > 0 {
		tip = "Peregrine\n" + s.get("active", "n", n, "speed", humanSpeed(speed))
	}
	if tip == t.lastTip || (n > 0 && time.Since(t.lastTipAt) < time.Second) {
		return
	}
	t.lastTip, t.lastTipAt = tip, time.Now()
	systray.SetTooltip(tip)
}
