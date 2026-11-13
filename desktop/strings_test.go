package main

import (
	"strings"
	"testing"
)

// Every locale must have exactly the English keys and keep its placeholders.
func TestLocalesMatchEnglish(t *testing.T) {
	stringsFor("en")
	en := locales["en"]
	if len(en) == 0 {
		t.Fatal("no English locale")
	}
	for lang, m := range locales {
		for k, v := range en {
			got, ok := m[k]
			if !ok || got == "" {
				t.Errorf("%s: missing %q", lang, k)
				continue
			}
			for _, ph := range []string{"{n}", "{speed}", "{action}", "{secs}"} {
				if strings.Contains(v, ph) != strings.Contains(got, ph) {
					t.Errorf("%s: %q placeholder %s mismatch: %q", lang, k, ph, got)
				}
			}
		}
		for k := range m {
			if _, ok := en[k]; !ok {
				t.Errorf("%s: unknown key %q", lang, k)
			}
		}
	}
}

func TestStringsFallbackAndVars(t *testing.T) {
	if got := stringsFor("xx").get("active", "n", 2, "speed", "1.0 MB/s"); got != "2 downloading · 1.0 MB/s" {
		t.Fatalf("got %q", got)
	}
}
