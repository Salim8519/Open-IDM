package autostart

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// agentsDir returns ~/Library/LaunchAgents; tests replace it.
var agentsDir = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

func plistPath(e Entry) (string, error) {
	dir, err := agentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, e.Name+".plist"), nil
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func plist(e Entry) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + esc(e.Name) + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range append([]string{e.Exe}, e.Args...) {
		b.WriteString("\t\t<string>" + esc(a) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`)
	return b.Bytes()
}

func enable(e Entry) error {
	p, err := plistPath(e)
	if err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	if err := os.WriteFile(p, plist(e), 0o644); err != nil {
		return fmt.Errorf("autostart: write LaunchAgent: %w", err)
	}
	return nil
}

func disable(e Entry) error {
	p, err := plistPath(e)
	if err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("autostart: remove LaunchAgent: %w", err)
	}
	return nil
}

func enabled(e Entry) (bool, error) {
	p, err := plistPath(e)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
