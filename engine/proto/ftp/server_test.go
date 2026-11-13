package ftp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"

	"github.com/Salim8519/Peregrine/engine"
)

// testServer is an in-process FTP server (fclairamb/ftpserverlib over an in-memory FS)
// that records what clients do.
type testServer struct {
	t        *testing.T
	fs       afero.Fs
	srv      *ftpserver.FtpServer
	addr     string // 127.0.0.1:port
	user     string // "" = anonymous only
	pass     string
	tlsCfg   *tls.Config // nil = no TLS on offer
	tlsMode  ftpserver.TLSRequirement
	throttle time.Duration // pause per 16 KB read, so tests can pause mid-transfer

	authCalls atomic.Int32
	mu        sync.Mutex
	opens     []*countingFile
	tlsCtrl   bool // last login came over TLS
	tlsData   bool // last transfer used PROT P
}

type serverOpt func(*testServer)

func withLogin(user, pass string) serverOpt {
	return func(s *testServer) { s.user, s.pass = user, pass }
}
func withTLS(cfg *tls.Config, mode ftpserver.TLSRequirement) serverOpt {
	return func(s *testServer) { s.tlsCfg, s.tlsMode = cfg, mode }
}
func withThrottle(d time.Duration) serverOpt { return func(s *testServer) { s.throttle = d } }

func startServer(t *testing.T, opts ...serverOpt) *testServer {
	t.Helper()
	s := &testServer{t: t, fs: afero.NewMemMapFs()}
	for _, o := range opts {
		o(s)
	}
	s.srv = ftpserver.NewFtpServer(s)
	s.srv.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := s.srv.Listen(); err != nil {
		t.Fatal(err)
	}
	s.addr = s.srv.Addr()
	go func() { _ = s.srv.Serve() }()
	t.Cleanup(func() { _ = s.srv.Stop() })
	return s
}

// put stores a file of n random bytes and returns them.
func (s *testServer) put(name string, n int) []byte {
	s.t.Helper()
	b := make([]byte, n)
	_, _ = rand.Read(b)
	if err := afero.WriteFile(s.fs, name, b, 0o644); err != nil {
		s.t.Fatal(err)
	}
	return b
}

func (s *testServer) transfers() []*countingFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*countingFile(nil), s.opens...)
}

// MainDriver

func (s *testServer) GetSettings() (*ftpserver.Settings, error) {
	return &ftpserver.Settings{
		ListenAddr:             "127.0.0.1:0",
		TLSRequired:            s.tlsMode,
		IdleTimeout:            30,
		ConnectionTimeout:      10,
		DefaultTransferType:    ftpserver.TransferTypeBinary,
		DisableActiveMode:      true,
		DisableASCIIConversion: true,
	}, nil
}

func (s *testServer) ClientConnected(ftpserver.ClientContext) (string, error) {
	return "Peregrine test server", nil
}

func (s *testServer) ClientDisconnected(ftpserver.ClientContext) {}

func (s *testServer) AuthUser(cc ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	s.authCalls.Add(1)
	if s.user == "" {
		if user != "anonymous" {
			return nil, errors.New("anonymous only")
		}
	} else if user != s.user || pass != s.pass {
		return nil, errors.New("bad credentials")
	}
	s.mu.Lock()
	s.tlsCtrl = cc.HasTLSForControl()
	s.mu.Unlock()
	return &clientFS{Fs: s.fs, s: s, cc: cc}, nil
}

func (s *testServer) GetTLSConfig() (*tls.Config, error) {
	if s.tlsCfg == nil {
		return nil, errors.New("TLS not configured")
	}
	return s.tlsCfg, nil
}

// clientFS counts every file opened for reading.
type clientFS struct {
	afero.Fs
	s  *testServer
	cc ftpserver.ClientContext
}

func (c *clientFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f, err := c.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	cf := &countingFile{File: f, throttle: c.s.throttle}
	c.s.mu.Lock()
	c.s.opens = append(c.s.opens, cf)
	c.s.tlsData = c.cc.HasTLSForTransfers()
	c.s.mu.Unlock()
	return cf, nil
}

type countingFile struct {
	afero.File
	throttle time.Duration
	offset   atomic.Int64 // REST offset (last seek)
	read     atomic.Int64 // bytes served
}

func (f *countingFile) Seek(off int64, whence int) (int64, error) {
	n, err := f.File.Seek(off, whence)
	if err == nil {
		f.offset.Store(n)
	}
	return n, err
}

func (f *countingFile) Read(p []byte) (int, error) {
	if f.throttle > 0 {
		time.Sleep(f.throttle)
		p = p[:min(len(p), 16<<10)]
	}
	n, err := f.File.Read(p)
	f.read.Add(int64(n))
	return n, err
}

// selfSigned returns a server TLS config for 127.0.0.1 and a pool that trusts it.
func selfSigned(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "peregrine-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}, pool
}

// newManager returns an engine.Manager with an in-memory store, a temp download folder and
// the given FTP protocol registered.
func newManager(t *testing.T, p *Protocol) *engine.Manager {
	t.Helper()
	st, err := engine.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	s := engine.DefaultSettings()
	s.DownloadDir = tempDir(t)
	s.Proxy = "none"
	if err := st.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	m, err := engine.NewManager(st)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close(); st.Close() })
	m.RegisterProtocol(p)
	return m
}

// tempDir is t.TempDir with a patient cleanup: on Windows a virus scanner may still hold a
// just-completed file open, leaving it "delete pending" for a moment.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "peregrine-ftp-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var err error
		for range 100 {
			if err = os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Errorf("remove temp dir: %v", err)
	})
	return dir
}

// waitFor polls until the download reaches one of the statuses.
func waitFor(t *testing.T, m *engine.Manager, id string, timeout time.Duration, want ...engine.Status) engine.Download {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		d, err := m.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if d.Status == w {
				return d
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %v: status=%s err=%q downloaded=%d/%d", want, d.Status, d.Error, d.Downloaded, d.Size)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
