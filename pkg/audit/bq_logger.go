package audit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
)

const (
	bqTableName         = "admin_audit_log"
	defaultBQBufferSize = 256
)

// bqRow is the BigQuery row schema for audit events.
type bqRow struct {
	Timestamp  bigquery.NullTimestamp `bigquery:"timestamp"`
	ActorEmail string                 `bigquery:"actor_email"`
	ActorID    string                 `bigquery:"actor_id"`
	Service    string                 `bigquery:"service"`
	Method     string                 `bigquery:"method"`
	Procedure  string                 `bigquery:"procedure"`
	StatusCode string                 `bigquery:"status_code"`
	Error      string                 `bigquery:"error"`
}

// BQLogger writes audit events to a BigQuery table asynchronously.
// Events are buffered in a channel and written by a background goroutine,
// keeping BigQuery latency off the RPC critical path.
// Create one via NewBQLogger; call Close when done.
type BQLogger struct {
	client     *bigquery.Client
	inserter   *bigquery.Inserter
	events     chan bqRow
	done       chan struct{}
	ownsClient bool
}

// BQConfig holds BigQuery audit logger configuration.
//
// Note: BufferSize makes the event channel buffer capacity configurable (defaulting
// to 256 when zero or negative). Adding BufferSize is a source-incompatible change
// for callers constructing unkeyed struct literals; callers must use keyed literals
// (e.g. BQConfig{ProjectID: "...", Dataset: "...", BufferSize: ...}).
type BQConfig struct {
	ProjectID string
	Dataset   string
	// BufferSize sets the channel buffer capacity for asynchronous BigQuery writes.
	// When zero or negative, a default of 256 is used.
	BufferSize int
	// Client is an optional existing BigQuery client to reuse.
	// When non-nil, NewBQLogger and EnsureTable reuse this client rather than creating a new one.
	// When NewBQLogger reuses an injected client, Close will not close the client.
	Client *bigquery.Client
}

func bqBufferSize(cfg BQConfig) int {
	if cfg.BufferSize > 0 {
		return cfg.BufferSize
	}
	return defaultBQBufferSize
}

// NewBQLogger creates a BigQuery audit logger with an async write loop.
// It does NOT create the table — call EnsureTable separately if needed.
// If cfg.Client is provided, it is reused and will NOT be closed by Close().
func NewBQLogger(ctx context.Context, cfg BQConfig) (*BQLogger, error) {
	var (
		client     *bigquery.Client
		ownsClient bool
	)
	if cfg.Client != nil {
		client = cfg.Client
		ownsClient = false
	} else {
		var err error
		client, err = bigquery.NewClient(ctx, cfg.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("audit: failed to create BigQuery client: %w", err)
		}
		ownsClient = true
	}

	table := client.Dataset(cfg.Dataset).Table(bqTableName)
	l := &BQLogger{
		client:     client,
		inserter:   table.Inserter(),
		events:     make(chan bqRow, bqBufferSize(cfg)),
		done:       make(chan struct{}),
		ownsClient: ownsClient,
	}
	go l.writeLoop()
	return l, nil
}

// writeLoop drains the events channel and writes rows to BigQuery.
// Each insert gets a 30-second timeout to prevent hung BigQuery from
// blocking the loop and causing the channel buffer to fill up.
func (l *BQLogger) writeLoop() {
	defer close(l.done)
	for row := range l.events {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := l.inserter.Put(ctx, row); err != nil {
			slog.Warn("audit: failed to write to BigQuery",
				"error", err,
				"procedure", row.Procedure,
				"actor", row.ActorEmail)
		}
		cancel()
	}
}

// Log enqueues an audit event for async writing to BigQuery.
// If the buffer is full, the event is dropped with a warning.
func (l *BQLogger) Log(_ context.Context, e Event) {
	row := bqRow{
		Timestamp:  bigquery.NullTimestamp{Timestamp: e.Timestamp, Valid: true},
		ActorEmail: e.ActorEmail,
		ActorID:    e.ActorID,
		Service:    e.Service,
		Method:     e.Method,
		Procedure:  e.Procedure,
		StatusCode: e.StatusCode,
		Error:      e.Error,
	}
	select {
	case l.events <- row:
	default:
		slog.Warn("audit: BQ event buffer full, dropping event",
			"procedure", e.Procedure,
			"actor", e.ActorEmail)
	}
}

// Close drains the event buffer and releases the BigQuery client (if owned by BQLogger).
func (l *BQLogger) Close() error {
	close(l.events)
	<-l.done // wait for writeLoop to finish
	if l.ownsClient && l.client != nil {
		return l.client.Close()
	}
	return nil
}

// EnsureTable creates the admin_audit_log table if it does not already exist.
// If cfg.Client is non-nil, it is reused directly without creating or closing a client.
// Otherwise, a new client is created and closed when the operation completes.
func EnsureTable(ctx context.Context, cfg BQConfig) error {
	if cfg.Client != nil {
		return EnsureTableWithClient(ctx, cfg.Client, cfg.Dataset)
	}

	client, err := bigquery.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("audit: failed to create BigQuery client: %w", err)
	}
	defer func() { _ = client.Close() }()

	return EnsureTableWithClient(ctx, client, cfg.Dataset)
}

// EnsureTableWithClient creates the admin_audit_log table using the provided BigQuery client
// if it does not already exist. It does not close the client.
func EnsureTableWithClient(ctx context.Context, client *bigquery.Client, dataset string) error {
	if client == nil {
		return errors.New("audit: bigquery client is required")
	}

	schema := bigquery.Schema{
		{Name: "timestamp", Type: bigquery.TimestampFieldType, Required: true},
		{Name: "actor_email", Type: bigquery.StringFieldType, Required: true},
		{Name: "actor_id", Type: bigquery.StringFieldType},
		{Name: "service", Type: bigquery.StringFieldType, Required: true},
		{Name: "method", Type: bigquery.StringFieldType, Required: true},
		{Name: "procedure", Type: bigquery.StringFieldType, Required: true},
		{Name: "status_code", Type: bigquery.StringFieldType, Required: true},
		{Name: "error", Type: bigquery.StringFieldType},
	}

	table := client.Dataset(dataset).Table(bqTableName)
	md := &bigquery.TableMetadata{
		Schema: schema,
		TimePartitioning: &bigquery.TimePartitioning{
			Field: "timestamp",
			Type:  bigquery.DayPartitioningType,
		},
		Clustering: &bigquery.Clustering{
			Fields: []string{"service", "actor_email"},
		},
	}

	if err := table.Create(ctx, md); err != nil {
		// Idempotent: if table already exists, that's fine.
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code == 409 {
			slog.Info("audit: BigQuery table already exists", "table", bqTableName)
			return nil
		}
		return fmt.Errorf("audit: failed to create table %s: %w", bqTableName, err)
	}
	slog.Info("audit: BigQuery table created", "table", bqTableName, "dataset", dataset)
	return nil
}
