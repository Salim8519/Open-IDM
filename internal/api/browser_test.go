package api

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Salim8519/Open-IDM/engine"
)

func newService(t *testing.T, hooks Hooks) *Service {
	t.Helper()
	st, err := engine.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	m, err := engine.NewManager(st)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(m, hooks)
	t.Cleanup(func() { svc.Close(); m.Close(); st.Close() })
	return svc
}

func status(t *testing.T, s *Service) BrowserStatus {
	t.Helper()
	res, err := s.Call(context.Background(), "browserStatus", nil)
	if err != nil {
		t.Fatal(err)
	}
	return res.(BrowserStatus)
}

// The first-run prompt depends on this: it must stay "connected" after a restart.
func TestExtensionHelloPersists(t *testing.T) {
	file := filepath.Join(t.TempDir(), "browser.json")
	s := newService(t, Hooks{})
	s.SetBrowserStateFile(file)
	if st := status(t, s); st.Connected || st.ExtensionID != ExtensionID {
		t.Fatalf("fresh install: %+v", st)
	}
	var got []Event
	s.Subscribe(func(e Event) { got = append(got, e) })
	if _, err := s.Call(context.Background(), "extensionHello", nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Type != "browser" || got[0].Browser == nil || !got[0].Browser.Connected {
		t.Fatalf("event: %+v", got)
	}
	s2 := newService(t, Hooks{})
	s2.SetBrowserStateFile(file)
	if st := status(t, s2); !st.Connected || st.LastSeen == 0 {
		t.Fatalf("after restart: %+v", st)
	}
}

// Only known browsers may be opened; the page URL comes from our table, not the caller.
func TestOpenExtensionsPageAllowlist(t *testing.T) {
	var opened []string
	s := newService(t, Hooks{OpenBrowserPage: func(b, page string) error { opened = append(opened, b+" "+page); return nil }})
	for _, bad := range []string{"", "calc.exe", "chrome --flag", "../chrome"} {
		params, _ := json.Marshal(map[string]string{"browser": bad})
		if _, err := s.Call(context.Background(), "openExtensionsPage", params); !errors.Is(err, errBadParams) {
			t.Fatalf("%q: want bad params, got %v", bad, err)
		}
	}
	params, _ := json.Marshal(map[string]string{"browser": "edge"})
	if _, err := s.Call(context.Background(), "openExtensionsPage", params); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != "edge edge://extensions" {
		t.Fatalf("opened %v", opened)
	}
}
