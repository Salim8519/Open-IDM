package engine

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"report.pdf":           "report.pdf",
		"../../etc/passwd":     "passwd",
		`C:\Windows\evil.exe`:  "evil.exe",
		"CON":                  "_CON",
		"nul.txt":              "_nul.txt",
		"a<b>c:d\"e|f?g*h.txt": "a_b_c_d_e_f_g_h.txt",
		"trailing. . ":         "trailing",
		"":                     "download",
		"..":                   "download",
		"evil\u202Etxt.exe":    "eviltxt.exe",
		"tab\there\x00.bin":    "tabhere.bin",
		"ملف عربي.zip":         "ملف عربي.zip",
	}
	for in, want := range cases {
		if got := SanitizeFilename(in); got != want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("a", 500) + ".mp4"
	if got := SanitizeFilename(long); len(got) > maxNameBytes || !strings.HasSuffix(got, ".mp4") {
		t.Errorf("long name not trimmed correctly: len=%d", len(got))
	}
}

func FuzzSanitizeFilename(f *testing.F) {
	for _, s := range []string{"a.txt", "../x", "CON", "\x00", "ملف", "a/b\\c", strings.Repeat("é", 300)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := SanitizeFilename(in)
		if out == "" || out == "." || out == ".." {
			t.Fatalf("bad output %q", out)
		}
		if strings.ContainsAny(out, `/\<>:"|?*`) {
			t.Fatalf("unsafe char in %q", out)
		}
		if !utf8.ValidString(out) {
			t.Fatalf("invalid utf8 %q", out)
		}
		if len(out) > maxNameBytes {
			t.Fatalf("too long: %d", len(out))
		}
		for _, r := range out {
			if r < 32 {
				t.Fatalf("control char in %q", out)
			}
		}
		if strings.HasSuffix(out, ".") || strings.HasSuffix(out, " ") {
			t.Fatalf("trailing dot/space in %q", out)
		}
	})
}

func TestFilenameFrom(t *testing.T) {
	if got := filenameFrom("https://x.com/a/b/file%20name.zip?x=1", "", ""); got != "file name.zip" {
		t.Errorf("got %q", got)
	}
	if got := filenameFrom("https://x.com/dl", `attachment; filename="r.pdf"`, ""); got != "r.pdf" {
		t.Errorf("got %q", got)
	}
	if got := filenameFrom("https://x.com/dl", `attachment; filename*=UTF-8''%D9%85%D9%84%D9%81.txt`, ""); got != "ملف.txt" {
		t.Errorf("got %q", got)
	}
	if got := filenameFrom("https://x.com/", "", "video/mp4"); got != "download.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRedact(t *testing.T) {
	got := redact(`Get "https://user:pw@cdn.example.com/f.zip?token=SECRET": EOF`)
	if strings.Contains(got, "SECRET") || strings.Contains(got, "pw@") {
		t.Fatalf("not redacted: %s", got)
	}
}
