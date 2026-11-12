package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// builtinExtensionIDs are the extension IDs compiled into the host.
//
// bcnjpgddmjlffmcfgnfjaadkdjphdhfd is the unpacked/development ID, fixed by the public
// key in extension/manifest.json ("key"). Store builds get their own IDs from the Chrome
// Web Store / Edge Add-ons; add them here (or, until a release, to
// <config>/Peregrine/allowed-extensions.json).
var builtinExtensionIDs = []string{
	"bcnjpgddmjlffmcfgnfjaadkdjphdhfd",
}

const extraIDsFile = "allowed-extensions.json"

var extIDRe = regexp.MustCompile(`^[a-p]{32}$`)

// allowedIDs returns the compiled-in IDs plus valid IDs from the optional extra file in
// configDir. The extra file is either a JSON array of IDs or {"ids": [...]}.
func allowedIDs(configDir string) []string {
	ids := append([]string(nil), builtinExtensionIDs...)
	if configDir == "" {
		return ids
	}
	b, err := os.ReadFile(filepath.Join(configDir, extraIDsFile))
	if err != nil {
		return ids
	}
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		var obj struct {
			IDs []string `json:"ids"`
		}
		if json.Unmarshal(b, &obj) != nil {
			return ids
		}
		list = obj.IDs
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range list {
		id = strings.TrimSpace(id)
		if extIDRe.MatchString(id) && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// callerOrigin finds the chrome-extension:// origin Chrome passes on the command line.
// Windows adds "--parent-window=N" after it; other platforms pass only the origin.
func callerOrigin(args []string) string {
	for _, a := range args {
		if strings.HasPrefix(a, "chrome-extension://") {
			return a
		}
	}
	return ""
}

// originAllowed reports whether origin is exactly chrome-extension://<allowed id>/.
func originAllowed(origin string, ids []string) bool {
	id, ok := strings.CutPrefix(origin, "chrome-extension://")
	if !ok {
		return false
	}
	id = strings.TrimSuffix(id, "/")
	if !extIDRe.MatchString(id) {
		return false
	}
	for _, a := range ids {
		if a == id {
			return true
		}
	}
	return false
}

func originsFor(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, "chrome-extension://"+id+"/")
	}
	return out
}
