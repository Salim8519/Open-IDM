package autostart

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// runKey is the per-user Run key; tests point it elsewhere.
var runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func command(e Entry) string {
	parts := []string{quote(e.Exe)}
	for _, a := range e.Args {
		parts = append(parts, quote(a))
	}
	return strings.Join(parts, " ")
}

func quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func enable(e Entry) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("autostart: open Run key: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(e.Name, command(e)); err != nil {
		return fmt.Errorf("autostart: write Run value: %w", err)
	}
	return nil
}

func disable(e Entry) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("autostart: open Run key: %w", err)
	}
	defer k.Close()
	if err := k.DeleteValue(e.Name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("autostart: delete Run value: %w", err)
	}
	return nil
}

func enabled(e Entry) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer k.Close()
	_, _, err = k.GetStringValue(e.Name)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
