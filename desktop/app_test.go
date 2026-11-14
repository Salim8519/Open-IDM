package main

import (
	"os"
	"path/filepath"
	"testing"

	ptorrent "github.com/Salim8519/Peregrine/engine/proto/torrent"
)

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

// A .torrent file double-clicked in Explorer/Finder arrives as a plain path.
func TestURLArgTorrentFile(t *testing.T) {
	tf := filepath.Join(t.TempDir(), "ubuntu.torrent")
	if err := os.WriteFile(tf, []byte("d4:infod4:name1:xee"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := urlArg([]string{"--hidden", tf}), ptorrent.FileURL(tf); got != want {
		t.Fatalf("urlArg(torrent) = %q, want %q", got, want)
	}
	// Missing files and other file types are ignored.
	for _, a := range []string{filepath.Join(t.TempDir(), "gone.torrent"), filepath.Join(t.TempDir())} {
		if got := urlArg([]string{a}); got != "" {
			t.Errorf("urlArg(%q) = %q", a, got)
		}
	}
}
