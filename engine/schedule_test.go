package engine

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/Salim8519/Peregrine/internal/testsrv"
)

// fakeClock is a goroutine-safe settable clock.
type fakeClock struct{ ns atomic.Int64 }

func newFakeClock(t time.Time) *fakeClock { c := &fakeClock{}; c.set(t); return c }
func (c *fakeClock) set(t time.Time)      { c.ns.Store(t.UnixNano()) }
func (c *fakeClock) now() time.Time       { return time.Unix(0, c.ns.Load()).In(time.Local) }

// local returns a local time on Wednesday 2026-11-11 (weekday 3).
func local(h, m int) time.Time { return time.Date(2026, 11, 11, h, m, 0, 0, time.Local) }

func TestScheduleStatus(t *testing.T) {
	wed := func(h, m int) time.Time { return local(h, m) }
	cases := []struct {
		name  string
		s     Schedule
		now   time.Time
		open  bool
		start time.Time // expected NextStart (zero = none)
		stop  time.Time // expected NextStop (zero = none)
	}{
		{"disabled is always open", Schedule{Start: "02:00", Stop: "08:00"}, wed(12, 0), true, time.Time{}, time.Time{}},
		{"before window", Schedule{Enabled: true, Start: "02:00", Stop: "08:00"}, wed(1, 59), false, wed(2, 0), time.Time{}},
		{"at start", Schedule{Enabled: true, Start: "02:00", Stop: "08:00"}, wed(2, 0), true, time.Time{}, wed(8, 0)},
		{"at stop is closed", Schedule{Enabled: true, Start: "02:00", Stop: "08:00"}, wed(8, 0), false, wed(2, 0).AddDate(0, 0, 1), time.Time{}},
		{"overnight, before midnight", Schedule{Enabled: true, Start: "23:00", Stop: "06:00"}, wed(23, 30), true, time.Time{}, wed(6, 0).AddDate(0, 0, 1)},
		{"overnight, after midnight", Schedule{Enabled: true, Start: "23:00", Stop: "06:00"}, wed(5, 0), true, time.Time{}, wed(6, 0)},
		{"overnight, daytime", Schedule{Enabled: true, Start: "23:00", Stop: "06:00"}, wed(12, 0), false, wed(23, 0), time.Time{}},
		{"day not selected", Schedule{Enabled: true, Start: "02:00", Stop: "08:00", Days: []int{4}}, wed(3, 0), false, wed(2, 0).AddDate(0, 0, 1), time.Time{}},
		{"overnight from selected day", Schedule{Enabled: true, Start: "23:00", Stop: "06:00", Days: []int{2}}, wed(5, 0), true, time.Time{}, wed(6, 0)},
		{"no stop lasts 24h", Schedule{Enabled: true, Start: "22:00", Days: []int{2}}, wed(21, 59), true, time.Time{}, wed(22, 0)},
		{"no stop ends next day", Schedule{Enabled: true, Start: "22:00", Days: []int{2}}, wed(22, 0), false, wed(22, 0).AddDate(0, 0, 6), time.Time{}},
		{"invalid schedule never blocks", Schedule{Enabled: true, Start: "25:00"}, wed(12, 0), true, time.Time{}, time.Time{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.s.WhenDone == "" {
				c.s.WhenDone = WhenDoneNone
			}
			got := c.s.Status(c.now)
			if got.Open != c.open {
				t.Fatalf("open = %v, want %v", got.Open, c.open)
			}
			checkTime(t, "nextStart", got.NextStart, c.start)
			checkTime(t, "nextStop", got.NextStop, c.stop)
		})
	}
}

