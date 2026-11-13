package engine

import (
	"fmt"
	"slices"
	"time"
)

// Schedule limits when the queue may run (Q3). While it is enabled, queued downloads start
// only inside the time window; at the stop time running downloads go back to the queue.
type Schedule struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"` // "HH:MM", local time
	// Stop is "HH:MM" or "" for no stop time (then the window lasts 24 hours).
	// A stop earlier than the start means the window runs past midnight.
	Stop string `json:"stop"`
	// Days the window starts on: 0 = Sunday … 6 = Saturday. Empty means every day.
	Days []int `json:"days"`
	// WhenDone is what the app does after a scheduled run finishes the whole queue:
	// "none", "quit", "sleep" or "shutdown". The engine only reports EventQueueDone;
	// the desktop app performs the action (after a cancellable countdown).
	WhenDone string `json:"whenDone"`
}

// ScheduleStatus tells the UI whether the queue may run now and when that changes.
type ScheduleStatus struct {
	Enabled   bool       `json:"enabled"`
	Open      bool       `json:"open"`                // downloads may run now
	NextStart *time.Time `json:"nextStart,omitempty"` // next window start (when closed)
	NextStop  *time.Time `json:"nextStop,omitempty"`  // end of the current window (when open)
}

// When-done actions.
const (
	WhenDoneNone     = "none"
	WhenDoneQuit     = "quit"
	WhenDoneSleep    = "sleep"
	WhenDoneShutdown = "shutdown"
)

func defaultSchedule() Schedule {
	return Schedule{Start: "02:00", Stop: "08:00", WhenDone: WhenDoneNone}
}

// parseClock parses "HH:MM" into minutes after midnight.
func parseClock(s string) (int, error) {
	var h, m int
	if n, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || n != 2 || h < 0 || h > 23 || m < 0 || m > 59 || len(s) > 5 {
		return 0, fmt.Errorf("invalid time %q (use HH:MM)", s)
	}
	return h*60 + m, nil
}

// validate checks a schedule; normalizeSchedule fills harmless defaults first.
func (s *Schedule) validate() error {
	if _, err := parseClock(s.Start); err != nil {
		return fmt.Errorf("schedule start: %w", err)
	}
	if s.Stop != "" {
		stop, err := parseClock(s.Stop)
		if err != nil {
			return fmt.Errorf("schedule stop: %w", err)
		}
		if start, _ := parseClock(s.Start); start == stop {
			return fmt.Errorf("schedule stop must differ from start")
		}
	}
	for _, d := range s.Days {
		if d < 0 || d > 6 {
			return fmt.Errorf("schedule day %d out of range 0-6", d)
		}
	}
	switch s.WhenDone {
	case WhenDoneNone, WhenDoneQuit, WhenDoneSleep, WhenDoneShutdown:
	default:
		return fmt.Errorf("unknown when-done action %q", s.WhenDone)
	}
	return nil
}

func normalizeSchedule(s *Schedule) {
	if s.Start == "" {
		s.Start = defaultSchedule().Start
	}
	if s.WhenDone == "" {
		s.WhenDone = WhenDoneNone
	}
	days := slices.Clone(s.Days)
	slices.Sort(days)
	s.Days = slices.Compact(days)
	if len(s.Days) == 7 {
		s.Days = nil // every day
	}
}

func (s *Schedule) onDay(wd time.Weekday) bool {
	return len(s.Days) == 0 || slices.Contains(s.Days, int(wd))
}

// at returns the time on day's date at minute-of-day min, in day's location (DST-safe).
func at(day time.Time, min int) time.Time {
	y, mo, d := day.Date()
	return time.Date(y, mo, d, min/60, min%60, 0, 0, day.Location())
}

// window returns the window that starts on day's date: [start, end). Without a stop time
// the window lasts 24 hours, until the same time the next day.
func (s *Schedule) window(day time.Time) (start, end time.Time) {
	sm, _ := parseClock(s.Start)
	start = at(day, sm)
	em := sm
	if s.Stop != "" {
		em, _ = parseClock(s.Stop)
	}
	end = at(day, em)
	if em <= sm {
		end = at(day.AddDate(0, 0, 1), em)
	}
	return start, end
}

// Status reports whether downloads may run at now.
func (s Schedule) Status(now time.Time) ScheduleStatus {
	st := ScheduleStatus{Enabled: s.Enabled, Open: true}
	if !s.Enabled || s.validate() != nil {
		return st
	}
	st.Open = false
	// A window is at most 24 hours long, so only yesterday's can still be open.
	for i := -1; i <= 7; i++ {
		day := now.AddDate(0, 0, i)
		if !s.onDay(day.Weekday()) {
			continue
		}
		start, end := s.window(day)
		if !start.After(now) && now.Before(end) {
			st.Open = true
			e := end
			if st.NextStop == nil || e.After(*st.NextStop) {
				st.NextStop = &e
			}
		}
		if start.After(now) && st.NextStart == nil {
			ns := start
			st.NextStart = &ns
		}
	}
	if st.Open {
		st.NextStart = nil
	}
	return st
}
