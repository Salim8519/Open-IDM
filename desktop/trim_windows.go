//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// trimProcesses empties the working set of this process and its WebView2 processes
// (SetProcessWorkingSetSizeEx with -1, -1, the documented way to do it). Windows moves the
// pages to the standby list or the page file and brings them back on first use.
func trimProcesses() error {
	procs, err := snapshotProcesses()
	if err != nil {
		return err
	}
	self := uint32(os.Getpid())
	var errs []error
	for _, pid := range append([]uint32{self}, webviewDescendants(procs, self)...) {
		if err := emptyWorkingSet(pid); err != nil {
			errs = append(errs, fmt.Errorf("pid %d: %w", pid, err))
		}
	}
	return errors.Join(errs...)
}

func emptyWorkingSet(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err // e.g. the process just exited
	}
	defer windows.CloseHandle(h)
	all := ^uintptr(0) // (SIZE_T)-1
	return windows.SetProcessWorkingSetSizeEx(h, all, all, 0)
}

func snapshotProcesses() ([]procEntry, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("process snapshot: %w", err)
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []procEntry
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out = append(out, procEntry{pid: e.ProcessID, parent: e.ParentProcessID, exe: windows.UTF16ToString(e.ExeFile[:])})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("process snapshot: %w", err)
	}
	return out, nil
}
