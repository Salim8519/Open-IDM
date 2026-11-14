package video

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const cookiePattern = "cookies-*.txt"

// writeCookieFile turns a browser Cookie header into a Netscape cookies file for the URL's
// host, which is how yt-dlp takes cookies. The file is created 0600 in dir; the caller
// deletes it when yt-dlp exits. It returns "" when the header holds no usable cookie.
func writeCookieFile(dir, rawURL, header string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("cookies: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", nil
	}
	// The browser sent these for this host; most were set for the site's parent domain
	// (".youtube.com"), so share them with its subdomains. IP hosts get no domain cookie.
	domain, sub := host, "FALSE"
	if net.ParseIP(host) == nil {
		for _, p := range []string{"www.", "m.", "mobile."} {
			if t := strings.TrimPrefix(domain, p); t != domain && strings.Contains(t, ".") {
				domain = t
				break
			}
		}
		domain, sub = "."+domain, "TRUE"
	}
	expires := time.Now().Add(24 * time.Hour).Unix()
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n# Written by Peregrine for one yt-dlp run; deleted afterwards.\n")
	n := 0
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name+value, "\t\r\n") {
			continue
		}
		fmt.Fprintf(&b, "%s\t%s\t/\tFALSE\t%d\t%s\t%s\n", domain, sub, expires, name, value)
		n++
	}
	if n == 0 {
		return "", nil
	}
	f, err := os.CreateTemp(dir, cookiePattern) // CreateTemp makes the file 0600
	if err != nil {
		return "", fmt.Errorf("cookies: %w", err)
	}
	_, werr := f.WriteString(b.String())
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("cookies: %w", werr)
	}
	return f.Name(), nil
}

// removeStaleCookies deletes cookie files a crashed run may have left in dir.
func removeStaleCookies(dir string) {
	old, _ := filepath.Glob(filepath.Join(dir, cookiePattern))
	for _, f := range old {
		_ = os.Remove(f)
	}
}

// header looks up an HTTP header case-insensitively.
func header(h map[string]string, key string) string {
	if v, ok := h[key]; ok {
		return v
	}
	for k, v := range h {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}
