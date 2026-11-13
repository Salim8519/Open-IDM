package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Salim8519/Peregrine/internal/testsrv"
)

var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "peregrine-cli")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "peregrine")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func sha(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func run(t *testing.T, env []string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.String(), err
}

func TestCLIGet(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	out := t.TempDir()
	env := []string{"PEREGRINE_HOME=" + t.TempDir()}
	log, err := run(t, env, "get", srv.FileURL("size=2000000&name=hello.bin"), "-o", out, "-c", "4")
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	if got := sha(t, filepath.Join(out, "hello.bin")); got != testsrv.SHA256(2000000, 1) {
		t.Fatal("content mismatch")
	}
	if !strings.Contains(log, testsrv.SHA256(2000000, 1)) {
		t.Errorf("SHA-256 not printed:\n%s", log)
	}
}

// TestCLIKillAndResume kills the process mid-download (like a crash or power cut) and
// checks that running the same command again resumes and produces a correct file.
func TestCLIKillAndResume(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	out := t.TempDir()
	env := []string{"PEREGRINE_HOME=" + t.TempDir()}
	url := srv.FileURL("size=6000000&name=big.bin&throttle=300000")

	cmd := exec.Command(bin, "get", url, "-o", out, "-c", "4", "-q")
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	_ = cmd.Process.Kill() // hard kill: no cleanup runs
	_ = cmd.Wait()

	part := filepath.Join(out, "big.bin.peregrine.part")
	if _, err := os.Stat(part); err != nil {
		t.Fatalf("expected a part file after kill: %v", err)
	}
	log, err := run(t, env, "get", url, "-o", out)
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	if !strings.Contains(log, "Resuming") {
		t.Errorf("did not resume:\n%s", log)
	}
	if got := sha(t, filepath.Join(out, "big.bin")); got != testsrv.SHA256(6000000, 1) {
		t.Fatal("content mismatch after kill + resume")
	}
}

func TestCLIErrorExitCode(t *testing.T) {
	srv := testsrv.New()
	defer srv.Close()
	env := []string{"PEREGRINE_HOME=" + t.TempDir()}
	log, err := run(t, env, "get", srv.FileURL("size=10&expire=0"), "-o", t.TempDir(), "-q")
	if err == nil {
		t.Fatalf("expected failure:\n%s", log)
	}
	if !strings.Contains(log, "expired") {
		t.Errorf("unexpected message:\n%s", log)
	}
}

func TestParseRate(t *testing.T) {
	cases := map[string]int64{"": 0, "0": 0, "500K": 512000, "2M": 2 << 20, "1.5M": 1572864, "100": 100, "1MB/s": 1 << 20}
	for in, want := range cases {
		got, err := parseRate(in)
		if err != nil || got != want {
			t.Errorf("parseRate(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
}
