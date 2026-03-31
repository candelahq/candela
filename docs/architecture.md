# Candela Storage Architecture

## Overview

Candela uses a **CQRS (Command Query Responsibility Segregation)** storage architecture. This separates write and read concerns into distinct interfaces, enabling flexible multi-sink configurations.

## Interface Design

```go
// SpanWriter is a write-only destination for spans.
type SpanWriter interface {
    IngestSpans(ctx context.Context, spans []Span) error
    Ping(ctx context.Context) error
}

// SpanReader is a read-only source for querying spans and traces.
type SpanReader interface {
    GetTrace(ctx context.Context, traceID string) (*Trace, error)
    QueryTraces(ctx context.Context, query TraceQuery) (*TraceResult, error)
    SearchSpans(ctx context.Context, query SpanQuery) (*SpanResult, error)
    GetUsageSummary(ctx context.Context, query UsageQuery) (*UsageSummary, error)
    GetModelBreakdown(ctx context.Context, query UsageQuery) ([]ModelUsage, error)
    Ping(ctx context.Context) error
}

// TraceStore combines both for backends that support full read/write.
type TraceStore interface {
    SpanWriter
    SpanReader
}
```

## Data Flow

```
                    ┌─────────────┐
                    │  Proxy /    │
                    │  ConnectRPC │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │    Span     │
                    │  Processor  │  ← batches spans, applies cost calc
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │  DuckDB  │ │ BigQuery │ │  Pub/Sub │
        │ (Writer  │ │ (Writer  │ │ (Writer  │
        │ + Reader)│ │ + Reader)│ │  Only)   │
        └──────────┘ └──────────┘ └──────────┘
              │            │
              ▼            ▼
        ┌──────────────────────┐
        │   Dashboard / API    │  ← reads from one SpanReader
        │  (ConnectRPC + REST) │
        └──────────────────────┘
```

## Storage Backends

### DuckDB (Default)

**Best for**: Local dev, edge deployments, single-server production.

- **Driver**: `github.com/duckdb/duckdb-go/v2` (official)
- **Write API**: DuckDB `Appender` (columnar batch insert, not SQL INSERT)
- **Schema**: OLAP-optimized — no `PRIMARY KEY` (duplicates rare, handled at query time)
- **Attributes**: `ARRAY<STRUCT<key VARCHAR, value VARCHAR>>`
- **File**: Single `.duckdb` file, supports concurrent reads

```yaml
storage:
  backend: "duckdb"
  duckdb:
    path: "candela.duckdb"
```

### SQLite

**Best for**: Lightweight development, embedded testing.

- **Driver**: `modernc.org/sqlite` (pure Go, CGO-free)
- **Write API**: Batched SQL INSERT with transaction wrapping
- **Schema**: Standard relational with `PRIMARY KEY`
- **Attributes**: JSON-serialized `TEXT` column

```yaml
storage:
  backend: "sqlite"
  sqlite:
    path: "candela.db"  # or ":memory:" for ephemeral
```

### BigQuery

**Best for**: Production at scale, serverless analytics.

- **Driver**: `cloud.google.com/go/bigquery`
- **Write API**: BigQuery streaming insert with dedup keys (`trace_id-span_id`)
- **Schema**: Auto-provisioned with time partitioning (`start_time`, DAY) and clustering (`project_id`, `trace_id`)
- **Attributes**: `ARRAY<STRUCT<key STRING, value STRING>>`
- **Auth**: Application Default Credentials (ADC)

```yaml
storage:
  backend: "bigquery"
  bigquery:
    project_id: "my-gcp-project"
    dataset: "candela"
    table: "spans"       # default
    location: "US"       # default
```

### Pub/Sub (Sink Only)

**Best for**: Event-driven fan-out to downstream consumers (analytics pipelines, alerting, data lakes).

- **Driver**: `cloud.google.com/go/pubsub`
- **Format**: JSON serialization with `trace_id`, `span_id`, `project_id` as message attributes
- **Batching**: 100 messages or 1MB threshold
- **Ordering**: Not guaranteed (Pub/Sub semantics)
- **Auth**: Application Default Credentials (ADC)
- **Note**: Write-only `SpanWriter` — does NOT implement `SpanReader`

```yaml
sinks:
  pubsub:
    enabled: true
    project_id: "my-gcp-project"
    topic: "candela-spans"
```

## Schema Design

All backends share the same logical schema:

| Column | DuckDB | BigQuery | SQLite |
|--------|--------|----------|--------|
| span_id | VARCHAR | STRING | TEXT |
| trace_id | VARCHAR | STRING | TEXT |
| parent_span_id | VARCHAR | STRING | TEXT |
| name | VARCHAR | STRING | TEXT |
| kind | INTEGER | INTEGER | INTEGER |
| status | INTEGER | INTEGER | INTEGER |
| status_message | VARCHAR | STRING | TEXT |
| start_time | TIMESTAMP | TIMESTAMP | TEXT (RFC3339) |
| end_time | TIMESTAMP | TIMESTAMP | TEXT (RFC3339) |
| duration_ns | BIGINT | INT64 | INTEGER |
| project_id | VARCHAR | STRING | TEXT |
| environment | VARCHAR | STRING | TEXT |
| service_name | VARCHAR | STRING | TEXT |
| gen_ai_model | VARCHAR | STRING | TEXT |
| gen_ai_provider | VARCHAR | STRING | TEXT |
| gen_ai_input_tokens | BIGINT | INT64 | INTEGER |
| gen_ai_output_tokens | BIGINT | INT64 | INTEGER |
| gen_ai_total_tokens | BIGINT | INT64 | INTEGER |
| gen_ai_cost_usd | DOUBLE | FLOAT64 | REAL |
| attributes | STRUCT[] | STRUCT[] | TEXT (JSON) |

### Key Design Decisions

1. **No PRIMARY KEY** (DuckDB/BigQuery): OLAP convention. Duplicates are rare in tracing and handled at query time. This enables high-throughput batch ingestion.
2. **Structured attributes** (DuckDB/BigQuery): `ARRAY<STRUCT<key, value>>` instead of JSON enables efficient per-key filtering with standard SQL.
3. **Time partitioning** (BigQuery): `start_time` partitioned by DAY reduces scan costs for time-scoped queries.
4. **Clustering** (BigQuery): `(project_id, trace_id)` optimizes the two most common access patterns.

## CORS Configuration

CORS origins are configurable for frontend development:

```yaml
cors:
  allowed_origins:
    - "http://localhost:3000"   # Next.js dev server
    - "http://localhost:8080"   # Same-origin
    # - "*"                     # Allow all (not for production)
```

Defaults to `localhost:3000` + `localhost:8080` if omitted.

## Adding a New Backend

1. Create `pkg/storage/mybackend/mybackend.go`
2. Implement `storage.SpanWriter` (minimum) or `storage.TraceStore` (full)
3. Add config struct and `initStorage` case in `cmd/candela-server/main.go`
4. For write-only sinks, add to the `sinks` config section instead
