package engine

import (
	"math"
	"testing"
	"time"
)

// feed runs a speedMeter over a speed trace (bytes/s per 0.5 s tick) and returns the shown
// ETA after each tick plus the true remaining time at that tick.
func feed(t *testing.T, size int64, speeds []float64) (shown []int64, truth []float64) {
	t.Helper()
	sm := newSpeedMeter(0, time.Time{})
	var cur float64
	for i, v := range speeds {
		cur += v * 0.5
		if cur > float64(size) {
			cur = float64(size)
		}
		sm.sample(int64(cur), size, 0.5)
		shown = append(shown, sm.shownETA())
		// truth: time left at the average speed of the remaining trace
		var left, secs float64 = float64(size) - cur, 0
		for _, w := range speeds[i+1:] {
			if left <= 0 {
				break
			}
			left -= w * 0.5
			secs += 0.5
		}
		truth = append(truth, secs)
	}
	return shown, truth
}

// bursty alternates fast and slow every few ticks around avg (like a torrent swarm).
func bursty(avg float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		switch (i / 3) % 3 {
		case 0:
			out[i] = avg * 1.8
		case 1:
			out[i] = avg * 0.4
		default:
			out[i] = avg * 0.8
		}
	}
	return out
}

func TestETADoesNotJumpWithBurstySpeed(t *testing.T) {
	const mb = 1 << 20
	speeds := bursty(3*mb, 2000) // ~3 MB/s average, swinging 1.2..5.4 MB/s
	shown, _ := feed(t, 1700*mb, speeds)
	// After the warm-up (5 s) and a settle period, each 0.5 s step may change the shown
	// time by the countdown (0.5 s) plus small corrections, never by tens of seconds.
	var worst float64
	for i := 40; i < len(shown); i++ {
		if shown[i] < 0 || shown[i-1] < 0 {
			continue
		}
		if d := math.Abs(float64(shown[i] - shown[i-1])); d > worst {
			worst = d
		}
	}
	if worst > 30 {
		t.Fatalf("shown ETA jumped by %.0f s in one tick; want a steady countdown", worst)
	}
	// The naive estimate (remaining / current speed) swings by minutes on the same trace.
}

func TestETAIsCloseToTheTruth(t *testing.T) {
	const mb = 1 << 20
	speeds := bursty(3*mb, 1400)
	shown, truth := feed(t, 1700*mb, speeds)
	for i := 120; i < len(shown); i += 60 { // from 60 s on, sampled every 30 s
		if shown[i] < 0 || truth[i] < 30 {
			continue
		}
		if err := math.Abs(float64(shown[i])-truth[i]) / truth[i]; err > 0.25 {
			t.Errorf("tick %d: shown %d s, true %.0f s (%.0f%% off)", i, shown[i], truth[i], err*100)
		}
	}
}

func TestETAFollowsARealSpeedChange(t *testing.T) {
	const mb = 1 << 20
	speeds := make([]float64, 0, 400)
	for i := 0; i < 120; i++ { // 60 s at 2 MB/s
		speeds = append(speeds, 2*mb)
	}
	for i := 0; i < 600; i++ { // then 8 MB/s until done
		speeds = append(speeds, 8*mb)
	}
	shown, truth := feed(t, 2000*mb, speeds)
	i := 120 + 80 // 40 s after the speed quadrupled
	if shown[i] < 0 {
		t.Fatal("no estimate")
	}
	if err := math.Abs(float64(shown[i])-truth[i]) / truth[i]; err > 0.3 {
		t.Fatalf("40 s after the speed quadrupled: shown %d s, true %.0f s", shown[i], truth[i])
	}
}

func TestETACountsDownSteadily(t *testing.T) {
	const mb = 1 << 20
	speeds := make([]float64, 200)
	for i := range speeds {
		speeds[i] = 4 * mb
	}
	shown, _ := feed(t, 2000*mb, speeds)
	// Constant speed: after warm-up each second takes about one second off.
	for i := 20; i < len(shown); i += 2 {
		if d := shown[i-2] - shown[i]; d < 0 || d > 2 {
			t.Fatalf("tick %d: went from %d to %d in 1 s", i, shown[i-2], shown[i])
		}
	}
}

