//go:build windows || darwin || linux

package autostart

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetAndRemove(t *testing.T) {
	useTestLocation(t)
	e := Entry{Name: "PeregrineAutostartTest", Exe: filepath.Join(t.TempDir(), "Pere grine"), Args: []string{"--hidden"}}
	if on, err := Enabled(e); err != nil || on {
		t.Fatalf("before: on=%v err=%v", on, err)
	}
	if err := Set(e, true); err != nil {
		t.Fatal(err)
	}
	if on, err := Enabled(e); err != nil || !on {
		t.Fatalf("after enable: on=%v err=%v", on, err)
	}
	if err := Set(e, true); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := Set(e, false); err != nil {
		t.Fatal(err)
	}
	if on, err := Enabled(e); err != nil || on {
		t.Fatalf("after disable: on=%v err=%v", on, err)
	}
	if err := Set(e, false); err != nil {
		t.Fatalf("removing twice: %v", err)
	}
}

func TestRejectsBadNames(t *testing.T) {
	useTestLocation(t)
	for _, n := range []string{"", "../evil", `a\b`, "x:y", ".."} {
		if err := Set(Entry{Name: n, Exe: os.Args[0]}, true); err == nil {
			t.Errorf("name %q accepted", n)
		}
	}
}
