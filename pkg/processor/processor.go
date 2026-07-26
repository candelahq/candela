// Package processor provides a shared span processing pipeline that buffers
// incoming spans and flushes them to one or more storage sinks in batches.
// Used by both candela-server and candela-local for consistent span handling.
package processor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/candelahq/candela/pkg/anomaly"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// SpanProcessor buffers incoming spans and flushes them to storage in batches.
// Supports fan-out to multiple SpanWriter sinks (e.g. DuckDB + Pub/Sub).
// Each sink is wrapped in a ResilientWriter with circuit breaking, bulkhead
// isolation, and configurable timeouts.
type SpanProcessor struct {
	writers      []*ResilientWriter
	calc         *costcalc.Calculator
	detector     *anomaly.Detector
	batchSize    int
	spanCh       chan storage.Span
	done         chan struct{}
	once         sync.Once
	droppedSpans atomic.Int64

	subMu       sync.RWMutex
	subscribers []*subscriber
}

// TraceBroadcast is the data sent to watch subscribers.
type TraceBroadcast struct {
	TraceID      string  `json:"trace_id"`
	RootSpanName string  `json:"root_name"`
	Model        string  `json:"model"`
	Provider     string  `json:"provider"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMs   int64   `json:"duration_ms"`
	SpanCount    int     `json:"span_count"`
	Status       string  `json:"status"` // "ok" or "error"
	Timestamp    string  `json:"time"`
	ProjectID    string  `json:"project_id,omitempty"`
}

// WatchFilter controls which traces a subscriber receives.
type WatchFilter struct {
	ProjectID string
	Model     string
	Provider  string
}

type subscriber struct {
	ch      chan TraceBroadcast
	filter  WatchFilter
	dropped atomic.Int64
}

// WithAnomalyDetector attaches a Detector to the processor. When set, the
// detector runs after cost enrichment on every flush batch and logs flagged
// spans. Passing nil is a no-op (detector stays disabled).
func (p *SpanProcessor) WithAnomalyDetector(d *anomaly.Detector) {
	p.detector = d
}

// New creates a new in-process span processor with default resilience settings.
// All provided writers receive every batch on flush, each wrapped in a
// ResilientWriter with default circuit breaker and bulkhead config.
// For fine-grained control, use NewWithConfig.
func New(writers []storage.SpanWriter, calc *costcalc.Calculator, batchSize int) *SpanProcessor {
	configs := make([]SinkConfig, len(writers))
	for i, w := range writers {
		configs[i] = SinkConfig{
			Writer: w,
			Name:   fmt.Sprintf("sink-%d", i),
		}
	}
	return NewWithConfig(configs, calc, batchSize)
}

// NewWithConfig creates a span processor with explicit per-sink configuration.
// Each SinkConfig controls the circuit breaker threshold, bulkhead size,
// and write timeout for its sink independently.
func NewWithConfig(configs []SinkConfig, calc *costcalc.Calculator, batchSize int) *SpanProcessor {
	if batchSize <= 0 {
		batchSize = 100
	}

	writers := make([]*ResilientWriter, len(configs))
	for i, cfg := range configs {
		if cfg.Name == "" {
			cfg.Name = fmt.Sprintf("sink-%d", i)
		}
		writers[i] = NewResilientWriter(cfg)
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
		// NOTE: Proxy-generated spans already have CostUSD set by buildSpan,
		// so the CostUSD==0 guard below is a no-op for them. This path exists
		// for SDK-ingested spans (enrichment SDKs, OTel) that arrive without cost.
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

		// Run anomaly detection after cost enrichment, before writing to sinks.
		// Flagged spans get candela.anomaly=true stamped into their Attributes so
		// the dashboard and audit trail can surface them without schema changes.
		if p.detector != nil {
			results, err := p.detector.Detect(ctx, batch)
			if err != nil {
				slog.Warn("anomaly detection error", "err", err)
			} else if len(results) > 0 {
				// Build a spanID-to-index map once so attribute propagation is
				// O(N + M) instead of O(N * M).
				spanIdx := make(map[string]int, len(batch))
				for i := range batch {
					spanIdx[batch[i].SpanID] = i
				}
				for _, r := range results {
					slog.Warn("anomaly detected",
						"span_id", r.Span.SpanID,
						"metric", r.Metric,
						"value", r.Value,
						"mean", r.Mean,
						"sigma", r.Sigma,
						"tenant_id", r.Span.TenantID,
						"model", r.Span.GenAI.Model,
					)
					// Propagate anomaly attributes back into the batch span so
					// downstream sinks store the flag without a second write.
					if idx, ok := spanIdx[r.Span.SpanID]; ok {
						if batch[idx].Attributes == nil {
							batch[idx].Attributes = make(map[string]string)
						}
						batch[idx].Attributes[anomaly.AttrAnomaly] = r.Span.Attributes[anomaly.AttrAnomaly]
						batch[idx].Attributes[anomaly.AttrAnomalyReason] = r.Span.Attributes[anomaly.AttrAnomalyReason]
					}
				}
			}
		}

		// Fan-out: write to all sinks independently and in parallel.
		// Each ResilientWriter handles its own circuit breaker, bulkhead,
		// and timeout — no raw goroutine management needed here.
		// Deep-clone each span's reference types to prevent cross-sink mutation.
		var wg sync.WaitGroup
		for _, rw := range p.writers {
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
			go func(rw *ResilientWriter, batch []storage.Span) {
				defer wg.Done()
				// ResilientWriter applies its own timeout, circuit breaker,
				// and bulkhead internally — just call IngestSpans.
				_ = rw.IngestSpans(ctx, batch)
			}(rw, sinkBatch)
		}
		wg.Wait()
		// Broadcast to watch subscribers (best-effort, non-blocking).
		p.broadcastToWatchers(batch)
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
			// Drain remaining buffered spans before exiting so we don't silently
			// drop data when the context is cancelled. This mirrors the p.done path.
			// NOTE: we do NOT close p.spanCh here (only Stop() owns that), so we
			// use a non-blocking drain loop instead of ranging over the channel.
		drainCtx:
			for {
				select {
				case span := <-p.spanCh:
					batch = append(batch, span)
				default:
					break drainCtx
				}
			}
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

// SinkHealth returns the health status of all configured sinks.
// Each entry reports the circuit breaker state, write counts, and
// failure metrics for a single sink.
func (p *SpanProcessor) SinkHealth() []SinkHealth {
	health := make([]SinkHealth, len(p.writers))
	for i, rw := range p.writers {
		health[i] = rw.Health()
	}
	return health
}

// Subscribe registers a trace watcher. Returns a read channel and cleanup func.
// The channel is buffered (cap=100). Slow consumers get dropped events, never blocking ingestion.
func (p *SpanProcessor) Subscribe(filter WatchFilter) (<-chan TraceBroadcast, func()) {
	ch := make(chan TraceBroadcast, 100)
	sub := &subscriber{ch: ch, filter: filter}

	p.subMu.Lock()
	p.subscribers = append(p.subscribers, sub)
	p.subMu.Unlock()

	slog.Info("watch subscriber added", "total", len(p.subscribers))

	cleanup := func() {
		p.subMu.Lock()
		defer p.subMu.Unlock()
		for i, s := range p.subscribers {
			if s == sub {
				p.subscribers = append(p.subscribers[:i], p.subscribers[i+1:]...)
				close(ch)
				slog.Info("watch subscriber removed", "total", len(p.subscribers), "dropped", sub.dropped.Load())
				break
			}
		}
	}
	return ch, cleanup
}

func (p *SpanProcessor) broadcastToWatchers(spans []storage.Span) {
	p.subMu.RLock()
	defer p.subMu.RUnlock()
	if len(p.subscribers) == 0 {
		return
	}

	// Group spans by trace to build per-trace summaries.
	traces := make(map[string]*TraceBroadcast)
	for _, s := range spans {
		tb, ok := traces[s.TraceID]
		if !ok {
			status := "ok"
			if s.Status == storage.SpanStatusError {
				status = "error"
			}
			tb = &TraceBroadcast{
				TraceID:   s.TraceID,
				Timestamp: s.StartTime.Format("15:04:05"),
				ProjectID: s.ProjectID,
				Status:    status,
			}
			traces[s.TraceID] = tb
		}
		tb.SpanCount++
		if s.ParentSpanID == "" {
			tb.RootSpanName = s.Name
			tb.DurationMs = s.EndTime.Sub(s.StartTime).Milliseconds()
		}
		if s.GenAI != nil {
			tb.CostUSD += s.GenAI.CostUSD
			if tb.Model == "" {
				tb.Model = s.GenAI.Model
				tb.Provider = s.GenAI.Provider
			}
		}
		if s.Status == storage.SpanStatusError {
			tb.Status = "error"
		}
	}

	for _, tb := range traces {
		for _, sub := range p.subscribers {
			if sub.filter.ProjectID != "" && sub.filter.ProjectID != tb.ProjectID {
				continue
			}
			if sub.filter.Model != "" && sub.filter.Model != tb.Model {
				continue
			}
			if sub.filter.Provider != "" && sub.filter.Provider != tb.Provider {
				continue
			}
			select {
			case sub.ch <- *tb:
			default:
				sub.dropped.Add(1)
			}
		}
	}
}
