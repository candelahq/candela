package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/candelahq/candela/pkg/storage"
	"github.com/duckdb/duckdb-go/v2"
)

// Store implements storage.TraceStore for DuckDB.
// DuckDB is the default local development backend — zero external dependencies,
// columnar analytical performance for aggregation queries.
type Store struct {
	db *sql.DB
}

var _ storage.TraceStore = (*Store)(nil) // satisfies both SpanWriter + SpanReader

// Config holds DuckDB connection settings.
type Config struct {
	Path string `yaml:"path" json:"path"` // e.g. "candela.duckdb"
}

// New creates a new DuckDB-backed TraceStore.
func New(cfg Config) (*Store, error) {
	if cfg.Path == "" {
		cfg.Path = "candela.duckdb"
	}

	db, err := sql.Open("duckdb", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("opening duckdb: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	slog.Info("DuckDB store initialized", "path", cfg.Path)
	return s, nil
}

func (s *Store) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS spans (
			span_id        VARCHAR NOT NULL,
			trace_id       VARCHAR NOT NULL,
			parent_span_id VARCHAR DEFAULT '',
			name           VARCHAR NOT NULL,
			kind           INTEGER DEFAULT 0,
			status         INTEGER DEFAULT 0,
			status_message VARCHAR DEFAULT '',
			start_time     TIMESTAMP NOT NULL,
			end_time       TIMESTAMP NOT NULL,
			duration_ns    BIGINT DEFAULT 0,
			project_id     VARCHAR DEFAULT '',
			environment    VARCHAR DEFAULT '',
			service_name   VARCHAR DEFAULT '',

			gen_ai_model          VARCHAR DEFAULT '',
			gen_ai_provider       VARCHAR DEFAULT '',
			gen_ai_input_tokens   BIGINT DEFAULT 0,
			gen_ai_output_tokens  BIGINT DEFAULT 0,
			gen_ai_total_tokens   BIGINT DEFAULT 0,
			gen_ai_cost_usd       DOUBLE DEFAULT 0,
			gen_ai_temperature    DOUBLE DEFAULT 0,
			gen_ai_max_tokens     BIGINT DEFAULT 0,
			gen_ai_input_content  VARCHAR DEFAULT '',
			gen_ai_output_content VARCHAR DEFAULT '',

			attributes STRUCT(key VARCHAR, value VARCHAR)[],

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
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("getting conn for ingest: %w", err)
	}
	defer conn.Close()

	return conn.Raw(func(driverConn any) error {
		duckConn, ok := driverConn.(*duckdb.Conn)
		if !ok {
			return fmt.Errorf("driverConn is not *duckdb.Conn")
		}

		appender, err := duckdb.NewAppenderFromConn(duckConn, "", "spans")
		if err != nil {
			return fmt.Errorf("creating appender: %w", err)
		}
		defer appender.Close()

		for _, span := range spans {
			genAI := span.GenAI
			if genAI == nil {
				genAI = &storage.GenAIAttributes{}
			}

			// DuckDB Appender requires []map[string]any for STRUCT columns.
			var attrs []map[string]any
			for k, v := range span.Attributes {
				attrs = append(attrs, map[string]any{"key": k, "value": v})
			}

			if err := appender.AppendRow(
				span.SpanID, span.TraceID, span.ParentSpanID,
				span.Name, int32(span.Kind), int32(span.Status), span.StatusMessage,
				span.StartTime,
				span.EndTime,
				span.Duration.Nanoseconds(),
				span.ProjectID, span.Environment, span.ServiceName,
				genAI.Model, genAI.Provider,
				genAI.InputTokens, genAI.OutputTokens, genAI.TotalTokens,
				genAI.CostUSD, genAI.Temperature, genAI.MaxTokens,
				genAI.InputContent, genAI.OutputContent,
				attrs,
			); err != nil {
				return fmt.Errorf("appending span %s: %w", span.SpanID, err)
			}
		}

		if err := appender.Flush(); err != nil {
			return fmt.Errorf("flushing appender: %w", err)
		}
		return nil
	})
}

func (s *Store) GetTrace(ctx context.Context, traceID string) (*storage.Trace, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT span_id, trace_id, parent_span_id, name, kind, status, status_message,
			start_time, end_time, duration_ns, project_id, environment, service_name,
			gen_ai_model, gen_ai_provider, gen_ai_input_tokens, gen_ai_output_tokens,
			gen_ai_total_tokens, gen_ai_cost_usd, gen_ai_temperature, gen_ai_max_tokens,
			gen_ai_input_content, gen_ai_output_content, attributes
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
			COUNT(*)::INTEGER as span_count,
			SUM(CASE WHEN kind = 1 THEN 1 ELSE 0 END)::INTEGER as llm_count,
			COALESCE(SUM(gen_ai_total_tokens), 0)::BIGINT as total_tokens,
			COALESCE(SUM(gen_ai_cost_usd), 0)::DOUBLE as total_cost,
			MAX(CASE WHEN parent_span_id = '' THEN name ELSE '' END) as root_name,
			MAX(gen_ai_model) as primary_model,
			MAX(gen_ai_provider) as primary_provider,
			MAX(status)::INTEGER as status
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
		GROUP BY trace_id
		ORDER BY MIN(start_time) DESC
		LIMIT ?
	`, q.ProjectID, q.StartTime, q.EndTime, q.PageSize)
	if err != nil {
		return nil, fmt.Errorf("querying traces: %w", err)
	}
	defer rows.Close()

	var traces []storage.TraceSummary
	for rows.Next() {
		var t storage.TraceSummary
		var status int
		var endTime time.Time

		err := rows.Scan(
			&t.TraceID, &t.StartTime, &endTime, &t.SpanCount, &t.LLMCallCount,
			&t.TotalTokens, &t.TotalCostUSD, &t.RootSpanName,
			&t.PrimaryModel, &t.PrimaryProvider, &status,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning trace: %w", err)
		}

		t.Duration = endTime.Sub(t.StartTime)
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
			gen_ai_input_content, gen_ai_output_content, attributes
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
			AND (? = 0 OR kind = ?)
			AND (? = '' OR gen_ai_model = ?)
			AND (? = '' OR name LIKE '%' || ? || '%')
		ORDER BY start_time DESC
		LIMIT ?
	`, q.ProjectID, q.StartTime, q.EndTime,
		int(q.Kind), int(q.Kind),
		q.Model, q.Model,
		q.NameContains, q.NameContains,
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
			COUNT(DISTINCT trace_id)::BIGINT,
			COUNT(*)::BIGINT,
			COALESCE(SUM(CASE WHEN kind = 1 THEN 1 ELSE 0 END), 0)::BIGINT,
			COALESCE(SUM(gen_ai_input_tokens), 0)::BIGINT,
			COALESCE(SUM(gen_ai_output_tokens), 0)::BIGINT,
			COALESCE(SUM(gen_ai_cost_usd), 0)::DOUBLE,
			COALESCE(AVG(duration_ns), 0)::DOUBLE / 1000000.0,
			CASE WHEN COUNT(*) > 0
				THEN CAST(SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END) AS DOUBLE) / COUNT(*)
				ELSE 0 END
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
	`, q.ProjectID, q.StartTime, q.EndTime).Scan(
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
			COUNT(*)::BIGINT,
			COALESCE(SUM(gen_ai_input_tokens), 0)::BIGINT,
			COALESCE(SUM(gen_ai_output_tokens), 0)::BIGINT,
			COALESCE(SUM(gen_ai_cost_usd), 0)::DOUBLE,
			COALESCE(AVG(duration_ns), 0)::DOUBLE / 1000000.0
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
			AND gen_ai_model != ''
		GROUP BY gen_ai_model, gen_ai_provider
		ORDER BY SUM(gen_ai_cost_usd) DESC
	`, q.ProjectID, q.StartTime, q.EndTime)
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
		var attrsAny any

		err := rows.Scan(
			&span.SpanID, &span.TraceID, &span.ParentSpanID,
			&span.Name, &kind, &status, &span.StatusMessage,
			&span.StartTime, &span.EndTime, &durationNs,
			&span.ProjectID, &span.Environment, &span.ServiceName,
			&genAI.Model, &genAI.Provider,
			&genAI.InputTokens, &genAI.OutputTokens, &genAI.TotalTokens,
			&genAI.CostUSD, &genAI.Temperature, &genAI.MaxTokens,
			&genAI.InputContent, &genAI.OutputContent,
			&attrsAny,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning span: %w", err)
		}

		span.Kind = storage.SpanKind(kind)
		span.Status = storage.SpanStatus(status)
		span.Duration = time.Duration(durationNs)

		if genAI.Model != "" {
			span.GenAI = &genAI
		}

		// DuckDB returns STRUCT(key, value)[] as []any containing map[string]any entries.
		if attrsAny != nil {
			if list, ok := attrsAny.([]any); ok && len(list) > 0 {
				span.Attributes = make(map[string]string, len(list))
				for _, item := range list {
					if m, ok := item.(map[string]any); ok {
						k, _ := m["key"].(string)
						v, _ := m["value"].(string)
						if k != "" {
							span.Attributes[k] = v
						}
					}
				}
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
