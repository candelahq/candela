package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// StateDB manages local settings, runtime state, and pull history
// in a SQLite database at ~/.candela/state.db.
type StateDB struct {
	db *sql.DB
}

// openStateDB opens (or creates) the state database at the given path.
// Parent directories are created as needed. The file is chmod 0600.
func openStateDB(path string) (*StateDB, error) {
	// Expand ~ if present.
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("state db: resolve home: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("state db: mkdir %q: %w", dir, err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("state db: open %q: %w", path, err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state db: enable WAL: %w", err)
	}

	s := &StateDB{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// migrate creates the schema tables if they don't exist.
func (s *StateDB) migrate() error {
	const schema = `
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS runtime_state (
			id           INTEGER PRIMARY KEY CHECK (id = 1),
			backend      TEXT NOT NULL DEFAULT '',
			last_started TEXT,
			last_model   TEXT DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS pull_history (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			model      TEXT NOT NULL,
			backend    TEXT NOT NULL,
			pulled_at  TEXT NOT NULL DEFAULT (datetime('now')),
			size_bytes INTEGER DEFAULT 0
		);
		INSERT OR IGNORE INTO runtime_state (id) VALUES (1);
	`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("state db: migrate: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (s *StateDB) Close() error {
	return s.db.Close()
}

// ── Settings ──

// GetSetting retrieves a setting value by key. Returns "" if not found.
func (s *StateDB) GetSetting(key string) string {
	var value string
	_ = s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	return value
}

// SetSetting stores a key-value setting. Overwrites if the key already exists.
func (s *StateDB) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?",
		key, value, value)
	return err
}

// DeleteSetting removes a setting by key.
func (s *StateDB) DeleteSetting(key string) error {
	_, err := s.db.Exec("DELETE FROM settings WHERE key = ?", key)
	return err
}

// ── Runtime State ──

// RuntimeState holds the persisted runtime state.
type RuntimeState struct {
	Backend     string
	LastStarted time.Time
	LastModel   string
}

// GetRuntimeState returns the persisted runtime state.
func (s *StateDB) GetRuntimeState() RuntimeState {
	var rs RuntimeState
	var started sql.NullString
	_ = s.db.QueryRow("SELECT backend, last_started, last_model FROM runtime_state WHERE id = 1").
		Scan(&rs.Backend, &started, &rs.LastModel)
	if started.Valid {
		// SQLite datetime('now') returns 'YYYY-MM-DD HH:MM:SS'
		rs.LastStarted, _ = time.Parse("2006-01-02 15:04:05", started.String)
	}
	return rs
}

// SetRuntimeState updates the persisted runtime state.
func (s *StateDB) SetRuntimeState(backend, model string) error {
	_, err := s.db.Exec(
		"UPDATE runtime_state SET backend = ?, last_started = datetime('now'), last_model = ? WHERE id = 1",
		backend, model)
	return err
}

// ── Pull History ──

// PullRecord represents a model pull event.
type PullRecord struct {
	Model     string
	Backend   string
	PulledAt  time.Time
	SizeBytes int64
}

// RecordPull adds a pull event to the history.
func (s *StateDB) RecordPull(model, backend string, sizeBytes int64) error {
	_, err := s.db.Exec(
		"INSERT INTO pull_history (model, backend, size_bytes) VALUES (?, ?, ?)",
		model, backend, sizeBytes)
	return err
}

// RecentPulls returns the last n pull events, newest first.
func (s *StateDB) RecentPulls(n int) ([]PullRecord, error) {
	rows, err := s.db.Query(
		"SELECT model, backend, pulled_at, size_bytes FROM pull_history ORDER BY id DESC LIMIT ?", n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var records []PullRecord
	for rows.Next() {
		var r PullRecord
		var at string
		if err := rows.Scan(&r.Model, &r.Backend, &at, &r.SizeBytes); err != nil {
			return nil, err
		}
		r.PulledAt, _ = time.Parse("2006-01-02 15:04:05", at)
		records = append(records, r)
	}
	return records, rows.Err()
}

// ── Reset ──

// Reset clears all state data. The schema is preserved.
func (s *StateDB) Reset() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM settings"); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE runtime_state SET backend = '', last_started = NULL, last_model = '' WHERE id = 1"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM pull_history"); err != nil {
		return err
	}
	return tx.Commit()
}
