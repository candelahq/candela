package processor

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// slowWriter simulates a hanging sink that blocks for a configurable duration.
type slowWriter struct {
	mu      sync.Mutex
	batches [][]storage.Span
	delay   time.Duration
}

func (s *slowWriter) IngestSpans(ctx context.Context, spans []storage.Span) error {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]storage.Span, len(spans))
	copy(cp, spans)
	s.batches = append(s.batches, cp)
	return nil
}

func (s *slowWriter) Close() error { return nil }

// mutatingWriter modifies the span slice it receives (simulating a misbehaving sink).
type mutatingWriter struct {
	mu      sync.Mutex
	batches [][]storage.Span
}

func (m *mutatingWriter) IngestSpans(_ context.Context, spans []storage.Span) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]storage.Span, len(spans))
	copy(cp, spans)
	m.batches = append(m.batches, cp)
	// Mutate the slice to simulate a misbehaving writer.
	for i := range spans {
		spans[i].Name = "MUTATED"
		spans[i].ProjectID = "MUTATED"
		if spans[i].GenAI != nil {
			spans[i].GenAI.Model = "MUTATED"
		}
		if spans[i].Attributes != nil {
			spans[i].Attributes["injected"] = "true"
		}
	}
	return nil
}

func (m *mutatingWriter) Close() error { return nil }

// TestFanout_SinkIsolation verifies that one failing sink doesn't prevent
// other sinks from receiving spans.
func TestFanout_SinkIsolation(t *testing.T) {
	failing := &mockWriter{err: fmt.Errorf("permanent failure")}
	healthy := &mockWriter{}

	calc := costcalc.New()
	proc := New([]storage.SpanWriter{failing, healthy}, calc, 2)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	proc.Submit(testSpan("iso-1"))
	proc.Submit(testSpan("iso-2"))

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	// Healthy writer should have received spans despite the other failing.
	s := healthy.allSpans()
	if len(s) != 2 {
		t.Errorf("healthy writer got %d spans, want 2", len(s))
	}
}

// TestFanout_SliceMutationSafety verifies that a writer mutating its batch
// (including GenAI pointer and Attributes map) doesn't affect other writers.
func TestFanout_SliceMutationSafety(t *testing.T) {
	mutator := &mutatingWriter{}
	clean := &mockWriter{}

	calc := costcalc.New()
	proc := New([]storage.SpanWriter{mutator, clean}, calc, 2)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	span := testSpan("mutation-test")
	span.Name = "original"
	span.ProjectID = "proj-original"
	span.Attributes = map[string]string{"key": "value"}
	proc.Submit(span)
	proc.Submit(testSpan("mutation-test-2"))

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	// The clean writer should see the original data, not the mutations.
	cleanSpans := clean.allSpans()
	if len(cleanSpans) == 0 {
		t.Fatal("expected spans in clean writer")
	}

	for _, s := range cleanSpans {
		if s.Name == "MUTATED" || s.ProjectID == "MUTATED" {
			t.Errorf("clean writer received mutated span: name=%q project=%q", s.Name, s.ProjectID)
		}
		if s.GenAI != nil && s.GenAI.Model == "MUTATED" {
			t.Error("clean writer received mutated GenAI.Model — deep copy failed")
		}
		if s.Attributes != nil && s.Attributes["injected"] == "true" {
			t.Error("clean writer received injected attribute — deep copy failed")
		}
	}
}

// TestProcessor_SlowSinkDoesNotBlock verifies that a slow (but not hanging) sink
// completes within the per-sink context timeout and both sinks receive data.
func TestProcessor_SlowSinkDoesNotBlock(t *testing.T) {
	// Sink that takes 2s to write (within the 30s per-sink timeout).
	slow := &slowWriter{delay: 2 * time.Second}
	fast := &mockWriter{}

	calc := costcalc.New()
	proc := New([]storage.SpanWriter{slow, fast}, calc, 1)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	proc.Submit(testSpan("slow-test"))

	// Wait for the slow writer to complete (2s) + buffer.
	time.Sleep(4 * time.Second)
	cancel()
	proc.Stop()

	// Both writers should have received the span.
	fastSpans := fast.allSpans()
	if len(fastSpans) == 0 {
		t.Error("fast writer should have received spans")
	}
}
