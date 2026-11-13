package main

import (
	"fmt"
	"os/exec"

	"golang.org/x/sys/windows"
)

func sleepNow() error {
	// SetSuspendState(hibernate=FALSE, force=FALSE, disableWakeEvent=FALSE)
	proc := windows.NewLazySystemDLL("powrprof.dll").NewProc("SetSuspendState")
	if err := proc.Find(); err != nil {
		return fmt.Errorf("sleep: %w", err)
	}
	if r, _, err := proc.Call(0, 0, 0); r == 0 {
		return fmt.Errorf("sleep: %w", err)
	}
	return nil
}

func shutdownNow() error {
	// No /f: apps with unsaved work may still ask the user.
	cmd := exec.Command("shutdown", "/s", "/t", "0")
	hideWindow(cmd)
	return cmd.Run()
}
