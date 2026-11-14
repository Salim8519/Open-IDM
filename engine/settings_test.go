package engine

import (
	"path/filepath"
	"testing"
)

func TestDefaultThemeIsFieldGuide(t *testing.T) {
	if got := DefaultSettings().Theme; got != "field" {
		t.Fatalf("default theme = %q, want field", got)
	}
}

// Choosing Classic ("system") in the UI must survive a restart even when the client
// leaves uiVersion out; an unknown theme falls back to the default.
func TestUpdateSettingsThemeSurvivesReload(t *testing.T) {
	db := filepath.Join(t.TempDir(), "p.db")
	m := newTestManager(t, db, nil)
	s := m.Settings()
	s.Theme, s.UIVersion = "system", 0
	if _, err := m.UpdateSettings(s); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "system" {
		t.Fatalf("theme after reload = %q, want system", got.Theme)
	}

	s.Theme = "neon"
	out, err := m.UpdateSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	if out.Theme != "field" {
		t.Fatalf("unknown theme normalized to %q, want field", out.Theme)
	}
}

// Settings saved before the Field Guide theme existed (no uiVersion) move from the old
// default ("system") to the new default once; an explicit light/dark choice is kept, and
// after the migration "system" (Classic, automatic) is a normal choice again.
func TestSettingsThemeMigration(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{"old default moves to field", `{"theme":"system","language":"en"}`, "field"},
		{"old empty moves to field", `{"language":"en"}`, "field"},
		{"old explicit dark is kept", `{"theme":"dark"}`, "dark"},
		{"old explicit light is kept", `{"theme":"light"}`, "light"},
		{"new classic system is kept", `{"theme":"system","uiVersion":1}`, "system"},
		{"new field night is kept", `{"theme":"field-night","uiVersion":1}`, "field-night"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, err := OpenStore(filepath.Join(t.TempDir(), "p.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if _, err := st.db.Exec(`INSERT INTO settings(k, v) VALUES('settings', ?)`, c.raw); err != nil {
				t.Fatal(err)
			}
			got, err := st.LoadSettings()
			if err != nil {
				t.Fatal(err)
			}
			if got.Theme != c.want {
				t.Fatalf("theme = %q, want %q", got.Theme, c.want)
			}
			if got.UIVersion != currentUIVersion {
				t.Fatalf("uiVersion = %d, want %d", got.UIVersion, currentUIVersion)
			}
		})
	}
}
