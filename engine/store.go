package engine

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// Store persists downloads and settings in SQLite (WAL mode).
type Store struct{ db *sql.DB }

// OpenStore opens (or creates) the database at path. Use ":memory:" for tests.
func OpenStore(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		dsn = "file:" + filepath.ToSlash(path) + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS downloads (id TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS settings (k TEXT PRIMARY KEY, v TEXT NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) SaveDownload(d *Download) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO downloads(id, data) VALUES(?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data`, d.ID, string(b))
	return err
}

func (s *Store) DeleteDownload(id string) error {
	_, err := s.db.Exec(`DELETE FROM downloads WHERE id=?`, id)
	return err
}

func (s *Store) LoadDownloads() ([]*Download, error) {
	rows, err := s.db.Query(`SELECT data FROM downloads`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Download
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		d := new(Download)
		if err := json.Unmarshal([]byte(raw), d); err != nil {
			continue // skip a corrupt row rather than refuse to start
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) LoadSettings() (Settings, error) {
	st := DefaultSettings()
	var raw string
	err := s.db.QueryRow(`SELECT v FROM settings WHERE k='settings'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	_ = json.Unmarshal([]byte(raw), &st)
	return st, nil
}

func (s *Store) SaveSettings(st Settings) error {
	b, _ := json.Marshal(st)
	_, err := s.db.Exec(`INSERT INTO settings(k, v) VALUES('settings', ?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, string(b))
	return err
}
