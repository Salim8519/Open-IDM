// Command Peregrine is the desktop app: a Wails window around the engine and RPC API.
package main

import (
	"embed"
	"log"

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
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.peregrine.app",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Bind: []any{app},
		Windows: &windows.Options{
			Theme: windows.SystemDefault,
		},
		Mac: &mac.Options{
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
