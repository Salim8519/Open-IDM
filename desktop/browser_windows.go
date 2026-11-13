//go:build windows

package main

import (
	"fmt"
	"os/exec"
)

// Program names registered under "App Paths", which `start` resolves.
var browserPrograms = map[string]string{"chrome": "chrome", "edge": "msedge", "brave": "brave"}

// openBrowserPage opens an extensions page in the chosen browser. Both arguments come from
// fixed tables in the api package and here, never from the caller.
func openBrowserPage(browser, page string) error {
	prog, ok := browserPrograms[browser]
	if !ok {
		return fmt.Errorf("unknown browser %q", browser)
	}
	cmd := exec.Command("cmd", "/c", "start", "", prog, page)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not open %s: %v %s", browser, err, out)
	}
	return nil
}
