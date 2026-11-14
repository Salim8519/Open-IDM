// Command e2eserver runs the real engine + RPC API with the built frontend and the "evil"
// test file server, for Playwright end-to-end tests. It prints one JSON line:
//
//	{"ui":"http://127.0.0.1:P/?token=T","files":"http://127.0.0.1:Q","home":"...","downloads":"..."}
//
// and runs until killed. Run it from desktop/ (it serves frontend/dist).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/Salim8519/Peregrine/engine"
	"github.com/Salim8519/Peregrine/internal/api"
	"github.com/Salim8519/Peregrine/internal/protocols"
	"github.com/Salim8519/Peregrine/internal/testsrv"
)

func main() {
	dist := flag.String("dist", "frontend/dist", "built frontend directory")
	stdinExit := flag.Bool("exit-on-stdin-eof", false, "shut down cleanly when stdin is closed (used by the Playwright setup)")
	flag.Parse()

	tmp, err := os.MkdirTemp("", "peregrine-e2e-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	home := filepath.Join(tmp, "home")
	dl := filepath.Join(tmp, "downloads")
	for _, d := range []string{home, dl} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			log.Fatal(err)
		}
	}
	os.Setenv("PEREGRINE_HOME", home)

	h, _ := testsrv.Handler()
	fln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go func() { _ = http.Serve(fln, h) }()

	store, err := engine.OpenStore(filepath.Join(home, "peregrine.db"))
	if err != nil {
		log.Fatal(err)
	}
	m, err := engine.NewManager(store)
	if err != nil {
		log.Fatal(err)
	}
	protocols.Register(m, protocols.Options{})
	st := m.Settings()
	st.DownloadDir = dl
	if _, err := m.UpdateSettings(st); err != nil {
		log.Fatal(err)
	}
	// Browser mode has no native dialogs: pickFolder returns a fixed folder so the
	// Browse button can be exercised.
	svc := api.NewService(m, api.Hooks{
		PickFolder: func(current string) (string, error) { return filepath.Join(dl, "picked"), nil },
		Clipboard:  func() (string, error) { return os.Getenv("PEREGRINE_E2E_CLIPBOARD"), nil },
		// The "Connect your browser" guide: a display-only folder and no real browser launch.
		ExtensionDir:    func() string { return filepath.Join(home, "Peregrine", "extension") },
		OpenBrowserPage: func(browser, page string) error { log.Printf("e2e: open %s %s", browser, page); return nil },
		// Open file / Show in folder: record the path instead of launching Explorer/Finder,
		// so tests can check which file the app asked the OS to show.
		OpenPath:   func(p string) error { return appendLine(filepath.Join(home, "os-hooks.log"), "open "+p) },
		RevealPath: func(p string) error { return appendLine(filepath.Join(home, "os-hooks.log"), "reveal "+p) },
	})
	svc.SetBrowserStateFile(filepath.Join(home, "browser.json"))
	token := api.NewToken()
	srv, err := api.Listen(svc, 0, token, os.DirFS(*dist))
	if err != nil {
		log.Fatal(err)
	}
	out, _ := json.Marshal(map[string]string{
		"ui":        fmt.Sprintf("http://127.0.0.1:%d/?token=%s", srv.Port(), token),
		"files":     "http://" + fln.Addr().String(),
		"home":      home,
		"downloads": dl,
	})
	fmt.Println(string(out))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	if *stdinExit {
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			sig <- os.Interrupt
		}()
	}
	<-sig
	_ = srv.Close()
	svc.Close()
	_ = m.Close()
}

func appendLine(file, line string) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, line)
	return err
}
