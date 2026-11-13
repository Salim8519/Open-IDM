package engine

import (
	"fmt"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

const maxNameBytes = 200

// SanitizeFilename turns an untrusted name (from a server or URL) into a safe single
// path component on Windows, macOS and Linux. It never returns "" or a path.
func SanitizeFilename(name string) string {
	if !utf8.ValidString(name) {
		name = strings.ToValidUTF8(name, "_")
	}
	// Keep only the last path element; servers may send "../../x" or "C:\x".
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 32 || r == 127:
			continue
		case strings.ContainsRune(`<>:"|?*`, r):
			b.WriteRune('_')
		case unicode.Is(unicode.Bidi_Control, r) || r == '\u200e' || r == '\u200f' || (r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069'):
			// Bidi overrides can disguise extensions (e.g. "exe.txt" shown as "txt.exe").
			continue
		default:
			b.WriteRune(r)
		}
	}
	name = strings.TrimSpace(b.String())
	name = strings.TrimRight(name, ". ")
	name = strings.TrimLeft(name, " ")
	if name == "" || name == "." || name == ".." {
		name = "download"
	}
	base := name
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if windowsReserved[strings.ToUpper(strings.TrimSpace(base))] {
		name = "_" + name
	}
	if len(name) > maxNameBytes {
		ext := filepath.Ext(name)
		if len(ext) > 20 || !utf8.ValidString(ext) {
			ext = ""
		}
		cut := maxNameBytes - len(ext)
		for cut > 0 && !utf8.RuneStart(name[cut]) {
			cut--
		}
		// Cutting can leave a trailing space or dot, which Windows forbids (found by fuzzing).
		name = strings.TrimRight(name[:cut], ". ") + ext
		if strings.Trim(name, ". ") == "" {
			name = "download"
		}
	}
	return name
}

// filenameFrom picks a filename from Content-Disposition, then the URL, then Content-Type.
func filenameFrom(rawURL, contentDisposition, contentType string) string {
	if contentDisposition != "" {
		if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
			if n := params["filename"]; n != "" { // mime decodes filename* into filename
				return SanitizeFilename(n)
			}
		}
	}
	name := ""
	if u, err := url.Parse(rawURL); err == nil {
		name = path.Base(u.Path)
		if un, err := url.PathUnescape(name); err == nil {
			name = un
		}
		if name == "/" || name == "." {
			name = ""
		}
	}
	if name == "" {
		name = "download"
	}
	if filepath.Ext(name) == "" && contentType != "" {
		if mt, _, err := mime.ParseMediaType(contentType); err == nil && mt != "application/octet-stream" {
			if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
				name += preferredExt(mt, exts)
			}
		}
	}
	return SanitizeFilename(name)
}

func preferredExt(mt string, exts []string) string {
	common := map[string]string{
		"text/html": ".html", "image/jpeg": ".jpg", "text/plain": ".txt", "application/zip": ".zip",
		"video/mp4": ".mp4", "audio/mpeg": ".mp3", "application/pdf": ".pdf",
	}
	if e, ok := common[mt]; ok {
		return e
	}
	return exts[0]
}

// Category returns the category for a filename, used for folders and filtering.
func Category(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mkv", ".avi", ".mov", ".webm", ".flv", ".wmv", ".m4v", ".ts", ".3gp":
		return "Video"
	case ".mp3", ".flac", ".wav", ".aac", ".ogg", ".m4a", ".wma", ".opus":
		return "Music"
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz", ".zst", ".tgz":
		return "Compressed"
	case ".exe", ".msi", ".dmg", ".pkg", ".deb", ".rpm", ".appimage", ".apk":
		return "Programs"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".epub", ".csv", ".odt":
		return "Documents"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".heic":
		return "Images"
	case ".iso", ".img":
		return "Disk Images"
	}
	return "Other"
}

// uniquePath returns p, or "name (n).ext" if p (or its part file) already exists or is reserved.
func uniquePath(p string, taken func(string) bool) string {
	if !exists(p) && !exists(p+PartSuffix) && !taken(p) {
		return p
	}
	dir, file := filepath.Split(p)
	ext := filepath.Ext(file)
	stem := strings.TrimSuffix(file, ext)
	for i := 1; ; i++ {
		c := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if !exists(c) && !exists(c+PartSuffix) && !taken(c) {
			return c
		}
	}
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func joinPath(dir, name string) string { return filepath.Join(dir, name) }

func defaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}
