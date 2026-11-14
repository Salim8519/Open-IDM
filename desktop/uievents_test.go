package main

import (
	"testing"

	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/internal/api"
)

func TestForwardToUI(t *testing.T) {
	ev := func(typ engine.EventType) api.Event { return api.Event{Event: engine.Event{Type: typ}} }
	for _, c := range []struct {
		e      api.Event
		hidden bool
		want   bool
	}{
		{ev(engine.EventProgress), false, true},
		{ev(engine.EventProgress), true, false}, // nobody sees it; the next tick after showing catches up
		{ev(engine.EventUpdated), true, true},   // status changes always reach the UI
		{ev(engine.EventAdded), true, true},
		{ev(engine.EventRemoved), true, true},
		{ev(engine.EventSettings), true, true},
		{ev("capture"), true, true}, // a captured link must open the Add dialog at once
		{ev(engine.EventQueueDone), true, true},
	} {
		if got := forwardToUI(c.e, c.hidden); got != c.want {
			t.Errorf("forwardToUI(%s, hidden=%v) = %v, want %v", c.e.Type, c.hidden, got, c.want)
		}
	}
}

func TestSetHiddenMirrorsFlag(t *testing.T) {
	l := &life{}
	l.mu.Lock()
	l.setHiddenLocked(true)
	l.mu.Unlock()
	if !l.uiHidden.Load() {
		t.Fatal("uiHidden not set")
	}
	l.mu.Lock()
	l.setHiddenLocked(false)
	l.mu.Unlock()
	if l.uiHidden.Load() {
		t.Fatal("uiHidden not cleared")
	}
}
