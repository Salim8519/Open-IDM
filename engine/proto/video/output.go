package video

import (
	"bytes"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// Markers that tag the machine-readable lines Peregrine asks yt-dlp to print.
const (
	progressMarker = "[peregrine-progress]"
	fileMarker     = "[peregrine-file] "
)

// progressTemplate makes yt-dlp print one line per progress update:
// "[peregrine-progress] <status> <downloaded> <total> <total estimate>" ("NA" if unknown).
const progressTemplate = "download:" + progressMarker +
	" %(progress.status)s %(progress.downloaded_bytes)s %(progress.total_bytes)s %(progress.total_bytes_estimate)s"

// progress turns yt-dlp's per-stream progress into one running total. A merged download
// fetches the video stream, then the audio stream, and each starts again at zero.
type progress struct {
	base  int64 // bytes of the streams already finished
	cur   int64 // bytes of the current stream
	total int64 // size of the current stream (exact or estimate), 0 if unknown
}

// parse reads one progress line; ok is false for any other line.
func (p *progress) parse(line string) (done, size int64, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(line), progressMarker)
	if !found {
		return 0, 0, false
	}
	f := strings.Fields(rest)
	if len(f) < 4 {
		return 0, 0, false
	}
	status := f[0]
	downloaded, hasDown := number(f[1])
	total, hasTotal := number(f[2])
	if !hasTotal {
		total, hasTotal = number(f[3])
	}
	if hasDown && downloaded < p.cur { // a new stream started without a "finished" line
		p.base += max(p.cur, p.total)
		p.cur, p.total = 0, 0
	}
	if hasDown {
		p.cur = downloaded
	}
	if hasTotal {
		p.total = total
	}
	done = p.base + p.cur
	size = p.base + max(p.total, p.cur)
	if status == "finished" {
		p.base += max(p.cur, p.total)
		p.cur, p.total = 0, 0
		done, size = p.base, p.base
	}
	return done, size, true
}

// number parses an integer or float field ("1234", "1234.5"); "NA" and junk are not numbers.
func number(s string) (int64, bool) {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n >= 0 {
		return n, true
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 0 && f < 1<<62 {
		return int64(f), true
	}
	return 0, false
}

// lineWriter splits a process's output into lines (on \n or \r) for a callback.
type lineWriter struct {
	mu  sync.Mutex
	buf []byte
	fn  func(string)
}

func (w *lineWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, b...)
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}
		if i > 0 {
			w.fn(string(w.buf[:i]))
		}
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > 64<<10 { // a runaway line without a newline
		w.fn(string(w.buf))
		w.buf = w.buf[:0]
	}
	return len(b), nil
}

// flush hands over a last line without a newline.
func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.fn(string(w.buf))
		w.buf = nil
	}
}

var (
	ansiRE      = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	urlRE       = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	extractorRE = regexp.MustCompile(`^\[[^\]]+\]\s+[^:\s]+:\s+`)
)

// errorMessage picks yt-dlp's most useful error line from its stderr and cleans it for the
// UI and logs: no "ERROR:" prefix, no colors, URLs without credentials or query strings.
func errorMessage(lines []string) string {
	var msg string
	for i := len(lines) - 1; i >= 0 && msg == ""; i-- {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(lines[i]), "ERROR:"); ok {
			msg = rest
		}
	}
	if msg == "" {
		return ""
	}
	return cleanMessage(msg)
}

func cleanMessage(msg string) string {
	msg = ansiRE.ReplaceAllString(msg, "")
	msg = strings.TrimSpace(msg)
	msg = extractorRE.ReplaceAllString(msg, "")
	msg = urlRE.ReplaceAllStringFunc(msg, redactURL)
	if len(msg) > 300 {
		cut := 300
		for cut > 0 && !utf8.RuneStart(msg[cut]) {
			cut--
		}
		msg = msg[:cut] + "…"
	}
	return msg
}

// redactURL drops credentials, query string and fragment, which may carry tokens.
func redactURL(s string) string {
	trail := ""
	for strings.HasSuffix(s, ".") || strings.HasSuffix(s, ",") || strings.HasSuffix(s, ")") || strings.HasSuffix(s, ";") {
		trail = s[len(s)-1:] + trail
		s = s[:len(s)-1]
	}
	u, err := url.Parse(s)
	if err != nil {
		return "<url>" + trail
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	return u.String() + trail
}
