package audit

import (
	"context"
	"fmt"
	"log/slog"

	"cloud.google.com/go/bigquery"
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

// BQLogger writes audit events to a BigQuery table.
// Create one via NewBQLogger; call Close when done.
type BQLogger struct {
	client   *bigquery.Client
	inserter *bigquery.Inserter
}

// BQConfig holds BigQuery audit logger configuration.
type BQConfig struct {
	ProjectID string
	Dataset   string
}

// NewBQLogger creates a BigQuery audit logger.
// It does NOT create the table — call EnsureTable separately if needed.
func NewBQLogger(ctx context.Context, cfg BQConfig) (*BQLogger, error) {
	client, err := bigquery.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("audit: failed to create BigQuery client: %w", err)
	}

	table := client.Dataset(cfg.Dataset).Table(bqTableName)
	return &BQLogger{
		client:   client,
		inserter: table.Inserter(),
	}, nil
}

// Log writes an audit event to BigQuery.
func (l *BQLogger) Log(ctx context.Context, e Event) {
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
	if err := l.inserter.Put(ctx, row); err != nil {
		slog.WarnContext(ctx, "audit: failed to write to BigQuery",
			"error", err,
			"procedure", e.Procedure,
			"actor", e.ActorEmail)
	}
}

// Close releases the BigQuery client resources.
func (l *BQLogger) Close() error {
	return l.client.Close()
}

// EnsureTable creates the admin_audit_log table if it does not exist.
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
		return fmt.Errorf("audit: failed to create table %s: %w", bqTableName, err)
	}
	slog.Info("audit: BigQuery table created", "table", bqTableName, "dataset", cfg.Dataset)
	return nil
}
