package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
void peregrineRunTrayOnMain(void);
*/
import "C"

import (
	_ "embed"

	"fyne.io/systray"
)

// On macOS Wails owns the main thread and NSApp, so the menu-bar item uses systray's
// external-loop mode and is created on the main queue (tray_darwin.m).
var trayStart func()

func runTray(onReady func()) {
	start, _ := systray.RunWithExternalLoop(onReady, nil)
	trayStart = start
	C.peregrineRunTrayOnMain()
}

//export peregrineTrayStart
func peregrineTrayStart() {
	if trayStart != nil {
		trayStart()
	}
}

func setTrayIcon() { systray.SetIcon(trayPNG) }

//go:embed build/tray.png
var trayPNG []byte
