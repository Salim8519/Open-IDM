//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

func hideWindow(cmd *exec.Cmd) {}

func openPath(path string) error {
	if runtime.GOOS == "darwin" {
		return startHidden(exec.Command("open", path))
	}
	return startHidden(exec.Command("xdg-open", path))
}

func revealPath(path string) error {
	if runtime.GOOS == "darwin" {
		return startHidden(exec.Command("open", "-R", path))
	}
	return startHidden(exec.Command("xdg-open", filepath.Dir(path)))
}
