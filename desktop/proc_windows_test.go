//go:build windows

package main

import "testing"

// Regression (0.4.1): "Show in folder" opened an invisible Explorer window because the
// command was started with HideWindow, which Explorer passes on to its window.
func TestRevealAndOpenStartVisible(t *testing.T) {
	r := revealCmd(`C:\Users\me\Downloads\a b.zip`)
	if r.SysProcAttr == nil || r.SysProcAttr.HideWindow {
		t.Fatal("revealCmd must not hide the Explorer window")
	}
	if want := `explorer /select,"C:\Users\me\Downloads\a b.zip"`; r.SysProcAttr.CmdLine != want {
		t.Fatalf("CmdLine = %q, want %q", r.SysProcAttr.CmdLine, want)
	}
	o := openCmd(`C:\Users\me\Downloads\a.pdf`)
	if o.SysProcAttr != nil && o.SysProcAttr.HideWindow {
		t.Fatal("openCmd must not hide the opened app")
	}
}