func checkTime(t *testing.T, name string, got *time.Time, want time.Time) {
	t.Helper()
	switch {
	case want.IsZero() && got != nil:
		t.Fatalf("%s = %v, want none", name, *got)
	case !want.IsZero() && (got == nil || !got.Equal(want)):
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestScheduleValidation(t *testing.T) {
	bad := []Schedule{
		{Start: "2am", WhenDone: WhenDoneNone},
		{Start: "02:00", Stop: "02:00", WhenDone: WhenDoneNone},
		{Start: "02:00", Stop: "24:00", WhenDone: WhenDoneNone},
		{Start: "02:00", Days: []int{7}, WhenDone: WhenDoneNone},
		{Start: "02:00", WhenDone: "format-disk"},
	}
	m := newTestManager(t, ":memory:", nil)
	for _, s := range bad {
		st := m.Settings()
		st.Schedule = s
		if _, err := m.UpdateSettings(st); err == nil {
			t.Errorf("schedule %+v accepted", s)
		}
	}
	st := m.Settings()
	st.Schedule = Schedule{Enabled: true, Start: "1:05", Stop: "", Days: []int{5, 1, 1, 0, 2, 3, 4, 6}}
	got, err := m.UpdateSettings(st)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schedule.Days != nil || got.Schedule.WhenDone != WhenDoneNone {
		t.Fatalf("not normalized: %+v", got.Schedule)
	}
}

func newClockManager(t *testing.T, clock *fakeClock, mutate func(*Settings)) *Manager {
	t.Helper()
	st, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	s := DefaultSettings()
	s.DownloadDir = t.TempDir()
	s.Proxy = "none"
	if mutate != nil {
		mutate(&s)
	}
	if err := st.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	m, err := newManager(st, clock.now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close(); st.Close() })
	return m
}

func nightly(s *Settings) {
	s.Schedule = Schedule{Enabled: true, Start: "02:00", Stop: "08:00", WhenDone: WhenDoneNone}
}

func TestScheduleHoldsQueueUntilWindowOpens(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	clock := newFakeClock(local(1, 0))
	m := newClockManager(t, clock, nightly)
	if m.ScheduleStatus().Open {
		t.Fatal("window should be closed at 01:00")
	}
	d, err := m.Add(Request{URL: srv.FileURL("size=300000&name=night.bin")})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond) // a few ticker rounds
	if got, _ := m.Get(d.ID); got.Status != StatusQueued || srv.Requests() != 0 {
		t.Fatalf("started outside the window: status=%s requests=%d", got.Status, srv.Requests())
	}
	clock.set(local(2, 0))
	waitStatus(t, m, d.ID, StatusCompleted, 15*time.Second)
}

func TestScheduleStopRequeuesAndResumes(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	clock := newFakeClock(local(7, 59))
	m := newClockManager(t, clock, nightly)
	const size = 4 << 20
	d, err := m.Add(Request{URL: srv.FileURL("size=4194304&name=slow.bin&throttle=800000"), Connections: 2})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, _ := m.Get(d.ID)
		if got.Downloaded > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("download did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	clock.set(local(8, 0))
	got := waitStatus(t, m, d.ID, StatusQueued, 5*time.Second)
	if got.Downloaded == 0 || got.Downloaded >= size {
		t.Fatalf("expected partial progress kept, got %d", got.Downloaded)
	}
	time.Sleep(700 * time.Millisecond)
	if again, _ := m.Get(d.ID); again.Status != StatusQueued {
		t.Fatalf("restarted outside the window: %s", again.Status)
	}
	clock.set(local(2, 0).AddDate(0, 0, 1))
	done := waitStatus(t, m, d.ID, StatusCompleted, 30*time.Second)
	if done.SHA256 != testsrv.SHA256(size, 1) {
		t.Fatal("content mismatch after schedule resume")
	}
}

func TestDisablingScheduleStartsQueue(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	clock := newFakeClock(local(12, 0))
	m := newClockManager(t, clock, nightly)
	d, err := m.Add(Request{URL: srv.FileURL("size=200000&name=now.bin")})
	if err != nil {
		t.Fatal(err)
	}
	st := m.Settings()
	st.Schedule.Enabled = false
	if _, err := m.UpdateSettings(st); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, d.ID, StatusCompleted, 15*time.Second)
}

func TestQueueDoneEvent(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	m := newTestManager(t, ":memory:", func(s *Settings) { s.MaxActive = 1 })
	var done atomic.Int32
	unsub := m.Subscribe(func(e Event) {
		if e.Type == EventQueueDone {
			done.Add(1)
		}
	})
	defer unsub()
	var ids []string
	for _, n := range []string{"a.bin", "b.bin"} {
		d, err := m.Add(Request{URL: srv.FileURL("size=100000&name=" + n)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
	}
	for _, id := range ids {
		waitStatus(t, m, id, StatusCompleted, 15*time.Second)
	}
	time.Sleep(100 * time.Millisecond)
	if n := done.Load(); n != 1 {
		t.Fatalf("queueDone fired %d times, want 1", n)
	}
}
