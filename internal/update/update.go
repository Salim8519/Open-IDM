// Package update checks whether a newer Peregrine release exists (ADR 0020).
//
// It only reads the latest release's version from GitHub and compares it with the running
// version. It never downloads or installs anything: the user opens the release page in the
// browser. Nothing about the user is sent — the request carries only a generic User-Agent.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// Repo is the GitHub repository releases are published to.
const Repo = "Salim8519/Peregrine"

// LatestURL is the GitHub API endpoint for the latest published (non-draft, non-prerelease) release.
const LatestURL = "https://api.github.com/repos/" + Repo + "/releases/latest"

// maxBody caps how much of the API response we read (a release JSON is a few KB).
const maxBody = 1 << 20

// Result is the outcome of one check.
type Result struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest"`
	Newer     bool      `json:"newer"`
	URL       string    `json:"url"` // release page, built locally from the version, never taken from the response
	CheckedAt time.Time `json:"checkedAt"`
}

// Checker asks the release API for the latest version.
type Checker struct {
	Client  *http.Client // nil: http.DefaultClient
	URL     string       // "" : LatestURL
	Current string       // the running version, e.g. "0.4.1"
}

// ErrBadResponse means the server answered with something that is not a valid release.
var ErrBadResponse = errors.New("unexpected update server response")

// Check fetches the latest release and compares it with c.Current.
func (c Checker) Check(ctx context.Context) (Result, error) {
	u := c.URL
	if u == "" {
		u = LatestURL
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Result{}, fmt.Errorf("update check: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Peregrine-update-check")
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("update check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("update check: %w: HTTP %d", ErrBadResponse, resp.StatusCode)
	}
	var rel struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&rel); err != nil {
		return Result{}, fmt.Errorf("update check: %w: %v", ErrBadResponse, err)
	}
	latest, ok := parse(rel.TagName)
	if !ok || rel.Draft || rel.Prerelease {
		return Result{}, fmt.Errorf("update check: %w: tag %q", ErrBadResponse, rel.TagName)
	}
	v := latest.String()
	return Result{
		Current:   c.Current,
		Latest:    v,
		Newer:     Newer(v, c.Current),
		URL:       "https://github.com/" + Repo + "/releases/tag/v" + v,
		CheckedAt: time.Now(),
	}, nil
}

type version [3]int

func (v version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

// Only plain release versions are accepted: "1.2.3" or "v1.2.3". The strict form also
// means the release-page URL we build can never contain anything unexpected.
var versionRE = regexp.MustCompile(`^v?(\d{1,5})\.(\d{1,5})\.(\d{1,5})$`)

func parse(s string) (version, bool) {
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	var v version
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

// Newer reports whether latest is a higher version than current. A current version that is
// not a plain release (a dev build) never reports an update.
func Newer(latest, current string) bool {
	l, ok1 := parse(latest)
	c, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
