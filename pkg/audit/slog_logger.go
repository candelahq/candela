package audit

import (
	"context"
	"log/slog"
)

// SlogLogger writes audit events as structured JSON via slog.
// On Cloud Run, slog output goes to Cloud Logging where it is
// queryable, alertable, and exportable to BigQuery via log sinks.
type SlogLogger struct{}

// NewSlogLogger creates a new SlogLogger.
func NewSlogLogger() *SlogLogger {
	return &SlogLogger{}
}

// Log writes a structured audit event at INFO level.
func (l *SlogLogger) Log(ctx context.Context, e Event) {
	slog.InfoContext(ctx, "audit",
		slog.String("actor_email", e.ActorEmail),
		slog.String("actor_id", e.ActorID),
		slog.String("service", e.Service),
		slog.String("method", e.Method),
		slog.String("procedure", e.Procedure),
		slog.String("status", e.StatusCode),
		slog.String("error", e.Error),
	)
}
