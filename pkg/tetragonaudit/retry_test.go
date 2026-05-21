package tetragonaudit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStreamEventsWithRetry_ReconnectsAfterFailure(t *testing.T) {
	src := &GRPCSource{addr: "test://localhost:54321"}

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	// Cancel after a short delay to let the retry loop attempt at least once.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// With nil conn, NewGRPCEventStreamAdapter will fail, triggering retry.
	// The retry should back off and eventually exit on context deadline.
	err := src.StreamEventsWithRetry(ctx, pipeline, RetryConfig{
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
	})
	// Should exit with context error (either deadline or cancel).
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got: %v", err)
	}

	// After retry loop exits, health should report disconnected.
	h := pipeline.Health()
	if h.Connected {
		t.Error("expected Connected=false after retry loop exits")
	}
	if h.Healthy {
		t.Error("expected Healthy=false after retry loop exits")
	}
}

func TestStreamEventsWithRetry_CancelledImmediately(t *testing.T) {
	src := &GRPCSource{addr: "test://localhost:54321"}
	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	start := time.Now()
	err := src.StreamEventsWithRetry(ctx, pipeline, RetryConfig{
		InitialDelay: time.Second,
	})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("took too long to exit: %v (should be near-instant)", elapsed)
	}
}

func TestBackoff_ExponentialWithCap(t *testing.T) {
	rc := RetryConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     2 * time.Second,
		Multiplier:   2.0,
	}

	tests := []struct {
		attempt  int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		// attempt 0: 100ms base ± 25% = [75ms, 125ms]
		{0, 75 * time.Millisecond, 125 * time.Millisecond},
		// attempt 1: 200ms ± 25% = [150ms, 250ms]
		{1, 150 * time.Millisecond, 250 * time.Millisecond},
		// attempt 2: 400ms ± 25% = [300ms, 500ms]
		{2, 300 * time.Millisecond, 500 * time.Millisecond},
		// attempt 10: capped at 2s ± 25% = [1.5s, 2.5s]
		{10, 1500 * time.Millisecond, 2500 * time.Millisecond},
	}

	for _, tt := range tests {
		// Run each sub-test multiple times to verify jitter range.
		for range 20 {
			d := backoff(tt.attempt, rc)
			if d < tt.minDelay || d > tt.maxDelay {
				t.Errorf("attempt %d: backoff=%v, want [%v, %v]",
					tt.attempt, d, tt.minDelay, tt.maxDelay)
			}
		}
	}
}

func TestBackoff_DefaultConfig(t *testing.T) {
	rc := RetryConfig{}.withDefaults()

	if rc.InitialDelay != time.Second {
		t.Errorf("InitialDelay=%v, want 1s", rc.InitialDelay)
	}
	if rc.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay=%v, want 30s", rc.MaxDelay)
	}
	if rc.Multiplier != 2.0 {
		t.Errorf("Multiplier=%v, want 2.0", rc.Multiplier)
	}
}
