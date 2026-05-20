package tetragonaudit

import (
	"context"
)

// MultiSink fanks out/multiplexes audit records to multiple sinks.
type MultiSink struct {
	Sinks []Sink
}

// Emit sends an audit record to all configured sinks.
// It returns the first error encountered, but guarantees that Emit is called on all sinks.
func (m *MultiSink) Emit(ctx context.Context, record AuditRecord) error {
	var firstErr error
	for _, sink := range m.Sinks {
		if err := sink.Emit(ctx, record); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
