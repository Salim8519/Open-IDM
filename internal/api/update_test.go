package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Salim8519/Peregrine/engine"
)

func newUpdateService(t *testing.T, hooks Hooks, tag string) *Service {
	t.Helper()
	rel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `"}`))
	}))
	st, err := engine.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	s := engine.DefaultSettings()
	s.DownloadDir, s.Proxy = t.TempDir(), "none"
	_ = st.SaveSettings(s)
	m, err := engine.NewManager(st)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(m, hooks)
	svc.SetUpdateURL(rel.URL)
	t.Cleanup(func() { svc.Close(); m.Close(); st.Close(); rel.Close() })
	return svc
}

func TestUpdateSettingDefaultsOn(t *testing.T) {
	if !engine.DefaultSettings().UpdateCheck {
		t.Fatal("the update check should be on by default (SPEC: the user can disable it)")
	}
}

func TestCheckUpdateFlow(t *testing.T) {
	var opened string
	svc := newUpdateService(t, Hooks{OpenURL: func(u string) error { opened = u; return nil }}, "v99.0.0")
	ctx := context.Background()

	if st := svc.updateStatus(); st != nil {
		t.Fatalf("status before a check = %+v", st)
	}
	if _, err := svc.Call(ctx, "openUpdatePage", nil); !errors.Is(err, errNoUpdate) {
		t.Fatalf("open before a check: err = %v", err)
	}

	events := make(chan Event, 4)
	defer svc.Subscribe(func(e Event) {
		if e.Type == "update" {
			events <- e
		}
	})()

	res, err := svc.CheckUpdate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Newer || res.Latest != "99.0.0" || res.Current != engine.Version {
		t.Fatalf("result = %+v", res)
	}
	select {
	case e := <-events:
		if e.Update == nil || e.Update.Latest != "99.0.0" {
			t.Fatalf("event = %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no update event")
	}
	if st := svc.updateStatus(); st == nil || st.Latest != "99.0.0" {
		t.Fatalf("status = %+v", st)
	}
	if _, err := svc.Call(ctx, "openUpdatePage", nil); err != nil {
		t.Fatal(err)
	}
	if opened != "https://github.com/Salim8519/Peregrine/releases/tag/v99.0.0" {
		t.Fatalf("opened %q", opened)
	}
}

func TestCheckUpdateUpToDateDoesNotOpen(t *testing.T) {
	called := false
	svc := newUpdateService(t, Hooks{OpenURL: func(string) error { called = true; return nil }}, "v"+engine.Version)
	res, err := svc.CheckUpdate(context.Background())
	if err != nil || res.Newer {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if _, err := svc.Call(context.Background(), "openUpdatePage", nil); !errors.Is(err, errNoUpdate) || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}
