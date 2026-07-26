package processor

import (
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

func TestSubscribe_ReceivesTraces(t *testing.T) {
	p := newTestProcessor()
	ch, cleanup := p.Subscribe(WatchFilter{})
	defer cleanup()

	spans := []storage.Span{
		testWatchSpan("trace-1", "root-span", "", "gpt-4", "openai"),
	}
	p.broadcastToWatchers(spans)

	select {
	case tb := <-ch:
		if tb.TraceID != "trace-1" {
			t.Errorf("expected trace-1, got %s", tb.TraceID)
		}
		if tb.Model != "gpt-4" {
			t.Errorf("expected gpt-4, got %s", tb.Model)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for trace")
	}
}

func TestSubscribe_NonBlocking(t *testing.T) {
	p := newTestProcessor()
	ch, cleanup := p.Subscribe(WatchFilter{})
	defer cleanup()

	// Fill the buffer (cap=100)
	span := testWatchSpan("trace-fill", "root", "", "gpt-4", "openai")
	for i := 0; i < 150; i++ {
		p.broadcastToWatchers([]storage.Span{span})
	}

	// Channel should have 100, rest dropped
	if len(ch) != 100 {
		t.Errorf("expected 100 buffered, got %d", len(ch))
	}
	// Should not block — this test completing proves it
}

func TestSubscribe_Cleanup(t *testing.T) {
	p := newTestProcessor()
	_, cleanup := p.Subscribe(WatchFilter{})
	_, cleanup2 := p.Subscribe(WatchFilter{})

	if len(p.subscribers) != 2 {
		t.Fatalf("expected 2 subscribers, got %d", len(p.subscribers))
	}

	cleanup()
	if len(p.subscribers) != 1 {
		t.Fatalf("expected 1 subscriber after cleanup, got %d", len(p.subscribers))
	}

	cleanup2()
	if len(p.subscribers) != 0 {
		t.Fatalf("expected 0 subscribers after cleanup, got %d", len(p.subscribers))
	}
}

func TestSubscribe_FilterMatch(t *testing.T) {
	p := newTestProcessor()
	ch, cleanup := p.Subscribe(WatchFilter{Model: "gpt-4"})
	defer cleanup()

	spans := []storage.Span{
		testWatchSpan("trace-gpt", "root", "", "gpt-4", "openai"),
		testWatchSpan("trace-claude", "root", "", "claude-3", "anthropic"),
	}
	p.broadcastToWatchers(spans)

	select {
	case tb := <-ch:
		if tb.TraceID != "trace-gpt" {
			t.Errorf("expected trace-gpt, got %s", tb.TraceID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	// claude trace should not have been sent
	select {
	case tb := <-ch:
		t.Errorf("unexpected trace: %s", tb.TraceID)
	default:
		// good
	}
}

// Test helpers
func newTestProcessor() *SpanProcessor {
	return &SpanProcessor{
		spanCh: make(chan storage.Span, 10),
		done:   make(chan struct{}),
	}
}

func testWatchSpan(traceID, name, parentID, model, provider string) storage.Span {
	s := storage.Span{
		TraceID:      traceID,
		SpanID:       traceID + "-span",
		ParentSpanID: parentID,
		Name:         name,
		StartTime:    time.Now(),
		EndTime:      time.Now().Add(100 * time.Millisecond),
		Status:       storage.SpanStatusOK,
	}
	if model != "" {
		s.GenAI = &storage.GenAIAttributes{
			Model:    model,
			Provider: provider,
			CostUSD:  0.0034,
		}
	}
	return s
}
