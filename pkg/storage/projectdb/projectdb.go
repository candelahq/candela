// Package projectdb implements storage.ProjectStore using SQLite.
// This is intentionally a separate database file from the span store
// because project/key metadata is relational (ACID, foreign keys)
// while span data is OLAP-optimized.
package projectdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"

	"github.com/candelahq/candela/pkg/storage"
)

// Store implements storage.ProjectStore backed by SQLite.
type Store struct {
	db *sql.DB
}

var _ storage.ProjectStore = (*Store)(nil)

// New creates a new ProjectStore and ensures the schema exists.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("projectdb: open %s: %w", path, err)
	}

	// Enable WAL mode for concurrent reads.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("projectdb: WAL mode: %w", err)
	}

	// Enable foreign key enforcement (required for CASCADE).
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("projectdb: foreign keys: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("projectdb: migrate: %w", err)
	}

	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		environment TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS api_keys (
		id         TEXT PRIMARY KEY,
		project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		name       TEXT NOT NULL,
		key_hash   TEXT NOT NULL,
		key_prefix TEXT NOT NULL,
		active     INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL DEFAULT ''
	);

	CREATE INDEX IF NOT EXISTS idx_api_keys_project ON api_keys(project_id);
	CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);
	`
	_, err := db.Exec(schema)
	return err
}

// --- Project CRUD ---

func (s *Store) CreateProject(ctx context.Context, p storage.Project) (*storage.Project, error) {
	if p.ID == "" {
		p.ID = generateID()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, description, environment, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, p.Environment,
		p.CreatedAt.Format(time.RFC3339), p.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("projectdb: create project: %w", err)
	}
	return &p, nil
}

func (s *Store) GetProject(ctx context.Context, id string) (*storage.Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, environment, created_at, updated_at
		 FROM projects WHERE id = ?`, id)

	var p storage.Project
	var createdAt, updatedAt string
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Environment, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("projectdb: project %q not found", id)
		}
		return nil, fmt.Errorf("projectdb: get project: %w", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &p, nil
}

func (s *Store) ListProjects(ctx context.Context, limit, offset int) ([]storage.Project, int, error) {
	if limit <= 0 {
		limit = 50
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM projects").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("projectdb: count projects: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, environment, created_at, updated_at
		 FROM projects ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("projectdb: list projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var projects []storage.Project
	for rows.Next() {
		var p storage.Project
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Environment, &createdAt, &updatedAt); err != nil {
			return nil, 0, fmt.Errorf("projectdb: scan project: %w", err)
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		projects = append(projects, p)
	}
	return projects, total, rows.Err()
}

func (s *Store) UpdateProject(ctx context.Context, p storage.Project) (*storage.Project, error) {
	p.UpdatedAt = time.Now().UTC()

	result, err := s.db.ExecContext(ctx,
		`UPDATE projects SET name = ?, description = ?, environment = ?, updated_at = ?
		 WHERE id = ?`,
		p.Name, p.Description, p.Environment, p.UpdatedAt.Format(time.RFC3339), p.ID)
	if err != nil {
		return nil, fmt.Errorf("projectdb: update project: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("projectdb: project %q not found", p.ID)
	}
	return s.GetProject(ctx, p.ID)
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("projectdb: delete project: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("projectdb: project %q not found", id)
	}
	return nil
}

// --- API Key Management ---

func (s *Store) CreateAPIKey(ctx context.Context, key storage.APIKey, fullKey string) (*storage.APIKey, error) {
	if key.ID == "" {
		key.ID = generateID()
	}
	now := time.Now().UTC()
	key.CreatedAt = now
	key.Active = true

	// Hash the full key.
	hash, err := bcrypt.GenerateFromPassword([]byte(fullKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("projectdb: hash key: %w", err)
	}
	key.KeyHash = string(hash)
	key.KeyPrefix = fullKey[:8]

	expiresAt := ""
	if !key.ExpiresAt.IsZero() {
		expiresAt = key.ExpiresAt.Format(time.RFC3339)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, project_id, name, key_hash, key_prefix, active, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		key.ID, key.ProjectID, key.Name, key.KeyHash, key.KeyPrefix,
		key.CreatedAt.Format(time.RFC3339), expiresAt)
	if err != nil {
		return nil, fmt.Errorf("projectdb: create api key: %w", err)
	}
	return &key, nil
}

func (s *Store) ListAPIKeys(ctx context.Context, projectID string) ([]storage.APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, key_prefix, active, created_at, expires_at
		 FROM api_keys WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("projectdb: list api keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []storage.APIKey
	for rows.Next() {
		var k storage.APIKey
		var createdAt, expiresAt string
		var active int
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyPrefix, &active, &createdAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("projectdb: scan api key: %w", err)
		}
		k.Active = active == 1
		k.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if expiresAt != "" {
			k.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) RevokeAPIKey(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE api_keys SET active = 0 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("projectdb: revoke api key: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("projectdb: api key %q not found", id)
	}
	return nil
}

func (s *Store) ValidateAPIKey(ctx context.Context, rawKey string) (*storage.APIKey, error) {
	if len(rawKey) < 8 {
		return nil, fmt.Errorf("projectdb: invalid key format")
	}
	prefix := rawKey[:8]

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, key_hash, key_prefix, active, created_at, expires_at
		 FROM api_keys WHERE key_prefix = ? AND active = 1`, prefix)
	if err != nil {
		return nil, fmt.Errorf("projectdb: validate key: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var k storage.APIKey
		var createdAt, expiresAt string
		var active int
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.KeyPrefix, &active, &createdAt, &expiresAt); err != nil {
			continue
		}
		k.Active = active == 1
		k.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if expiresAt != "" {
			k.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
			if time.Now().After(k.ExpiresAt) {
				continue // expired
			}
		}

		// Check bcrypt hash.
		if err := bcrypt.CompareHashAndPassword([]byte(k.KeyHash), []byte(rawKey)); err == nil {
			return &k, nil
		}
	}
	return nil, fmt.Errorf("projectdb: invalid api key")
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Helpers ---

// GenerateAPIKey creates a cryptographically secure API key.
// Format: cdla_<32 hex chars> (40 chars total).
func GenerateAPIKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "cdla_" + hex.EncodeToString(b)
}

func generateID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
