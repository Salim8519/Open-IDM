package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Strings shown by Go code outside the web UI: tray menu and system notifications, one JSON
// file per language in locales/. The web UI has its own translations in frontend/src/i18n.
//
//go:embed locales/*.json
var localeFS embed.FS

var (
	localesOnce sync.Once
	locales     map[string]map[string]string
)

func loadLocales() {
	locales = map[string]map[string]string{}
	entries, _ := localeFS.ReadDir("locales")
	for _, e := range entries {
		b, err := localeFS.ReadFile("locales/" + e.Name())
		if err != nil {
			continue
		}
		var m map[string]string
		if json.Unmarshal(b, &m) == nil {
			locales[strings.TrimSuffix(e.Name(), ".json")] = m
		}
	}
}

// uiStrings looks up keys in one language, falling back to English.
type uiStrings struct{ lang string }

func stringsFor(lang string) uiStrings {
	localesOnce.Do(loadLocales)
	return uiStrings{lang: lang}
}

// get returns the text for key with {name} placeholders filled from vars (name, value, ...).
func (s uiStrings) get(key string, vars ...any) string {
	txt, ok := locales[s.lang][key]
	if !ok || txt == "" {
		txt = locales["en"][key]
	}
	for i := 0; i+1 < len(vars); i += 2 {
		txt = strings.ReplaceAll(txt, "{"+fmt.Sprint(vars[i])+"}", fmt.Sprint(vars[i+1]))
	}
	return txt
}

// humanSpeed formats bytes/s like the UI does (KB/MB/GB per second, base 1024).
func humanSpeed(bps int64) string {
	const unit = 1024
	if bps < unit {
		return fmt.Sprintf("%d B/s", bps)
	}
	div, exp := int64(unit), 0
	for n := bps / unit; n >= unit && exp < 3; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB/s", float64(bps)/float64(div), "KMGT"[exp])
}
