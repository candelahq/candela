// Package bigquery implements the storage.SpanWriter and storage.SpanReader
// interfaces using Google BigQuery. Writes use the Storage Write API
// (managedwriter) for high-throughput streaming ingestion. Reads use
// standard BigQuery SQL queries.
//
// Schema design: No primary key (OLAP convention, matching DuckDB).
// Attributes stored as ARRAY<STRUCT<key STRING, value STRING>>.
package bigquery

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"github.com/candelahq/candela/pkg/storage"
)

// Config holds BigQuery connection settings.
type Config struct {
	ProjectID string `yaml:"project_id"` // GCP project ID
	Dataset   string `yaml:"dataset"`    // BigQuery dataset name
	Table     string `yaml:"table"`      // Table name (default: "spans")
	Location  string `yaml:"location"`   // Dataset location (default: "US")
}

// Store implements storage.TraceStore for BigQuery.
type Store struct {
	client  *bigquery.Client
	config  Config
	tableID string // fully-qualified: project.dataset.table
}

var _ storage.SpanWriter = (*Store)(nil)
var _ storage.SpanReader = (*Store)(nil)

// New creates a BigQuery store and ensures the dataset/table exist.
func New(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("bigquery: project_id is required")
	}
	if cfg.Dataset == "" {
		return nil, fmt.Errorf("bigquery: dataset is required")
	}
	if cfg.Table == "" {
		cfg.Table = "spans"
	}
	if cfg.Location == "" {
		cfg.Location = "US"
	}

	client, err := bigquery.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("bigquery: creating client: %w", err)
	}

	s := &Store{
		client:  client,
		config:  cfg,
		tableID: fmt.Sprintf("%s.%s.%s", cfg.ProjectID, cfg.Dataset, cfg.Table),
	}

	if err := s.ensureSchema(ctx); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("bigquery: ensuring schema: %w", err)
	}

	return s, nil
}

// AttributeKV represents a key-value pair for BigQuery STRUCT.
type AttributeKV struct {
	Key   string `bigquery:"key"`
	Value string `bigquery:"value"`
}

// spanRow is the BigQuery row schema for a span.
type spanRow struct {
	SpanID        string        `bigquery:"span_id"`
	TraceID       string        `bigquery:"trace_id"`
	ParentSpanID  string        `bigquery:"parent_span_id"`
	Name          string        `bigquery:"name"`
	Kind          int           `bigquery:"kind"`
	Status        int           `bigquery:"status"`
	StatusMessage string        `bigquery:"status_message"`
	StartTime     time.Time     `bigquery:"start_time"`
	EndTime       time.Time     `bigquery:"end_time"`
	DurationNs    int64         `bigquery:"duration_ns"`
	ProjectID     string        `bigquery:"project_id"`
	Environment   string        `bigquery:"environment"`
	ServiceName   string        `bigquery:"service_name"`
	GenAIModel    string        `bigquery:"gen_ai_model"`
	GenAIProvider string        `bigquery:"gen_ai_provider"`
	InputTokens   int64         `bigquery:"gen_ai_input_tokens"`
	OutputTokens  int64         `bigquery:"gen_ai_output_tokens"`
	TotalTokens   int64         `bigquery:"gen_ai_total_tokens"`
	CostUSD       float64       `bigquery:"gen_ai_cost_usd"`
	Temperature   float64       `bigquery:"gen_ai_temperature"`
	MaxTokens     int64         `bigquery:"gen_ai_max_tokens"`
	InputContent  string        `bigquery:"gen_ai_input_content"`
	OutputContent string        `bigquery:"gen_ai_output_content"`
	Attributes    []AttributeKV `bigquery:"attributes"`
	UserID        string        `bigquery:"user_id"`
	SessionID     string        `bigquery:"session_id"`
}

