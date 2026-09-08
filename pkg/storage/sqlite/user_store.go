// Package sqlite provides SQLite-backed storage implementations for Candela.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/candelahq/candela/pkg/billing"
	"github.com/candelahq/candela/pkg/storage"
)

const (
	defaultRateLimit = 60 // requests per minute
)

// UserStore implements storage.UserStore backed by SQLite.
// Provides complete self-hosted multi-user management, budgets,
// grants, model limits, task budgets, rate limits, and audit logging.
type UserStore struct {
	db             *sql.DB
	budgetLocation *time.Location
	mu             sync.RWMutex
}

var _ storage.UserStore = (*UserStore)(nil)

// NewUserStore creates a new SQLite-backed UserStore and initializes tables.
func NewUserStore(path string) (*UserStore, error) {
	if path == "" {
		path = "candela-users.db"
	}
	dsn := path
	if dsn == ":memory:" {
		dsn = "file::memory:?cache=shared"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: opening user db %s: %w", path, err)
	}

	// SQLite single-writer configuration
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// SQLite performance tuning & foreign keys
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("sqlite user store pragma %s: %w", pragma, err)
		}
	}

	return NewUserStoreWithDB(db)
}

// NewUserStoreWithDB initializes the schema on an existing *sql.DB.
func NewUserStoreWithDB(db *sql.DB) (*UserStore, error) {
	s := &UserStore{
		db:             db,
		budgetLocation: time.UTC,
	}

	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("sqlite user store migrate: %w", err)
	}

	return s, nil
}

// SetBudgetLocation configures the timezone for recurring budget period boundaries.
func (s *UserStore) SetBudgetLocation(loc *time.Location) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if loc == nil {
		loc = time.UTC
	}
	s.budgetLocation = loc
}

func (s *UserStore) getLocation() *time.Location {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.budgetLocation == nil {
		return time.UTC
	}
	return s.budgetLocation
}

func (s *UserStore) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			display_name TEXT,
			role TEXT NOT NULL DEFAULT 'developer',
			status TEXT NOT NULL DEFAULT 'provisioned',
			access_tags_json TEXT NOT NULL DEFAULT '[]',
			rate_limit INTEGER,
			created_at TEXT NOT NULL,
			last_seen_at TEXT,
			last_active_at TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);`,
		`CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);`,

		`CREATE TABLE IF NOT EXISTS budget_configs (
			user_id TEXT PRIMARY KEY,
			limit_usd REAL NOT NULL DEFAULT 0,
			period_type TEXT NOT NULL DEFAULT 'daily',
			soft_blocked INTEGER NOT NULL DEFAULT 0,
			soft_blocked_at TEXT,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		`CREATE TABLE IF NOT EXISTS user_budgets (
			user_id TEXT NOT NULL,
			period_key TEXT NOT NULL,
			limit_usd REAL NOT NULL DEFAULT 0,
			spent_usd REAL NOT NULL DEFAULT 0,
			tokens_used INTEGER NOT NULL DEFAULT 0,
			all_tokens_used INTEGER NOT NULL DEFAULT 0,
			period_type TEXT NOT NULL DEFAULT 'daily',
			period_start TEXT,
			period_end TEXT,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (user_id, period_key),
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_user_budgets_period ON user_budgets(user_id, period_key);`,

		`CREATE TABLE IF NOT EXISTS user_grants (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			amount_usd REAL NOT NULL DEFAULT 0,
			spent_usd REAL NOT NULL DEFAULT 0,
			reason TEXT NOT NULL DEFAULT '',
			granted_by TEXT NOT NULL DEFAULT '',
			starts_at TEXT,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_user_grants_expiry ON user_grants(user_id, expires_at);`,

		`CREATE TABLE IF NOT EXISTS user_model_limits (
			user_id TEXT NOT NULL,
			model_prefix TEXT NOT NULL,
			max_daily_usd REAL NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (user_id, model_prefix),
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		`CREATE TABLE IF NOT EXISTS task_budgets (
			task_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT '',
			limit_usd REAL NOT NULL DEFAULT 0,
			spent_usd REAL NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS rate_limits (
			window_key TEXT PRIMARY KEY,
			request_count INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS user_audit_log (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			actor_email TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			details TEXT NOT NULL DEFAULT '',
			timestamp TEXT NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_user_audit_log_user_ts ON user_audit_log(user_id, timestamp DESC);`,

		`CREATE TABLE IF NOT EXISTS global_audit_log (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			actor_email TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			details TEXT NOT NULL DEFAULT '',
			timestamp TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_global_audit_log_ts ON global_audit_log(timestamp DESC);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("executing migration query %q: %w", q, err)
		}
	}
	return nil
}

// ──────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t.UTC()
}

func normalizeID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func currentPeriodKey(periodType string, loc *time.Location, now time.Time) string {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)
	switch periodType {
	case "monthly":
		return now.Format("2006-01")
	case "weekly":
		year, week := now.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	default: // daily
		return now.Format("2006-01-02")
	}
}

// ──────────────────────────────────────────
// User CRUD
// ──────────────────────────────────────────

