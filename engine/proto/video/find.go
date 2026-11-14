package video

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrNoYtDlp means yt-dlp is neither configured, bundled next to Peregrine, nor on PATH.
var ErrNoYtDlp = errors.New("video downloads need yt-dlp, which was not found next to Peregrine or on PATH")

// tools finds yt-dlp (required) and ffmpeg (optional; without it only single-file formats
// are downloaded). It runs on every Run, so installing either later needs no restart.
func (p *Protocol) tools() (ytdlp, ffmpeg string, err error) {
	ytdlp, err = p.find(p.opts.YtDlpPath, "yt-dlp")
	if err != nil {
		return "", "", err
	}
	if ytdlp == "" {
		return "", "", ErrNoYtDlp
	}
	ffmpeg, err = p.find(p.opts.FFmpegPath, "ffmpeg")
	if err != nil {
		return "", "", err
	}
	return ytdlp, ffmpeg, nil
}

// find returns the explicit path if set (it must exist), else the tool next to the running
// executable (or in the macOS app's Resources), else on PATH; "" if not found.
func (p *Protocol) find(explicit, tool string) (string, error) {
	if explicit != "" {
		if info, err := os.Stat(explicit); err != nil || info.IsDir() {
			return "", fmt.Errorf("%s not found at %s", tool, explicit)
		}
		return explicit, nil
	}
	names := []string{tool}
	switch runtime.GOOS {
	case "windows":
		names = []string{tool + ".exe"}
	case "darwin":
		if tool == "yt-dlp" {
			names = append(names, "yt-dlp_macos") // the name of the official macOS build
		}
	}
	var dirs []string
	if dir, err := p.exeDir(); err == nil && dir != "" {
		dirs = append(dirs, dir, filepath.Join(dir, "..", "Resources"))
	}
	if c := firstIn(dirs, names); c != "" {
		return c, nil
	}
	for _, n := range names {
		if c, err := exec.LookPath(n); err == nil {
			return c, nil
		}
	}
	if runtime.GOOS == "darwin" {
		// Apps started from Finder get a minimal PATH without Homebrew.
		return firstIn([]string{"/opt/homebrew/bin", "/usr/local/bin"}, names), nil
	}
	return "", nil
}

func firstIn(dirs, names []string) string {
	for _, d := range dirs {
		for _, n := range names {
			c := filepath.Join(d, n)
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				return filepath.Clean(c)
			}
		}
	}
	return ""
}

func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}
