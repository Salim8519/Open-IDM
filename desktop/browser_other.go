//go:build !windows

package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

var browserApps = map[string]string{"chrome": "Google Chrome", "edge": "Microsoft Edge", "brave": "Brave Browser"}
var browserBins = map[string]string{"chrome": "google-chrome", "edge": "microsoft-edge", "brave": "brave-browser"}

// openBrowserPage opens an extensions page in the chosen browser. Both arguments come from
// fixed tables, never from the caller. Some browsers ignore internal pages sent from
// outside while they are running; the UI also shows the address to paste.
func openBrowserPage(browser, page string) error {
	if runtime.GOOS == "darwin" {
		app, ok := browserApps[browser]
		if !ok {
			return fmt.Errorf("unknown browser %q", browser)
		}
		if out, err := exec.Command("open", "-a", app, page).CombinedOutput(); err != nil {
			return fmt.Errorf("could not open %s: %v %s", app, err, out)
		}
		return nil
	}
	bin, ok := browserBins[browser]
	if !ok {
		return fmt.Errorf("unknown browser %q", browser)
	}
	return startHidden(exec.Command(bin, page))
}
