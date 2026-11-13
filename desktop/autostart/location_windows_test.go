package autostart

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// useTestLocation points the Run key at a throwaway key so tests never touch real startup.
func useTestLocation(t *testing.T) {
	old := runKey
	runKey = `Software\PeregrineTest\Run`
	t.Cleanup(func() {
		runKey = old
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\PeregrineTest\Run`)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\PeregrineTest`)
	})
}

func TestCommandQuoting(t *testing.T) {
	got := command(Entry{Exe: `C:\Program Files\Peregrine\Peregrine.exe`, Args: []string{"--hidden"}})
	if want := `"C:\Program Files\Peregrine\Peregrine.exe" --hidden`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
