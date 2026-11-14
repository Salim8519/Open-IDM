//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}

// openPath opens a file with its default app.
func openPath(path string) error {
	return startVisible(openCmd(path))
}

// revealPath opens Explorer with the file selected.
func revealPath(path string) error {
	return startVisible(revealCmd(path))
}

// openCmd and revealCmd start GUI programs, so they must not get HideWindow: Explorer and
// url.dll pass the start-up show state on to the window they open, and a hidden flag
// opened an invisible Explorer window on every "Show in folder" click (fixed in 0.4.1).
// Neither has a console, so nothing flashes.
func openCmd(path string) *exec.Cmd {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
}

func revealCmd(path string) *exec.Cmd {
	// explorer parses its own command line: /select,"<path>" must not be re-quoted by Go.
	cmd := exec.Command("explorer")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer /select,"` + path + `"`}
	return cmd
}

func startVisible(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
