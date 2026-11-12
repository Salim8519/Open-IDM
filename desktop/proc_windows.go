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

func openPath(path string) error {
	return startHidden(exec.Command("rundll32", "url.dll,FileProtocolHandler", path))
}

func revealPath(path string) error {
	// explorer parses its own command line: /select,"<path>" must not be re-quoted by Go.
	cmd := exec.Command("explorer")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer /select,"` + path + `"`}
	return startHidden(cmd)
}
