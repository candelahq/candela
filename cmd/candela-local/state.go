package main

import (
	"database/sql"
	"fmt"
	"log/slog"
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
		CREATE TABLE IF NOT EXISTS local_model_catalog (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT DEFAULT '',
			size_hint   TEXT DEFAULT '',
			backend     TEXT DEFAULT 'ollama',
			pinned      BOOLEAN DEFAULT 0,
			added_at    TEXT DEFAULT (datetime('now'))
		);
		INSERT OR IGNORE INTO runtime_state (id) VALUES (1);
	`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("state db: migrate: %w", err)
	}

	// Seed default catalog if empty.
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM local_model_catalog").Scan(&count); err == nil && count == 0 {
		s.seedCatalog()
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
		// SQLite datetime('now') returns 'YYYY-MM-DD HH:MM:SS' in UTC.
		t, err := time.ParseInLocation("2006-01-02 15:04:05", started.String, time.UTC)
		if err != nil {
			slog.Warn("state db: failed to parse last_started", "value", started.String, "error", err)
		} else {
			rs.LastStarted = t
		}
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
		t, err := time.Parse("2006-01-02 15:04:05", at)
		if err != nil {
			slog.Warn("state db: failed to parse pulled_at", "value", at, "error", err)
		} else {
			r.PulledAt = t
		}
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

// ── Model Catalog ──

// CatalogEntry represents a model in the user's catalog.
type CatalogEntry struct {
	ID          string
	Name        string
	Description string
	SizeHint    string
	Pinned      bool
}

// seedCatalog inserts the default popular models.
func (s *StateDB) seedCatalog() {
	models := []CatalogEntry{
		{ID: "llama3.2:3b", Name: "Llama 3.2 3B", Description: "Fast, versatile small model", SizeHint: "2.0 GB"},
		{ID: "llama3.2:1b", Name: "Llama 3.2 1B", Description: "Ultra-light for quick tasks", SizeHint: "1.3 GB"},
		{ID: "gemma3:4b", Name: "Gemma 3 4B", Description: "Google's efficient model", SizeHint: "3.3 GB"},
		{ID: "qwen3:4b", Name: "Qwen 3 4B", Description: "Strong multilingual model", SizeHint: "2.6 GB"},
		{ID: "phi4-mini:3.8b", Name: "Phi-4 Mini", Description: "Microsoft's compact reasoning model", SizeHint: "2.5 GB"},
		{ID: "deepseek-r1:7b", Name: "DeepSeek R1 7B", Description: "Advanced reasoning model", SizeHint: "4.7 GB"},
		{ID: "mistral:7b", Name: "Mistral 7B", Description: "High-quality general purpose", SizeHint: "4.1 GB"},
		{ID: "codellama:7b", Name: "Code Llama 7B", Description: "Optimized for code generation", SizeHint: "3.8 GB"},
	}
	for _, m := range models {
		if _, err := s.db.Exec(
			"INSERT OR IGNORE INTO local_model_catalog (id, name, description, size_hint) VALUES (?, ?, ?, ?)",
			m.ID, m.Name, m.Description, m.SizeHint); err != nil {
			slog.Warn("state db: seed catalog entry", "id", m.ID, "error", err)
		}
	}
}

// ListCatalog returns all models in the catalog, pinned first.
func (s *StateDB) ListCatalog() []CatalogEntry {
	rows, err := s.db.Query(
		"SELECT id, name, description, size_hint, pinned FROM local_model_catalog ORDER BY pinned DESC, name ASC")
	if err != nil {
		slog.Warn("state db: list catalog", "error", err)
		return nil
	}
	defer func() { _ = rows.Close() }()

	var entries []CatalogEntry
	for rows.Next() {
		var e CatalogEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.Description, &e.SizeHint, &e.Pinned); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("state db: list catalog rows", "error", err)
	}
	return entries
}

// AddToCatalog adds a model to the catalog.
func (s *StateDB) AddToCatalog(e CatalogEntry) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO local_model_catalog (id, name, description, size_hint, pinned) VALUES (?, ?, ?, ?, ?)",
		e.ID, e.Name, e.Description, e.SizeHint, e.Pinned)
	return err
}

// RemoveFromCatalog removes a model from the catalog.
func (s *StateDB) RemoveFromCatalog(id string) error {
	_, err := s.db.Exec("DELETE FROM local_model_catalog WHERE id = ?", id)
	return err
}