func spanToRow(span storage.Span) spanRow {
	row := spanRow{
		SpanID:        span.SpanID,
		TraceID:       span.TraceID,
		ParentSpanID:  span.ParentSpanID,
		Name:          span.Name,
		Kind:          int(span.Kind),
		Status:        int(span.Status),
		StatusMessage: span.StatusMessage,
		StartTime:     span.StartTime,
		EndTime:       span.EndTime,
		DurationNs:    span.Duration.Nanoseconds(),
		ProjectID:     span.ProjectID,
		Environment:   span.Environment,
		ServiceName:   span.ServiceName,
		UserID:        span.UserID,
		SessionID:     span.SessionID,
	}

	if span.GenAI != nil {
		row.GenAIModel = span.GenAI.Model
		row.GenAIProvider = span.GenAI.Provider
		row.InputTokens = span.GenAI.InputTokens
		row.OutputTokens = span.GenAI.OutputTokens
		row.TotalTokens = span.GenAI.TotalTokens
		row.CostUSD = span.GenAI.CostUSD
		row.Temperature = span.GenAI.Temperature
		row.MaxTokens = span.GenAI.MaxTokens
		row.InputContent = span.GenAI.InputContent
		row.OutputContent = span.GenAI.OutputContent
	}

	for k, v := range span.Attributes {
		row.Attributes = append(row.Attributes, AttributeKV{Key: k, Value: v})
	}

	return row
}

func rowToSpan(row spanRow) storage.Span {
	span := storage.Span{
		SpanID:        row.SpanID,
		TraceID:       row.TraceID,
		ParentSpanID:  row.ParentSpanID,
		Name:          row.Name,
		Kind:          storage.SpanKind(row.Kind),
		Status:        storage.SpanStatus(row.Status),
		StatusMessage: row.StatusMessage,
		StartTime:     row.StartTime,
		EndTime:       row.EndTime,
		Duration:      time.Duration(row.DurationNs),
		ProjectID:     row.ProjectID,
		Environment:   row.Environment,
		ServiceName:   row.ServiceName,
		UserID:        row.UserID,
		SessionID:     row.SessionID,
	}

	if row.GenAIModel != "" {
		span.GenAI = &storage.GenAIAttributes{
			Model:         row.GenAIModel,
			Provider:      row.GenAIProvider,
			InputTokens:   row.InputTokens,
			OutputTokens:  row.OutputTokens,
			TotalTokens:   row.TotalTokens,
			CostUSD:       row.CostUSD,
			Temperature:   row.Temperature,
			MaxTokens:     row.MaxTokens,
			InputContent:  row.InputContent,
			OutputContent: row.OutputContent,
		}
	}

	if len(row.Attributes) > 0 {
		span.Attributes = make(map[string]string, len(row.Attributes))
		for _, attr := range row.Attributes {
			span.Attributes[attr.Key] = attr.Value
		}
	}

	return span
}

// ensureSchema creates the dataset and table if they don't exist,
// and evolves the schema by adding any missing columns to existing tables.
func (s *Store) ensureSchema(ctx context.Context) error {
	dataset := s.client.Dataset(s.config.Dataset)

	// Create dataset if not exists.
	meta := &bigquery.DatasetMetadata{Location: s.config.Location}
	if err := dataset.Create(ctx, meta); err != nil {
		if !strings.Contains(err.Error(), "Already Exists") {
			return fmt.Errorf("creating dataset: %w", err)
		}
	}

	// Infer the desired schema from the Go struct.
	schema, err := bigquery.InferSchema(spanRow{})
	if err != nil {
		return fmt.Errorf("inferring schema: %w", err)
	}

	table := dataset.Table(s.config.Table)
	tableMeta := &bigquery.TableMetadata{
		Schema: schema,
		TimePartitioning: &bigquery.TimePartitioning{
			Field: "start_time",
			Type:  bigquery.DayPartitioningType,
		},
		Clustering: &bigquery.Clustering{
			Fields: []string{"project_id", "user_id", "trace_id"},
		},
	}
	if err := table.Create(ctx, tableMeta); err != nil {
		if !strings.Contains(err.Error(), "Already Exists") {
			return fmt.Errorf("creating table: %w", err)
		}

		// Table already exists — check for missing columns and add them.
		if err := s.evolveSchema(ctx, table, schema); err != nil {
			return fmt.Errorf("evolving schema: %w", err)
		}
	}

	slog.Info("bigquery schema ready",
		"project", s.config.ProjectID,
		"dataset", s.config.Dataset,
		"table", s.config.Table)

	return nil
}

