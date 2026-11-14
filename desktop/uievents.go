package main

import (
	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/internal/api"
)

// forwardToUI decides which engine events reach the web UI. Progress ticks (twice a second
// per batch of active downloads) are skipped while the window is hidden in the tray: nobody
// sees them, and drawing them cost ~13% of a core during heavy downloads (ADR 0015). The
// first tick after the window is shown brings the numbers up to date within 0.5 s. Every
// other event, status changes and captured links included, always goes through.
func forwardToUI(e api.Event, hidden bool) bool {
	return !(hidden && e.Type == engine.EventProgress)
}
