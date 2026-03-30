// Package ingestion defines the Queue interface for decoupling span ingestion
// from processing/storage.
package ingestion

import (
	"context"

	"github.com/candelahq/candela/pkg/storage"
)

// Queue decouples span ingestion from storage writes.
// The server pushes spans into the queue; the worker pulls and processes them.
type Queue interface {
	// Push enqueues a batch of spans for processing.
	Push(ctx context.Context, spans []storage.Span) error

	// Pull dequeues a batch of spans. Blocks until spans are available or ctx is cancelled.
	Pull(ctx context.Context, batchSize int) ([]storage.Span, error)

	// Ping checks that the queue backend is reachable.
	Ping(ctx context.Context) error

	// Close releases queue resources.
	Close() error
}
