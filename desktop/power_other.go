//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

func sleepNow() error {
	if runtime.GOOS == "darwin" {
		return exec.Command("pmset", "sleepnow").Run()
	}
	return exec.Command("systemctl", "suspend").Run()
}

func shutdownNow() error {
	if runtime.GOOS == "darwin" {
		// Asks running apps to quit like the Apple menu does (may need Automation permission).
		return exec.Command("osascript", "-e", `tell application "System Events" to shut down`).Run()
	}
	return exec.Command("systemctl", "poweroff").Run()
}
