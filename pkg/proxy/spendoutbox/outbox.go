// Package spendoutbox implements a SQLite-backed durable queue for
// failed DeductSpend calls. Records are enqueued when the primary
// billing store is unreachable and retried by SpendSyncWorker.
package spendoutbox

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// sqliteTimeFormat is a fixed-width RFC3339 layout with 9 fractional digits.
// Unlike time.RFC3339Nano, this never trims trailing zeros, which ensures
// correct lexicographical ordering in SQLite text comparisons.
const sqliteTimeFormat = "2006-01-02T15:04:05.000000000Z"

// SpendRecord represents a single failed spend deduction waiting for retry.
type SpendRecord struct {
	ID           string
	UserID       string
	TaskID       string // optional: from X-Candela-Job-Id header
	CostUSD      float64
	Tokens       int64
	AttemptCount int
	CreatedAt    time.Time
}

// Outbox is a SQLite-backed durable queue for spend records.
type Outbox struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at path and initialises the
// spend_outbox table. The database is configured with WAL mode,
// synchronous=NORMAL, and a 5-second busy timeout.
func New(path string) (*Outbox, error) {
	if path == "" {
		path = "spend_outbox.db"
	}

	dsn := path
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	// Harden file permissions — billing data should be owner-only.
	if dsn != ":memory:" {
		if err := os.Chmod(dsn, 0o600); err != nil {
			slog.Warn("failed to set outbox DB permissions", "error", err)
		}
	}

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("setting pragma: %w", err)
		}
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS spend_outbox (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		task_id TEXT DEFAULT '',
		cost_usd REAL NOT NULL,
		tokens INTEGER NOT NULL,
		attempt_count INTEGER DEFAULT 0,
		created_at TEXT NOT NULL,
		next_retry_at TEXT NOT NULL
	)`)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("creating table: %w", err)
	}

	// Backward-compat migration: add task_id column if the table
	// already exists from a pre-task-budget version.
	_, _ = db.Exec(`ALTER TABLE spend_outbox ADD COLUMN task_id TEXT DEFAULT ''`)

	return &Outbox{db: db}, nil
}

// Enqueue inserts a spend record into the outbox. If rec.ID is empty, a
// random 16-byte hex ID is generated.
func (o *Outbox) Enqueue(ctx context.Context, rec SpendRecord) error {
	if rec.ID == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generating id: %w", err)
		}
		rec.ID = hex.EncodeToString(b)
	}
	if rec.CostUSD < 0 {
		return fmt.Errorf("spend_outbox: rejecting negative cost: %f", rec.CostUSD)
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	// Use the same timestamp for both created_at and next_retry_at so that
	// a Peek immediately after Enqueue always finds the record. A separate
	// time.Now() call here could race with Peek's own time.Now() at
	// nanosecond precision, causing the record to be invisible.
	createdStr := rec.CreatedAt.UTC().Format(sqliteTimeFormat)

	_, err := o.db.ExecContext(ctx,
		`INSERT INTO spend_outbox (id, user_id, task_id, cost_usd, tokens, attempt_count, created_at, next_retry_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.UserID, rec.TaskID, rec.CostUSD, rec.Tokens, rec.AttemptCount,
		createdStr,
		createdStr,
	)
	if err != nil {
		return fmt.Errorf("inserting spend record: %w", err)
	}
	return nil
}

// Peek returns up to limit records whose next_retry_at is at or before now,
// ordered by created_at ASC (oldest first).
func (o *Outbox) Peek(ctx context.Context, limit int) ([]SpendRecord, error) {
	now := time.Now().UTC().Format(sqliteTimeFormat)
	rows, err := o.db.QueryContext(ctx,
		`SELECT id, user_id, task_id, cost_usd, tokens, attempt_count, created_at
		 FROM spend_outbox
		 WHERE next_retry_at <= ?
		 ORDER BY created_at ASC LIMIT ?`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("querying spend outbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []SpendRecord
	for rows.Next() {
		var rec SpendRecord
		var createdAt string
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.TaskID, &rec.CostUSD, &rec.Tokens, &rec.AttemptCount, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning spend record: %w", err)
		}
		t, err := time.Parse(sqliteTimeFormat, createdAt)
		if err != nil {
			t = time.Time{}
		}
		rec.CreatedAt = t

		// Defense-in-depth: reject tampered records.
		if rec.CostUSD < 0 {
			slog.Error("spend_outbox: rejecting negative cost record (possible tampering)",
				"id", rec.ID, "user_id", rec.UserID, "cost_usd", rec.CostUSD)
			continue
		}
		if rec.CostUSD > 1000 { // $1000 per request is unreasonable
			slog.Error("spend_outbox: rejecting excessive cost record (possible tampering)",
				"id", rec.ID, "user_id", rec.UserID, "cost_usd", rec.CostUSD)
			continue
		}

		records = append(records, rec)
	}
	return records, rows.Err()
}

// Delete removes records by ID. IDs are chunked to stay within SQLite's
// variable limit (matching the pattern in pkg/storage/sqlite).
func (o *Outbox) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	const chunkSize = 500
	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]

		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		query := fmt.Sprintf("DELETE FROM spend_outbox WHERE id IN (%s)", placeholders)

		args := make([]any, len(chunk))
		for j, id := range chunk {
			args[j] = id
		}

		if _, err := o.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("deleting spend records: %w", err)
		}
	}
	return nil
}

// IncrementAttempt increments attempt_count for the given IDs and sets
// next_retry_at to now (immediate retry) for batch compatibility.
func (o *Outbox) IncrementAttempt(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(sqliteTimeFormat)

	const chunkSize = 500
	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]

		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		query := fmt.Sprintf(
			"UPDATE spend_outbox SET attempt_count = attempt_count + 1, next_retry_at = ? WHERE id IN (%s)",
			placeholders,
		)

		args := make([]any, 0, len(chunk)+1)
		args = append(args, now)
		for _, id := range chunk {
			args = append(args, id)
		}

		if _, err := o.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("incrementing attempt count: %w", err)
		}
	}
	return nil
}

// backoffDelay returns the retry delay for the given attempt number.
// Uses exponential backoff: 10s × 2^attempt, capped at 1 hour.
func backoffDelay(attempt int) time.Duration {
	delay := 10 * time.Second
	for i := 0; i < attempt && i < 10; i++ {
		delay *= 2
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	return delay
}

// RetryLater increments the attempt count and schedules the next retry
// with exponential backoff.
func (o *Outbox) RetryLater(ctx context.Context, id string, currentAttempt int) error {
	nextRetry := time.Now().UTC().Add(backoffDelay(currentAttempt)).Format(sqliteTimeFormat)
	_, err := o.db.ExecContext(ctx,
		`UPDATE spend_outbox SET attempt_count = attempt_count + 1, next_retry_at = ? WHERE id = ?`,
		nextRetry, id)
	return err
}

// Pending returns the total number of records in the outbox.
func (o *Outbox) Pending(ctx context.Context) (int64, error) {
	var count int64
	err := o.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM spend_outbox").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting pending records: %w", err)
	}
	return count, nil
}

// Close closes the underlying database connection.
func (o *Outbox) Close() error {
	return o.db.Close()
}