// evolveSchema compares the desired schema against the live table and adds
// any missing columns. BigQuery supports additive schema changes (new nullable
// columns) without requiring a full table rebuild.
//
// Limitation: this only detects missing top-level columns. Changes to nested
// STRUCT fields (e.g. adding a field inside the "attributes" REPEATED STRUCT)
// are not detected and must be handled manually or via a migration tool.
func (s *Store) evolveSchema(ctx context.Context, table *bigquery.Table, desired bigquery.Schema) error {
	md, err := table.Metadata(ctx)
	if err != nil {
		return fmt.Errorf("reading table metadata: %w", err)
	}

	// Build set of existing column names.
	existing := make(map[string]bool, len(md.Schema))
	for _, field := range md.Schema {
		existing[field.Name] = true
	}

	// Find columns in desired schema that are missing from the live table.
	var missing bigquery.Schema
	for _, field := range desired {
		if !existing[field.Name] {
			missing = append(missing, field)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	// Build a new slice to avoid mutating md.Schema's backing array.
	updatedSchema := make(bigquery.Schema, 0, len(md.Schema)+len(missing))
	updatedSchema = append(updatedSchema, md.Schema...)
	updatedSchema = append(updatedSchema, missing...)

	var names []string
	for _, f := range missing {
		names = append(names, f.Name)
	}
	slog.Info("evolving BigQuery schema — adding missing columns",
		"table", table.FullyQualifiedName(),
		"columns", names)

	_, err = table.Update(ctx, bigquery.TableMetadataToUpdate{
		Schema: updatedSchema,
	}, md.ETag)
	if err != nil {
		return fmt.Errorf("updating table schema: %w", err)
	}

	return nil
}

// IngestSpans writes spans using the BigQuery Inserter (streaming insert).
// For higher throughput, consider switching to managedwriter (Storage Write API).
func (s *Store) IngestSpans(ctx context.Context, spans []storage.Span) error {
	if len(spans) == 0 {
		return nil
	}

	inserter := s.client.Dataset(s.config.Dataset).Table(s.config.Table).Inserter()

	var rows []*bigquery.StructSaver
	for _, span := range spans {
		row := spanToRow(span)
		rows = append(rows, &bigquery.StructSaver{
			Struct:   row,
			InsertID: span.TraceID + "-" + span.SpanID, // dedup key
		})
	}

	if err := inserter.Put(ctx, rows); err != nil {
		return fmt.Errorf("bigquery insert: %w", err)
	}

	return nil
}

// Ping checks connectivity to BigQuery by reading table metadata.
// Uses a metadata read instead of a query job to avoid unnecessary cost
// and latency on frequent health checks.
func (s *Store) Ping(ctx context.Context) error {
	_, err := s.client.Dataset(s.config.Dataset).Table(s.config.Table).Metadata(ctx)
	if err != nil {
		return fmt.Errorf("bigquery ping: %w", err)
	}
	return nil
}

// Close closes the BigQuery client.
func (s *Store) Close() error {
	return s.client.Close()
}

// GetTrace retrieves all spans for a given trace ID.
func (s *Store) GetTrace(ctx context.Context, traceID string) (*storage.Trace, error) {
	query := fmt.Sprintf(`
		SELECT * FROM %s
		WHERE trace_id = @traceID
		ORDER BY start_time ASC
	`, quoteTable(s.tableID))

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "traceID", Value: traceID},
	}

	spans, err := s.querySpans(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("getting trace: %w", err)
	}
	if len(spans) == 0 {
		return nil, fmt.Errorf("trace %s not found", traceID)
	}

	return buildTrace(traceID, spans), nil
}

