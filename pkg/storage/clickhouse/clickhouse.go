// Package clickhouse implements the storage.TraceStore interface using ClickHouse.
package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/candelahq/candela/pkg/storage"
)

// Store implements storage.TraceStore for ClickHouse.
type Store struct {
	conn driver.Conn
}

var _ storage.TraceStore = (*Store)(nil)

// Config holds ClickHouse connection settings.
type Config struct {
	Addr     string `yaml:"addr" json:"addr"`
	Database string `yaml:"database" json:"database"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

// New creates a new ClickHouse-backed TraceStore.
func New(cfg Config) (*Store, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:     5 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 10 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to clickhouse: %w", err)
	}

	s := &Store{conn: conn}
	if err := s.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("pinging clickhouse: %w", err)
	}

	return s, nil
}

// Migrate creates the required tables if they don't exist.
func (s *Store) Migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS spans (
			trace_id       String,
			span_id        String,
			parent_span_id String,
			name           String,
			kind           UInt8,
			status         UInt8,
			status_message String,
			start_time     DateTime64(9, 'UTC'),
			end_time       DateTime64(9, 'UTC'),
			duration_ns    Int64,
			project_id     String,
			environment    String,
			service_name   String,

			-- GenAI attributes
			gen_ai_model          String,
			gen_ai_provider       String,
			gen_ai_input_tokens   Int64,
			gen_ai_output_tokens  Int64,
			gen_ai_total_tokens   Int64,
			gen_ai_cost_usd       Float64,
			gen_ai_temperature    Float64,
			gen_ai_max_tokens     Int64,
			gen_ai_input_content  String,
			gen_ai_output_content String,

			-- General attributes
			attributes    Map(String, String)
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(start_time)
		ORDER BY (project_id, start_time, trace_id, span_id)
		TTL toDateTime(start_time) + INTERVAL 90 DAY`,

		`CREATE TABLE IF NOT EXISTS traces (
			trace_id         String,
			start_time       DateTime64(9, 'UTC'),
			end_time         DateTime64(9, 'UTC'),
			duration_ns      Int64,
			project_id       String,
			environment      String,
			span_count       UInt32,
			llm_call_count   UInt32,
			total_tokens     Int64,
			total_cost_usd   Float64,
			root_span_name   String,
			status           UInt8,
			primary_model    String,
			primary_provider String
		) ENGINE = ReplacingMergeTree()
		PARTITION BY toYYYYMM(start_time)
		ORDER BY (project_id, start_time, trace_id)
		TTL toDateTime(start_time) + INTERVAL 90 DAY`,
	}

	for _, q := range queries {
		if err := s.conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("executing migration: %w", err)
		}
	}
	return nil
}

