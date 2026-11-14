//go:build windows

package main

import (
	"os"
	"testing"
)

// The real Windows calls work on our own process (the test binary has no WebView children).
func TestTrimProcessesWindows(t *testing.T) {
	procs, err := snapshotProcesses()
	if err != nil {
		t.Fatal(err)
	}
	self := uint32(os.Getpid())
	found := false
	for _, p := range procs {
		if p.pid == self {
			found = true
		}
	}
	if !found {
		t.Fatalf("own pid %d not in the snapshot", self)
	}
	if err := trimProcesses(); err != nil {
		t.Fatalf("trimProcesses: %v", err)
	}
	trimMemory() // must not panic
}
