package tetragonaudit

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPipeline_Health_InitialState(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	h := p.Health()
	if h.Healthy {
		t.Error("expected Healthy=false initially (not connected)")
	}
	if h.Connected {
		t.Error("expected Connected=false initially")
	}
	if h.Processed != 0 || h.Dropped != 0 || h.Errors != 0 {
		t.Errorf("expected zero stats, got processed=%d dropped=%d errors=%d",
			h.Processed, h.Dropped, h.Errors)
	}
}

func TestPipeline_Health_ConnectedNoEvents(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	p.SetConnected(true)
	h := p.Health()

	if !h.Healthy {
		t.Error("expected Healthy=true when connected with no prior events")
	}
	if !h.Connected {
		t.Error("expected Connected=true")
	}
}

func TestPipeline_Health_AfterProcessing(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})
	p.SetConnected(true)

	// Process an event.
	event := Event{
		ProcessKprobe: &ProcessKprobe{
			Action:       "post",
			FunctionName: "tcp_connect",
			Process:      &Process{Binary: "/usr/bin/curl"},
		},
	}
	if err := p.ProcessEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	h := p.Health()
	if !h.Healthy {
		t.Error("expected Healthy=true after processing")
	}
	if h.Processed != 1 {
		t.Errorf("expected Processed=1, got %d", h.Processed)
	}
	if h.LastEventAt.IsZero() {
		t.Error("expected LastEventAt to be set")
	}
}

func TestPipeline_Health_Disconnected(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	p.SetConnected(true)
	p.SetConnected(false)

	h := p.Health()
	if h.Healthy {
		t.Error("expected Healthy=false when disconnected")
	}
	if h.Connected {
		t.Error("expected Connected=false")
	}
}

// mockFlushSink records whether Flush was called.
type mockFlushSink struct {
	emitCount  atomic.Int32
	flushed    atomic.Bool
	flushError error
}

func (s *mockFlushSink) Emit(_ context.Context, _ AuditRecord) error {
	s.emitCount.Add(1)
	return nil
}

func (s *mockFlushSink) Flush(_ context.Context) error {
	s.flushed.Store(true)
	return s.flushError
}

// Verify mockFlushSink implements both interfaces.
var _ Sink = (*mockFlushSink)(nil)
var _ Flusher = (*mockFlushSink)(nil)

func TestPipeline_Drain_CallsFlush(t *testing.T) {
	sink := &mockFlushSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	err := p.Drain(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sink.flushed.Load() {
		t.Error("expected Flush to be called on sink")
	}
}

func TestPipeline_Drain_PropagatesFlushError(t *testing.T) {
	wantErr := errors.New("flush failed")
	sink := &mockFlushSink{flushError: wantErr}
	p := NewPipeline(PipelineConfig{Sink: sink})

	err := p.Drain(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestPipeline_Drain_NonFlushSinkNoError(t *testing.T) {
	// CollectorSink does NOT implement Flusher.
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	err := p.Drain(context.Background())
	if err != nil {
		t.Fatalf("expected nil error for non-Flusher sink, got: %v", err)
	}
}

func TestPipeline_Drain_MultiSinkFlushes(t *testing.T) {
	flushable := &mockFlushSink{}
	nonFlushable := &CollectorSink{}

	multi := &MultiSink{Sinks: []Sink{flushable, nonFlushable}}
	p := NewPipeline(PipelineConfig{Sink: multi})

	err := p.Drain(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !flushable.flushed.Load() {
		t.Error("expected Flush to be called on the flushable child of MultiSink")
	}
}

func TestPipeline_Drain_MultiSinkFirstFlushError(t *testing.T) {
	wantErr := errors.New("first flush failed")
	flushable1 := &mockFlushSink{flushError: wantErr}
	flushable2 := &mockFlushSink{}

	multi := &MultiSink{Sinks: []Sink{flushable1, flushable2}}
	p := NewPipeline(PipelineConfig{Sink: multi})

	err := p.Drain(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
	// Second sink should still have been flushed.
	if !flushable2.flushed.Load() {
		t.Error("expected second sink to be flushed even after first error")
	}
}

func TestOTelSink_ImplementsFlusher(t *testing.T) {
	sink, err := NewOTelSink(OTelSinkConfig{Endpoint: "http://localhost:4318"})
	if err != nil {
		t.Fatal(err)
	}
	var f Flusher = sink
	if err := f.Flush(context.Background()); err != nil {
		t.Fatalf("unexpected flush error: %v", err)
	}
}

func TestPipeline_LastEventTracking(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})
	p.SetConnected(true)

	before := time.Now()
	event := Event{
		ProcessKprobe: &ProcessKprobe{
			Action:  "post",
			Process: &Process{Binary: "/bin/test"},
		},
	}
	_ = p.ProcessEvent(context.Background(), event)
	after := time.Now()

	h := p.Health()
	if h.LastEventAt.Before(before) || h.LastEventAt.After(after) {
		t.Errorf("LastEventAt=%v not in [%v, %v]", h.LastEventAt, before, after)
	}
}
