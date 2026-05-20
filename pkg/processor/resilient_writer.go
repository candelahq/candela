package processor

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// SinkConfig configures a resilient writer wrapper for a storage sink.
// All fields have sensible defaults; only Writer and Name are required.
type SinkConfig struct {
	Writer              storage.SpanWriter
	Name                string
	Timeout             time.Duration // Per-write timeout (default: 30s)
	BreakerThreshold    int           // Consecutive failures to trip (default: 5)
	BreakerResetTimeout time.Duration // Wait before half-open probe (default: 30s)
	BreakerHalfOpenMax  int           // Successes to re-close (default: 2)
	BulkheadSize        int           // Max concurrent writes (default: 4)
}

// ResilientWriter wraps a storage.SpanWriter with per-sink circuit breaking,
// bulkhead isolation (semaphore), and configurable timeouts.
//
// When the circuit is open, writes are silently skipped — secondary sinks
// losing data is acceptable since the primary always receives the write.
//
// The bulkhead prevents a slow sink from consuming unbounded goroutines.
// If the semaphore is full, writes are dropped with a warning rather than
// blocking other sinks.
type ResilientWriter struct {
	inner     storage.SpanWriter
	name      string
	breaker   *Breaker
	semaphore chan struct{} // Bounded goroutine pool
	timeout   time.Duration

	// Metrics (atomic for lock-free reads).
	totalWrites   atomic.Int64
	totalFailures atomic.Int64
	totalDropped  atomic.Int64
	lastSuccess   atomic.Int64 // Unix nano timestamp
	lastFailure   atomic.Int64 // Unix nano timestamp
}

// NewResilientWriter creates a ResilientWriter with the given config.
// Uses realClock for production; tests should use newResilientWriterWithClock.
func NewResilientWriter(cfg SinkConfig) *ResilientWriter {
	return newResilientWriterWithClock(cfg, realClock{})
}

// newResilientWriterWithClock creates a ResilientWriter with an injected clock.
func newResilientWriterWithClock(cfg SinkConfig, clock Clock) *ResilientWriter {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.BulkheadSize <= 0 {
		cfg.BulkheadSize = 4
	}

	breaker := NewBreaker(BreakerConfig{
		Threshold:    cfg.BreakerThreshold,
		ResetTimeout: cfg.BreakerResetTimeout,
		HalfOpenMax:  cfg.BreakerHalfOpenMax,
	}, clock)

	return &ResilientWriter{
		inner:     cfg.Writer,
		name:      cfg.Name,
		breaker:   breaker,
		semaphore: make(chan struct{}, cfg.BulkheadSize),
		timeout:   cfg.Timeout,
	}
}

// IngestSpans writes a batch of spans through the resilient wrapper.
// Returns nil if the write was skipped (circuit open or bulkhead saturated).
func (rw *ResilientWriter) IngestSpans(ctx context.Context, spans []storage.Span) error {
	// Circuit breaker check.
	if !rw.breaker.Allow() {
		slog.Debug("sink circuit open, skipping write",
			"sink", rw.name,
			"state", rw.breaker.State().String(),
			"spans", len(spans),
		)
		rw.totalDropped.Add(int64(len(spans)))
		return nil
	}

	// Bulkhead: try to acquire a semaphore slot without blocking.
	select {
	case rw.semaphore <- struct{}{}:
		defer func() { <-rw.semaphore }()
	default:
		// Bulkhead saturated — drop the write to avoid blocking other sinks.
		slog.Warn("sink bulkhead saturated, dropping write",
			"sink", rw.name,
			"spans", len(spans),
		)
		rw.totalDropped.Add(int64(len(spans)))
		return nil
	}

	// Apply per-sink timeout.
	writeCtx, cancel := context.WithTimeout(ctx, rw.timeout)
	defer cancel()

	rw.totalWrites.Add(1)

	if err := rw.inner.IngestSpans(writeCtx, spans); err != nil {
		rw.totalFailures.Add(1)
		rw.lastFailure.Store(time.Now().UnixNano())
		rw.breaker.RecordFailure()
		slog.Error("sink write failed",
			"sink", rw.name,
			"error", err,
			"spans", len(spans),
			"breaker_state", rw.breaker.State().String(),
		)
		return err
	}

	rw.lastSuccess.Store(time.Now().UnixNano())
	rw.breaker.RecordSuccess()
	return nil
}

// Close releases resources held by the underlying writer.
func (rw *ResilientWriter) Close() error {
	return rw.inner.Close()
}

// Health returns the current health status of this sink.
func (rw *ResilientWriter) Health() SinkHealth {
	h := SinkHealth{
		Name:          rw.name,
		State:         rw.breaker.State().String(),
		TotalWrites:   rw.totalWrites.Load(),
		TotalFailures: rw.totalFailures.Load(),
		TotalDropped:  rw.totalDropped.Load(),
	}

	if ts := rw.lastSuccess.Load(); ts > 0 {
		t := time.Unix(0, ts)
		h.LastSuccess = &t
	}
	if ts := rw.lastFailure.Load(); ts > 0 {
		t := time.Unix(0, ts)
		h.LastFailure = &t
	}

	return h
}