func TestETAWaitingWhenNoData(t *testing.T) {
	const mb = 1 << 20
	sm := newSpeedMeter(0, time.Time{})
	var cur int64
	for i := 0; i < 40; i++ {
		cur += 2 * mb
		sm.sample(cur, 1000*mb, 0.5)
	}
	if e := sm.shownETA(); e <= 0 {
		t.Fatalf("eta while downloading = %d", e)
	}
	before := sm.shownETA()
	for i := 0; i < 8; i++ { // 4 s without data: still an estimate
		sm.sample(cur, 1000*mb, 0.5)
	}
	if e := sm.shownETA(); e < 0 {
		t.Fatalf("eta after a 4 s gap = %d, want a number", e)
	}
	for i := 0; i < 4; i++ { // 6 s without data: waiting
		sm.sample(cur, 1000*mb, 0.5)
	}
	if e := sm.shownETA(); e != ETAWaiting {
		t.Fatalf("eta after 6 s without data = %d, want ETAWaiting", e)
	}
	if s := sm.shownSpeed(); s != 0 {
		t.Fatalf("speed while waiting = %d, want 0", s)
	}
	cur += 2 * mb // data again
	sm.sample(cur, 1000*mb, 0.5)
	if e := sm.shownETA(); e < 0 || float64(e) > 3*float64(before) {
		t.Fatalf("eta after data resumed = %d (before the gap %d)", e, before)
	}
}

func TestETAWaitingBeforeFirstByte(t *testing.T) {
	sm := newSpeedMeter(0, time.Time{})
	sm.sample(0, 0, 0.5)
	if e := sm.shownETA(); e != ETAUnknown {
		t.Fatalf("eta right after start = %d, want unknown", e)
	}
	for i := 0; i < 12; i++ { // e.g. a magnet link still finding peers
		sm.sample(0, 0, 0.5)
	}
	if e := sm.shownETA(); e != ETAWaiting {
		t.Fatalf("eta after 6 s without any data = %d, want ETAWaiting", e)
	}
}

func TestETAUnknownWithoutSize(t *testing.T) {
	sm := newSpeedMeter(0, time.Time{})
	for i := 1; i <= 20; i++ {
		sm.sample(int64(i)<<20, 0, 0.5)
	}
	if e := sm.shownETA(); e != ETAUnknown {
		t.Fatalf("eta without a size = %d, want unknown", e)
	}
	if s := sm.shownSpeed(); s <= 0 {
		t.Fatalf("speed without a size = %d, want > 0", s)
	}
}

func TestSpeedReactsQuickly(t *testing.T) {
	const mb = 1 << 20
	sm := newSpeedMeter(0, time.Time{})
	var cur int64
	for i := 0; i < 40; i++ { // 20 s at 1 MB/s
		cur += mb / 2
		sm.sample(cur, 0, 0.5)
	}
	for i := 0; i < 8; i++ { // 4 s at 5 MB/s
		cur += 5 * mb / 2
		sm.sample(cur, 0, 0.5)
	}
	if s := float64(sm.shownSpeed()) / mb; s < 4 {
		t.Fatalf("shown speed 4 s after a jump to 5 MB/s = %.2f MB/s; it should be live", s)
	}
}

func TestSpeedMeterCounterReset(t *testing.T) {
	sm := newSpeedMeter(0, time.Time{})
	for i := 1; i <= 20; i++ {
		sm.sample(int64(i)<<20, 100<<20, 0.5)
	}
	sm.sample(0, 100<<20, 0.5) // restarted from zero
	if e := sm.shownETA(); e != ETAUnknown {
		t.Fatalf("eta right after a counter reset = %d, want unknown", e)
	}
}
