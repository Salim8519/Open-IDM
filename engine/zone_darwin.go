package engine

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// markFromInternet sets the macOS quarantine attribute so Gatekeeper checks the file.
func markFromInternet(path, sourceURL, referrer string) {
	v := fmt.Sprintf("0081;%x;Peregrine;", time.Now().Unix())
	_ = unix.Setxattr(path, "com.apple.quarantine", []byte(v), 0)
}
