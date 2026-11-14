package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"0.4.2", "0.4.1", true},
		{"v0.5.0", "0.4.9", true},
		{"1.0.0", "0.99.99", true},
		{"0.10.0", "0.9.0", true}, // numeric, not string, comparison
		{"0.4.1", "0.4.1", false},
		{"0.4.0", "0.4.1", false},
		{"0.4.2", "dev", false},
		{"0.4.2-rc1", "0.4.1", false},
		{"", "0.4.1", false},
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func serve(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != "Peregrine-update-check" {
			t.Errorf("User-Agent = %q", ua)
		}
		if r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("request carries extra data: %q %v", r.URL.RawQuery, r.Header)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckNewer(t *testing.T) {
	srv := serve(t, 200, `{"tag_name":"v0.5.0","html_url":"https://evil.example/x","draft":false,"prerelease":false}`)
	r, err := Checker{URL: srv.URL, Current: "0.4.1"}.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Newer || r.Latest != "0.5.0" || r.Current != "0.4.1" {
		t.Fatalf("result = %+v", r)
	}
	// The page URL is built from the version, never copied from the response.
	if r.URL != "https://github.com/Salim8519/Peregrine/releases/tag/v0.5.0" {
		t.Fatalf("URL = %q", r.URL)
	}
	if time.Since(r.CheckedAt) > time.Minute {
		t.Fatalf("CheckedAt = %v", r.CheckedAt)
	}
}

func TestCheckUpToDate(t *testing.T) {
	srv := serve(t, 200, `{"tag_name":"v0.4.1"}`)
	r, err := Checker{URL: srv.URL, Current: "0.4.1"}.Check(context.Background())
	if err != nil || r.Newer {
		t.Fatalf("r=%+v err=%v", r, err)
	}
}

func TestCheckRejectsBadResponses(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"server error":  {500, `oops`},
		"rate limited":  {403, `{"message":"API rate limit exceeded"}`},
		"not json":      {200, `<html>`},
		"no tag":        {200, `{}`},
		"odd tag":       {200, `{"tag_name":"v1.0.0/../../evil"}`},
		"prerelease":    {200, `{"tag_name":"v9.0.0","prerelease":true}`},
		"draft":         {200, `{"tag_name":"v9.0.0","draft":true}`},
		"tag with junk": {200, `{"tag_name":"v9.0.0?x=1"}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv := serve(t, c.status, c.body)
			_, err := Checker{URL: srv.URL, Current: "0.4.1"}.Check(context.Background())
			if !errors.Is(err, ErrBadResponse) {
				t.Fatalf("err = %v, want ErrBadResponse", err)
			}
		})
	}
}

func TestCheckHugeBodyIsCapped(t *testing.T) {
	srv := serve(t, 200, `{"tag_name":"v9.0.0","body":"`+strings.Repeat("a", 2*maxBody)+`"}`)
	if _, err := (Checker{URL: srv.URL, Current: "0.4.1"}).Check(context.Background()); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("err = %v, want ErrBadResponse", err)
	}
}

func TestCheckStopsOnCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(block); srv.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := (Checker{URL: srv.URL, Current: "0.4.1"}).Check(ctx); err == nil {
		t.Fatal("expected an error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("check did not stop on cancel")
	}
}
