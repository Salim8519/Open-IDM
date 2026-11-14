package stream

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

// needFFmpeg skips the test when ffmpeg is not installed.
func needFFmpeg(t *testing.T) string {
	t.Helper()
	ff := findFFmpeg("")
	if ff == "" {
		t.Skip("ffmpeg not installed")
	}
	return ff
}

// generate runs ffmpeg to make 4 s of test video (MPEG-2) + AAC audio into dir.
func generate(t *testing.T, ff, dir string, out ...string) {
	t.Helper()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=4:size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=4",
		"-c:v", "mpeg2video", "-g", "10", "-c:a", "aac", "-b:a", "64k"}
	cmd := exec.CommandContext(context.Background(), ff, append(args, out...)...)
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate media: %v: %s", err, b)
	}
}

// probe returns ffmpeg's description of a media file's streams.
func probe(t *testing.T, ff, file string) string {
	t.Helper()
	b, _ := exec.CommandContext(context.Background(), ff, "-hide_banner", "-i", file).CombinedOutput()
	return string(b)
}

func checkMP4(t *testing.T, ff string, d engine.Download, name string) {
	t.Helper()
	if d.Filename != name {
		t.Fatalf("filename = %q, want %q", d.Filename, name)
	}
	b, err := os.ReadFile(d.Path())
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 12 || !bytes.Equal(b[4:8], []byte("ftyp")) {
		t.Fatal("output is not an MP4")
	}
	info := probe(t, ff, d.Path())
	if !strings.Contains(info, "Video: mpeg2video") || !strings.Contains(info, "Audio: aac") {
		t.Fatalf("streams missing:\n%s", info)
	}
	if _, err := os.Stat(strings.TrimSuffix(d.PartPath(), engine.PartSuffix) + ".peregrine.tmp"); !os.IsNotExist(err) {
		t.Fatal("temp folder left behind")
	}
}

func TestFFmpegHLSRemux(t *testing.T) {
	ff := needFFmpeg(t)
	dir := t.TempDir()
	generate(t, ff, dir, "-f", "hls", "-hls_time", "1", "-hls_list_size", "0",
		"-hls_segment_filename", "s%d.ts", "clip.m3u8")
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	m := newManager(t, testProto(NewHLS(Options{}), true))
	d, err := m.Add(engine.Request{URL: srv.URL + "/clip.m3u8"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, m, d.ID, engine.StatusCompleted, 60*time.Second)
	checkMP4(t, ff, done, "clip.mp4")
}

// TestFFmpegHLSAudioRendition: a master playlist whose variant takes its audio from a
// separate EXT-X-MEDIA rendition; both are downloaded and muxed.
func TestFFmpegHLSAudioRendition(t *testing.T) {
	ff := needFFmpeg(t)
	dir := t.TempDir()
	generate(t, ff, dir, "-map", "0:v", "-map", "1:a", "-f", "hls", "-hls_time", "1", "-hls_list_size", "0",
		"-var_stream_map", "v:0,agroup:aud a:0,agroup:aud,default:yes",
		"-master_pl_name", "master.m3u8", "-hls_segment_filename", "s_%v_%d.ts", "out_%v.m3u8")
	master, err := os.ReadFile(filepath.Join(dir, "master.m3u8"))
	if err != nil || !strings.Contains(string(master), "TYPE=AUDIO") {
		t.Skipf("this ffmpeg did not write an audio rendition: %v\n%s", err, master)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	m := newManager(t, testProto(NewHLS(Options{}), true))
	d, err := m.Add(engine.Request{URL: srv.URL + "/master.m3u8"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, m, d.ID, engine.StatusCompleted, 60*time.Second)
	checkMP4(t, ff, done, "master.mp4")
}

func TestFFmpegDASHMux(t *testing.T) {
	ff := needFFmpeg(t)
	dir := t.TempDir()
	generate(t, ff, dir, "-map", "0:v", "-map", "1:a", "-f", "dash", "-seg_duration", "1",
		"-use_template", "1", "-use_timeline", "1", "-adaptation_sets", "id=0,streams=v id=1,streams=a", "show.mpd")
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()
	m := newManager(t, testProto(NewDASH(Options{}), true))
	d, err := m.Add(engine.Request{URL: srv.URL + "/show.mpd"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, m, d.ID, engine.StatusCompleted, 60*time.Second)
	checkMP4(t, ff, done, "show.mp4")
}