// QueryTraces returns trace summaries matching the query.
func (s *Store) QueryTraces(ctx context.Context, tq storage.TraceQuery) (*storage.TraceResult, error) {
	if tq.PageSize == 0 {
		tq.PageSize = 20
	}

	query := fmt.Sprintf(`
		SELECT trace_id, MIN(start_time) AS earliest
		FROM %s
		WHERE project_id = @projectID
		  AND start_time >= @startTime
		  AND start_time <= @endTime
		GROUP BY trace_id
		ORDER BY earliest DESC
		LIMIT @pageSize
	`, quoteTable(s.tableID))

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "projectID", Value: tq.ProjectID},
		{Name: "startTime", Value: tq.StartTime},
		{Name: "endTime", Value: tq.EndTime},
		{Name: "pageSize", Value: tq.PageSize},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying traces: %w", err)
	}

	var traceIDs []string
	for {
		var row struct {
			TraceID  string    `bigquery:"trace_id"`
			Earliest time.Time `bigquery:"earliest"`
		}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterating trace IDs: %w", err)
		}
		traceIDs = append(traceIDs, row.TraceID)
	}

	if len(traceIDs) == 0 {
		return &storage.TraceResult{Traces: nil, TotalCount: 0}, nil
	}

	// Batch-fetch only the columns buildTrace needs (avoids scanning
	// large text fields like gen_ai_input_content/gen_ai_output_content).
	batchQuery := fmt.Sprintf(`
		SELECT span_id, trace_id, parent_span_id, name, kind, status,
		       start_time, end_time, duration_ns, project_id, environment,
		       gen_ai_total_tokens, gen_ai_cost_usd
		FROM %s
		WHERE trace_id IN UNNEST(@traceIDs)
		ORDER BY start_time ASC, span_id ASC
	`, quoteTable(s.tableID))

	bq := s.client.Query(batchQuery)
	bq.Parameters = []bigquery.QueryParameter{
		{Name: "traceIDs", Value: traceIDs},
	}

	allSpans, err := s.querySpans(ctx, bq)
	if err != nil {
		return nil, fmt.Errorf("batch-fetching trace spans: %w", err)
	}

	// Group spans by trace ID, preserving the order from traceIDs.
	spansByTrace := make(map[string][]storage.Span, len(traceIDs))
	for _, span := range allSpans {
		spansByTrace[span.TraceID] = append(spansByTrace[span.TraceID], span)
	}

	var traces []storage.TraceSummary
	for _, tid := range traceIDs {
		spans, ok := spansByTrace[tid]
		if !ok || len(spans) == 0 {
			continue
		}
		trace := buildTrace(tid, spans)
		traces = append(traces, storage.TraceSummary{
			TraceID:      trace.TraceID,
			RootSpanName: trace.RootSpanName,
			StartTime:    trace.StartTime,
			Duration:     trace.Duration,
			SpanCount:    trace.SpanCount,
			TotalTokens:  trace.TotalTokens,
			TotalCostUSD: trace.TotalCostUSD,
			Environment:  trace.Environment,
		})
	}

	return &storage.TraceResult{Traces: traces, TotalCount: len(traces)}, nil
}

// SearchSpans searches spans with filtering.
func (s *Store) SearchSpans(ctx context.Context, sq storage.SpanQuery) (*storage.SpanResult, error) {
	if sq.PageSize == 0 {
		sq.PageSize = 50
	}

	query := fmt.Sprintf(`
		SELECT * FROM %s
		WHERE project_id = @projectID
		  AND start_time >= @startTime
		  AND start_time <= @endTime
		  AND (@kind = 0 OR kind = @kind)
		  AND (@model = '' OR gen_ai_model = @model)
		  AND (@nameContains = '' OR name LIKE CONCAT('%%', @escapedName, '%%'))
		ORDER BY start_time DESC
		LIMIT @pageSize
	`, quoteTable(s.tableID))

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "projectID", Value: sq.ProjectID},
		{Name: "startTime", Value: sq.StartTime},
		{Name: "endTime", Value: sq.EndTime},
		{Name: "kind", Value: int(sq.Kind)},
		{Name: "model", Value: sq.Model},
		{Name: "nameContains", Value: sq.NameContains},
		{Name: "escapedName", Value: storage.EscapeLike(sq.NameContains)},
		{Name: "pageSize", Value: sq.PageSize},
	}

	spans, err := s.querySpans(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("searching spans: %w", err)
	}

	return &storage.SpanResult{Spans: spans, TotalCount: len(spans)}, nil
}

