package stream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrFFmpegMissing is returned when a stream needs ffmpeg (separate video and audio
// tracks) and none is installed.
var ErrFFmpegMissing = errors.New("this stream has separate video and audio tracks and needs ffmpeg to combine them into an .mp4; " +
	"install ffmpeg (from ffmpeg.org, or with winget / Homebrew) or put ffmpeg next to Peregrine, then retry")

// findFFmpeg looks for ffmpeg: the configured path, else next to the running executable,
// else on PATH. It returns "" when there is none.
func findFFmpeg(configured string) string {
	if configured != "" {
		if isFile(configured) {
			return configured
		}
		return ""
	}
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	if exe, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(exe), name); isFile(p) {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil { // LookPath never returns ./ffmpeg (ErrDot)
		return p
	}
	return ""
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// remux copies the streams into an MP4 at out without re-encoding: video (and audio, if
// muxed in) from the first input, audio from the second when there is one. Paths are
// separate arguments; no shell is involved.
func remux(ctx context.Context, ffmpeg string, inputs []string, out string) error {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}
	for _, in := range inputs {
		args = append(args, "-i", in)
	}
	if len(inputs) == 1 {
		args = append(args, "-map", "0:v?", "-map", "0:a?")
	} else {
		args = append(args, "-map", "0:v?", "-map", "1:a?")
	}
	args = append(args, "-c", "copy", "-f", "mp4", out)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 400 {
			msg = "..." + msg[len(msg)-400:]
		}
		return fmt.Errorf("ffmpeg: %w: %s", err, msg)
	}
	return nil
}
