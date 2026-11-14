// Command peregrine-host is the Chrome native messaging host that bridges the Peregrine
// browser extension and the Peregrine desktop app.
//
// Browsers start it with the caller origin as an argument and talk to it over
// stdin/stdout (uint32 little-endian length + JSON). It forwards an allowlisted subset of
// methods to the app's local RPC API (internal/api), starting the app when needed.
//
//	peregrine-host install   [--browsers all|chrome,edge,...]   register for browsers
//	peregrine-host uninstall [--browsers all|chrome,edge,...]   remove the registration
//	peregrine-host version
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Salim8519/Peregrine/internal/ipc"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "install", "uninstall":
			return runInstall(args[0], args[1:], stderr)
		case "version", "--version", "-v":
			fmt.Fprintln(stderr, "peregrine-host", hostVersion)
			return 0
		case "help", "--help", "-h":
			fmt.Fprintln(stderr, "usage: peregrine-host install|uninstall [--browsers all|"+browserNames()+"]")
			return 0
		}
	}
	return runNative(args, stdin, stdout)
}

// runNative is the browser-launched mode. Nothing but protocol frames goes to stdout.
func runNative(args []string, stdin io.Reader, stdout io.Writer) int {
	cfg, _ := ipc.ConfigDir()
	logger, closeLog := openLog(cfg)
	defer closeLog()

	origin := callerOrigin(args)
	if !originAllowed(origin, allowedIDs(cfg)) {
		logger.Printf("rejected caller origin %q", truncate(origin, 80))
		return 1
	}
	h := &host{
		log:        logger,
		dial:       func() (caller, error) { return ipc.Dial() },
		launch:     launchApp,
		launchWait: 15 * time.Second,
		pollEvery:  250 * time.Millisecond,
	}
	if err := h.serve(stdin, stdout); err != nil {
		return 1
	}
	return 0
}

// appPath returns the app executable that ships next to the host binary.
func appPath() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	return filepath.Join(filepath.Dir(self), appExeName), nil
}

func launchApp() error {
	p, err := appPath()
	if err != nil {
		return err
	}
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		return fmt.Errorf("%w: %s", errAppMissing, p)
	}
	return startDetached(p)
}

func runInstall(cmd string, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	spec := fs.String("browsers", "all", "all, or a comma-separated list of: "+browserNames())
	if err := fs.Parse(args); err != nil {
		return 2
	}
	bs, err := selectBrowsers(*spec)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	cfg, err := ipc.ConfigDir()
	if err != nil {
		fmt.Fprintln(stderr, "config dir:", err)
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	self, _ = filepath.Abs(self)
	home, _ := os.UserHomeDir()
	in := &installer{
		configDir: cfg,
		hostPath:  self,
		ids:       allowedIDs(cfg),
		setReg:    setRegistry,
		delReg:    deleteRegistry,
		logf:      func(f string, a ...any) { fmt.Fprintf(stderr, f+"\n", a...) },
	}
	ts := targetsFor(runtime.GOOS, home, os.Getenv(regRootEnv), bs)
	if cmd == "install" {
		err = in.install(ts)
	} else {
		err = in.uninstall(ts, *spec == "all")
	}
	if err != nil {
		fmt.Fprintln(stderr, cmd+":", err)
		return 1
	}
	return 0
}