// GetUsageSummary returns aggregated usage statistics.
func (s *Store) GetUsageSummary(ctx context.Context, uq storage.UsageQuery) (*storage.UsageSummary, error) {
	query := fmt.Sprintf(`
		SELECT
			COUNT(DISTINCT trace_id) AS total_traces,
			COUNT(*) AS total_spans,
			COUNTIF(kind = @llmKind) AS total_llm_calls,
			COALESCE(SUM(gen_ai_input_tokens), 0) AS total_input_tokens,
			COALESCE(SUM(gen_ai_output_tokens), 0) AS total_output_tokens,
			COALESCE(SUM(gen_ai_total_tokens), 0) AS total_tokens,
			COALESCE(SUM(gen_ai_cost_usd), 0) AS total_cost_usd,
			COALESCE(AVG(duration_ns), 0) AS avg_duration_ns
		FROM %s
		WHERE project_id = @projectID
		  AND start_time >= @startTime
		  AND start_time <= @endTime
		  AND (@userID = '' OR user_id = @userID)
	`, quoteTable(s.tableID))

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "projectID", Value: uq.ProjectID},
		{Name: "startTime", Value: uq.StartTime},
		{Name: "endTime", Value: uq.EndTime},
		{Name: "llmKind", Value: int(storage.SpanKindLLM)},
		{Name: "userID", Value: uq.UserID},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying usage: %w", err)
	}

	var summary storage.UsageSummary
	var row struct {
		TotalTraces       int64   `bigquery:"total_traces"`
		TotalSpans        int64   `bigquery:"total_spans"`
		TotalLLMCalls     int64   `bigquery:"total_llm_calls"`
		TotalInputTokens  int64   `bigquery:"total_input_tokens"`
		TotalOutputTokens int64   `bigquery:"total_output_tokens"`
		TotalTokens       int64   `bigquery:"total_tokens"`
		TotalCostUSD      float64 `bigquery:"total_cost_usd"`
		AvgDurationNs     float64 `bigquery:"avg_duration_ns"`
	}
	if err := it.Next(&row); err != nil && err != iterator.Done {
		return nil, fmt.Errorf("reading usage: %w", err)
	}

	summary.TotalTraces = row.TotalTraces
	summary.TotalSpans = row.TotalSpans
	summary.TotalLLMCalls = row.TotalLLMCalls
	summary.TotalInputTokens = row.TotalInputTokens
	summary.TotalOutputTokens = row.TotalOutputTokens
	summary.TotalCostUSD = row.TotalCostUSD
	summary.AvgLatencyMs = float64(row.AvgDurationNs) / 1e6

	return &summary, nil
}

// GetModelBreakdown returns per-model usage statistics.
func (s *Store) GetModelBreakdown(ctx context.Context, uq storage.UsageQuery) ([]storage.ModelUsage, error) {
	query := fmt.Sprintf(`
		SELECT
			gen_ai_model AS model,
			gen_ai_provider AS provider,
			COUNT(*) AS call_count,
			COALESCE(SUM(gen_ai_input_tokens), 0) AS input_tokens,
			COALESCE(SUM(gen_ai_output_tokens), 0) AS output_tokens,
			COALESCE(SUM(gen_ai_total_tokens), 0) AS total_tokens,
			COALESCE(SUM(gen_ai_cost_usd), 0) AS total_cost_usd,
			COALESCE(AVG(duration_ns), 0) AS avg_duration_ns
		FROM %s
		WHERE project_id = @projectID
		  AND start_time >= @startTime
		  AND start_time <= @endTime
		  AND gen_ai_model != ''
		  AND (@userID = '' OR user_id = @userID)
		GROUP BY gen_ai_model, gen_ai_provider
		ORDER BY total_cost_usd DESC
	`, quoteTable(s.tableID))

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "projectID", Value: uq.ProjectID},
		{Name: "startTime", Value: uq.StartTime},
		{Name: "endTime", Value: uq.EndTime},
		{Name: "userID", Value: uq.UserID},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying model breakdown: %w", err)
	}

	var models []storage.ModelUsage
	for {
		var row struct {
			Model         string  `bigquery:"model"`
			Provider      string  `bigquery:"provider"`
			CallCount     int64   `bigquery:"call_count"`
			InputTokens   int64   `bigquery:"input_tokens"`
			OutputTokens  int64   `bigquery:"output_tokens"`
			TotalTokens   int64   `bigquery:"total_tokens"`
			TotalCostUSD  float64 `bigquery:"total_cost_usd"`
			AvgDurationNs float64 `bigquery:"avg_duration_ns"`
		}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterating model breakdown: %w", err)
		}
		models = append(models, storage.ModelUsage{
			Model:        row.Model,
			Provider:     row.Provider,
			CallCount:    row.CallCount,
			InputTokens:  row.InputTokens,
			OutputTokens: row.OutputTokens,
			CostUSD:      row.TotalCostUSD,
			AvgLatencyMs: float64(row.AvgDurationNs) / 1e6,
		})
	}

	return models, nil
}

