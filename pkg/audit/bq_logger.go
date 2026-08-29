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

const bqTableName = "admin_audit_log"

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
	client   *bigquery.Client
	inserter *bigquery.Inserter
	events   chan bqRow
	done     chan struct{}
}

// BQConfig holds BigQuery audit logger configuration.
type BQConfig struct {
	ProjectID string
	Dataset   string
}

// NewBQLogger creates a BigQuery audit logger with an async write loop.
// It does NOT create the table — call EnsureTable separately if needed.
func NewBQLogger(ctx context.Context, cfg BQConfig) (*BQLogger, error) {
	client, err := bigquery.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("audit: failed to create BigQuery client: %w", err)
	}

	table := client.Dataset(cfg.Dataset).Table(bqTableName)
	l := &BQLogger{
		client:   client,
		inserter: table.Inserter(),
		events:   make(chan bqRow, 256),
		done:     make(chan struct{}),
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

// Close drains the event buffer and releases the BigQuery client.
func (l *BQLogger) Close() error {
	close(l.events)
	<-l.done // wait for writeLoop to finish
	return l.client.Close()
}

// EnsureTable creates the admin_audit_log table if it does not already exist.
func EnsureTable(ctx context.Context, cfg BQConfig) error {
	client, err := bigquery.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("audit: failed to create BigQuery client: %w", err)
	}
	defer func() { _ = client.Close() }()

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

	table := client.Dataset(cfg.Dataset).Table(bqTableName)
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
	slog.Info("audit: BigQuery table created", "table", bqTableName, "dataset", cfg.Dataset)
	return nil
}