func (s *UserStore) CreateUser(ctx context.Context, user *storage.UserRecord) error {
	if user == nil || strings.TrimSpace(user.Email) == "" {
		return fmt.Errorf("sqlite: email is required for user creation")
	}

	user.Email = normalizeID(user.Email)
	if user.ID == "" {
		user.ID = user.Email
	} else {
		user.ID = normalizeID(user.ID)
	}

	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	if user.Status == "" {
		user.Status = storage.StatusProvisioned
	}
	if user.Role == "" {
		user.Role = storage.RoleDeveloper
	}

	tagsJSON, err := json.Marshal(user.AccessTags)
	if err != nil {
		tagsJSON = []byte("[]")
	}

	query := `INSERT INTO users (
		id, email, display_name, role, status, access_tags_json, rate_limit, created_at, last_seen_at, last_active_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.DisplayName,
		user.Role,
		user.Status,
		string(tagsJSON),
		user.RateLimit,
		formatTime(user.CreatedAt),
		formatTime(user.LastSeenAt),
		formatTime(user.LastActiveAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("sqlite: user already exists: %s", user.ID)
		}
		return fmt.Errorf("sqlite: creating user: %w", err)
	}

	return nil
}

func (s *UserStore) GetUser(ctx context.Context, id string) (*storage.UserRecord, error) {
	id = normalizeID(id)
	query := `SELECT id, email, display_name, role, status, access_tags_json, rate_limit, created_at, last_seen_at, last_active_at
		FROM users WHERE id = ? OR email = ?`

	var (
		u                          storage.UserRecord
		displayName                sql.NullString
		rateLimit                  sql.NullInt64
		tagsJSON                   string
		createdAt, seenAt, activAt string
	)

	err := s.db.QueryRowContext(ctx, query, id, id).Scan(
		&u.ID,
		&u.Email,
		&displayName,
		&u.Role,
		&u.Status,
		&tagsJSON,
		&rateLimit,
		&createdAt,
		&seenAt,
		&activAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("sqlite: user %s: %w", id, storage.ErrNotFound)
		}
		return nil, fmt.Errorf("sqlite: getting user: %w", err)
	}

	if displayName.Valid {
		u.DisplayName = &displayName.String
	}
	if rateLimit.Valid {
		rl := int(rateLimit.Int64)
		u.RateLimit = &rl
	}
	u.CreatedAt = parseTime(createdAt)
	u.LastSeenAt = parseTime(seenAt)
	u.LastActiveAt = parseTime(activAt)
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &u.AccessTags)
	}
	if u.AccessTags == nil {
		u.AccessTags = []string{}
	}

	return &u, nil
}

func (s *UserStore) GetUserByEmail(ctx context.Context, email string) (*storage.UserRecord, error) {
	return s.GetUser(ctx, email)
}

func (s *UserStore) GetUsers(ctx context.Context, ids []string) (map[string]*storage.UserRecord, error) {
	res := make(map[string]*storage.UserRecord, len(ids))
	if len(ids) == 0 {
		return res, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = normalizeID(id)
	}

	query := fmt.Sprintf(`SELECT id, email, display_name, role, status, access_tags_json, rate_limit, created_at, last_seen_at, last_active_at
		FROM users WHERE id IN (%s)`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: batch fetching users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			u                          storage.UserRecord
			displayName                sql.NullString
			rateLimit                  sql.NullInt64
			tagsJSON                   string
			createdAt, seenAt, activAt string
		)
		if err := rows.Scan(
			&u.ID,
			&u.Email,
			&displayName,
			&u.Role,
			&u.Status,
			&tagsJSON,
			&rateLimit,
			&createdAt,
			&seenAt,
			&activAt,
		); err != nil {
			continue
		}

		if displayName.Valid {
			u.DisplayName = &displayName.String
		}
		if rateLimit.Valid {
			rl := int(rateLimit.Int64)
			u.RateLimit = &rl
		}
		u.CreatedAt = parseTime(createdAt)
		u.LastSeenAt = parseTime(seenAt)
		u.LastActiveAt = parseTime(activAt)
		if tagsJSON != "" {
			_ = json.Unmarshal([]byte(tagsJSON), &u.AccessTags)
		}
		if u.AccessTags == nil {
			u.AccessTags = []string{}
		}

		res[u.ID] = &u
	}

	return res, rows.Err()
}

func (s *UserStore) ListUsers(ctx context.Context, statusFilter string, limit, offset int) ([]*storage.UserRecord, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var (
		countQuery string
		listQuery  string
		args       []any
	)

	if statusFilter != "" {
		countQuery = `SELECT COUNT(*) FROM users WHERE status = ?`
		listQuery = `SELECT id, email, display_name, role, status, access_tags_json, rate_limit, created_at, last_seen_at, last_active_at
			FROM users WHERE status = ? ORDER BY id ASC LIMIT ? OFFSET ?`
		args = []any{statusFilter}
	} else {
		countQuery = `SELECT COUNT(*) FROM users`
		listQuery = `SELECT id, email, display_name, role, status, access_tags_json, rate_limit, created_at, last_seen_at, last_active_at
			FROM users ORDER BY id ASC LIMIT ? OFFSET ?`
	}

	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("sqlite: counting users: %w", err)
	}

	listArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("sqlite: listing users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := make([]*storage.UserRecord, 0)
	for rows.Next() {
		var (
			u                          storage.UserRecord
			displayName                sql.NullString
			rateLimit                  sql.NullInt64
			tagsJSON                   string
			createdAt, seenAt, activAt string
		)
		if err := rows.Scan(
			&u.ID,
			&u.Email,
			&displayName,
			&u.Role,
			&u.Status,
			&tagsJSON,
			&rateLimit,
			&createdAt,
			&seenAt,
			&activAt,
		); err != nil {
			continue
		}

		if displayName.Valid {
			u.DisplayName = &displayName.String
		}
		if rateLimit.Valid {
			rl := int(rateLimit.Int64)
			u.RateLimit = &rl
		}
		u.CreatedAt = parseTime(createdAt)
		u.LastSeenAt = parseTime(seenAt)
		u.LastActiveAt = parseTime(activAt)
		if tagsJSON != "" {
			_ = json.Unmarshal([]byte(tagsJSON), &u.AccessTags)
		}
		if u.AccessTags == nil {
			u.AccessTags = []string{}
		}

		users = append(users, &u)
	}

	return users, total, rows.Err()
}

func (s *UserStore) UpdateUser(ctx context.Context, user *storage.UserRecord) error {
	if user == nil || user.ID == "" {
		return fmt.Errorf("sqlite: user ID is required for update")
	}

	id := normalizeID(user.ID)
	var sets []string
	var args []any

	if user.DisplayName != nil {
		sets = append(sets, "display_name = ?")
		args = append(args, *user.DisplayName)
	}
	if user.Role != "" {
		sets = append(sets, "role = ?")
		args = append(args, user.Role)
	}
	if user.Status != "" {
		sets = append(sets, "status = ?")
		args = append(args, user.Status)
	}
	if user.RateLimit != nil {
		sets = append(sets, "rate_limit = ?")
		args = append(args, *user.RateLimit)
	}
	if !user.LastSeenAt.IsZero() {
		sets = append(sets, "last_seen_at = ?")
		args = append(args, formatTime(user.LastSeenAt))
	}
	if user.AccessTags != nil {
		tagsJSON, _ := json.Marshal(user.AccessTags)
		sets = append(sets, "access_tags_json = ?")
		args = append(args, string(tagsJSON))
	}

	if len(sets) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE users SET %s WHERE id = ?", strings.Join(sets, ", "))
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("sqlite: updating user: %w", err)
	}

	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sqlite: user %s: %w", id, storage.ErrNotFound)
	}

	return nil
}

func (s *UserStore) TouchLastSeen(ctx context.Context, id string) error {
	id = normalizeID(id)
	now := formatTime(time.Now().UTC())
	query := `UPDATE users SET last_seen_at = ?, status = 'active' WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("sqlite: touching last_seen: %w", err)
	}
	return nil
}

