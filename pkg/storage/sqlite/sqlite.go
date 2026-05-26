// Package sqlite implements the storage.TraceStore interface using SQLite.
// This is the default backend for local development — zero external dependencies.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/candelahq/candela/pkg/storage"
)

// Store implements storage.TraceStore for SQLite.
type Store struct {
	db *sql.DB
}

var _ storage.TraceStore = (*Store)(nil)

// Config holds SQLite connection settings.
type Config struct {
	Path string `yaml:"path" json:"path"` // e.g. "candela.db" or ":memory:"
}

// New creates a new SQLite-backed TraceStore.
func New(cfg Config) (*Store, error) {
	if cfg.Path == "" {
		cfg.Path = "candela.db"
	}

	dsn := cfg.Path
	if dsn == ":memory:" {
		dsn = "file::memory:?cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	// SQLite does not support concurrent writers; constraining to 1 connection
	// also ensures :memory: databases share a single in-memory instance.
	db.SetMaxOpenConns(1)

	// SQLite performance tuning.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000", // 64MB cache
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("setting pragma: %w", err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return s, nil
}

func (s *Store) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS spans (
			span_id        TEXT NOT NULL,
			trace_id       TEXT NOT NULL,
			parent_span_id TEXT DEFAULT '',
			name           TEXT NOT NULL,
			kind           INTEGER DEFAULT 0,
			status         INTEGER DEFAULT 0,
			status_message TEXT DEFAULT '',
			start_time     TEXT NOT NULL,
			end_time       TEXT NOT NULL,
			duration_ns    INTEGER DEFAULT 0,
			project_id     TEXT DEFAULT '',
			environment    TEXT DEFAULT '',
			service_name   TEXT DEFAULT '',

			gen_ai_model          TEXT DEFAULT '',
			gen_ai_provider       TEXT DEFAULT '',
			gen_ai_input_tokens   INTEGER DEFAULT 0,
			gen_ai_output_tokens  INTEGER DEFAULT 0,
			gen_ai_total_tokens   INTEGER DEFAULT 0,
			gen_ai_cost_usd       REAL DEFAULT 0,
			gen_ai_temperature    REAL DEFAULT 0,
			gen_ai_max_tokens     INTEGER DEFAULT 0,
			gen_ai_input_content  TEXT DEFAULT '',
			gen_ai_output_content TEXT DEFAULT '',

			attributes_json TEXT DEFAULT '{}',

			session_id     TEXT DEFAULT '',

			PRIMARY KEY (trace_id, span_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_project_time ON spans(project_id, start_time)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_trace ON spans(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_kind ON spans(kind)`,
		// Migration: add user_id column to existing tables.
		`ALTER TABLE spans ADD COLUMN user_id TEXT DEFAULT ''`,
		// Migration: add session_id column to existing tables.
		`ALTER TABLE spans ADD COLUMN session_id TEXT DEFAULT ''`,
		// Migration: add tenant_id for multitenant cost attribution.
		`ALTER TABLE spans ADD COLUMN tenant_id TEXT DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_spans_tenant ON spans(tenant_id)`,
		// Migration: add job_id for job-level cost attribution.
		`ALTER TABLE spans ADD COLUMN job_id TEXT DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_spans_job ON spans(job_id)`,
		// Migration: add cache token columns for prompt caching visibility.
		`ALTER TABLE spans ADD COLUMN gen_ai_cache_read_tokens INTEGER DEFAULT 0`,
		`ALTER TABLE spans ADD COLUMN gen_ai_cache_creation_tokens INTEGER DEFAULT 0`,
		// Sync Outbox
		`CREATE TABLE IF NOT EXISTS outbox_spans (
			span_id TEXT PRIMARY KEY,
			payload_json TEXT NOT NULL,
			attempt_count INTEGER DEFAULT 0,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			// Ignore "duplicate column" errors from migration ALTERs.
			if !isDuplicateColumn(err) {
				return fmt.Errorf("executing migration: %w", err)
			}
		}
	}
	return nil
}

func (s *Store) IngestSpans(ctx context.Context, spans []storage.Span) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO spans (
		span_id, trace_id, parent_span_id, name, kind, status, status_message,
		start_time, end_time, duration_ns, project_id, environment, service_name,
		gen_ai_model, gen_ai_provider, gen_ai_input_tokens, gen_ai_output_tokens,
		gen_ai_total_tokens, gen_ai_cost_usd, gen_ai_temperature, gen_ai_max_tokens,
		gen_ai_input_content, gen_ai_output_content, attributes_json, user_id, session_id, tenant_id, job_id,
		gen_ai_cache_read_tokens, gen_ai_cache_creation_tokens
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing spans stmt: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	outboxStmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO outbox_spans (
		span_id, payload_json, attempt_count, created_at
	) VALUES (?, ?, 0, ?)`)
	if err != nil {
		return fmt.Errorf("preparing outbox stmt: %w", err)
	}
	defer func() { _ = outboxStmt.Close() }()

	for _, span := range spans {
		genAI := span.GenAI
		if genAI == nil {
			genAI = &storage.GenAIAttributes{}
		}

		attrsJSON, err := json.Marshal(span.Attributes)
		if err != nil {
			return fmt.Errorf("marshaling attributes for span %s: %w", span.SpanID, err)
		}

		_, err = stmt.ExecContext(ctx,
			span.SpanID, span.TraceID, span.ParentSpanID,
			span.Name, int(span.Kind), int(span.Status), span.StatusMessage,
			span.StartTime.Format(time.RFC3339Nano),
			span.EndTime.Format(time.RFC3339Nano),
			span.Duration.Nanoseconds(),
			span.ProjectID, span.Environment, span.ServiceName,
			genAI.Model, genAI.Provider,
			genAI.InputTokens, genAI.OutputTokens, genAI.TotalTokens,
			genAI.CostUSD, genAI.Temperature, genAI.MaxTokens,
			genAI.InputContent, genAI.OutputContent,
			string(attrsJSON),
			span.UserID,
			span.SessionID,
			span.TenantID,
			span.JobID,
			genAI.CacheReadTokens, genAI.CacheCreationTokens,
		)
		if err != nil {
			return fmt.Errorf("inserting span: %w", err)
		}

		// Also write to sync outbox
		payloadBytes, err := json.Marshal(span)
		if err != nil {
			return fmt.Errorf("marshaling span for outbox %s: %w", span.SpanID, err)
		}
		_, err = outboxStmt.ExecContext(ctx, span.SpanID, string(payloadBytes), time.Now().Format(time.RFC3339Nano))
		if err != nil {
			return fmt.Errorf("inserting to outbox: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetTrace(ctx context.Context, traceID string) (*storage.Trace, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT span_id, trace_id, parent_span_id, name, kind, status, status_message,
			start_time, end_time, duration_ns, project_id, environment, service_name,
			gen_ai_model, gen_ai_provider, gen_ai_input_tokens, gen_ai_output_tokens,
			gen_ai_total_tokens, gen_ai_cost_usd, gen_ai_temperature, gen_ai_max_tokens,
			gen_ai_input_content, gen_ai_output_content, attributes_json, user_id, session_id, tenant_id, job_id
		FROM spans WHERE trace_id = ? ORDER BY start_time ASC
	`, traceID)
	if err != nil {
		return nil, fmt.Errorf("querying spans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	spans, err := scanSpans(rows)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return nil, fmt.Errorf("trace %s: %w", traceID, storage.ErrNotFound)
	}

	return buildTrace(traceID, spans), nil
}

func (s *Store) QueryTraces(ctx context.Context, q storage.TraceQuery) (*storage.TraceResult, error) {
	if q.PageSize == 0 {
		q.PageSize = 50
	}

	// Allowlisted ORDER BY columns — never interpolate user input directly.
	orderCols := map[string]string{
		"start_time":   "MIN(start_time)",
		"total_tokens": "SUM(gen_ai_total_tokens)",
		"total_cost":   "SUM(gen_ai_cost_usd)",
		"duration":     "(MAX(end_time) - MIN(start_time))",
		"span_count":   "COUNT(*)",
	}
	orderExpr, ok := orderCols[q.OrderBy]
	if !ok {
		orderExpr = "MIN(start_time)"
	}
	// Default to DESC (most recent first). Only go ASC when the caller
	// explicitly requests ascending order AND has named a sort column.
	dir := "DESC"
	if ok && !q.Descending {
		dir = "ASC"
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			trace_id,
			MIN(start_time) as start_time,
			MAX(end_time) as end_time,
			COUNT(*) as span_count,
			SUM(CASE WHEN kind = 1 THEN 1 ELSE 0 END) as llm_count,
			SUM(gen_ai_total_tokens) as total_tokens,
			SUM(gen_ai_cost_usd) as total_cost,
			MAX(CASE WHEN parent_span_id = '' THEN name ELSE '' END) as root_name,
			COALESCE((
				SELECT s2.gen_ai_model FROM spans s2
				WHERE s2.trace_id = spans.trace_id
					AND (? = '' OR s2.project_id = ?)
					AND s2.gen_ai_model != ''
				GROUP BY s2.gen_ai_model
				ORDER BY SUM(s2.gen_ai_cost_usd) DESC
				LIMIT 1
			), '') as primary_model,
			COALESCE((
				SELECT s2.gen_ai_provider FROM spans s2
				WHERE s2.trace_id = spans.trace_id
					AND (? = '' OR s2.project_id = ?)
					AND s2.gen_ai_model != ''
				GROUP BY s2.gen_ai_model, s2.gen_ai_provider
				ORDER BY SUM(s2.gen_ai_cost_usd) DESC
				LIMIT 1
			), '') as primary_provider,
			MAX(CASE WHEN status = 2 THEN 2 ELSE 0 END) as status
		FROM spans
		WHERE (? = '' OR project_id = ?) AND start_time >= ? AND start_time <= ?
			AND (? = '' OR user_id = ?)
			AND (? = '' OR tenant_id = ?)
			AND (? = '' OR environment = ?)
		GROUP BY trace_id
		ORDER BY `+orderExpr+` `+dir+`
		LIMIT ?
	`, q.ProjectID, q.ProjectID, // subquery 1: primary_model
		q.ProjectID, q.ProjectID, // subquery 2: primary_provider
		q.ProjectID, q.ProjectID, // WHERE clause
		q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano),
		q.UserID, q.UserID, q.TenantID, q.TenantID, q.Environment, q.Environment, q.PageSize)
	if err != nil {
		return nil, fmt.Errorf("querying traces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var traces []storage.TraceSummary
	for rows.Next() {
		var t storage.TraceSummary
		var startStr, endStr string
		var status int

		err := rows.Scan(
			&t.TraceID, &startStr, &endStr, &t.SpanCount, &t.LLMCallCount,
			&t.TotalTokens, &t.TotalCostUSD, &t.RootSpanName,
			&t.PrimaryModel, &t.PrimaryProvider, &status,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning trace: %w", err)
		}

		t.StartTime, err = time.Parse(time.RFC3339Nano, startStr)
		if err != nil {
			t.StartTime = time.Time{}
		}

		end, err := time.Parse(time.RFC3339Nano, endStr)
		if err != nil {
			end = time.Time{}
		}
		t.Duration = end.Sub(t.StartTime)
		t.Status = storage.SpanStatus(status)
		t.ProjectID = q.ProjectID
		traces = append(traces, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating traces: %w", err)
	}

	return &storage.TraceResult{Traces: traces, TotalCount: len(traces)}, nil
}

func (s *Store) SearchSpans(ctx context.Context, q storage.SpanQuery) (*storage.SpanResult, error) {
	if q.PageSize == 0 {
		q.PageSize = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT span_id, trace_id, parent_span_id, name, kind, status, status_message,
			start_time, end_time, duration_ns, project_id, environment, service_name,
			gen_ai_model, gen_ai_provider, gen_ai_input_tokens, gen_ai_output_tokens,
			gen_ai_total_tokens, gen_ai_cost_usd, gen_ai_temperature, gen_ai_max_tokens,
			gen_ai_input_content, gen_ai_output_content, attributes_json, user_id, session_id, tenant_id, job_id
		FROM spans
		WHERE (? = '' OR project_id = ?) AND start_time >= ? AND start_time <= ?
			AND (? = 0 OR kind = ?)
			AND (? = '' OR gen_ai_model = ?)
			AND (? = '' OR name LIKE '%' || ? || '%' ESCAPE '\')
			AND (? = '' OR user_id = ?)
			AND (? = '' OR tenant_id = ?)
		ORDER BY start_time DESC
		LIMIT ?
	`, q.ProjectID, q.ProjectID,
		q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano),
		int(q.Kind), int(q.Kind),
		q.Model, q.Model,
		q.NameContains, storage.EscapeLike(q.NameContains),
		q.UserID, q.UserID,
		q.TenantID, q.TenantID,
		q.PageSize,
	)
	if err != nil {
		return nil, fmt.Errorf("searching spans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	spans, err := scanSpans(rows)
	if err != nil {
		return nil, err
	}

	return &storage.SpanResult{Spans: spans, TotalCount: len(spans)}, nil
}

func (s *Store) GetUsageSummary(ctx context.Context, q storage.UsageQuery) (*storage.UsageSummary, error) {
	var summary storage.UsageSummary
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(DISTINCT trace_id),
			COUNT(*),
			COALESCE(SUM(CASE WHEN kind = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(gen_ai_input_tokens), 0),
			COALESCE(SUM(gen_ai_output_tokens), 0),
			COALESCE(SUM(gen_ai_cost_usd), 0),
			COALESCE(
				AVG(CASE WHEN parent_span_id = '' THEN duration_ns ELSE NULL END),
				AVG(CASE WHEN kind = ? THEN duration_ns ELSE NULL END),
				0
			) / 1000000.0,
			CASE WHEN COUNT(DISTINCT trace_id) > 0
				THEN CAST(COUNT(DISTINCT CASE WHEN status = 2 THEN trace_id ELSE NULL END) AS REAL) / COUNT(DISTINCT trace_id)
				ELSE 0 END,
			COALESCE(SUM(gen_ai_cache_read_tokens), 0),
			COALESCE(SUM(gen_ai_cache_creation_tokens), 0)
		FROM spans
		WHERE (? = '' OR project_id = ?) AND start_time >= ? AND start_time <= ?
			AND (? = '' OR user_id = ?)
			AND (? = '' OR tenant_id = ?)
	`, int(storage.SpanKindLLM), q.ProjectID, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano), q.UserID, q.UserID, q.TenantID, q.TenantID).Scan(
		&summary.TotalTraces, &summary.TotalSpans, &summary.TotalLLMCalls,
		&summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD,
		&summary.AvgLatencyMs, &summary.ErrorRate,
		&summary.TotalCacheReadTokens, &summary.TotalCacheCreationTokens,
	)
	if err != nil {
		return nil, fmt.Errorf("querying usage: %w", err)
	}
	return &summary, nil
}

func (s *Store) GetModelBreakdown(ctx context.Context, q storage.UsageQuery) ([]storage.ModelUsage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT gen_ai_model, gen_ai_provider,
			COUNT(*), SUM(gen_ai_input_tokens), SUM(gen_ai_output_tokens),
			SUM(gen_ai_cost_usd), AVG(duration_ns) / 1000000.0,
			COALESCE(SUM(gen_ai_cache_read_tokens), 0),
			COALESCE(SUM(gen_ai_cache_creation_tokens), 0)
		FROM spans
		WHERE (? = '' OR project_id = ?) AND start_time >= ? AND start_time <= ?
			AND gen_ai_model != ''
			AND (? = '' OR user_id = ?)
			AND (? = '' OR tenant_id = ?)
		GROUP BY gen_ai_model, gen_ai_provider
		ORDER BY SUM(gen_ai_cost_usd) DESC
	`, q.ProjectID, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano), q.UserID, q.UserID, q.TenantID, q.TenantID)
	if err != nil {
		return nil, fmt.Errorf("querying model breakdown: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var models []storage.ModelUsage
	for rows.Next() {
		var m storage.ModelUsage
		err := rows.Scan(&m.Model, &m.Provider, &m.CallCount,
			&m.InputTokens, &m.OutputTokens, &m.CostUSD, &m.AvgLatencyMs,
			&m.CacheReadTokens, &m.CacheCreationTokens)
		if err != nil {
			return nil, fmt.Errorf("scanning model: %w", err)
		}
		models = append(models, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating models: %w", err)
	}
	return models, nil
}

func (s *Store) GetUserLeaderboard(ctx context.Context, q storage.UsageQuery, limit int) ([]storage.UserUsageSummary, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id,
			COUNT(DISTINCT trace_id),
			COALESCE(SUM(gen_ai_total_tokens), 0),
			COALESCE(SUM(gen_ai_cost_usd), 0),
			COALESCE(
				AVG(CASE WHEN parent_span_id = '' THEN duration_ns ELSE NULL END),
				AVG(CASE WHEN kind = ? THEN duration_ns ELSE NULL END),
				0
			) / 1000000.0,
			COALESCE((
				SELECT s2.gen_ai_model FROM spans s2
				WHERE s2.user_id = spans.user_id
					AND (? = '' OR s2.project_id = ?) AND s2.start_time >= ? AND s2.start_time <= ?
					AND s2.gen_ai_model != ''
				GROUP BY s2.gen_ai_model
				ORDER BY SUM(s2.gen_ai_cost_usd) DESC
				LIMIT 1
			), '') AS top_model
		FROM spans
		WHERE (? = '' OR project_id = ?) AND start_time >= ? AND start_time <= ?
			AND user_id != ''
		GROUP BY user_id
		ORDER BY SUM(gen_ai_cost_usd) DESC
		LIMIT ?
	`, int(storage.SpanKindLLM), q.ProjectID, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano),
		q.ProjectID, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, fmt.Errorf("querying user leaderboard: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []storage.UserUsageSummary
	for rows.Next() {
		var u storage.UserUsageSummary
		err := rows.Scan(&u.UserID, &u.CallCount, &u.TotalTokens,
			&u.CostUSD, &u.AvgLatencyMs, &u.TopModel)
		if err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating users: %w", err)
	}
	return users, nil
}

// GetTenantLeaderboard returns per-tenant cost aggregations ranked by cost.
func (s *Store) GetTenantLeaderboard(ctx context.Context, q storage.UsageQuery, limit int) ([]storage.TenantUsageSummary, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT tenant_id,
			COUNT(DISTINCT trace_id),
			COALESCE(SUM(gen_ai_total_tokens), 0),
			COALESCE(SUM(gen_ai_cost_usd), 0),
			COALESCE(
				AVG(CASE WHEN parent_span_id = '' THEN duration_ns ELSE NULL END),
				AVG(CASE WHEN kind = ? THEN duration_ns ELSE NULL END),
				0
			) / 1000000.0,
			COALESCE((
				SELECT s2.gen_ai_model FROM spans s2
				WHERE s2.tenant_id = spans.tenant_id
					AND (? = '' OR s2.project_id = ?) AND s2.start_time >= ? AND s2.start_time <= ?
					AND s2.gen_ai_model != ''
				GROUP BY s2.gen_ai_model
				ORDER BY SUM(s2.gen_ai_cost_usd) DESC
				LIMIT 1
			), '') AS top_model
		FROM spans
		WHERE (? = '' OR project_id = ?) AND start_time >= ? AND start_time <= ?
			AND tenant_id IS NOT NULL AND tenant_id != ''
		GROUP BY tenant_id
		ORDER BY SUM(gen_ai_cost_usd) DESC
		LIMIT ?
	`, int(storage.SpanKindLLM), q.ProjectID, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano),
		q.ProjectID, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, fmt.Errorf("querying tenant leaderboard: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tenants []storage.TenantUsageSummary
	for rows.Next() {
		var t storage.TenantUsageSummary
		if err := rows.Scan(&t.TenantID, &t.CallCount, &t.TotalTokens,
			&t.CostUSD, &t.AvgLatencyMs, &t.TopModel); err != nil {
			return nil, fmt.Errorf("scanning tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tenants: %w", err)
	}
	return tenants, nil
}

func (s *Store) GetJobLeaderboard(ctx context.Context, q storage.UsageQuery, limit int) ([]storage.JobUsageSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT job_id, COUNT(DISTINCT trace_id),
			COALESCE(SUM(gen_ai_total_tokens), 0),
			COALESCE(SUM(gen_ai_cost_usd), 0),
			COALESCE(
				AVG(CASE WHEN parent_span_id = '' THEN duration_ns ELSE NULL END),
				AVG(CASE WHEN kind = ? THEN duration_ns ELSE NULL END),
				0
			) / 1000000.0,
			COALESCE((
				SELECT s2.gen_ai_model FROM spans s2
				WHERE s2.job_id = spans.job_id
					AND (? = '' OR s2.project_id = ?) AND s2.start_time >= ? AND s2.start_time <= ?
					AND s2.gen_ai_model != ''
				GROUP BY s2.gen_ai_model
				ORDER BY SUM(s2.gen_ai_cost_usd) DESC
				LIMIT 1
			), '') AS top_model
		FROM spans
		WHERE (? = '' OR project_id = ?) AND start_time >= ? AND start_time <= ?
			AND job_id IS NOT NULL AND job_id != ''
		GROUP BY job_id
		ORDER BY SUM(gen_ai_cost_usd) DESC
		LIMIT ?
	`, int(storage.SpanKindLLM), q.ProjectID, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano),
		q.ProjectID, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, fmt.Errorf("querying job leaderboard: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []storage.JobUsageSummary
	for rows.Next() {
		var j storage.JobUsageSummary
		if err := rows.Scan(&j.JobID, &j.CallCount, &j.TotalTokens, &j.CostUSD, &j.AvgLatencyMs, &j.TopModel); err != nil {
			return nil, fmt.Errorf("scanning job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) Close() error {
	return s.db.Close()
}

// --- helpers ---

func scanSpans(rows *sql.Rows) ([]storage.Span, error) {
	var spans []storage.Span
	for rows.Next() {
		var span storage.Span
		var genAI storage.GenAIAttributes
		var durationNs int64
		var kind, status int
		var startStr, endStr string
		var attrsJSON string

		err := rows.Scan(
			&span.SpanID, &span.TraceID, &span.ParentSpanID,
			&span.Name, &kind, &status, &span.StatusMessage,
			&startStr, &endStr, &durationNs,
			&span.ProjectID, &span.Environment, &span.ServiceName,
			&genAI.Model, &genAI.Provider,
			&genAI.InputTokens, &genAI.OutputTokens, &genAI.TotalTokens,
			&genAI.CostUSD, &genAI.Temperature, &genAI.MaxTokens,
			&genAI.InputContent, &genAI.OutputContent,
			&attrsJSON,
			&span.UserID,
			&span.SessionID,
			&span.TenantID,
			&span.JobID,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning span: %w", err)
		}

		span.Kind = storage.SpanKind(kind)
		span.Status = storage.SpanStatus(status)
		span.StartTime, err = time.Parse(time.RFC3339Nano, startStr)
		if err != nil {
			return nil, fmt.Errorf("parsing start_time for span %s: %w", span.SpanID, err)
		}
		span.EndTime, err = time.Parse(time.RFC3339Nano, endStr)
		if err != nil {
			return nil, fmt.Errorf("parsing end_time for span %s: %w", span.SpanID, err)
		}
		span.Duration = time.Duration(durationNs)

		// Populate GenAI whenever any meaningful field is set — not just when
		// model is known. A span may carry token/cost data from an unrecognized
		// provider, or may report input/output counts without a total.
		if genAI.Model != "" || genAI.Provider != "" ||
			genAI.InputTokens > 0 || genAI.OutputTokens > 0 ||
			genAI.TotalTokens > 0 || genAI.CostUSD > 0 {
			span.GenAI = &genAI
		}

		if attrsJSON != "" && attrsJSON != "{}" {
			span.Attributes = make(map[string]string)
			if err := json.Unmarshal([]byte(attrsJSON), &span.Attributes); err != nil {
				return nil, fmt.Errorf("unmarshaling attributes for span %s: %w", span.SpanID, err)
			}
		}

		spans = append(spans, span)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating spans: %w", err)
	}
	return spans, nil
}

func buildTrace(traceID string, spans []storage.Span) *storage.Trace {
	trace := &storage.Trace{
		TraceID:     traceID,
		StartTime:   spans[0].StartTime,
		EndTime:     spans[0].EndTime,
		ProjectID:   spans[0].ProjectID,
		Environment: spans[0].Environment,
		SpanCount:   len(spans),
		Spans:       spans,
	}

	for _, sp := range spans {
		if sp.StartTime.Before(trace.StartTime) {
			trace.StartTime = sp.StartTime
		}
		if sp.EndTime.After(trace.EndTime) {
			trace.EndTime = sp.EndTime
		}
		if sp.ParentSpanID == "" {
			trace.RootSpanName = sp.Name
		}
		if sp.GenAI != nil {
			trace.TotalTokens += sp.GenAI.TotalTokens
			trace.TotalCostUSD += sp.GenAI.CostUSD
		}
	}
	trace.Duration = trace.EndTime.Sub(trace.StartTime)
	return trace
}

// --- SyncStore Implementation ---

func (s *Store) GetOutboxSpans(ctx context.Context, limit int) ([]storage.OutboxSpan, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT span_id, payload_json, attempt_count, created_at
		FROM outbox_spans
		ORDER BY created_at ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying outbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var spans []storage.OutboxSpan
	for rows.Next() {
		var span storage.OutboxSpan
		var createdAt string
		if err := rows.Scan(&span.SpanID, &span.PayloadJSON, &span.AttemptCount, &createdAt); err != nil {
			return nil, fmt.Errorf("scanning outbox span: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			t = time.Time{}
		}
		span.CreatedAt = t
		spans = append(spans, span)
	}
	return spans, rows.Err()
}

func (s *Store) DeleteOutboxSpans(ctx context.Context, spanIDs []string) error {
	if len(spanIDs) == 0 {
		return nil
	}

	const chunkSize = 500
	for i := 0; i < len(spanIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(spanIDs) {
			end = len(spanIDs)
		}
		chunk := spanIDs[i:end]

		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		query := fmt.Sprintf("DELETE FROM outbox_spans WHERE span_id IN (%s)", placeholders)

		args := make([]any, len(chunk))
		for j, id := range chunk {
			args[j] = id
		}

		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) IncrementOutboxAttempt(ctx context.Context, spanIDs []string) error {
	if len(spanIDs) == 0 {
		return nil
	}

	const chunkSize = 500
	for i := 0; i < len(spanIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(spanIDs) {
			end = len(spanIDs)
		}
		chunk := spanIDs[i:end]

		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		query := fmt.Sprintf("UPDATE outbox_spans SET attempt_count = attempt_count + 1 WHERE span_id IN (%s)", placeholders)

		args := make([]any, len(chunk))
		for j, id := range chunk {
			args[j] = id
		}

		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PruneLocalSpans(ctx context.Context, keepCount int) error {
	// Prune main observability table
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM spans
		WHERE span_id NOT IN (
			SELECT span_id FROM spans ORDER BY start_time DESC LIMIT ?
		)`, keepCount)
	if err != nil {
		return err
	}

	// Prune offline sync outbox queue
	_, err = s.db.ExecContext(ctx, `
		DELETE FROM outbox_spans
		WHERE span_id NOT IN (
			SELECT span_id FROM outbox_spans ORDER BY created_at DESC LIMIT ?
		)`, keepCount)
	return err
}

// isDuplicateColumn returns true if the error is a "duplicate column" error
// from an ALTER TABLE ADD COLUMN migration that already ran.
func isDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}
