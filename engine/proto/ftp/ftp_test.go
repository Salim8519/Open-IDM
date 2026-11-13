package ftp

import (
	"bytes"
	"crypto/tls"
	"os"
	"strings"
	"testing"
	"time"

	ftpserver "github.com/fclairamb/ftpserverlib"

	"github.com/Salim8519/Peregrine/engine"
)

func TestMatch(t *testing.T) {
	p := New()
	for u, want := range map[string]bool{
		"ftp://host/file.iso":           true,
		"FTPS://host/file.iso":          true,
		"  ftp://u:p@host:2121/a/b.zip": true,
		"https://host/file.iso":         false,
		"ftpx://host/file":              false,
		"ftp:/host":                     false,
		"":                              false,
	} {
		if got := p.Match(u); got != want {
			t.Errorf("Match(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestParseTarget(t *testing.T) {
	tg, err := parseTarget("ftp://alice:p%40ss@example.com/pub/dir/file.iso;type=i")
	if err != nil {
		t.Fatal(err)
	}
	if tg.user != "alice" || tg.pass != "p@ss" || tg.anonymous || tg.addr != "example.com:21" ||
		tg.path != "pub/dir/file.iso" || tg.implicitTLS {
		t.Fatalf("parsed %+v", tg)
	}
	tg, err = parseTarget("ftps://example.com/%2Fetc/file")
	if err != nil {
		t.Fatal(err)
	}
	if !tg.anonymous || tg.addr != "example.com:990" || tg.path != "/etc/file" || !tg.implicitTLS {
		t.Fatalf("parsed %+v", tg)
	}
	for _, bad := range []string{
		"ftp://host/",                     // a folder
		"ftp://host",                      // nothing
		"ftp:///file",                     // no host
		"ftp://host/a%0D%0ADELE%20x",      // command injection
		"ftp://user:se%0Acret@host/f",     // injection through the password
		"ftp://user:topsecret@ho st/f%zz", // unparsable
	} {
		_, err := parseTarget(bad)
		if err == nil {
			t.Errorf("parseTarget(%q) accepted", bad)
			continue
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "se\ncret") {
			t.Errorf("error leaks the password: %v", err)
		}
	}
}

func TestPassiveReplies(t *testing.T) {
	if p := parseEPSV("Entering Extended Passive Mode (|||6446|)"); p != 6446 {
		t.Errorf("EPSV port = %d", p)
	}
	if p := parsePASV("Entering Passive Mode (10,0,0,1,25,26)"); p != 25*256+26 {
		t.Errorf("PASV port = %d", p)
	}
	if p := parsePASV("Entering Passive Mode 10,0,0,1,25,26"); p != 25*256+26 {
		t.Errorf("PASV without parentheses = %d", p)
	}
	for _, bad := range []string{"(|||0|)", "(|||x|)", "()", "nothing"} {
		if p := parseEPSV(bad); p != -1 {
			t.Errorf("parseEPSV(%q) = %d", bad, p)
		}
	}
	if p := parsePASV("(1,2,3,4,300,1)"); p != -1 {
		t.Errorf("bad PASV accepted: %d", p)
	}
}

func TestFullDownload(t *testing.T) {
	srv := startServer(t) // anonymous, no TLS on offer: AUTH TLS is refused
	want := srv.put("/pub/data.bin", 1<<20+123)
	m := newManager(t, New())

	d, err := m.Add(engine.Request{URL: "ftp://" + srv.addr + "/pub/data.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != Kind {
		t.Fatalf("kind = %q, want %q", d.Kind, Kind)
	}
	done := waitFor(t, m, d.ID, 15*time.Second, engine.StatusCompleted, engine.StatusError)
	if done.Status != engine.StatusCompleted {
		t.Fatalf("download failed: %s", done.Error)
	}
	got, err := os.ReadFile(done.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("content differs: got %d bytes, want %d", len(got), len(want))
	}
	if done.Filename != "data.bin" || done.Size != int64(len(want)) || done.Downloaded != int64(len(want)) || done.SHA256 == "" {
		t.Fatalf("completed: name=%s size=%d downloaded=%d sha=%q", done.Filename, done.Size, done.Downloaded, done.SHA256)
	}
	if !done.Resumable {
		t.Fatal("FTP download not marked resumable")
	}
}

func TestPauseResumeContinuesFromOffset(t *testing.T) {
	srv := startServer(t, withThrottle(3*time.Millisecond)) // ~5 MB/s
	const size = 3 << 20
	want := srv.put("/big.bin", size)
	m := newManager(t, New())

	d, err := m.Add(engine.Request{URL: "ftp://" + srv.addr + "/big.bin"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, _ := m.Get(d.ID)
		if got.Downloaded >= 400<<10 {
			break
		}
		if got.Status == engine.StatusError || time.Now().After(deadline) {
			t.Fatalf("no progress: %+v", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	if err := m.Pause(d.ID); err != nil {
		t.Fatal(err)
	}
	paused := waitFor(t, m, d.ID, 5*time.Second, engine.StatusPaused)
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("pause took %v", took)
	}
	info, err := os.Stat(paused.PartPath())
	if err != nil {
		t.Fatal(err)
	}
	partSize := info.Size()
	if partSize == 0 || partSize >= size {
		t.Fatalf("part file has %d bytes", partSize)
	}
	time.Sleep(100 * time.Millisecond) // let the server notice the closed connection

	if err := m.Resume(d.ID); err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, m, d.ID, 20*time.Second, engine.StatusCompleted, engine.StatusError)
	if done.Status != engine.StatusCompleted {
		t.Fatalf("download failed: %s", done.Error)
	}
	got, err := os.ReadFile(done.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("content differs after resume")
	}

	tr := srv.transfers()
	if len(tr) != 2 {
		t.Fatalf("server saw %d transfers, want 2", len(tr))
	}
	second := tr[1]
	if off := second.offset.Load(); off != partSize {
		t.Fatalf("resume asked for REST %d, part file had %d bytes", off, partSize)
	}
	if n := second.read.Load(); n != size-partSize {
		t.Fatalf("second transfer read %d bytes, want %d (bytes re-fetched)", n, size-partSize)
	}
}

func TestLoginWithPassword(t *testing.T) {
	srv := startServer(t, withLogin("alice", "Corr3ct-Horse"))
	want := srv.put("/home/alice/report.pdf", 70_000)
	m := newManager(t, New())

	d, err := m.Add(engine.Request{URL: "ftp://alice:Corr3ct-Horse@" + srv.addr + "/home/alice/report.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, m, d.ID, 10*time.Second, engine.StatusCompleted, engine.StatusError)
	if done.Status != engine.StatusCompleted {
		t.Fatalf("download failed: %s", done.Error)
	}
	if got, _ := os.ReadFile(done.Path()); !bytes.Equal(got, want) {
		t.Fatal("content differs")
	}
	if done.Filename != "report.pdf" {
		t.Fatalf("name = %q", done.Filename)
	}
}

func TestWrongPassword(t *testing.T) {
	srv := startServer(t, withLogin("alice", "Corr3ct-Horse"))
	srv.put("/f.bin", 10)
	m := newManager(t, New())

	const secret = "Wr0ng-Battery"
	d, err := m.Add(engine.Request{URL: "ftp://alice:" + secret + "@" + srv.addr + "/f.bin"})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitFor(t, m, d.ID, 10*time.Second, engine.StatusError, engine.StatusCompleted)
	if failed.Status != engine.StatusError {
		t.Fatal("download with a wrong password completed")
	}
	if strings.Contains(failed.Error, secret) {
		t.Fatalf("error leaks the password: %q", failed.Error)
	}
	if !strings.Contains(failed.Error, "login failed") || !strings.Contains(failed.Error, "530") {
		t.Fatalf("unclear error: %q", failed.Error)
	}
	if n := srv.authCalls.Load(); n != 1 {
		t.Fatalf("login attempted %d times; a rejected password must not be retried", n)
	}
}

func TestMissingFile(t *testing.T) {
	srv := startServer(t)
	m := newManager(t, New())
	d, err := m.Add(engine.Request{URL: "ftp://" + srv.addr + "/nope/missing.iso"})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitFor(t, m, d.ID, 10*time.Second, engine.StatusError, engine.StatusCompleted)
	if failed.Status != engine.StatusError {
		t.Fatal("missing file completed")
	}
	if !strings.Contains(failed.Error, "not found") || !strings.Contains(failed.Error, "550") {
		t.Fatalf("unclear error: %q", failed.Error)
	}
	if n := srv.authCalls.Load(); n != 1 {
		t.Fatalf("missing file retried: %d logins", n)
	}
}

func TestFTPS(t *testing.T) {
	serverTLS, roots := selfSigned(t)
	clientTLS := &tls.Config{RootCAs: roots}

	cases := []struct {
		name   string
		mode   ftpserver.TLSRequirement
		scheme string
	}{
		{"implicit", ftpserver.ImplicitEncryption, "ftps"},
		{"explicit AUTH TLS", ftpserver.ClearOrEncrypted, "ftp"},
		{"explicit, server requires TLS", ftpserver.MandatoryEncryption, "ftp"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := startServer(t, withLogin("bob", "s3cret-pass"), withTLS(serverTLS, c.mode))
			want := srv.put("/secure.bin", 300_000)
			m := newManager(t, New(WithTLSConfig(clientTLS)))
			d, err := m.Add(engine.Request{URL: c.scheme + "://bob:s3cret-pass@" + srv.addr + "/secure.bin"})
			if err != nil {
				t.Fatal(err)
			}
			done := waitFor(t, m, d.ID, 15*time.Second, engine.StatusCompleted, engine.StatusError)
			if done.Status != engine.StatusCompleted {
				t.Fatalf("download failed: %s", done.Error)
			}
			if got, _ := os.ReadFile(done.Path()); !bytes.Equal(got, want) {
				t.Fatal("content differs")
			}
			srv.mu.Lock()
			ctrl, data := srv.tlsCtrl, srv.tlsData
			srv.mu.Unlock()
			if !ctrl || !data {
				t.Fatalf("TLS control=%v data=%v, want both", ctrl, data)
			}
		})
	}
}

// A server offering AUTH TLS with a certificate we do not trust: with a password the
// download fails before the password is sent; anonymous downloads fall back to plain FTP.
func TestExplicitTLSFailureNeverSendsPassword(t *testing.T) {
	serverTLS, _ := selfSigned(t)

	t.Run("with password", func(t *testing.T) {
		srv := startServer(t, withLogin("bob", "s3cret-pass"), withTLS(serverTLS, ftpserver.ClearOrEncrypted))
		srv.put("/f.bin", 1000)
		m := newManager(t, New()) // production config: system roots only
		d, err := m.Add(engine.Request{URL: "ftp://bob:s3cret-pass@" + srv.addr + "/f.bin"})
		if err != nil {
			t.Fatal(err)
		}
		failed := waitFor(t, m, d.ID, 10*time.Second, engine.StatusError, engine.StatusCompleted)
		if failed.Status != engine.StatusError {
			t.Fatal("completed over an untrusted TLS connection")
		}
		if n := srv.authCalls.Load(); n != 0 {
			t.Fatalf("credentials reached the server %d times after the TLS failure", n)
		}
		if strings.Contains(failed.Error, "s3cret-pass") || !strings.Contains(failed.Error, "password was not sent") {
			t.Fatalf("error: %q", failed.Error)
		}
	})

	t.Run("anonymous", func(t *testing.T) {
		srv := startServer(t, withTLS(serverTLS, ftpserver.ClearOrEncrypted))
		want := srv.put("/pub.bin", 5000)
		m := newManager(t, New())
		d, err := m.Add(engine.Request{URL: "ftp://" + srv.addr + "/pub.bin"})
		if err != nil {
			t.Fatal(err)
		}
		done := waitFor(t, m, d.ID, 10*time.Second, engine.StatusCompleted, engine.StatusError)
		if done.Status != engine.StatusCompleted {
			t.Fatalf("anonymous fallback failed: %s", done.Error)
		}
		if got, _ := os.ReadFile(done.Path()); !bytes.Equal(got, want) {
			t.Fatal("content differs")
		}
	})
}

// Implicit FTPS must verify certificates by default.
func TestFTPSRejectsUntrustedCertificate(t *testing.T) {
	serverTLS, _ := selfSigned(t)
	srv := startServer(t, withLogin("bob", "s3cret-pass"), withTLS(serverTLS, ftpserver.ImplicitEncryption))
	srv.put("/f.bin", 1000)
	m := newManager(t, New())
	d, err := m.Add(engine.Request{URL: "ftps://bob:s3cret-pass@" + srv.addr + "/f.bin"})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitFor(t, m, d.ID, 10*time.Second, engine.StatusError, engine.StatusCompleted)
	if failed.Status != engine.StatusError || srv.authCalls.Load() != 0 {
		t.Fatalf("status=%s logins=%d", failed.Status, srv.authCalls.Load())
	}
	if strings.Contains(failed.Error, "s3cret-pass") || !strings.Contains(failed.Error, "certificate") {
		t.Fatalf("error: %q", failed.Error)
	}
}