func (s *UserStore) TouchLastActive(ctx context.Context, id string) error {
	id = normalizeID(id)
	now := formatTime(time.Now().UTC())
	query := `UPDATE users SET last_active_at = ? WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("sqlite: touching last_active: %w", err)
	}
	return nil
}

func (s *UserStore) DeleteUser(ctx context.Context, id string) error {
	id = normalizeID(id)
	query := `DELETE FROM users WHERE id = ?`
	_, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("sqlite: deleting user: %w", err)
	}
	return nil
}

// ──────────────────────────────────────────
// Budgets
// ──────────────────────────────────────────

func (s *UserStore) SetBudget(ctx context.Context, budget *storage.BudgetRecord) error {
	if budget == nil || budget.UserID == "" {
		return fmt.Errorf("sqlite: user_id is required for budget")
	}

	userID := normalizeID(budget.UserID)
	if budget.PeriodType == "" {
		budget.PeriodType = "daily"
	}

	now := time.Now().UTC()
	loc := s.getLocation()
	periodKey := currentPeriodKey(budget.PeriodType, loc, now)

	// Update or insert budget config
	configQuery := `INSERT INTO budget_configs (user_id, limit_usd, period_type, soft_blocked, updated_at)
		VALUES (?, ?, ?, 0, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			limit_usd = excluded.limit_usd,
			period_type = excluded.period_type,
			updated_at = excluded.updated_at`

	if _, err := s.db.ExecContext(ctx, configQuery, userID, budget.LimitUSD, budget.PeriodType, formatTime(now)); err != nil {
		return fmt.Errorf("sqlite: saving budget config: %w", err)
	}

	// Update limit in period spend table if row exists, or insert initial row
	spendQuery := `INSERT INTO user_budgets (
		user_id, period_key, limit_usd, spent_usd, tokens_used, all_tokens_used, period_type, updated_at
	) VALUES (?, ?, ?, 0, 0, 0, ?, ?)
	ON CONFLICT(user_id, period_key) DO UPDATE SET
		limit_usd = excluded.limit_usd,
		updated_at = excluded.updated_at`

	if _, err := s.db.ExecContext(ctx, spendQuery, userID, periodKey, budget.LimitUSD, budget.PeriodType, formatTime(now)); err != nil {
		return fmt.Errorf("sqlite: saving period budget: %w", err)
	}

	return nil
}

func (s *UserStore) GetBudget(ctx context.Context, userID string) (*storage.BudgetRecord, error) {
	userID = normalizeID(userID)
	loc := s.getLocation()
	now := time.Now().In(loc)

	// First load config doc to know the period type and configured limit
	var (
		limitUSD    float64
		periodType  string
		softBlocked int
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT limit_usd, period_type, soft_blocked FROM budget_configs WHERE user_id = ?`,
		userID,
	).Scan(&limitUSD, &periodType, &softBlocked)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No budget configured at all (matches Firestore behavior)
		}
		return nil, fmt.Errorf("sqlite: getting budget config: %w", err)
	}

	if periodType == "" {
		periodType = "daily"
	}
	periodKey := currentPeriodKey(periodType, loc, now)

	// Now check if current period record exists
	var b storage.BudgetRecord
	var spentUSD float64
	var tokensUsed, allTokensUsed int64
	var pStart, pEnd sql.NullString

	spendErr := s.db.QueryRowContext(ctx,
		`SELECT limit_usd, spent_usd, tokens_used, all_tokens_used, period_start, period_end
		 FROM user_budgets WHERE user_id = ? AND period_key = ?`,
		userID, periodKey,
	).Scan(&b.LimitUSD, &spentUSD, &tokensUsed, &allTokensUsed, &pStart, &pEnd)

	if spendErr != nil {
		if errors.Is(spendErr, sql.ErrNoRows) {
			// Auto-rollover: period doc doesn't exist yet, return config limit with 0 spend
			return &storage.BudgetRecord{
				UserID:     userID,
				LimitUSD:   limitUSD,
				SpentUSD:   0,
				TokensUsed: 0,
				PeriodType: periodType,
				PeriodKey:  periodKey,
			}, nil
		}
		return nil, fmt.Errorf("sqlite: getting period budget: %w", spendErr)
	}

	b.UserID = userID
	b.SpentUSD = spentUSD
	b.TokensUsed = tokensUsed
	b.AllTokensUsed = allTokensUsed
	b.PeriodType = periodType
	b.PeriodKey = periodKey
	if pStart.Valid {
		b.PeriodStart = parseTime(pStart.String)
	}
	if pEnd.Valid {
		b.PeriodEnd = parseTime(pEnd.String)
	}

	return &b, nil
}

