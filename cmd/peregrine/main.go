// Command peregrine is the Peregrine command-line tool.
//
//	peregrine get <url> [flags]   download in the terminal (resumable: run again to continue)
//	peregrine add <url>           send a link to the running desktop app
//	peregrine list                list downloads in the running desktop app
//	peregrine serve [flags]       run the engine headless with the local API
//	peregrine version
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Salim8519/Open-IDM/engine"
	"github.com/Salim8519/Open-IDM/internal/api"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "get":
		err = cmdGet(os.Args[2:])
	case "add":
		err = cmdAdd(os.Args[2:])
	case "list", "ls":
		err = cmdList()
	case "serve":
		err = cmdServe(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("peregrine", engine.Version)
	case "help", "-h", "--help":
		usage()
	default:
		// Allow "peregrine <url>" as a shortcut for get.
		if strings.HasPrefix(os.Args[1], "http://") || strings.HasPrefix(os.Args[1], "https://") {
			err = cmdGet(os.Args[1:])
		} else {
			usage()
			os.Exit(2)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Peregrine — fast, free download manager

Usage:
  peregrine get <url> [-o dir] [-n name] [-c connections] [--limit 2M] [--checksum sha256:...] [-H "Header: v"]
  peregrine add <url>        send to the running desktop app
  peregrine list             show downloads in the running desktop app
  peregrine serve [--port N] [--ui dir]   headless engine + local API
  peregrine version

Run "get" again with the same URL and folder to resume an interrupted download.
`)
}

type headerFlags map[string]string

func (h headerFlags) String() string { return "" }
func (h headerFlags) Set(v string) error {
	k, val, ok := strings.Cut(v, ":")
	if !ok {
		return fmt.Errorf("header must look like 'Name: value'")
	}
	h[strings.TrimSpace(k)] = strings.TrimSpace(val)
	return nil
}

// parseRate parses "500K", "2M", "1.5M", "123" (bytes/s).
func parseRate(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(strings.TrimSuffix(strings.TrimSuffix(s, "/s"), "B")))
	if s == "" || s == "0" {
		return 0, nil
	}
	mult := 1.0
	switch s[len(s)-1] {
	case 'K':
		mult, s = 1<<10, s[:len(s)-1]
	case 'M':
		mult, s = 1<<20, s[:len(s)-1]
	case 'G':
		mult, s = 1<<30, s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("invalid rate %q", s)
	}
	return int64(f * mult), nil
}

// splitArgs lets flags appear after the URL ("get URL -c 4").
func splitArgs(args []string) (pos []string, flags []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		pos = append(pos, a)
	}
	return
}

func cmdGet(args []string) error {
	fsg := flag.NewFlagSet("get", flag.ContinueOnError)
	out := fsg.String("o", ".", "output folder")
	name := fsg.String("n", "", "file name (default: from server)")
	conns := fsg.Int("c", 8, "connections (1-32)")
	limit := fsg.String("limit", "", "speed limit, e.g. 500K or 2M")
	sum := fsg.String("checksum", "", "verify, e.g. sha256:<hex>")
	quiet := fsg.Bool("q", false, "no progress output")
	home := fsg.String("state", "", "state folder (default: Peregrine config dir)")
	headers := headerFlags{}
	fsg.Var(headers, "H", "extra header 'Name: value' (repeatable)")
	pos, flags := splitArgs(args)
	if err := fsg.Parse(flags); err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: peregrine get <url> [flags]")
	}
	url := pos[0]
	bps, err := parseRate(*limit)
	if err != nil {
		return err
	}
	dir, err := filepath.Abs(*out)
	if err != nil {
		return err
	}

	stateDir := *home
	if stateDir == "" {
		if stateDir, err = api.ConfigDir(); err != nil {
			return err
		}
	}
	store, err := engine.OpenStore(filepath.Join(stateDir, "cli.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	st, _ := store.LoadSettings()
	st.MaxActive = 1
	st.DownloadDir = dir
	_ = store.SaveSettings(st)
	// Park everything else so only this download runs.
	prev, _ := store.LoadDownloads()
	for _, d := range prev {
		if d.Status == engine.StatusQueued || d.Status == engine.StatusDownloading {
			d.Status = engine.StatusPaused
			_ = store.SaveDownload(d)
		}
	}
	m, err := engine.NewManager(store)
	if err != nil {
		return err
	}
	defer m.Close()

	// Resume an earlier attempt for the same URL and folder.
	var id string
	for _, d := range m.List() {
		if d.URL == url && filepath.Clean(d.Dir) == dir && d.Status != engine.StatusCompleted {
			if *name == "" || d.Filename == *name {
				id = d.ID
				if !*quiet {
					fmt.Fprintf(os.Stderr, "Resuming %s (%s already downloaded)\n", d.Filename, human(d.Downloaded))
				}
				_ = m.SetSpeedLimit(id, bps)
				_ = m.Resume(id)
				break
			}
		}
	}
	if id == "" {
		d, err := m.Add(engine.Request{URL: url, Dir: dir, Filename: *name, Connections: *conns, Headers: headers, Checksum: *sum, SpeedLimit: bps})
		if err != nil {
			return err
		}
		id = d.ID
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = m.Pause(id)
			fmt.Fprintln(os.Stderr, "\nPaused. Run the same command again to resume.")
			return nil
		case <-tick.C:
			d, err := m.Get(id)
			if err != nil {
				return err
			}
			if !*quiet {
				printProgress(d)
			}
			switch d.Status {
			case engine.StatusCompleted:
				if !*quiet {
					fmt.Fprintf(os.Stderr, "\nSaved %s\nSHA-256 %s\n", d.Path(), d.SHA256)
				}
				_ = m.Remove(id, false) // keep the CLI state small; the file stays
				return nil
			case engine.StatusError:
				if !*quiet {
					fmt.Fprintln(os.Stderr)
				}
				return errors.New(d.Error)
			case engine.StatusPaused:
				return errors.New("download paused")
			}
		}
	}
}

func printProgress(d engine.Download) {
	pct := ""
	bar := ""
	if d.Size > 0 {
		p := float64(d.Downloaded) / float64(d.Size)
		pct = fmt.Sprintf("%5.1f%%", p*100)
		n := int(p * 24)
		bar = "[" + strings.Repeat("█", n) + strings.Repeat("░", 24-n) + "] "
	}
	eta := ""
	if d.ETA >= 0 {
		eta = " ETA " + (time.Duration(d.ETA) * time.Second).String()
	}
	size := "?"
	if d.Size >= 0 {
		size = human(d.Size)
	}
	fmt.Fprintf(os.Stderr, "\r%s%s %s/%s %s/s ×%d%s   ", bar, pct, human(d.Downloaded), size, human(d.Speed), d.Active, eta)
}

func human(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(u), 0
	for m := n / u; m >= u; m /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func cmdAdd(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: peregrine add <url>")
	}
	c, err := api.Dial()
	if err != nil {
		return err
	}
	_, err = c.Call("capture", map[string]any{"url": args[0], "source": "cli"})
	if err == nil {
		fmt.Println("Sent to Peregrine.")
	}
	return err
}

func cmdList() error {
	c, err := api.Dial()
	if err != nil {
		return err
	}
	raw, err := c.Call("list", nil)
	if err != nil {
		return err
	}
	var list []engine.Download
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	for _, d := range list {
		size := "?"
		if d.Size >= 0 {
			size = human(d.Size)
		}
		fmt.Printf("%-12s %-11s %10s  %s\n", d.ID[:min(12, len(d.ID))], d.Status, size, d.Filename)
	}
	return nil
}

func cmdServe(args []string) error {
	fsg := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fsg.Int("port", 0, "port on 127.0.0.1 (0 = random)")
	ui := fsg.String("ui", "", "folder with a built web UI to serve (optional)")
	token := fsg.String("token", "", "auth token (default: random)")
	if err := fsg.Parse(args); err != nil {
		return err
	}
	dir, err := api.ConfigDir()
	if err != nil {
		return err
	}
	store, err := engine.OpenStore(filepath.Join(dir, "peregrine.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	m, err := engine.NewManager(store)
	if err != nil {
		return err
	}
	defer m.Close()
	svc := api.NewService(m, api.Hooks{})
	defer svc.Close()
	var static fs.FS
	if *ui != "" {
		static = os.DirFS(*ui)
	}
	tok := *token
	if tok == "" {
		tok = api.NewToken()
	}
	srv, err := api.Listen(svc, *port, tok, static)
	if err != nil {
		return err
	}
	defer srv.Close()
	if err := api.WriteIPCInfo(srv.Port(), tok); err != nil {
		return err
	}
	defer api.RemoveIPCInfo()
	fmt.Printf("Peregrine engine listening on http://127.0.0.1:%d/?token=%s\n", srv.Port(), tok)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}
