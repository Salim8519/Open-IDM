package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
)

const (
	logFileName = "peregrine-host.log"
	maxLogSize  = 1 << 20
)

// openLog returns a logger writing to stderr and <configDir>/peregrine-host.log.
// stdout is reserved for the native messaging protocol and is never used here.
func openLog(configDir string) (*log.Logger, func()) {
	var w io.Writer = os.Stderr
	closeFn := func() {}
	if configDir != "" && os.MkdirAll(configDir, 0o700) == nil {
		p := filepath.Join(configDir, logFileName)
		if fi, err := os.Stat(p); err == nil && fi.Size() > maxLogSize {
			_ = os.Rename(p, p+".old")
		}
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			w = io.MultiWriter(os.Stderr, f)
			closeFn = func() { _ = f.Close() }
		}
	}
	return log.New(w, fmt.Sprintf("peregrine-host[%d] ", os.Getpid()), log.LstdFlags), closeFn
}

// redactURL drops userinfo, query and fragment so tokens never reach the log.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return "<invalid url>"
	}
	u.User = nil
	if u.RawQuery != "" {
		u.RawQuery = "REDACTED"
	}
	u.Fragment = ""
	return u.String()
}

var urlInText = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"']+`)

// redactText redacts every URL found in free text (e.g. error messages).
func redactText(s string) string {
	return urlInText.ReplaceAllStringFunc(s, redactURL)
}
