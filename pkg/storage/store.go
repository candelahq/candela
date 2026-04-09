// Package storage defines the TraceStore interface — the core storage abstraction
// in Candela. Every storage backend (DuckDB, SQLite, BigQuery) implements
// this interface.
package storage

import (
	"context"
	"strings"
	"time"
)

// EscapeLike escapes SQL LIKE wildcard characters (% and _) in user input.
// Without this, a search for "100%" would match "100", "1000", etc.
// Both DuckDB and SQLite use backslash as the ESCAPE character.
func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

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
	Model         string  `json:"model,omitempty"`
	Provider      string  `json:"provider,omitempty"`
	InputTokens   int64   `json:"input_tokens,omitempty"`
	OutputTokens  int64   `json:"output_tokens,omitempty"`
	TotalTokens   int64   `json:"total_tokens,omitempty"`
	CostUSD       float64 `json:"cost_usd,omitempty"`
	Temperature   float64 `json:"temperature,omitempty"`
	MaxTokens     int64   `json:"max_tokens,omitempty"`
	TopP          float64 `json:"top_p,omitempty"`
	InputContent  string  `json:"input_content,omitempty"`
	OutputContent string  `json:"output_content,omitempty"`
}

// Span represents a single span in the storage layer.
type Span struct {
	SpanID        string            `json:"span_id"`
	TraceID       string            `json:"trace_id"`
	ParentSpanID  string            `json:"parent_span_id,omitempty"`
	Name          string            `json:"name"`
	Kind          SpanKind          `json:"kind"`
	Status        SpanStatus        `json:"status"`
	StatusMessage string            `json:"status_message,omitempty"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	Duration      time.Duration     `json:"duration"`
	GenAI         *GenAIAttributes  `json:"gen_ai,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
	ProjectID     string            `json:"project_id"`
	Environment   string            `json:"environment,omitempty"`
	ServiceName   string            `json:"service_name,omitempty"`
	UserID        string            `json:"user_id,omitempty"`
}

// TraceSummary is a lightweight summary for list views.
type TraceSummary struct {
	TraceID         string        `json:"trace_id"`
	StartTime       time.Time     `json:"start_time"`
	Duration        time.Duration `json:"duration"`
	RootSpanName    string        `json:"root_span_name"`
	ProjectID       string        `json:"project_id"`
	Environment     string        `json:"environment"`
	SpanCount       int           `json:"span_count"`
	LLMCallCount    int           `json:"llm_call_count"`
	TotalTokens     int64         `json:"total_tokens"`
	TotalCostUSD    float64       `json:"total_cost_usd"`
	Status          SpanStatus    `json:"status"`
	PrimaryModel    string        `json:"primary_model"`
	PrimaryProvider string        `json:"primary_provider"`
	UserID          string        `json:"user_id,omitempty"`
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
	UserID       string        `json:"user_id,omitempty"`
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
	UserID      string // Filter by user (empty = all, for admins)
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
	TotalTraces       int64
	TotalSpans        int64
	TotalLLMCalls     int64
	TotalInputTokens  int64
	TotalOutputTokens int64
	TotalCostUSD      float64
	AvgLatencyMs      float64
	ErrorRate         float64
}

// UsageQuery defines filters for usage summary queries.
type UsageQuery struct {
	ProjectID   string
	Environment string
	StartTime   time.Time
	EndTime     time.Time
	UserID      string // Filter by user (empty = all, for admins)
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

// SpanWriter is a write-only destination for spans.
// Any backend that can receive spans implements this — databases, Pub/Sub,
// S3 archivers, webhook forwarders, etc.
type SpanWriter interface {
	// IngestSpans writes a batch of spans to the destination.
	IngestSpans(ctx context.Context, spans []Span) error

	// Close releases any resources held by the writer.
	Close() error
}

// SpanReader serves the dashboard and API — it can query stored spans.
// Only backends that support querying implement this (databases).
type SpanReader interface {
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

	// Close releases any resources held by the reader.
	Close() error
}

// TraceStore combines read and write capabilities.
// Embedded databases (DuckDB, SQLite) implement this. In production,
// the write side (BigQuery Storage Write API) and read side (BigQuery SQL)
// may be separate implementations wired to separate SpanWriter/SpanReader consumers.
type TraceStore interface {
	SpanWriter
	SpanReader
}

// --- Project & API Key Management ---

// Project is a top-level organizational unit in Candela.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Environment string    `json:"environment,omitempty"` // default env for all spans in the project
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// APIKey authenticates ingestion and queries for a project.
type APIKey struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	KeyHash   string    `json:"-"`          // bcrypt hash (never exposed)
	KeyPrefix string    `json:"key_prefix"` // first 8 chars for identification
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// ProjectStore manages projects and API keys.
type ProjectStore interface {
	// CreateProject creates a new project.
	CreateProject(ctx context.Context, p Project) (*Project, error)

	// GetProject retrieves a project by ID.
	GetProject(ctx context.Context, id string) (*Project, error)

	// ListProjects returns all projects.
	ListProjects(ctx context.Context, limit, offset int) ([]Project, int, error)

	// UpdateProject updates a project's mutable fields (name, description, environment).
	UpdateProject(ctx context.Context, p Project) (*Project, error)

	// DeleteProject removes a project and its API keys.
	DeleteProject(ctx context.Context, id string) error

	// CreateAPIKey creates a new API key for a project.
	// Returns the full key only at creation time.
	CreateAPIKey(ctx context.Context, key APIKey, fullKey string) (*APIKey, error)

	// ListAPIKeys returns all keys for a project.
	ListAPIKeys(ctx context.Context, projectID string) ([]APIKey, error)

	// RevokeAPIKey deactivates an API key.
	RevokeAPIKey(ctx context.Context, id string) error

	// ValidateAPIKey checks a raw key against stored hashes. Returns the key record if valid.
	ValidateAPIKey(ctx context.Context, rawKey string) (*APIKey, error)
}
