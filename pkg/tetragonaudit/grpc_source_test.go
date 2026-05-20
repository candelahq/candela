package tetragonaudit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// mockStream implements TetragonEventStream for testing.
type mockStream struct {
	events [][]byte
	idx    int
	err    error
}

func (m *mockStream) Recv() ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.idx >= len(m.events) {
		return nil, io.EOF
	}
	data := m.events[m.idx]
	m.idx++
	return data, nil
}

func newMockStream(events ...Event) *mockStream {
	var raw [][]byte
	for _, e := range events {
		b, _ := json.Marshal(e)
		raw = append(raw, b)
	}
	return &mockStream{events: raw}
}

func TestGRPCSource_StreamEvents(t *testing.T) {
	src, err := NewGRPCSource("localhost:12345")
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	stream := newMockStream(
		Event{
			Time:     time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
			NodeName: "node-1",
			ProcessKprobe: &ProcessKprobe{
				Process:      &Process{Binary: "/usr/bin/curl", UID: 1000},
				FunctionName: "tcp_connect",
				Action:       "post",
				PolicyName:   "candela-audit",
				Args: []KprobeArg{
					{SockArg: &SockArg{Daddr: "1.2.3.4", Dport: 443}},
				},
			},
		},
		Event{
			Time:     time.Date(2026, 5, 18, 12, 0, 1, 0, time.UTC),
			NodeName: "node-1",
			ProcessKprobe: &ProcessKprobe{
				Process:      &Process{Binary: "/usr/bin/python3"},
				FunctionName: "tcp_connect",
				Action:       "SIGKILL",
				PolicyName:   "candela-enforce",
			},
		},
	)

	err = src.StreamEvents(context.Background(), stream, pipeline)
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}

	records := sink.GetRecords()
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].Binary != "/usr/bin/curl" {
		t.Errorf("record[0].binary = %q", records[0].Binary)
	}
	if records[1].Severity != "CRITICAL" {
		t.Errorf("record[1].severity = %q, want CRITICAL", records[1].Severity)
	}
}

func TestGRPCSource_StreamContextCancel(t *testing.T) {
	src, err := NewGRPCSource("localhost:12345")
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	// Use a context that is already cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stream := newMockStream(Event{
		NodeName: "node-1",
		ProcessKprobe: &ProcessKprobe{
			Process: &Process{Binary: "/bin/test"},
			Action:  "post",
		},
	})

	err = src.StreamEvents(ctx, stream, pipeline)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestGRPCSource_StreamError(t *testing.T) {
	src, err := NewGRPCSource("localhost:12345")
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	stream := &mockStream{err: errors.New("transport error")}

	err = src.StreamEvents(context.Background(), stream, pipeline)
	if err == nil {
		t.Error("expected error from broken stream")
	}
	if len(sink.GetRecords()) != 0 {
		t.Error("expected no records from broken stream")
	}
}

func TestGRPCSource_MalformedEvent(t *testing.T) {
	src, err := NewGRPCSource("localhost:12345")
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	// Mix of valid JSON and garbage.
	validEvent, _ := json.Marshal(Event{
		NodeName: "node-1",
		ProcessKprobe: &ProcessKprobe{
			Process: &Process{Binary: "/bin/good"},
			Action:  "post",
		},
	})

	stream := &mockStream{
		events: [][]byte{
			[]byte("{invalid json"),
			validEvent,
		},
	}

	err = src.StreamEvents(context.Background(), stream, pipeline)
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}

	// Only the valid event should have been processed.
	records := sink.GetRecords()
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Binary != "/bin/good" {
		t.Errorf("binary = %q", records[0].Binary)
	}
}

func TestNewGRPCSource_EmptyAddr(t *testing.T) {
	_, err := NewGRPCSource("")
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestGRPCSource_Conn(t *testing.T) {
	src, err := NewGRPCSource("localhost:12345")
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	if src.Conn() == nil {
		t.Error("Conn() should not be nil")
	}
}

func TestNewGRPCEventStreamAdapter_Error(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context to force NewStream to return an error.

	conn, err := grpc.NewClient("localhost:12345", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, err = NewGRPCEventStreamAdapter(ctx, conn)
	if err == nil {
		t.Error("expected error when context is canceled, got nil")
	}
}