// GetUserLeaderboard returns per-user usage aggregations ranked by cost.
func (s *Store) GetUserLeaderboard(ctx context.Context, uq storage.UsageQuery, limit int) ([]storage.UserUsageSummary, error) {
	if limit <= 0 {
		limit = 20
	}

	query := fmt.Sprintf(`
		SELECT
			user_id,
			COUNT(*) AS call_count,
			COALESCE(SUM(gen_ai_total_tokens), 0) AS total_tokens,
			COALESCE(SUM(gen_ai_cost_usd), 0) AS total_cost_usd,
			COALESCE(AVG(duration_ns), 0) AS avg_duration_ns
		FROM %s
		WHERE project_id = @projectID
		  AND start_time >= @startTime
		  AND start_time <= @endTime
		  AND user_id != ''
		GROUP BY user_id
		ORDER BY total_cost_usd DESC
		LIMIT @limit
	`, quoteTable(s.tableID))

	q := s.client.Query(query)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "projectID", Value: uq.ProjectID},
		{Name: "startTime", Value: uq.StartTime},
		{Name: "endTime", Value: uq.EndTime},
		{Name: "limit", Value: limit},
	}

	it, err := q.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying user leaderboard: %w", err)
	}

	var users []storage.UserUsageSummary
	for {
		var row struct {
			UserID        string  `bigquery:"user_id"`
			CallCount     int64   `bigquery:"call_count"`
			TotalTokens   int64   `bigquery:"total_tokens"`
			TotalCostUSD  float64 `bigquery:"total_cost_usd"`
			AvgDurationNs float64 `bigquery:"avg_duration_ns"`
		}
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterating user leaderboard: %w", err)
		}
		users = append(users, storage.UserUsageSummary{
			UserID:       row.UserID,
			CallCount:    row.CallCount,
			TotalTokens:  row.TotalTokens,
			CostUSD:      row.TotalCostUSD,
			AvgLatencyMs: float64(row.AvgDurationNs) / 1e6,
		})
	}

	return users, nil
}

// querySpans executes a query and scans results into storage.Span.
func (s *Store) querySpans(ctx context.Context, q *bigquery.Query) ([]storage.Span, error) {
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}

	var spans []storage.Span
	for {
		var row spanRow
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterating spans: %w", err)
		}
		spans = append(spans, rowToSpan(row))
	}

	return spans, nil
}

// buildTrace assembles a Trace from a list of spans.
func buildTrace(traceID string, spans []storage.Span) *storage.Trace {
	trace := &storage.Trace{
		TraceID:   traceID,
		Spans:     spans,
		SpanCount: len(spans),
	}

	if len(spans) > 0 {
		trace.StartTime = spans[0].StartTime
		trace.Environment = spans[0].Environment
	}

	for _, span := range spans {
		if span.ParentSpanID == "" {
			trace.RootSpanName = span.Name
			trace.Duration = span.EndTime.Sub(span.StartTime)
		}
		if span.GenAI != nil {
			trace.TotalTokens += span.GenAI.TotalTokens
			trace.TotalCostUSD += span.GenAI.CostUSD
		}
	}

	return trace
}

// quoteTable wraps a fully-qualified table ID in backticks for BigQuery SQL.
// Escapes any embedded backticks to prevent SQL injection via config values.
func quoteTable(tableID string) string {
	return "`" + strings.ReplaceAll(tableID, "`", "\\`") + "`"
}