func (s *Store) IngestSpans(ctx context.Context, spans []storage.Span) error {
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO spans (
		trace_id, span_id, parent_span_id, name, kind, status, status_message,
		start_time, end_time, duration_ns, project_id, environment, service_name,
		gen_ai_model, gen_ai_provider, gen_ai_input_tokens, gen_ai_output_tokens,
		gen_ai_total_tokens, gen_ai_cost_usd, gen_ai_temperature, gen_ai_max_tokens,
		gen_ai_input_content, gen_ai_output_content, attributes
	)`)
	if err != nil {
		return fmt.Errorf("preparing batch: %w", err)
	}

	for _, span := range spans {
		genAI := span.GenAI
		if genAI == nil {
			genAI = &storage.GenAIAttributes{}
		}

		err := batch.Append(
			span.TraceID, span.SpanID, span.ParentSpanID,
			span.Name, uint8(span.Kind), uint8(span.Status), span.StatusMessage,
			span.StartTime, span.EndTime, span.Duration.Nanoseconds(),
			span.ProjectID, span.Environment, span.ServiceName,
			genAI.Model, genAI.Provider,
			genAI.InputTokens, genAI.OutputTokens, genAI.TotalTokens,
			genAI.CostUSD, genAI.Temperature, genAI.MaxTokens,
			genAI.InputContent, genAI.OutputContent,
			span.Attributes,
		)
		if err != nil {
			return fmt.Errorf("appending to batch: %w", err)
		}
	}

	return batch.Send()
}

func (s *Store) GetTrace(ctx context.Context, traceID string) (*storage.Trace, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT trace_id, span_id, parent_span_id, name, kind, status, status_message,
			start_time, end_time, duration_ns, project_id, environment, service_name,
			gen_ai_model, gen_ai_provider, gen_ai_input_tokens, gen_ai_output_tokens,
			gen_ai_total_tokens, gen_ai_cost_usd, gen_ai_temperature, gen_ai_max_tokens,
			gen_ai_input_content, gen_ai_output_content, attributes
		FROM spans
		WHERE trace_id = ?
		ORDER BY start_time ASC
	`, traceID)
	if err != nil {
		return nil, fmt.Errorf("querying spans: %w", err)
	}
	defer rows.Close()

	var spans []storage.Span
	for rows.Next() {
		var span storage.Span
		var genAI storage.GenAIAttributes
		var durationNs int64
		var kind, status uint8

		err := rows.Scan(
			&span.TraceID, &span.SpanID, &span.ParentSpanID,
			&span.Name, &kind, &status, &span.StatusMessage,
			&span.StartTime, &span.EndTime, &durationNs,
			&span.ProjectID, &span.Environment, &span.ServiceName,
			&genAI.Model, &genAI.Provider,
			&genAI.InputTokens, &genAI.OutputTokens, &genAI.TotalTokens,
			&genAI.CostUSD, &genAI.Temperature, &genAI.MaxTokens,
			&genAI.InputContent, &genAI.OutputContent,
			&span.Attributes,
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
		spans = append(spans, span)
	}

	if len(spans) == 0 {
		return nil, fmt.Errorf("trace %s not found", traceID)
	}

	// Build the Trace from spans.
	trace := &storage.Trace{
		TraceID:   traceID,
		StartTime: spans[0].StartTime,
		EndTime:   spans[len(spans)-1].EndTime,
		ProjectID: spans[0].ProjectID,
		Environment: spans[0].Environment,
		SpanCount: len(spans),
		Spans:     spans,
	}

	for _, sp := range spans {
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

	return trace, nil
}

func (s *Store) QueryTraces(ctx context.Context, q storage.TraceQuery) (*storage.TraceResult, error) {
	// For now, query the traces table directly.
	// TODO: implement full filtering, pagination, and sorting.
	rows, err := s.conn.Query(ctx, `
		SELECT trace_id, start_time, duration_ns, root_span_name, project_id,
			environment, span_count, llm_call_count, total_tokens, total_cost_usd,
			status, primary_model, primary_provider
		FROM traces
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
		ORDER BY start_time DESC
		LIMIT ?
	`, q.ProjectID, q.StartTime, q.EndTime, q.PageSize)
	if err != nil {
		return nil, fmt.Errorf("querying traces: %w", err)
	}
	defer rows.Close()

	var traces []storage.TraceSummary
	for rows.Next() {
		var t storage.TraceSummary
		var durationNs int64
		var status uint8

		err := rows.Scan(
			&t.TraceID, &t.StartTime, &durationNs, &t.RootSpanName,
			&t.ProjectID, &t.Environment, &t.SpanCount, &t.LLMCallCount,
			&t.TotalTokens, &t.TotalCostUSD, &status,
			&t.PrimaryModel, &t.PrimaryProvider,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning trace: %w", err)
		}
		t.Duration = time.Duration(durationNs)
		t.Status = storage.SpanStatus(status)
		traces = append(traces, t)
	}

	return &storage.TraceResult{
		Traces:     traces,
		TotalCount: len(traces),
	}, nil
}

func (s *Store) SearchSpans(ctx context.Context, q storage.SpanQuery) (*storage.SpanResult, error) {
	// TODO: implement span search
	return &storage.SpanResult{}, nil
}

func (s *Store) GetUsageSummary(ctx context.Context, q storage.UsageQuery) (*storage.UsageSummary, error) {
	var summary storage.UsageSummary
	err := s.conn.QueryRow(ctx, `
		SELECT
			uniqExact(trace_id) as total_traces,
			count() as total_spans,
			countIf(kind = 1) as total_llm_calls,
			sum(gen_ai_input_tokens) as total_input_tokens,
			sum(gen_ai_output_tokens) as total_output_tokens,
			sum(gen_ai_cost_usd) as total_cost_usd,
			avg(duration_ns) / 1000000 as avg_latency_ms,
			countIf(status = 2) / count() as error_rate
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
	`, q.ProjectID, q.StartTime, q.EndTime).Scan(
		&summary.TotalTraces, &summary.TotalSpans, &summary.TotalLLMCalls,
		&summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD,
		&summary.AvgLatencyMs, &summary.ErrorRate,
	)
	if err != nil {
		return nil, fmt.Errorf("querying usage summary: %w", err)
	}
	return &summary, nil
}

func (s *Store) GetModelBreakdown(ctx context.Context, q storage.UsageQuery) ([]storage.ModelUsage, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT
			gen_ai_model, gen_ai_provider,
			count() as call_count,
			sum(gen_ai_input_tokens) as input_tokens,
			sum(gen_ai_output_tokens) as output_tokens,
			sum(gen_ai_cost_usd) as cost_usd,
			avg(duration_ns) / 1000000 as avg_latency_ms
		FROM spans
		WHERE project_id = ? AND start_time >= ? AND start_time <= ?
			AND gen_ai_model != ''
		GROUP BY gen_ai_model, gen_ai_provider
		ORDER BY cost_usd DESC
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
			return nil, fmt.Errorf("scanning model usage: %w", err)
		}
		models = append(models, m)
	}
	return models, nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.conn.Ping(ctx)
}

func (s *Store) Close() error {
	return s.conn.Close()
}
