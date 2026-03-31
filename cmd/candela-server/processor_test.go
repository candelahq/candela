package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// mockWriter records all spans it receives for assertion.
type mockWriter struct {
	mu      sync.Mutex
	batches [][]storage.Span
	err     error // if set, IngestSpans returns this error
}

func (m *mockWriter) IngestSpans(_ context.Context, spans []storage.Span) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Copy the slice to avoid races with batch[:0] reset.
	cp := make([]storage.Span, len(spans))
	copy(cp, spans)
	m.batches = append(m.batches, cp)
	return m.err
}

func (m *mockWriter) Close() error { return nil }

func (m *mockWriter) allSpans() []storage.Span {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []storage.Span
	for _, b := range m.batches {
		all = append(all, b...)
	}
	return all
}

func testSpan(id string) storage.Span {
	now := time.Now()
	return storage.Span{
		SpanID:    id,
		TraceID:   "trace-" + id,
		Name:      "test." + id,
		Kind:      storage.SpanKindLLM,
		Status:    storage.SpanStatusOK,
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
		ProjectID: "proj-test",
		GenAI: &storage.GenAIAttributes{
			Model:        "gpt-4o",
			Provider:     "openai",
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	}
}

func TestProcessorFanOut(t *testing.T) {
	w1 := &mockWriter{}
	w2 := &mockWriter{}

	calc := costcalc.New()
	proc := NewSpanProcessor([]storage.SpanWriter{w1, w2}, calc, 10)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	// Submit spans.
	for i := 0; i < 5; i++ {
		proc.Submit(testSpan("span-" + string(rune('a'+i))))
	}

	// Wait for flush (ticker is 2s, give it time).
	time.Sleep(3 * time.Second)
	cancel()
	proc.Stop()

	// Both writers should have received all 5 spans.
	s1 := w1.allSpans()
	s2 := w2.allSpans()

	if len(s1) != 5 {
		t.Errorf("writer1 got %d spans, want 5", len(s1))
	}
	if len(s2) != 5 {
		t.Errorf("writer2 got %d spans, want 5", len(s2))
	}
}

func TestProcessorFanOut_OneFailsOtherSucceeds(t *testing.T) {
	w1 := &mockWriter{err: fmt.Errorf("simulated failure")}
	w2 := &mockWriter{}

	calc := costcalc.New()
	proc := NewSpanProcessor([]storage.SpanWriter{w1, w2}, calc, 2)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	proc.Submit(testSpan("s1"))
	proc.Submit(testSpan("s2"))

	// Wait for batch flush (batch size = 2, should flush immediately).
	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	// w1 failed but w2 should still have received the batch.
	s2 := w2.allSpans()
	if len(s2) != 2 {
		t.Errorf("writer2 got %d spans, want 2 (other writer's failure should not block)", len(s2))
	}
}

func TestProcessorBackPressure(t *testing.T) {
	w := &mockWriter{}
	calc := costcalc.New()
	// Tiny batch + buffer: batchSize=2, channel=2*10=20
	proc := NewSpanProcessor([]storage.SpanWriter{w}, calc, 2)

	// Don't start Run — channel will fill up.
	// Submit more than the buffer can hold.
	dropped := 0
	for i := 0; i < 30; i++ {
		// The channel is 20 deep. Beyond that, Submit drops.
		proc.Submit(testSpan(fmt.Sprintf("s%d", i)))
	}
	// Count how many actually made it.
	close(proc.spanCh)
	received := 0
	for range proc.spanCh {
		received++
	}
	dropped = 30 - received
	if dropped == 0 {
		t.Error("expected some spans to be dropped due to back-pressure")
	}
	t.Logf("received=%d, dropped=%d (channel capacity=20)", received, dropped)
}
