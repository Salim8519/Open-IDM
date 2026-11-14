package main

import (
	"log"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

// Memory trimming (ADR 0015). While the window is hidden and nothing is downloading, the
// app hands memory it is not using back to the OS: the Go heap and, on Windows, the working
// set of its own process and its WebView2 processes. The WebView is not closed or reloaded,
// so opening the window or a captured download's dialog stays instant; pages it touches
// again are paged back in by the OS. No data is involved, so nothing can be corrupted.
const (
	trimAfter = 30 * time.Second // hidden this long before the first trim
	trimEvery = 5 * time.Minute  // then again at most this often while still hidden
	trimTick  = 15 * time.Second // how often the loop checks
)

// trimState is what shouldTrim decides on (a snapshot, so the policy is a pure function).
type trimState struct {
	hidden      bool
	hiddenSince time.Time // when the window was last hidden
	busy        bool      // a download is running or a when-done countdown is pending
	lastTrim    time.Time
}

func shouldTrim(s trimState, now time.Time) bool {
	if !s.hidden || s.busy || now.Sub(s.hiddenSince) < trimAfter {
		return false
	}
	// First trim since this hide, or the periodic one.
	return s.lastTrim.Before(s.hiddenSince) || now.Sub(s.lastTrim) >= trimEvery
}

// procEntry is one row of a process snapshot.
type procEntry struct {
	pid, parent uint32
	exe         string
}

// webviewDescendants returns the WebView2 processes started (directly or not) by root.
// Only those are trimmed: never root's other children, never another app's processes.
func webviewDescendants(procs []procEntry, root uint32) []uint32 {
	children := map[uint32][]procEntry{}
	for _, p := range procs {
		children[p.parent] = append(children[p.parent], p)
	}
	var out []uint32
	seen := map[uint32]bool{root: true}
	queue := []uint32{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, c := range children[pid] {
			if seen[c.pid] {
				continue // a loop in a stale snapshot (PID reuse)
			}
			seen[c.pid] = true
			queue = append(queue, c.pid)
			if strings.EqualFold(c.exe, "msedgewebview2.exe") {
				out = append(out, c.pid)
			}
		}
	}
	return out
}

// trimSnapshot reads the state shouldTrim needs.
func (l *life) trimSnapshot(lastTrim time.Time) trimState {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := trimState{hidden: l.hidden, hiddenSince: l.hiddenSince, lastTrim: lastTrim, busy: l.powerTimer != nil}
	for _, st := range l.status {
		if st == engine.StatusDownloading {
			s.busy = true
			break
		}
	}
	return s
}

// trimLoop runs until the app stops.
func (l *life) trimLoop() {
	t := time.NewTicker(trimTick)
	defer t.Stop()
	var last time.Time
	for {
		select {
		case <-l.stopped:
			return
		case now := <-t.C:
			if !shouldTrim(l.trimSnapshot(last), now) {
				continue
			}
			last = now
			trimMemory()
		}
	}
}

// trimMemory never fails the app: errors are only logged.
func trimMemory() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("peregrine: memory trim: %v", r)
		}
	}()
	debug.FreeOSMemory()
	if err := trimProcesses(); err != nil {
		log.Printf("peregrine: memory trim: %v", err)
	}
}
