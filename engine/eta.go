package engine

import (
	"math"
	"time"
)

// Speed and remaining-time estimation (ADR 0019).
//
// The UI shows one speed and one remaining time, but they answer different questions:
//   - speed: "how fast is it right now?" — a short average (about 2 s), so it reacts at once;
//   - remaining time: "when will it finish?" — based on a long average (about 20 s), and
//     shown as a countdown that glides to a new estimate only when it stays clearly off
//     for a few seconds. Short bursts and dips
//     (normal for torrents and busy servers) no longer make it jump back and forth.
// When no data arrives for a few seconds the time is reported as ETAWaiting instead of
// a huge or stale number.

const (
	speedTau    = 2.0  // seconds: time constant of the shown speed
	etaTau      = 20.0 // seconds: time constant of the speed behind the remaining time
	etaWarmup   = 5.0  // seconds of data before the countdown starts smoothing
	etaOffRatio = 0.15 // the estimate must differ by more than this fraction...
	etaOffMin   = 3.0  // ...and by more than this many seconds...
	etaOffHold  = 4.0  // ...for this long before the countdown corrects itself
	etaSlewTau  = 4.0  // seconds: how fast a correction glides to the new estimate
	stallAfter  = 5.0  // seconds without new bytes before the time says "waiting"
	shiftRatio  = 0.25 // the live speed staying this far above or below the long average...
	shiftHold   = 4.0  // ...for this long is a real change (new peers, server slowed down)
	shiftTau    = 4.0  // seconds: the long average then catches up this fast...
	shiftDone   = 0.05 // ...until it is this close to the live speed
)

// ETA values besides a number of seconds.
const (
	ETAUnknown int64 = -1 // no estimate (no size, no data yet)
	ETAWaiting int64 = -2 // downloading, but no data for a few seconds (peers, reconnect)
)

// speedMeter estimates one running download's speed and remaining time. It is fed the
// byte counter at every tick; it holds no lock of its own (the Manager's mutex guards it).
type speedMeter struct {
	at    time.Time // time of the previous sample
	last  int64     // byte counter at the previous sample
	speed float64   // shown speed, bytes/s
	avg   float64   // long average behind the remaining time, bytes/s

	runTime  float64 // seconds since the first byte of this run
	shift    float64 // how long the live speed has stayed far from the long average
	shiftDir int     // +1 above, -1 below, 0 close
	catching bool    // a lasting change was seen: the long average is catching up
	idle     float64 // seconds since the last new byte
	eta      float64 // shown remaining time in seconds, -1 = none yet
	off      float64 // how long the estimate has been clearly different from eta
	started  bool    // a byte has arrived in this run
}

func newSpeedMeter(downloaded int64, now time.Time) *speedMeter {
	return &speedMeter{at: now, last: downloaded, eta: -1}
}

// sample feeds the byte counter cur, dt seconds after the previous sample, for a download
// of size bytes (<= 0 if unknown).
func (s *speedMeter) sample(cur, size int64, dt float64) {
	if dt <= 0 {
		return
	}
	got := cur - s.last
	if got < 0 { // counter went back (restart without range support): start over
		got = 0
		s.started, s.runTime, s.eta, s.off, s.catching = false, 0, -1, 0, false
	}
	s.last = cur
	inst := float64(got) / dt

	if got > 0 {
		s.idle = 0
		if !s.started {
			s.started = true
			s.speed, s.avg = inst, inst
		}
	} else {
		s.idle += dt
	}
	if !s.started {
		return
	}
	s.runTime += dt
	s.speed = ewma(s.speed, inst, dt, speedTau)
	// A burst or dip lasting a few seconds is noise; one that lasts is a new normal. Once
	// seen, the long average catches up quickly until it is close to the live speed
	// (stopping at the detection threshold would leave the time ~25% off).
	gap := math.Abs(s.speed - s.avg)
	dir := 0
	if gap > shiftRatio*s.avg {
		dir = 1
		if s.speed < s.avg {
			dir = -1
		}
	}
	if dir != 0 && dir == s.shiftDir {
		s.shift += dt
	} else if dir != 0 || !s.catching {
		s.shift, s.shiftDir = 0, dir
	}
	if s.shift >= shiftHold {
		s.catching = true
	}
	if s.catching && gap < shiftDone*s.avg {
		s.catching, s.shift, s.shiftDir = false, 0, 0
	}
	tau := etaTau
	if s.catching {
		tau = shiftTau
	}
	s.avg = ewma(s.avg, inst, dt, tau)

	if size <= 0 || cur >= size {
		s.eta, s.off = -1, 0
		return
	}
	if s.idle >= stallAfter {
		s.off = 0 // keep the last estimate; it is not counted down while nothing arrives
		return
	}
	basis := s.avg
	if s.catching {
		basis = s.speed // during a confirmed change the live speed is the better guess
	}
	if basis <= 0 {
		return
	}
	raw := float64(size-cur) / basis

	if s.eta < 0 || s.runTime < etaWarmup {
		s.eta, s.off = raw, 0
		return
	}
	s.eta -= dt // count down like a clock
	if s.eta < 1 {
		s.eta, s.off = raw, 0
		return
	}
	d := math.Abs(raw - s.eta)
	if d > etaOffMin && d > etaOffRatio*s.eta {
		s.off += dt
		if s.off >= etaOffHold {
			s.eta = ewma(s.eta, raw, dt, etaSlewTau) // glide toward it, no jump
		}
	} else {
		s.off = 0
	}
}

// shownSpeed is the speed to display, in bytes/s.
func (s *speedMeter) shownSpeed() int64 {
	if !s.started || s.idle >= stallAfter {
		return 0
	}
	return int64(s.speed)
}

// shownETA is the remaining time to display: seconds, ETAWaiting or ETAUnknown.
func (s *speedMeter) shownETA() int64 {
	if s.idle >= stallAfter { // also before the first byte (e.g. a torrent finding peers)
		return ETAWaiting
	}
	if s.eta < 0 {
		return ETAUnknown
	}
	return int64(math.Round(s.eta))
}

// ewma moves prev toward x with time constant tau for a step of dt seconds.
func ewma(prev, x, dt, tau float64) float64 {
	a := 1 - math.Exp(-dt/tau)
	return prev + a*(x-prev)
}
