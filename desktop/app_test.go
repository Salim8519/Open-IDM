package main

import "testing"

func TestURLArg(t *testing.T) {
	cases := map[string][]string{
		"https://example.com/a.zip":   {"--hidden", "https://example.com/a.zip"},
		"magnet:?xt=urn:btih:abc":     {"magnet:?xt=urn:btih:abc"},
		"ftp://ftp.example.com/f.iso": {"ftp://ftp.example.com/f.iso"},
		"ftps://h/f":                  {"ftps://h/f"},
		"":                            {"--hidden", "file:///etc/passwd", "javascript:alert(1)", `C:\x.exe`},
	}
	for want, args := range cases {
		if got := urlArg(args); got != want {
			t.Errorf("urlArg(%q) = %q, want %q", args, got, want)
		}
	}
}
