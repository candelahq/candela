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
	proc := New([]storage.SpanWriter{w1, w2}, calc, 10)

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
	proc := New([]storage.SpanWriter{w1, w2}, calc, 2)

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
	proc := New([]storage.SpanWriter{w}, calc, 2)

	// Don't start Run — channel will fill up.
	// Submit more than the buffer can hold.
	for i := 0; i < 30; i++ {
		// The channel is 20 deep. Beyond that, Submit drops.
		proc.Submit(testSpan(fmt.Sprintf("s%d", i)))
	}

	// Verify the DroppedSpans counter matches reality.
	reportedDropped := proc.DroppedSpans()
	if reportedDropped == 0 {
		t.Error("expected DroppedSpans() > 0 due to back-pressure")
	}

	// Count how many actually made it into the channel.
	close(proc.spanCh)
	received := 0
	for range proc.spanCh {
		received++
	}
	actualDropped := int64(30 - received)

	if reportedDropped != actualDropped {
		t.Errorf("DroppedSpans()=%d but actual dropped=%d", reportedDropped, actualDropped)
	}
	t.Logf("received=%d, dropped=%d (channel capacity=20)", received, actualDropped)
}

func TestProcessorCostEnrichment(t *testing.T) {
	w := &mockWriter{}
	calc := costcalc.New()
	proc := New([]storage.SpanWriter{w}, calc, 1) // batch size 1 = flush immediately

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	// Submit a span with zero cost — processor should enrich it.
	span := testSpan("cost-enrich")
	span.GenAI.CostUSD = 0
	proc.Submit(span)

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	spans := w.allSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least 1 span")
	}
	if spans[0].GenAI.CostUSD == 0 {
		t.Error("expected cost to be enriched (non-zero) by processor")
	}
}

func TestProcessorPreservesUserContext(t *testing.T) {
	w := &mockWriter{}
	calc := costcalc.New()
	proc := New([]storage.SpanWriter{w}, calc, 1)

	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	span := testSpan("user-ctx")
	span.UserID = "user-abc123"
	span.SessionID = "session-xyz789"
	proc.Submit(span)

	time.Sleep(500 * time.Millisecond)
	cancel()
	proc.Stop()

	spans := w.allSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least 1 span")
	}
	if spans[0].UserID != "user-abc123" {
		t.Errorf("UserID = %q, want user-abc123", spans[0].UserID)
	}
	if spans[0].SessionID != "session-xyz789" {
		t.Errorf("SessionID = %q, want session-xyz789", spans[0].SessionID)
	}
}
