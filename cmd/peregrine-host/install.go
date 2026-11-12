package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	hostName         = "com.peregrine.host"
	manifestFileName = hostName + ".json"
	// regRootEnv redirects registry keys (Windows: a key under HKCU, e.g.
	// "Software\PeregrineTest") or the home directory (macOS/Linux) for tests.
	regRootEnv = "PEREGRINE_REG_ROOT"
)

type hostManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

func buildManifest(hostPath string, ids []string) ([]byte, error) {
	if !filepath.IsAbs(hostPath) {
		return nil, fmt.Errorf("host path must be absolute: %s", hostPath)
	}
	m := hostManifest{
		Name:           hostName,
		Description:    "Peregrine download manager - browser bridge",
		Path:           hostPath,
		Type:           "stdio",
		AllowedOrigins: originsFor(ids),
	}
	return json.MarshalIndent(m, "", "  ")
}

// browser describes where one Chromium browser looks for native messaging hosts.
type browser struct {
	Name   string
	RegKey string // Windows, relative to HKCU\Software
	Mac    string // relative to ~/Library/Application Support
	Linux  string // relative to ~/.config
}

// Opera reads Chrome's registration, so it needs no entry of its own.
var browsers = []browser{
	{"chrome", `Google\Chrome`, "Google/Chrome", "google-chrome"},
	{"edge", `Microsoft\Edge`, "Microsoft Edge", "microsoft-edge"},
	{"brave", `BraveSoftware\Brave-Browser`, "BraveSoftware/Brave-Browser", "BraveSoftware/Brave-Browser"},
	{"chromium", `Chromium`, "Chromium", "chromium"},
	{"vivaldi", `Vivaldi`, "Vivaldi", "vivaldi"},
}

// target is one place a manifest gets registered: a registry key (Windows) or a directory.
type target struct {
	Browser string
	RegKey  string // full path under HKCU, e.g. Software\Google\Chrome\NativeMessagingHosts\com.peregrine.host
	Dir     string // directory that receives com.peregrine.host.json
}

func selectBrowsers(spec string) ([]browser, error) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "" || spec == "all" {
		return browsers, nil
	}
	var out []browser
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		found := false
		for _, b := range browsers {
			if b.Name == name {
				out = append(out, b)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown browser %q (want all or a list of: chrome, edge, brave, chromium, vivaldi)", name)
		}
	}
	return out, nil
}

// targetsFor computes registration targets. regRoot (from PEREGRINE_REG_ROOT) replaces
// "Software" on Windows and the home directory elsewhere.
func targetsFor(goos, home, regRoot string, bs []browser) []target {
	var out []target
	for _, b := range bs {
		switch goos {
		case "windows":
			root := `Software`
			if regRoot != "" {
				root = strings.TrimRight(regRoot, `\`)
			}
			out = append(out, target{Browser: b.Name, RegKey: root + `\` + b.RegKey + `\NativeMessagingHosts\` + hostName})
		case "darwin":
			base := home
			if regRoot != "" {
				base = regRoot
			}
			out = append(out, target{Browser: b.Name, Dir: filepath.Join(base, "Library", "Application Support", filepath.FromSlash(b.Mac), "NativeMessagingHosts")})
		default:
			base := filepath.Join(home, ".config")
			if regRoot != "" {
				base = filepath.Join(regRoot, ".config")
			}
			out = append(out, target{Browser: b.Name, Dir: filepath.Join(base, filepath.FromSlash(b.Linux), "NativeMessagingHosts")})
		}
	}
	return out
}

// installer performs install/uninstall; the registry functions are injected so the file
// logic is testable on every OS.
type installer struct {
	configDir string
	hostPath  string
	ids       []string
	setReg    func(key, manifestPath string) error
	delReg    func(key string) error
	logf      func(format string, a ...any)
}

func (in *installer) install(ts []target) error {
	data, err := buildManifest(in.hostPath, in.ids)
	if err != nil {
		return err
	}
	var errs []error
	needCentral := false
	for _, t := range ts {
		if t.RegKey != "" {
			needCentral = true
		}
	}
	central := filepath.Join(in.configDir, manifestFileName)
	if needCentral {
		if err := os.MkdirAll(in.configDir, 0o700); err != nil {
			return err
		}
		if err := writeFileAtomic(central, data); err != nil {
			return err
		}
		in.logf("wrote %s", central)
	}
	for _, t := range ts {
		switch {
		case t.RegKey != "":
			if err := in.setReg(t.RegKey, central); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", t.Browser, err))
				continue
			}
			in.logf("registered %s: HKCU\\%s", t.Browser, t.RegKey)
		case t.Dir != "":
			if err := os.MkdirAll(t.Dir, 0o755); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", t.Browser, err))
				continue
			}
			p := filepath.Join(t.Dir, manifestFileName)
			if err := writeFileAtomic(p, data); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", t.Browser, err))
				continue
			}
			in.logf("registered %s: %s", t.Browser, p)
		}
	}
	return errors.Join(errs...)
}

func (in *installer) uninstall(ts []target, all bool) error {
	var errs []error
	for _, t := range ts {
		switch {
		case t.RegKey != "":
			if err := in.delReg(t.RegKey); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", t.Browser, err))
				continue
			}
			in.logf("unregistered %s", t.Browser)
		case t.Dir != "":
			p := filepath.Join(t.Dir, manifestFileName)
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("%s: %w", t.Browser, err))
				continue
			}
			in.logf("unregistered %s", t.Browser)
		}
	}
	if all {
		if err := os.Remove(filepath.Join(in.configDir, manifestFileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func writeFileAtomic(p string, data []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func browserNames() string {
	names := make([]string, 0, len(browsers))
	for _, b := range browsers {
		names = append(names, b.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
