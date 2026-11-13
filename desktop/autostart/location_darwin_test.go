package autostart

import (
	"encoding/xml"
	"testing"
)

func useTestLocation(t *testing.T) {
	dir := t.TempDir()
	old := agentsDir
	agentsDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { agentsDir = old })
}

func TestPlistIsValidXML(t *testing.T) {
	b := plist(Entry{Name: "com.peregrine.app", Exe: "/Applications/A&B <x>.app/Contents/MacOS/Peregrine", Args: []string{"--hidden"}})
	var v any
	if err := xml.Unmarshal(b, &v); err != nil {
		t.Fatalf("invalid plist: %v\n%s", err, b)
	}
}
