// Command fakeytdlp stands in for yt-dlp in tests; it never touches the network.
//
// It accepts the options Peregrine passes and refuses a URL anywhere before "--". The
// URL's query controls it:
//
//	title=  video title (default "Fake Video")      ext=   extension (default mp4)
//	size=   bytes to "download" (default 100000)    chunk= bytes per step (default 10000)
//	delay=  milliseconds per step (default 0)       fail=  "probe" or "download"
//
// With --print filename it prints the name from the -o template and exits. Otherwise it
// appends to <output>.part (continuing an existing one, like yt-dlp --continue), prints
// progress lines in the --progress-template format on stderr, renames the part file and
// prints the --print after_move template on stdout. If FAKEYTDLP_LOG is set, it appends
// one JSON line per run with its arguments and the content of the --cookies file.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var withValue = map[string]bool{
	"-o": true, "--print": true, "--progress-template": true, "-f": true,
	"--merge-output-format": true, "--ffmpeg-location": true, "--limit-rate": true,
	"--cookies": true, "--referer": true, "--user-agent": true, "--encoding": true,
}

type logEntry struct {
	Args       []string `json:"args"`
	Cookies    string   `json:"cookies,omitempty"`
	CookieFile string   `json:"cookieFile,omitempty"`
	ResumeFrom int64    `json:"resumeFrom"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	opts := map[string][]string{}
	flags := map[string]bool{}
	var urls []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if a == "--" {
			urls = argv[i+1:]
			break
		}
		if withValue[a] {
			if i+1 >= len(argv) {
				fmt.Fprintf(os.Stderr, "ERROR: %s needs a value\n", a)
				return 2
			}
			opts[a] = append(opts[a], argv[i+1])
			i++
			continue
		}
		flags[a] = true
	}
	if len(urls) != 1 {
		fmt.Fprintln(os.Stderr, "ERROR: want exactly one URL after --")
		return 2
	}
	for _, a := range argv[:len(argv)-2] {
		if a == urls[0] {
			fmt.Fprintf(os.Stderr, "ERROR: URL %q passed before --\n", a)
			return 2
		}
	}
	if !flags["--no-config"] {
		fmt.Fprintln(os.Stderr, "ERROR: --no-config missing")
		return 2
	}
	u, err := url.Parse(urls[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR: bad URL")
		return 2
	}
	q := u.Query()
	title := def(q.Get("title"), "Fake Video")
	ext := def(q.Get("ext"), "mp4")
	size := num(q.Get("size"), 100000)
	chunk := num(q.Get("chunk"), 10000)
	delay := time.Duration(num(q.Get("delay"), 0)) * time.Millisecond

	entry := logEntry{Args: argv}
	if c := last(opts["--cookies"]); c != "" {
		b, err := os.ReadFile(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: cookies file: %v\n", err)
			return 3
		}
		entry.Cookies, entry.CookieFile = string(b), c
	}

	outTmpl := last(opts["-o"])
	name := regexp.MustCompile(`%\(title\)[.0-9]*[sB]`).ReplaceAllString(outTmpl, title)
	name = strings.ReplaceAll(name, "%(ext)s", ext)
	name = strings.ReplaceAll(name, "%%", "%")

	isProbe := false
	var afterMove string
	for _, p := range opts["--print"] {
		if p == "filename" {
			isProbe = true
		} else if t, ok := strings.CutPrefix(p, "after_move:"); ok {
			afterMove = t
		}
	}
	if isProbe {
		writeLog(entry)
		if q.Get("fail") == "probe" {
			fmt.Fprintln(os.Stderr, "WARNING: something minor")
			fmt.Fprintln(os.Stderr, "ERROR: [fake] abc123: This video is private. See https://user:pw@example.com/help?token=secret#x for details")
			return 1
		}
		fmt.Println(name)
		return 0
	}

	part := name + ".part"
	f, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return 1
	}
	defer f.Close()
	info, _ := f.Stat()
	done := info.Size()
	entry.ResumeFrom = done
	writeLog(entry)
	tmpl := strings.TrimPrefix(last(opts["--progress-template"]), "download:")
	progress := func(status string, n int64, total string) {
		line := strings.NewReplacer(
			"%(progress.status)s", status,
			"%(progress.downloaded_bytes)s", strconv.FormatInt(n, 10),
			"%(progress.total_bytes)s", total,
			"%(progress.total_bytes_estimate)s", strconv.FormatInt(size, 10)+".0",
		).Replace(tmpl)
		fmt.Fprintln(os.Stderr, line)
	}
	buf := make([]byte, chunk)
	for done < size {
		time.Sleep(delay)
		n := min(chunk, size-done)
		if _, err := f.WriteAt(buf[:n], done); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			return 1
		}
		done += n
		progress("downloading", done, "NA")
		if q.Get("fail") == "download" && done >= size/2 {
			fmt.Fprintln(os.Stderr, "ERROR: unable to download video data: HTTP Error 403: Forbidden")
			return 1
		}
	}
	f.Close()
	if err := os.Rename(part, name); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return 1
	}
	progress("finished", done, strconv.FormatInt(size, 10))
	if afterMove != "" {
		fmt.Println(strings.ReplaceAll(afterMove, "%(filepath)s", name))
	}
	return 0
}

func writeLog(e logEntry) {
	path := os.Getenv("FAKEYTDLP_LOG")
	if path == "" {
		return
	}
	b, _ := json.Marshal(e)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func def(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func num(s string, d int64) int64 {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return d
}

func last(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}
