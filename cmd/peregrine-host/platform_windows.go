//go:build windows

package main

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const appExeName = "Peregrine.exe"

// setRegistry creates HKCU\<key> with its default value = manifest path.
func setRegistry(key, manifestPath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue("", manifestPath)
}

// deleteRegistry removes HKCU\<key> (a leaf key); a missing key is not an error.
func deleteRegistry(key string) error {
	err := registry.DeleteKey(registry.CURRENT_USER, key)
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	return err
}

// allowForeground lets the app bring its window to the front for the next user-visible
// step (the Add dialog). Windows only lets the foreground process grant this; the host is
// started by the browser the user is clicking in, so it may. Without it the app can only
// flash its taskbar button. The grant ends with the user's next input.
func allowForeground() {
	const asfwAny = 0xFFFFFFFF
	_, _, _ = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow").Call(asfwAny)
}

// startDetached starts the app so it outlives the host. Chrome runs hosts inside a job
// object; try to break away from it, and fall back if the job forbids that.
func startDetached(path string) error {
	flags := uint32(windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP)
	cmd := exec.Command(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags | windows.CREATE_BREAKAWAY_FROM_JOB, HideWindow: false}
	err := cmd.Start()
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "access is denied") {
		cmd = exec.Command(path)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
		err = cmd.Start()
	}
	if err != nil {
		return err
	}
	return cmd.Process.Release()
}
