// Package sqlite implements the storage.TraceStore interface using SQLite.
// This is the default backend for local development — zero external dependencies.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}

	// SQLite performance tuning.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000", // 64MB cache
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma: %w", err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
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

			PRIMARY KEY (trace_id, span_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_project_time ON spans(project_id, start_time)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_trace ON spans(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_kind ON spans(kind)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("executing migration: %w", err)
		}
	}
	return nil
}

func (s *Store) IngestSpans(ctx context.Context, spans []storage.Span) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO spans (
		span_id, trace_id, parent_span_id, name, kind, status, status_message,
		start_time, end_time, duration_ns, project_id, environment, service_name,
		gen_ai_model, gen_ai_provider, gen_ai_input_tokens, gen_ai_output_tokens,
		gen_ai_total_tokens, gen_ai_cost_usd, gen_ai_temperature, gen_ai_max_tokens,
		gen_ai_input_content, gen_ai_output_content, attributes_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing stmt: %w", err)
	}
	defer stmt.Close()

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
		)
		if err != nil {
			return fmt.Errorf("inserting span: %w", err)
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
			gen_ai_input_content, gen_ai_output_content, attributes_json
		FROM spans WHERE trace_id = ? ORDER BY start_time ASC
	`, traceID)
	if err != nil {
		return nil, fmt.Errorf("querying spans: %w", err)
	}
	defer rows.Close()

	spans, err := scanSpans(rows)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return nil, fmt.Errorf("trace %s not found", traceID)
	}

	return buildTrace(traceID, spans), nil
}

func (s *Store) QueryTraces(ctx context.Context, q storage.TraceQuery) (*storage.TraceResult, error) {
	if q.PageSize == 0 {
		q.PageSize = 50
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
			MAX(gen_ai_model) as primary_model,
			MAX(gen_ai_provider) as primary_provider,
			MAX(status) as status
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
		GROUP BY trace_id
		ORDER BY MIN(start_time) DESC
		LIMIT ?
	`, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano), q.PageSize)
	if err != nil {
		return nil, fmt.Errorf("querying traces: %w", err)
	}
	defer rows.Close()

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

		t.StartTime, _ = time.Parse(time.RFC3339Nano, startStr)
		end, _ := time.Parse(time.RFC3339Nano, endStr)
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
			gen_ai_input_content, gen_ai_output_content, attributes_json
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
			AND (? = 0 OR kind = ?)
			AND (? = '' OR gen_ai_model = ?)
			AND (? = '' OR name LIKE '%' || ? || '%' ESCAPE '\')
		ORDER BY start_time DESC
		LIMIT ?
	`, q.ProjectID,
		q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano),
		int(q.Kind), int(q.Kind),
		q.Model, q.Model,
		q.NameContains, storage.EscapeLike(q.NameContains),
		q.PageSize,
	)
	if err != nil {
		return nil, fmt.Errorf("searching spans: %w", err)
	}
	defer rows.Close()

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
			SUM(CASE WHEN kind = 1 THEN 1 ELSE 0 END),
			COALESCE(SUM(gen_ai_input_tokens), 0),
			COALESCE(SUM(gen_ai_output_tokens), 0),
			COALESCE(SUM(gen_ai_cost_usd), 0),
			COALESCE(AVG(duration_ns), 0) / 1000000.0,
			CASE WHEN COUNT(*) > 0
				THEN CAST(SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END) AS REAL) / COUNT(*)
				ELSE 0 END
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
	`, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano)).Scan(
		&summary.TotalTraces, &summary.TotalSpans, &summary.TotalLLMCalls,
		&summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD,
		&summary.AvgLatencyMs, &summary.ErrorRate,
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
			SUM(gen_ai_cost_usd), AVG(duration_ns) / 1000000.0
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
			AND gen_ai_model != ''
		GROUP BY gen_ai_model, gen_ai_provider
		ORDER BY SUM(gen_ai_cost_usd) DESC
	`, q.ProjectID, q.StartTime.Format(time.RFC3339Nano), q.EndTime.Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("querying model breakdown: %w", err)
	}
	defer rows.Close()

	var models []storage.ModelUsage
	for rows.Next() {
		var m storage.ModelUsage
		err := rows.Scan(&m.Model, &m.Provider, &m.CallCount,
			&m.InputTokens, &m.OutputTokens, &m.CostUSD, &m.AvgLatencyMs)
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

		if genAI.Model != "" {
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
		TraceID:   traceID,
		StartTime: spans[0].StartTime,
		EndTime:   spans[0].EndTime,
		ProjectID: spans[0].ProjectID,
		Environment: spans[0].Environment,
		SpanCount: len(spans),
		Spans:     spans,
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
