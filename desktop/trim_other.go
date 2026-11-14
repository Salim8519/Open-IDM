//go:build !windows

package main

// trimProcesses is Windows-only; elsewhere only the Go heap is returned (trimMemory).
func trimProcesses() error { return nil }
