// Package storage defines the TraceStore interface — the core storage abstraction
// in Candela. Every storage backend (ClickHouse, BigQuery, PostgreSQL) implements
// this interface.
package storage

import (
	"context"
	"time"
)

// SpanKind mirrors the proto SpanKind enum for Go-native use.
type SpanKind int

const (
	SpanKindUnspecified SpanKind = iota
	SpanKindLLM
	SpanKindAgent
	SpanKindTool
	SpanKindRetrieval
	SpanKindEmbedding
	SpanKindChain
	SpanKindGeneral
)

// SpanStatus mirrors the proto SpanStatus enum.
type SpanStatus int

const (
	SpanStatusUnspecified SpanStatus = iota
	SpanStatusOK
	SpanStatusError
)

// GenAIAttributes holds LLM-specific attributes.
type GenAIAttributes struct {
	Model        string  `json:"model,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	TotalTokens  int64   `json:"total_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
	Temperature  float64 `json:"temperature,omitempty"`
	MaxTokens    int64   `json:"max_tokens,omitempty"`
	TopP         float64 `json:"top_p,omitempty"`
	InputContent string  `json:"input_content,omitempty"`
	OutputContent string `json:"output_content,omitempty"`
}

// Span represents a single span in the storage layer.
type Span struct {
	SpanID       string            `json:"span_id"`
	TraceID      string            `json:"trace_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Name         string            `json:"name"`
	Kind         SpanKind          `json:"kind"`
	Status       SpanStatus        `json:"status"`
	StatusMessage string           `json:"status_message,omitempty"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	Duration     time.Duration     `json:"duration"`
	GenAI        *GenAIAttributes  `json:"gen_ai,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	ProjectID    string            `json:"project_id"`
	Environment  string            `json:"environment,omitempty"`
	ServiceName  string            `json:"service_name,omitempty"`
}

// TraceSummary is a lightweight summary for list views.
type TraceSummary struct {
	TraceID         string     `json:"trace_id"`
	StartTime       time.Time  `json:"start_time"`
	Duration        time.Duration `json:"duration"`
	RootSpanName    string     `json:"root_span_name"`
	ProjectID       string     `json:"project_id"`
	Environment     string     `json:"environment"`
	SpanCount       int        `json:"span_count"`
	LLMCallCount    int        `json:"llm_call_count"`
	TotalTokens     int64      `json:"total_tokens"`
	TotalCostUSD    float64    `json:"total_cost_usd"`
	Status          SpanStatus `json:"status"`
	PrimaryModel    string     `json:"primary_model"`
	PrimaryProvider string     `json:"primary_provider"`
}

// Trace is a complete trace with all spans.
type Trace struct {
	TraceID      string        `json:"trace_id"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration"`
	ProjectID    string        `json:"project_id"`
	Environment  string        `json:"environment"`
	SpanCount    int           `json:"span_count"`
	TotalTokens  int64         `json:"total_tokens"`
	TotalCostUSD float64       `json:"total_cost_usd"`
	RootSpanName string        `json:"root_span_name"`
	Spans        []Span        `json:"spans"`
}

// TraceQuery defines filters for listing traces.
type TraceQuery struct {
	ProjectID   string
	Environment string
	StartTime   time.Time
	EndTime     time.Time
	Model       string
	Provider    string
	Status      SpanStatus
	Search      string
	OrderBy     string
	Descending  bool
	PageSize    int
	PageToken   string
}

// TraceResult is the paginated result of a trace query.
type TraceResult struct {
	Traces        []TraceSummary
	NextPageToken string
	TotalCount    int
}

// SpanQuery defines filters for searching individual spans.
type SpanQuery struct {
	ProjectID    string
	StartTime    time.Time
	EndTime      time.Time
	Kind         SpanKind
	Model        string
	NameContains string
	PageSize     int
	PageToken    string
}

// SpanResult is the paginated result of a span query.
type SpanResult struct {
	Spans         []Span
	NextPageToken string
	TotalCount    int
}

// UsageSummary holds aggregated usage metrics.
type UsageSummary struct {
	TotalTraces      int64
	TotalSpans       int64
	TotalLLMCalls    int64
	TotalInputTokens int64
	TotalOutputTokens int64
	TotalCostUSD     float64
	AvgLatencyMs     float64
	ErrorRate        float64
}

// UsageQuery defines filters for usage summary queries.
type UsageQuery struct {
	ProjectID   string
	Environment string
	StartTime   time.Time
	EndTime     time.Time
}

// ModelUsage holds per-model aggregated metrics.
type ModelUsage struct {
	Model        string
	Provider     string
	CallCount    int64
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
	AvgLatencyMs float64
}

// TraceStore is the core storage interface. Every backend implements this.
type TraceStore interface {
	// IngestSpans writes a batch of spans to storage.
	IngestSpans(ctx context.Context, spans []Span) error

	// GetTrace retrieves a single trace with all its spans.
	GetTrace(ctx context.Context, traceID string) (*Trace, error)

	// QueryTraces returns a paginated list of trace summaries.
	QueryTraces(ctx context.Context, q TraceQuery) (*TraceResult, error)

	// SearchSpans searches for individual spans matching criteria.
	SearchSpans(ctx context.Context, q SpanQuery) (*SpanResult, error)

	// GetUsageSummary returns aggregated usage metrics.
	GetUsageSummary(ctx context.Context, q UsageQuery) (*UsageSummary, error)

	// GetModelBreakdown returns usage broken down by model.
	GetModelBreakdown(ctx context.Context, q UsageQuery) ([]ModelUsage, error)

	// Ping verifies that the storage backend is reachable.
	Ping(ctx context.Context) error

	// Close releases any resources held by the store.
	Close() error
}
