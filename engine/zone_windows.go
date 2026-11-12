package engine

import (
	"fmt"
	"os"
	"strings"
)

// markFromInternet writes the Mark-of-the-Web so Windows SmartScreen and Office treat the
// file as downloaded from the internet.
func markFromInternet(path, sourceURL, referrer string) {
	var b strings.Builder
	b.WriteString("[ZoneTransfer]\r\nZoneId=3\r\n")
	if referrer != "" {
		fmt.Fprintf(&b, "ReferrerUrl=%s\r\n", stripCreds(referrer))
	}
	if sourceURL != "" {
		fmt.Fprintf(&b, "HostUrl=%s\r\n", stripCreds(sourceURL))
	}
	_ = os.WriteFile(path+":Zone.Identifier", []byte(b.String()), 0o644)
}
