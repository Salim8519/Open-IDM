package main

import (
	"context"
	"log"
	"time"
)

// Update check timing (ADR 0020): once shortly after start, then once a day while running.
const (
	updateFirstDelay = 20 * time.Second
	updateEvery      = 24 * time.Hour
)

// updateLoop checks for a newer release while the setting is on. It only looks; the user
// downloads the new version from the release page.
func (l *life) updateLoop(ctx context.Context) {
	wait := updateFirstDelay
	for {
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-l.stopped:
			t.Stop()
			return
		case <-t.C:
		}
		wait = updateEvery
		if l.app.m == nil || l.app.svc == nil || !l.app.m.Settings().UpdateCheck {
			continue
		}
		if _, err := l.app.svc.CheckUpdate(ctx); err != nil {
			log.Printf("peregrine: %v", err)
			wait = time.Hour // offline or rate-limited: try again sooner
		}
	}
}

// updateFound tells the user about a newer version once per version and run, as a system
// notification when the window is not in front (the window itself shows a banner).
func (l *life) updateFound(latest string) {
	l.mu.Lock()
	seen := l.updateSeen == latest
	l.updateSeen = latest
	l.mu.Unlock()
	if seen {
		return
	}
	body := stringsFor(l.app.m.Settings().Language).get("updateBody")
	l.notify("update-"+latest, func(s uiStrings) string { return s.get("updateAvailable", "version", latest) }, body)
}
