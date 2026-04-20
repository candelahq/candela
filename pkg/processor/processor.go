// Package processor provides a shared span processing pipeline that buffers
// incoming spans and flushes them to one or more storage sinks in batches.
// Used by both candela-server and candela-local for consistent span handling.
package processor

import (
	"context"
	"sync"
	"time"

	"log/slog"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// SpanProcessor buffers incoming spans and flushes them to storage in batches.
// Supports fan-out to multiple SpanWriter sinks (e.g. DuckDB + Pub/Sub).
type SpanProcessor struct {
	writers   []storage.SpanWriter
	calc      *costcalc.Calculator
	batchSize int
	spanCh    chan storage.Span
	done      chan struct{}
	once      sync.Once
}

// New creates a new in-process span processor.
// All provided writers receive every batch on flush.
func New(writers []storage.SpanWriter, calc *costcalc.Calculator, batchSize int) *SpanProcessor {
	if batchSize <= 0 {
		batchSize = 100
	}
	return &SpanProcessor{
		writers:   writers,
		calc:      calc,
		batchSize: batchSize,
		spanCh:    make(chan storage.Span, batchSize*10),
		done:      make(chan struct{}),
	}
}

// Submit adds a span to the processing pipeline.
func (p *SpanProcessor) Submit(span storage.Span) {
	select {
	case p.spanCh <- span:
	default:
		slog.Warn("span processor buffer full, dropping span")
	}
}

// SubmitBatch adds multiple spans to the processing pipeline.
func (p *SpanProcessor) SubmitBatch(spans []storage.Span) {
	for _, s := range spans {
		p.Submit(s)
	}
}

// Run starts the processing loop. Call from a goroutine.
func (p *SpanProcessor) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var batch []storage.Span

	flush := func() {
		if len(batch) == 0 {
			return
		}

		// Enrich with cost data.
		for i := range batch {
			if batch[i].GenAI != nil && batch[i].GenAI.CostUSD == 0 {
				batch[i].GenAI.CostUSD = p.calc.Calculate(
					batch[i].GenAI.Provider,
					batch[i].GenAI.Model,
					batch[i].GenAI.InputTokens,
					batch[i].GenAI.OutputTokens,
				)
			}
		}

		// Fan-out: write to all sinks independently.
		for _, w := range p.writers {
			if err := w.IngestSpans(ctx, batch); err != nil {
				slog.Error("failed to flush spans", "error", err, "count", len(batch))
			}
		}
		slog.Debug("flushed spans to storage", "count", len(batch), "sinks", len(p.writers))
		batch = batch[:0]
	}

	for {
		select {
		case span := <-p.spanCh:
			batch = append(batch, span)
			if len(batch) >= p.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-p.done:
			// Drain remaining spans.
			close(p.spanCh)
			for span := range p.spanCh {
				batch = append(batch, span)
			}
			flush()
			return
		case <-ctx.Done():
			flush()
			return
		}
	}
}

// Stop signals the processor to flush and stop.
func (p *SpanProcessor) Stop() {
	p.once.Do(func() {
		close(p.done)
	})
}
