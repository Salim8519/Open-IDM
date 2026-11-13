// Command Peregrine is the desktop app: a Wails window around the engine and RPC API.
package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "Peregrine",
		Width:     1100,
		Height:    700,
		MinWidth:  820,
		MinHeight: 520,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 11, G: 16, B: 32, A: 255},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.life.beforeClose,
		// --hidden comes from launch at login: start in the tray / menu bar.
		StartHidden: launchedHidden(),
		// macOS: the close button hides the app like other Mac apps (the Dock icon brings it
		// back, Cmd+Q quits). Windows/Linux decide in beforeClose (close to tray setting).
		HideWindowOnClose: runtime.GOOS == "darwin",
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.peregrine.app",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Bind: []any{app},
		Windows: &windows.Options{
			Theme: windows.SystemDefault,
		},
		Mac: &mac.Options{
			OnUrlOpen: app.onURLOpen, // magnet: links (registered in wails.json "protocols")
			About: &mac.AboutInfo{
				Title:   "Peregrine",
				Message: "Free, open-source download manager",
				Icon:    icon,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
