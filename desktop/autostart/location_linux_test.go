package autostart

import (
	"strings"
	"testing"
)

func useTestLocation(t *testing.T) {
	dir := t.TempDir()
	old := autostartDir
	autostartDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { autostartDir = old })
}

func TestExecQuoting(t *testing.T) {
	got := string(entry(Entry{Name: "peregrine", Exe: "/opt/my app/peregrine", Args: []string{"--hidden"}}))
	if !strings.Contains(got, `Exec="/opt/my app/peregrine" --hidden`) {
		t.Fatalf("bad Exec line:\n%s", got)
	}
	if got := execQuote(`a"$b`); got != `"a\\"\\$b"` {
		t.Fatalf("execQuote = %s", got)
	}
}