func (s *UserStore) ResetSpend(ctx context.Context, userID string) error {
	userID = normalizeID(userID)
	loc := s.getLocation()
	now := time.Now().In(loc)

	var periodType string
	_ = s.db.QueryRowContext(ctx, `SELECT period_type FROM budget_configs WHERE user_id = ?`, userID).Scan(&periodType)
	if periodType == "" {
		periodType = "daily"
	}
	periodKey := currentPeriodKey(periodType, loc, now)

	// Reset spend for current period
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_budgets SET spent_usd = 0, tokens_used = 0, updated_at = ? WHERE user_id = ? AND period_key = ?`,
		formatTime(time.Now().UTC()), userID, periodKey,
	)
	if err != nil {
		return fmt.Errorf("sqlite: resetting budget spend: %w", err)
	}

	// Clear soft_blocked flag
	_, _ = s.db.ExecContext(ctx,
		`UPDATE budget_configs SET soft_blocked = 0, soft_blocked_at = NULL, updated_at = ? WHERE user_id = ?`,
		formatTime(time.Now().UTC()), userID,
	)

	return nil
}

func (s *UserStore) GetSpendHistory(ctx context.Context, userID string, days int) ([]storage.DailySpendRecord, error) {
	userID = normalizeID(userID)
	loc := s.getLocation()
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -days).Format("2006-01-02")

	query := `SELECT period_key, spent_usd, all_tokens_used FROM user_budgets
		WHERE user_id = ? AND period_key < ? AND period_key >= ? AND (spent_usd > 0 OR all_tokens_used > 0)
		ORDER BY period_key ASC`

	rows, err := s.db.QueryContext(ctx, query, userID, today, startDate)
	if err != nil {
		return nil, fmt.Errorf("sqlite: getting spend history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []storage.DailySpendRecord
	for rows.Next() {
		var r storage.DailySpendRecord
		if err := rows.Scan(&r.Date, &r.SpendUSD, &r.TokenCount); err != nil {
			continue
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// ──────────────────────────────────────────
// Grants
// ──────────────────────────────────────────

func (s *UserStore) CreateGrant(ctx context.Context, grant *storage.GrantRecord) error {
	if grant == nil || grant.UserID == "" {
		return fmt.Errorf("sqlite: user_id is required for grant")
	}

	if grant.ID == "" {
		grant.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if grant.CreatedAt.IsZero() {
		grant.CreatedAt = now
	}
	grant.UserID = normalizeID(grant.UserID)

	query := `INSERT INTO user_grants (
		id, user_id, amount_usd, spent_usd, reason, granted_by, starts_at, expires_at, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		grant.ID,
		grant.UserID,
		grant.AmountUSD,
		grant.SpentUSD,
		grant.Reason,
		grant.GrantedBy,
		formatTime(grant.StartsAt),
		formatTime(grant.ExpiresAt),
		formatTime(grant.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite: creating grant: %w", err)
	}

	return nil
}

func (s *UserStore) ListGrants(ctx context.Context, userID string, activeOnly bool) ([]*storage.GrantRecord, error) {
	userID = normalizeID(userID)
	now := time.Now().UTC()

	var query string
	var args []any
	if activeOnly {
		query = `SELECT id, user_id, amount_usd, spent_usd, reason, granted_by, starts_at, expires_at, created_at
			FROM user_grants
			WHERE user_id = ? AND expires_at > ? AND spent_usd < amount_usd
			ORDER BY expires_at ASC`
		args = []any{userID, formatTime(now)}
	} else {
		query = `SELECT id, user_id, amount_usd, spent_usd, reason, granted_by, starts_at, expires_at, created_at
			FROM user_grants
			WHERE user_id = ?
			ORDER BY created_at DESC`
		args = []any{userID}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: listing grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []*storage.GrantRecord
	for rows.Next() {
		var (
			g                   storage.GrantRecord
			starts, exp, create string
		)
		if err := rows.Scan(
			&g.ID,
			&g.UserID,
			&g.AmountUSD,
			&g.SpentUSD,
			&g.Reason,
			&g.GrantedBy,
			&starts,
			&exp,
			&create,
		); err != nil {
			continue
		}

		g.StartsAt = parseTime(starts)
		g.ExpiresAt = parseTime(exp)
		g.CreatedAt = parseTime(create)

		// Filter out future grants if activeOnly
		if activeOnly && !g.StartsAt.IsZero() && g.StartsAt.After(now) {
			continue
		}

		grants = append(grants, &g)
	}

	return grants, rows.Err()
}

func (s *UserStore) RevokeGrant(ctx context.Context, userID, grantID string) error {
	userID = normalizeID(userID)
	now := formatTime(time.Now().UTC())
	query := `UPDATE user_grants SET expires_at = ? WHERE user_id = ? AND id = ?`
	_, err := s.db.ExecContext(ctx, query, now, userID, grantID)
	if err != nil {
		return fmt.Errorf("sqlite: revoking grant: %w", err)
	}
	return nil
}

func (s *UserStore) GetGrant(ctx context.Context, userID, grantID string) (*storage.GrantRecord, error) {
	userID = normalizeID(userID)
	query := `SELECT id, user_id, amount_usd, spent_usd, reason, granted_by, starts_at, expires_at, created_at
		FROM user_grants WHERE user_id = ? AND id = ?`

	var (
		g                   storage.GrantRecord
		starts, exp, create string
	)
	err := s.db.QueryRowContext(ctx, query, userID, grantID).Scan(
		&g.ID,
		&g.UserID,
		&g.AmountUSD,
		&g.SpentUSD,
		&g.Reason,
		&g.GrantedBy,
		&starts,
		&exp,
		&create,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("sqlite: grant %s: %w", grantID, storage.ErrNotFound)
		}
		return nil, fmt.Errorf("sqlite: getting grant: %w", err)
	}

	g.StartsAt = parseTime(starts)
	g.ExpiresAt = parseTime(exp)
	g.CreatedAt = parseTime(create)

	return &g, nil
}

// ──────────────────────────────────────────
// Model Limits
// ──────────────────────────────────────────

func (s *UserStore) SetModelLimit(ctx context.Context, limit *storage.ModelLimitRecord) error {
	if limit == nil || limit.UserID == "" || limit.ModelPrefix == "" {
		return fmt.Errorf("sqlite: user_id and model_prefix are required")
	}

	now := time.Now().UTC()
	if limit.CreatedAt.IsZero() {
		limit.CreatedAt = now
	}
	limit.UpdatedAt = now
	limit.UserID = normalizeID(limit.UserID)

	query := `INSERT INTO user_model_limits (user_id, model_prefix, max_daily_usd, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, model_prefix) DO UPDATE SET
			max_daily_usd = excluded.max_daily_usd,
			updated_at = excluded.updated_at`

	_, err := s.db.ExecContext(ctx, query,
		limit.UserID,
		limit.ModelPrefix,
		limit.MaxDailyUSD,
		formatTime(limit.CreatedAt),
		formatTime(limit.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite: setting model limit: %w", err)
	}

	return nil
}

func (s *UserStore) GetModelLimits(ctx context.Context, userID string) ([]*storage.ModelLimitRecord, error) {
	userID = normalizeID(userID)
	query := `SELECT user_id, model_prefix, max_daily_usd, created_at, updated_at
		FROM user_model_limits WHERE user_id = ? ORDER BY model_prefix ASC`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: getting model limits: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var limits []*storage.ModelLimitRecord
	for rows.Next() {
		var (
			l                storage.ModelLimitRecord
			created, updated string
		)
		if err := rows.Scan(&l.UserID, &l.ModelPrefix, &l.MaxDailyUSD, &created, &updated); err != nil {
			continue
		}
		l.CreatedAt = parseTime(created)
		l.UpdatedAt = parseTime(updated)
		limits = append(limits, &l)
	}

	return limits, rows.Err()
}

func (s *UserStore) DeleteModelLimit(ctx context.Context, userID, modelPrefix string) error {
	userID = normalizeID(userID)
	query := `DELETE FROM user_model_limits WHERE user_id = ? AND model_prefix = ?`
	_, err := s.db.ExecContext(ctx, query, userID, modelPrefix)
	if err != nil {
		return fmt.Errorf("sqlite: deleting model limit: %w", err)
	}
	return nil
}

// ──────────────────────────────────────────
// Budget Enforcement
// ──────────────────────────────────────────

func (s *UserStore) CheckBudget(ctx context.Context, userID string, estimatedCostUSD float64) (*storage.BudgetCheckResult, error) {
	userID = normalizeID(userID)

	// 1. Check user status (fail-closed if inactive)
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("sqlite: verifying user status: %w", err)
		}
	} else if user != nil && user.Status == storage.StatusInactive {
		return &storage.BudgetCheckResult{
			Allowed:       false,
			Reason:        billing.ReasonNoBudget,
			RemainingUSD:  0,
			EstimatedCost: estimatedCostUSD,
		}, nil
	}

	// 2. Check soft_blocked flag
	var softBlocked int
	_ = s.db.QueryRowContext(ctx, `SELECT soft_blocked FROM budget_configs WHERE user_id = ?`, userID).Scan(&softBlocked)
	if softBlocked != 0 {
		return &storage.BudgetCheckResult{
			Allowed:       false,
			Reason:        billing.ReasonSoftBlocked,
			RemainingUSD:  0,
			EstimatedCost: estimatedCostUSD,
		}, nil
	}

	// 3. Sum remaining grants
	grants, err := s.ListGrants(ctx, userID, true)
	if err != nil {
		return nil, err
	}
	var grantsRemaining float64
	for _, g := range grants {
		grantsRemaining += g.Remaining()
	}

	// 4. Get recurring budget remaining
	var budgetRemaining float64
	budget, err := s.GetBudget(ctx, userID)
	if err != nil {
		return nil, err
	}
	if budget != nil {
		budgetRemaining = budget.LimitUSD - budget.SpentUSD
		if budgetRemaining < 0 {
			budgetRemaining = 0
		}
	}

	totalRemaining := grantsRemaining + budgetRemaining
	allowed := totalRemaining >= estimatedCostUSD
	reason := billing.ReasonAllowed
	if !allowed {
		if budget == nil && grantsRemaining == 0 {
			reason = billing.ReasonNoBudget
		} else {
			reason = billing.ReasonBudgetExhausted
		}
	}

	return &storage.BudgetCheckResult{
		Allowed:       allowed,
		Reason:        reason,
		RemainingUSD:  totalRemaining,
		GrantsUSD:     grantsRemaining,
		BudgetUSD:     budgetRemaining,
		EstimatedCost: estimatedCostUSD,
	}, nil
}

