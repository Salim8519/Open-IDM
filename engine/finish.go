package engine

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

// hashFile returns the SHA-256 of path and, if expected is set ("algo:hex"), verifies it.
func hashFile(path, expected string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sha := sha256.New()
	writers := []io.Writer{sha}
	var check hash.Hash
	var want string
	if expected != "" {
		algo, hexsum, ok := strings.Cut(strings.TrimSpace(expected), ":")
		if !ok {
			return "", fmt.Errorf("checksum must look like sha256:<hex>")
		}
		want = strings.ToLower(hexsum)
		switch strings.ToLower(algo) {
		case "sha256":
			check = sha
		case "sha1":
			check = sha1.New()
		case "md5":
			check = md5.New()
		case "sha512":
			check = sha512.New()
		default:
			return "", fmt.Errorf("unsupported checksum algorithm %q", algo)
		}
		if check != sha {
			writers = append(writers, check)
		}
	}
	if _, err := io.Copy(io.MultiWriter(writers...), f); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(sha.Sum(nil))
	if check != nil {
		if got := hex.EncodeToString(check.Sum(nil)); got != want {
			return sum, fmt.Errorf("%w: expected %s, got %s", ErrChecksum, want, got)
		}
	}
	return sum, nil
}

// ValidChecksum reports whether s is empty or a supported "algo:hex" string.
func ValidChecksum(s string) bool {
	if s == "" {
		return true
	}
	algo, h, ok := strings.Cut(s, ":")
	if !ok {
		return false
	}
	lens := map[string]int{"md5": 32, "sha1": 40, "sha256": 64, "sha512": 128}
	n, ok := lens[strings.ToLower(algo)]
	if !ok || len(h) != n {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil
}
