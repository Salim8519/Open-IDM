package autostart

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// autostartDir returns $XDG_CONFIG_HOME/autostart (default ~/.config/autostart).
var autostartDir = func() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "autostart"), nil
}

func desktopPath(e Entry) (string, error) {
	dir, err := autostartDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, e.Name+".desktop"), nil
}

// execQuote quotes one argument for the Exec key (Desktop Entry Specification): inside
// double quotes, ", `, $ and \ are escaped with a backslash, and the string is then
// escaped again as a desktop-entry string value. A literal % is written as %%.
func execQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\"'\\><~|&;$*?#()`%") {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '"', '`', '$':
			b.WriteString(`\\`)
			b.WriteRune(r)
		case '\\':
			b.WriteString(`\\\\`)
		case '%':
			b.WriteString("%%")
		case '\n':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return `"` + b.String() + `"`
}

func entry(e Entry) []byte {
	parts := []string{execQuote(e.Exe)}
	for _, a := range e.Args {
		parts = append(parts, execQuote(a))
	}
	return []byte("[Desktop Entry]\nType=Application\nName=" + e.Name +
		"\nExec=" + strings.Join(parts, " ") +
		"\nX-GNOME-Autostart-enabled=true\nTerminal=false\n")
}

func enable(e Entry) error {
	p, err := desktopPath(e)
	if err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	if err := os.WriteFile(p, entry(e), 0o644); err != nil {
		return fmt.Errorf("autostart: write entry: %w", err)
	}
	return nil
}

func disable(e Entry) error {
	p, err := desktopPath(e)
	if err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("autostart: remove entry: %w", err)
	}
	return nil
}

func enabled(e Entry) (bool, error) {
	p, err := desktopPath(e)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
