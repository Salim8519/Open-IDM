//go:build !darwin

package main

import (
	_ "embed"
	"runtime"

	"fyne.io/systray"
)

// On Windows and Linux the tray runs its own message loop on a dedicated OS thread, next
// to the Wails window loop. (Linux uses the StatusNotifierItem D-Bus protocol; desktops
// without a tray host simply show no icon, and closing then quits as usual.)
func runTray(onReady func()) {
	go func() {
		runtime.LockOSThread()
		systray.Run(onReady, nil)
	}()
}

func setTrayIcon() {
	if runtime.GOOS == "windows" {
		systray.SetIcon(trayICO)
	} else {
		systray.SetIcon(trayPNG)
	}
}

//go:embed build/windows/icon.ico
var trayICO []byte

//go:embed build/tray.png
var trayPNG []byte