func (s *UserStore) DeductSpend(ctx context.Context, userID string, costUSD float64, tokens int64) error {
	if costUSD < 0 {
		return fmt.Errorf("sqlite: negative cost not allowed: %f", costUSD)
	}
	if costUSD == 0 && tokens == 0 {
		return nil
	}

	userID = normalizeID(userID)
	loc := s.getLocation()
	now := time.Now().In(loc)

	// Transaction with timeout
	txCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(txCtx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: starting deduct tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Read budget config
	var (
		limitUSD   float64
		periodType string
	)
	hasConfig := true
	configErr := tx.QueryRowContext(txCtx,
		`SELECT limit_usd, period_type FROM budget_configs WHERE user_id = ?`,
		userID,
	).Scan(&limitUSD, &periodType)

	if configErr != nil {
		if errors.Is(configErr, sql.ErrNoRows) {
			hasConfig = false
		} else {
			return fmt.Errorf("sqlite: reading budget config in tx: %w", configErr)
		}
	}

	if periodType == "" {
		periodType = "daily"
	}
	periodKey := currentPeriodKey(periodType, loc, now)

	// 2. Read active grants (ordered by earliest expiry)
	grantsRows, err := tx.QueryContext(txCtx,
		`SELECT id, amount_usd, spent_usd, starts_at, expires_at
		 FROM user_grants
		 WHERE user_id = ? AND expires_at > ? AND spent_usd < amount_usd
		 ORDER BY expires_at ASC`,
		userID, formatTime(now.UTC()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: reading grants in tx: %w", err)
	}

	type activeGrant struct {
		id        string
		amountUSD float64
		spentUSD  float64
		startsAt  time.Time
		expiresAt time.Time
	}
	var grants []activeGrant
	for grantsRows.Next() {
		var g activeGrant
		var starts, exp string
		if err := grantsRows.Scan(&g.id, &g.amountUSD, &g.spentUSD, &starts, &exp); err == nil {
			g.startsAt = parseTime(starts)
			g.expiresAt = parseTime(exp)
			if g.startsAt.IsZero() || !g.startsAt.After(now.UTC()) {
				grants = append(grants, g)
			}
		}
	}
	_ = grantsRows.Close()

	// 3. Read current period spend
	var (
		currentSpentUSD float64
		tokensUsed      int64
		allTokensUsed   int64
	)
	hasPeriod := true
	periodErr := tx.QueryRowContext(txCtx,
		`SELECT spent_usd, tokens_used, all_tokens_used FROM user_budgets WHERE user_id = ? AND period_key = ?`,
		userID, periodKey,
	).Scan(&currentSpentUSD, &tokensUsed, &allTokensUsed)

	if periodErr != nil {
		if errors.Is(periodErr, sql.ErrNoRows) {
			hasPeriod = false
		} else {
			return fmt.Errorf("sqlite: reading period budget in tx: %w", periodErr)
		}
	}

	remaining := costUSD

	// Write 1: Deduct from recurring budget first
	if hasConfig || hasPeriod {
		budgetAvailable := limitUSD - currentSpentUSD
		if budgetAvailable < 0 {
			budgetAvailable = 0
		}
		budgetDeduct := min(remaining, budgetAvailable)

		var grantCoverage float64
		for _, g := range grants {
			rem := g.amountUSD - g.spentUSD
			if rem > 0 {
				grantCoverage += rem
			}
		}

		overflow := math.Max(0, costUSD-budgetDeduct)
		unabsorbed := math.Max(0, overflow-grantCoverage)
		budgetCharge := budgetDeduct + unabsorbed

		budgetTokens := tokens
		if costUSD > 0 && budgetCharge < costUSD {
			budgetTokens = int64(math.Round(float64(tokens) * (budgetCharge / costUSD)))
		}

		nowStr := formatTime(now.UTC())
		if !hasPeriod {
			insertPeriod := `INSERT INTO user_budgets (
				user_id, period_key, limit_usd, spent_usd, tokens_used, all_tokens_used, period_type, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
			if _, err := tx.ExecContext(txCtx, insertPeriod,
				userID, periodKey, limitUSD, budgetCharge, budgetTokens, tokens, periodType, nowStr,
			); err != nil {
				return fmt.Errorf("sqlite: creating period spend in tx: %w", err)
			}
		} else {
			updatePeriod := `UPDATE user_budgets SET
				spent_usd = spent_usd + ?,
				tokens_used = tokens_used + ?,
				all_tokens_used = all_tokens_used + ?,
				updated_at = ?
				WHERE user_id = ? AND period_key = ?`
			if _, err := tx.ExecContext(txCtx, updatePeriod,
				budgetCharge, budgetTokens, tokens, nowStr, userID, periodKey,
			); err != nil {
				return fmt.Errorf("sqlite: updating period spend in tx: %w", err)
			}
		}

		remaining -= budgetCharge
	}

	// Write 2: Overflow into active grants (earliest-expiring first)
	for _, g := range grants {
		if remaining <= 0 {
			break
		}
		available := g.amountUSD - g.spentUSD
		if available <= 0 {
			continue
		}
		deduct := min(remaining, available)
		_, err := tx.ExecContext(txCtx,
			`UPDATE user_grants SET spent_usd = spent_usd + ? WHERE id = ?`,
			deduct, g.id,
		)
		if err != nil {
			return fmt.Errorf("sqlite: updating grant spend in tx: %w", err)
		}
		remaining -= deduct
	}

	// 4. Overdraft soft-blocking
	if remaining > 0.000001 {
		slog.Warn("deduct_spend: unabsorbed cost — balance exhausted by concurrent request; user soft-blocked",
			"user_id", userID,
			"total_cost_usd", costUSD,
			"unabsorbed_usd", remaining,
		)
		nowStr := formatTime(now.UTC())
		_, _ = tx.ExecContext(txCtx,
			`UPDATE budget_configs SET soft_blocked = 1, soft_blocked_at = ?, updated_at = ? WHERE user_id = ?`,
			nowStr, nowStr, userID,
		)
	}

	return tx.Commit()
}

// ──────────────────────────────────────────
// Task Budgets
// ──────────────────────────────────────────

func (s *UserStore) CreateTaskBudget(ctx context.Context, budget *storage.TaskBudget) error {
	if budget == nil {
		return fmt.Errorf("sqlite: budget is nil")
	}
	if err := budget.Validate(); err != nil {
		return fmt.Errorf("sqlite: invalid task budget: %w", err)
	}

	now := time.Now().UTC()
	if budget.CreatedAt.IsZero() {
		budget.CreatedAt = now
	}

	query := `INSERT INTO task_budgets (task_id, user_id, limit_usd, spent_usd, created_at, expires_at)
		VALUES (?, ?, ?, 0.0, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		budget.TaskID,
		normalizeID(budget.UserID),
		budget.LimitUSD,
		formatTime(budget.CreatedAt),
		formatTime(budget.ExpiresAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return fmt.Errorf("sqlite: task budget already exists: %s", budget.TaskID)
		}
		return fmt.Errorf("sqlite: creating task budget: %w", err)
	}

	return nil
}

func (s *UserStore) GetTaskBudget(ctx context.Context, taskID string) (*storage.TaskBudget, error) {
	if taskID == "" {
		return nil, fmt.Errorf("sqlite: task_id is required")
	}

	query := `SELECT task_id, user_id, limit_usd, spent_usd, created_at, expires_at FROM task_budgets WHERE task_id = ?`
	var (
		b          storage.TaskBudget
		createdStr string
		expiresStr string
	)

	err := s.db.QueryRowContext(ctx, query, taskID).Scan(
		&b.TaskID,
		&b.UserID,
		&b.LimitUSD,
		&b.SpentUSD,
		&createdStr,
		&expiresStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("sqlite: getting task budget: %w", err)
	}

	b.CreatedAt = parseTime(createdStr)
	b.ExpiresAt = parseTime(expiresStr)

	return &b, nil
}

func (s *UserStore) DeleteTaskBudget(ctx context.Context, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("sqlite: task_id is required")
	}

	query := `DELETE FROM task_budgets WHERE task_id = ?`
	_, err := s.db.ExecContext(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("sqlite: deleting task budget: %w", err)
	}
	return nil
}

func (s *UserStore) CheckTaskBudget(ctx context.Context, taskID string, estimatedCostUSD float64) (*storage.TaskBudgetCheckResult, error) {
	budget, err := s.GetTaskBudget(ctx, taskID)
	if err != nil {
		return nil, err
	}

	if budget.IsExpired() {
		return &billing.TaskBudgetCheckResult{
			Allowed:      false,
			Reason:       billing.ReasonTaskBudgetExpired,
			RemainingUSD: 0,
			LimitUSD:     budget.LimitUSD,
			SpentUSD:     budget.SpentUSD,
		}, nil
	}

	remaining := budget.Remaining()
	if remaining < estimatedCostUSD {
		return &billing.TaskBudgetCheckResult{
			Allowed:      false,
			Reason:       billing.ReasonTaskBudgetExhausted,
			RemainingUSD: remaining,
			LimitUSD:     budget.LimitUSD,
			SpentUSD:     budget.SpentUSD,
		}, nil
	}

	return &billing.TaskBudgetCheckResult{
		Allowed:      true,
		RemainingUSD: remaining,
		LimitUSD:     budget.LimitUSD,
		SpentUSD:     budget.SpentUSD,
	}, nil
}

func (s *UserStore) DeductTaskSpend(ctx context.Context, taskID string, costUSD float64) error {
	if taskID == "" {
		return fmt.Errorf("sqlite: task_id is required")
	}
	if costUSD < 0 {
		return fmt.Errorf("sqlite: cost must be non-negative, got %.6f", costUSD)
	}
	if costUSD == 0 {
		return nil
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE task_budgets SET spent_usd = spent_usd + ? WHERE task_id = ?`,
		costUSD, taskID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: deducting task spend: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sqlite: no budget for task %q", taskID)
	}
	return nil
}

// ──────────────────────────────────────────
// Rate Limiting
// ──────────────────────────────────────────

func (s *UserStore) CheckRateLimit(ctx context.Context, userID string) (bool, int, int, error) {
	userID = normalizeID(userID)

	// Resolve user's configured rate limit
	limit := defaultRateLimit
	user, err := s.GetUser(ctx, userID)
	if err == nil && user != nil && user.RateLimit != nil && *user.RateLimit > 0 {
		limit = *user.RateLimit
	}

	now := time.Now().UTC()
	windowKey := fmt.Sprintf("%s:%s", userID, now.Format("2006-01-02T15:04"))

	query := `INSERT INTO rate_limits (window_key, request_count, updated_at)
		VALUES (?, 1, ?)
		ON CONFLICT(window_key) DO UPDATE SET
			request_count = request_count + 1,
			updated_at = excluded.updated_at
		RETURNING request_count`

	var count int
	err = s.db.QueryRowContext(ctx, query, windowKey, formatTime(now)).Scan(&count)
	if err != nil {
		return false, 0, 0, fmt.Errorf("sqlite: checking rate limit: %w", err)
	}

	allowed := count <= limit
	return allowed, count, limit, nil
}

// ──────────────────────────────────────────
// Audit Log
// ──────────────────────────────────────────

func (s *UserStore) LogAction(ctx context.Context, entry *storage.AuditRecord) error {
	if entry == nil {
		return nil
	}
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	query := `INSERT INTO user_audit_log (id, user_id, actor_email, action, details, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		entry.ID,
		normalizeID(entry.UserID),
		entry.ActorEmail,
		entry.Action,
		entry.Details,
		formatTime(entry.Timestamp),
	)
	if err != nil {
		return fmt.Errorf("sqlite: logging user audit: %w", err)
	}
	return nil
}

func (s *UserStore) LogGlobalAction(ctx context.Context, entry *storage.AuditRecord) error {
	if entry == nil {
		return nil
	}
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	query := `INSERT INTO global_audit_log (id, user_id, actor_email, action, details, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		entry.ID,
		normalizeID(entry.UserID),
		entry.ActorEmail,
		entry.Action,
		entry.Details,
		formatTime(entry.Timestamp),
	)
	if err != nil {
		return fmt.Errorf("sqlite: logging global audit: %w", err)
	}
	return nil
}

func (s *UserStore) ListAuditLog(ctx context.Context, userID string, limit int) ([]*storage.AuditRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	userID = normalizeID(userID)

	query := `SELECT id, user_id, actor_email, action, details, timestamp
		FROM user_audit_log WHERE user_id = ? ORDER BY timestamp DESC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: listing audit log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []*storage.AuditRecord
	for rows.Next() {
		var r storage.AuditRecord
		var ts string
		if err := rows.Scan(&r.ID, &r.UserID, &r.ActorEmail, &r.Action, &r.Details, &ts); err != nil {
			continue
		}
		r.Timestamp = parseTime(ts)
		records = append(records, &r)
	}

	return records, rows.Err()
}

func (s *UserStore) Close() error {
	return s.db.Close()
}
