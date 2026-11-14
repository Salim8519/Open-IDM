package main

import (
	"testing"
	"time"
)

func TestShouldTrim(t *testing.T) {
	t0 := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		s    trimState
		now  time.Time
		want bool
	}{
		{"visible window", trimState{hidden: false}, t0.Add(time.Hour), false},
		{"just hidden", trimState{hidden: true, hiddenSince: t0}, t0.Add(10 * time.Second), false},
		{"hidden long enough", trimState{hidden: true, hiddenSince: t0}, t0.Add(trimAfter), true},
		{"busy downloading", trimState{hidden: true, hiddenSince: t0, busy: true}, t0.Add(time.Hour), false},
		{"trimmed recently", trimState{hidden: true, hiddenSince: t0, lastTrim: t0.Add(trimAfter)}, t0.Add(trimAfter + time.Minute), false},
		{"time to trim again", trimState{hidden: true, hiddenSince: t0, lastTrim: t0.Add(trimAfter)}, t0.Add(trimAfter + trimEvery), true},
		// Shown and hidden again since the last trim: wait trimAfter, then trim once more.
		{"hidden again, too soon", trimState{hidden: true, hiddenSince: t0.Add(time.Hour), lastTrim: t0}, t0.Add(time.Hour + 5*time.Second), false},
		{"hidden again, long enough", trimState{hidden: true, hiddenSince: t0.Add(time.Hour), lastTrim: t0}, t0.Add(time.Hour + trimAfter), true},
	}
	for _, c := range cases {
		if got := shouldTrim(c.s, c.now); got != c.want {
			t.Errorf("%s: shouldTrim = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWebviewDescendants(t *testing.T) {
	procs := []procEntry{
		{pid: 10, parent: 1, exe: "Peregrine.exe"},
		{pid: 11, parent: 10, exe: "msedgewebview2.exe"},
		{pid: 12, parent: 11, exe: "msedgewebview2.exe"}, // renderer under the browser process
		{pid: 13, parent: 10, exe: "yt-dlp.exe"},         // ours, but not a WebView process
		{pid: 14, parent: 99, exe: "msedgewebview2.exe"}, // another app's WebView
		{pid: 15, parent: 13, exe: "MSEdgeWebView2.EXE"}, // case-insensitive, deeper
	}
	got := map[uint32]bool{}
	for _, pid := range webviewDescendants(procs, 10) {
		got[pid] = true
	}
	for _, want := range []uint32{11, 12, 15} {
		if !got[want] {
			t.Errorf("pid %d missing from %v", want, got)
		}
	}
	for _, not := range []uint32{10, 13, 14} {
		if got[not] {
			t.Errorf("pid %d must not be trimmed: %v", not, got)
		}
	}
}

// A parent loop in a stale snapshot (PID reuse) must not hang.
func TestWebviewDescendantsLoop(t *testing.T) {
	procs := []procEntry{
		{pid: 20, parent: 21, exe: "msedgewebview2.exe"},
		{pid: 21, parent: 20, exe: "msedgewebview2.exe"},
	}
	if got := webviewDescendants(procs, 20); len(got) != 1 || got[0] != 21 {
		t.Fatalf("got %v, want [21]", got)
	}
}
