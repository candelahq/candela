package tetragonaudit

import (
	"context"
	"errors"
	"testing"
)

// erringSink always returns an error from Emit.
type erringSink struct {
	err error
}

func (s *erringSink) Emit(_ context.Context, _ AuditRecord) error {
	return s.err
}

func TestMultiSink_FanOut(t *testing.T) {
	sink1 := &CollectorSink{}
	sink2 := &CollectorSink{}

	ms := &MultiSink{Sinks: []Sink{sink1, sink2}}
	record := AuditRecord{Binary: "/bin/test", Severity: "INFO"}

	if err := ms.Emit(context.Background(), record); err != nil {
		t.Fatalf("Emit returned unexpected error: %v", err)
	}

	if got := len(sink1.GetRecords()); got != 1 {
		t.Errorf("sink1 got %d records, want 1", got)
	}
	if got := len(sink2.GetRecords()); got != 1 {
		t.Errorf("sink2 got %d records, want 1", got)
	}
}

func TestMultiSink_ErrorDoesNotShortCircuit(t *testing.T) {
	// First sink errors, second should still receive the record.
	errSink := &erringSink{err: errors.New("sink1 failed")}
	sink2 := &CollectorSink{}

	ms := &MultiSink{Sinks: []Sink{errSink, sink2}}
	record := AuditRecord{Binary: "/bin/test"}

	err := ms.Emit(context.Background(), record)
	if err == nil {
		t.Fatal("expected error from MultiSink.Emit, got nil")
	}
	if err.Error() != "sink1 failed" {
		t.Errorf("got error %q, want %q", err.Error(), "sink1 failed")
	}

	// Verify second sink was called despite first error.
	if got := len(sink2.GetRecords()); got != 1 {
		t.Errorf("sink2 got %d records, want 1 (should not short-circuit)", got)
	}
}

func TestMultiSink_ReturnsFirstError(t *testing.T) {
	// Both sinks error; should get the first one.
	err1 := errors.New("first")
	err2 := errors.New("second")
	ms := &MultiSink{Sinks: []Sink{
		&erringSink{err: err1},
		&erringSink{err: err2},
	}}

	err := ms.Emit(context.Background(), AuditRecord{})
	if !errors.Is(err, err1) {
		t.Errorf("got error %v, want %v", err, err1)
	}
}

func TestMultiSink_Empty(t *testing.T) {
	ms := &MultiSink{Sinks: nil}
	if err := ms.Emit(context.Background(), AuditRecord{}); err != nil {
		t.Fatalf("empty MultiSink should not error, got: %v", err)
	}
}

func TestMultiSink_NilSinks(t *testing.T) {
	ms := &MultiSink{}
	if err := ms.Emit(context.Background(), AuditRecord{}); err != nil {
		t.Fatalf("nil-sinks MultiSink should not error, got: %v", err)
	}
}
