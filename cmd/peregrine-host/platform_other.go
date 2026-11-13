//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"runtime"
	"syscall"
)

var appExeName = func() string {
	if runtime.GOOS == "darwin" {
		return "Peregrine" // Peregrine.app/Contents/MacOS/Peregrine, beside the host
	}
	return "peregrine"
}()

var errNoRegistry = errors.New("registry is only available on Windows")

func setRegistry(string, string) error { return errNoRegistry }
func deleteRegistry(string) error      { return errNoRegistry }

// startDetached starts the app in its own session so it outlives the host.
func startDetached(path string) error {
	cmd := exec.Command(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// allowForeground is a Windows-only concern; macOS/Linux apps may activate themselves.
func allowForeground() {}
