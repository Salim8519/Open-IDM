package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProto "downloads" fake://<name>?size: writes size bytes in 10 KB chunks, slowly,
// saving its offset as resume state. fake-dir:// produces a folder with two files.
type fakeProto struct{ runs atomic.Int32 }

func (*fakeProto) Kind() string { return "fake" }
func (*fakeProto) Match(u string) bool {
	return strings.HasPrefix(u, "fake://") || strings.HasPrefix(u, "fake-dir://")
}

type fakeState struct{ Offset int64 }

func (f *fakeProto) Run(ctx context.Context, j *Job) error {
	f.runs.Add(1)
	if strings.HasPrefix(j.URL, "fake-dir://") {
		if err := j.SetName("album"); err != nil {
			return err
		}
		if err := j.SetDirectory(); err != nil {
			return err
		}
		for _, n := range []string{"a.txt", "b.txt"} {
			if err := os.WriteFile(filepath.Join(j.PartPath, n), []byte(n), 0o644); err != nil {
				return err
			}
			_ = j.Received(ctx, len(n))
		}
		return nil
	}
	const size = 200_000
	j.SetSize(size)
	j.SetResumable(true)
	if j.Fresh {
		if err := j.SetName("movie.bin"); err != nil {
			return err
		}
	}
	var st fakeState
	if len(j.State) > 0 {
		_ = json.Unmarshal(j.State, &st)
	}
	file, err := os.OpenFile(j.PartPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	chunk := make([]byte, 10_000)
	for st.Offset < size {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Millisecond):
		}
		if _, err := file.WriteAt(chunk, st.Offset); err != nil {
			return err
		}
		st.Offset += int64(len(chunk))
		if err := j.Received(ctx, len(chunk)); err != nil {
			return err
		}
		_ = j.SaveState(st)
	}
	return nil
}

func TestProtocolLifecycle(t *testing.T) {
	m := newTestManager(t, ":memory:", nil)
	fp := &fakeProto{}
	m.RegisterProtocol(fp)

	d, err := m.Add(Request{URL: "fake://whatever"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != "fake" {
		t.Fatalf("kind = %q", d.Kind)
	}
	// Pause in the middle, then resume: the second run continues from the saved offset.
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := m.Get(d.ID)
		if got.Downloaded >= 50_000 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no progress")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := m.Pause(d.ID); err != nil {
		t.Fatal(err)
	}
	paused, _ := m.Get(d.ID)
	if paused.Status != StatusPaused || paused.Downloaded == 0 || len(paused.ProtoState) == 0 {
		t.Fatalf("after pause: %+v", paused)
	}
	if err := m.Resume(d.ID); err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, d.ID, StatusCompleted, 10*time.Second)
	if fp.runs.Load() != 2 {
		t.Fatalf("runs = %d, want 2", fp.runs.Load())
	}
	if done.Filename != "movie.bin" || done.Downloaded != 200_000 || done.SHA256 == "" {
		t.Fatalf("completed: name=%s downloaded=%d sha=%q", done.Filename, done.Downloaded, done.SHA256)
	}
	if info, err := os.Stat(done.Path()); err != nil || info.Size() != 200_000 {
		t.Fatalf("final file: %v %v", info, err)
	}
	if exists(done.PartPath()) {
		t.Fatal("part file left behind")
	}
}

func TestProtocolFolderOutput(t *testing.T) {
	m := newTestManager(t, ":memory:", nil)
	m.RegisterProtocol(&fakeProto{})
	d, err := m.Add(Request{URL: "fake-dir://x"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, d.ID, StatusCompleted, 10*time.Second)
	if !done.IsDir || done.Filename != "album" {
		t.Fatalf("folder download: %+v", done)
	}
	for _, n := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(done.Path(), n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Remove(d.ID, true); err != nil {
		t.Fatal(err)
	}
	if exists(done.Path()) {
		t.Fatal("remove + delete file left the folder")
	}
}

func TestKindSelection(t *testing.T) {
	m := newTestManager(t, ":memory:", nil)
	if _, err := m.Add(Request{URL: "fake://x"}); err == nil {
		t.Fatal("unregistered scheme accepted")
	}
	if _, err := m.Add(Request{URL: "https://example.com/a", Kind: "torrent", Paused: true}); err == nil {
		t.Fatal("unregistered explicit kind accepted")
	}
	m.RegisterProtocol(&fakeProto{})
	if d, err := m.Add(Request{URL: "https://example.com/a.zip", Paused: true}); err != nil || d.Kind != "" {
		t.Fatalf("http download got kind %q (%v)", d.Kind, err)
	}
	if d, err := m.Add(Request{URL: "https://example.com/page", Kind: "fake", Paused: true}); err != nil || d.Kind != "fake" {
		t.Fatalf("explicit kind: %q (%v)", d.Kind, err)
	}
	if got := m.Kinds(); len(got) != 1 || got[0] != "fake" {
		t.Fatalf("Kinds() = %v", got)
	}
}
