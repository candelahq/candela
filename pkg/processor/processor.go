// Package processor provides a shared span processing pipeline that buffers
// incoming spans and flushes them to one or more storage sinks in batches.
// Used by both candela-server and candela-local for consistent span handling.
package processor

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// SpanProcessor buffers incoming spans and flushes them to storage in batches.
// Supports fan-out to multiple SpanWriter sinks (e.g. DuckDB + Pub/Sub).
type SpanProcessor struct {
	writers      []storage.SpanWriter
	calc         *costcalc.Calculator
	batchSize    int
	spanCh       chan storage.Span
	done         chan struct{}
	once         sync.Once
	droppedSpans atomic.Int64
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
		dropped := p.droppedSpans.Add(1)
		slog.Warn("span processor buffer full, dropping span",
			"total_dropped", dropped)
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

		// Fan-out: write to all sinks independently and in parallel.
		// Deep-clone each span's reference types to prevent cross-sink mutation.
		var wg sync.WaitGroup
		for _, w := range p.writers {
			sinkBatch := make([]storage.Span, len(batch))
			for i, s := range batch {
				sinkBatch[i] = s
				// Deep-copy the GenAI pointer.
				if s.GenAI != nil {
					cp := *s.GenAI
					sinkBatch[i].GenAI = &cp
				}
				// Deep-copy the Attributes map.
				if s.Attributes != nil {
					attrs := make(map[string]string, len(s.Attributes))
					for k, v := range s.Attributes {
						attrs[k] = v
					}
					sinkBatch[i].Attributes = attrs
				}
			}
			wg.Add(1)
			go func(w storage.SpanWriter, batch []storage.Span) {
				defer wg.Done()
				sinkCtx, sinkCancel := context.WithTimeout(ctx, 30*time.Second)
				defer sinkCancel()
				if err := w.IngestSpans(sinkCtx, batch); err != nil {
					slog.Error("failed to flush spans", "error", err, "count", len(batch))
				}
			}(w, sinkBatch)
		}
		wg.Wait()
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

// DroppedSpans returns the total number of spans dropped due to buffer pressure.
func (p *SpanProcessor) DroppedSpans() int64 {
	return p.droppedSpans.Load()
}
