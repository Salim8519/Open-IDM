// Package autostart registers an app to start when the user signs in, per user, without
// admin rights: the HKCU Run key on Windows, a LaunchAgent on macOS and an XDG autostart
// entry on Linux.
package autostart

import (
	"fmt"
	"strings"
)

// Entry describes what to start at login.
type Entry struct {
	Name string   // identifier: registry value name, LaunchAgent label, .desktop file name
	Exe  string   // absolute path of the executable
	Args []string // extra arguments, e.g. "--hidden"
}

func (e Entry) check() error {
	if e.Name == "" || strings.ContainsAny(e.Name, `/\:*?"<>|`) || strings.Contains(e.Name, "..") {
		return fmt.Errorf("autostart: invalid name %q", e.Name)
	}
	if e.Exe == "" {
		return fmt.Errorf("autostart: missing executable path")
	}
	return nil
}

// Set enables (on=true) or removes the entry. Removing a missing entry is not an error.
// Enabling again overwrites the entry, which also repairs it after the app moved.
func Set(e Entry, on bool) error {
	if err := e.check(); err != nil {
		return err
	}
	if on {
		return enable(e)
	}
	return disable(e)
}

// Enabled reports whether an entry with this name exists.
func Enabled(e Entry) (bool, error) {
	if err := e.check(); err != nil {
		return false, err
	}
	return enabled(e)
}
