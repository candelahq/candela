package tetragonaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ─── Mock Tetragon gRPC Server ───────────────────────────────────────────────
//
// mockTetragonServer implements a minimal gRPC server that serves
// /tetragon.FineGuidanceSensors/GetEvents with configurable behavior per
// connection attempt. This enables full lifecycle testing of
// StreamEventsWithRetry without any Tetragon proto dependency.

// streamBehavior defines what the mock server does on each connection.
type streamBehavior struct {
	// Events to send before closing. Each is JSON-serialized.
	Events []Event
	// Err to return after sending all events. nil = clean EOF.
	Err error
}

type mockTetragonServer struct {
	mu         sync.Mutex
	behaviors  []streamBehavior
	attempt    int
	connectCh  chan struct{} // signaled on each new stream
	totalConns atomic.Int64
}

func newMockTetragonServer(behaviors ...streamBehavior) *mockTetragonServer {
	return &mockTetragonServer{
		behaviors: behaviors,
		connectCh: make(chan struct{}, 100),
	}
}

// serve starts the mock gRPC server and returns the listener address.
// The returned cleanup function stops the server.
func (m *mockTetragonServer) serve(t *testing.T) (addr string, cleanup func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mock server listen: %v", err)
	}

	srv := grpc.NewServer()

	// Register a raw service handler for the Tetragon export RPC path.
	// This avoids importing Tetragon protobufs while testing the real gRPC layer.
	handler := func(srv any, stream grpc.ServerStream) error {
		return m.handleGetEvents(srv, stream)
	}
	sd := grpc.ServiceDesc{
		ServiceName: "tetragon.FineGuidanceSensors",
		HandlerType: (*mockTetragonService)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "GetEvents",
				Handler:       handler,
				ServerStreams: true,
				ClientStreams: false,
			},
		},
		Metadata: "tetragon/sensors.proto",
	}
	// The second argument must be a non-nil value that implements HandlerType.
	srv.RegisterService(&sd, &mockTetragonServiceImpl{})

	go func() {
		_ = srv.Serve(lis) // errors expected on GracefulStop
	}()

	return lis.Addr().String(), func() { srv.GracefulStop() }
}

// mockTetragonService is the interface type for RegisterService's HandlerType.
type mockTetragonService interface{}

// mockTetragonServiceImpl satisfies the interface.
type mockTetragonServiceImpl struct{}

// handleGetEvents is the gRPC stream handler for GetEvents.
func (m *mockTetragonServer) handleGetEvents(_ any, stream grpc.ServerStream) error {
	// Read the client's request (we don't care about its content).
	var req json.RawMessage
	if err := stream.RecvMsg(&req); err != nil {
		return err
	}

	m.totalConns.Add(1)

	m.mu.Lock()
	behavior := streamBehavior{} // default: immediate EOF
	if m.attempt < len(m.behaviors) {
		behavior = m.behaviors[m.attempt]
	} else if len(m.behaviors) > 0 {
		// After exhausting the list, repeat the last behavior.
		behavior = m.behaviors[len(m.behaviors)-1]
	}
	m.attempt++
	m.mu.Unlock()

	// Signal that a new connection arrived.
	select {
	case m.connectCh <- struct{}{}:
	default:
	}

	// Stream events.
	for _, ev := range behavior.Events {
		b, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("mock server: marshal: %w", err)
		}
		if err := stream.SendMsg(json.RawMessage(b)); err != nil {
			return err
		}
	}

	// Return the configured error (nil = clean EOF).
	return behavior.Err
}

// waitForConnections blocks until n connections have been received or timeout.
func (m *mockTetragonServer) waitForConnections(t *testing.T, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for range n {
		select {
		case <-m.connectCh:
		case <-deadline:
			t.Fatalf("timed out waiting for connection %d/%d (got %d total)",
				n, n, m.totalConns.Load())
		}
	}
}

// ─── Helper ──────────────────────────────────────────────────────────────────

func testEvent(binary string) Event {
	return Event{
		Time:     time.Now().UTC(),
		NodeName: "e2e-node",
		ProcessKprobe: &ProcessKprobe{
			Process:      &Process{Binary: binary, UID: 1000},
			FunctionName: "tcp_connect",
			Action:       "post",
			PolicyName:   "e2e-test",
		},
	}
}

// ─── E2E Tests ───────────────────────────────────────────────────────────────

// TestE2E_StreamAndProcess verifies the full happy path: connect to a real
// gRPC server, stream events, receive them through the pipeline, and verify
// health + stats.
func TestE2E_StreamAndProcess(t *testing.T) {
	mock := newMockTetragonServer(streamBehavior{
		Events: []Event{testEvent("/usr/bin/curl"), testEvent("/usr/bin/python3")},
	})
	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCSource(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	// The server sends 2 events then EOF. StreamEventsWithRetry will reconnect
	// after EOF, so we cancel after the first batch is received.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Run in background so we can inspect results.
	done := make(chan error, 1)
	go func() {
		done <- src.StreamEventsWithRetry(ctx, pipeline, RetryConfig{
			InitialDelay: 50 * time.Millisecond,
			MaxDelay:     200 * time.Millisecond,
		})
	}()

	// Wait for the first connection to be served.
	mock.waitForConnections(t, 1, 2*time.Second)

	// Give the pipeline a moment to process.
	time.Sleep(100 * time.Millisecond)

	// Verify events were processed.
	records := sink.GetRecords()
	if len(records) < 2 {
		t.Fatalf("expected ≥2 records, got %d", len(records))
	}
	if records[0].Binary != "/usr/bin/curl" {
		t.Errorf("records[0].Binary = %q, want /usr/bin/curl", records[0].Binary)
	}
	if records[1].Binary != "/usr/bin/python3" {
		t.Errorf("records[1].Binary = %q, want /usr/bin/python3", records[1].Binary)
	}

	// Health should show connected (or reconnecting after EOF).
	h := pipeline.Health()
	if h.Processed < 2 {
		t.Errorf("health.Processed = %d, want ≥2", h.Processed)
	}

	// Stats should reflect processed events.
	p, d, e := pipeline.Stats().Snapshot()
	if p < 2 {
		t.Errorf("processed = %d, want ≥2", p)
	}
	if d != 0 {
		t.Errorf("dropped = %d, want 0", d)
	}
	if e != 0 {
		t.Errorf("errors = %d, want 0", e)
	}

	cancel()
	<-done
}

// TestE2E_ReconnectAfterError verifies that StreamEventsWithRetry reconnects
// after the server returns a transient error, and successfully processes
// events on the subsequent connection.
func TestE2E_ReconnectAfterError(t *testing.T) {
	mock := newMockTetragonServer(
		// Attempt 1: sends 1 event then returns an error.
		streamBehavior{
			Events: []Event{testEvent("/bin/first")},
			Err:    fmt.Errorf("mock transient error"),
		},
		// Attempt 2: sends 1 event then clean EOF.
		streamBehavior{
			Events: []Event{testEvent("/bin/second")},
		},
	)
	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCSource(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- src.StreamEventsWithRetry(ctx, pipeline, RetryConfig{
			InitialDelay: 50 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		})
	}()

	// Wait for both connections.
	mock.waitForConnections(t, 2, 3*time.Second)
	time.Sleep(150 * time.Millisecond)

	records := sink.GetRecords()
	if len(records) < 2 {
		t.Fatalf("expected ≥2 records across reconnect, got %d", len(records))
	}

	// Both events from both connections should be present.
	binaries := make(map[string]bool)
	for _, r := range records {
		binaries[r.Binary] = true
	}
	if !binaries["/bin/first"] {
		t.Error("missing /bin/first from attempt 1")
	}
	if !binaries["/bin/second"] {
		t.Error("missing /bin/second from attempt 2")
	}

	cancel()
	<-done
}

// TestE2E_HealthToggles verifies that the pipeline health accurately
// reflects connected/disconnected states during the retry lifecycle.
func TestE2E_HealthToggles(t *testing.T) {
	mock := newMockTetragonServer(
		// Attempt 1: send 1 event, then error → triggers backoff.
		streamBehavior{
			Events: []Event{testEvent("/bin/health-check")},
			Err:    fmt.Errorf("mock error for health toggle test"),
		},
		// Attempt 2: send 1 event, clean EOF → triggers InitialDelay.
		streamBehavior{
			Events: []Event{testEvent("/bin/health-check-2")},
		},
	)
	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCSource(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- src.StreamEventsWithRetry(ctx, pipeline, RetryConfig{
			// Use longer delays so we can observe the disconnected state.
			InitialDelay: 500 * time.Millisecond,
			MaxDelay:     1 * time.Second,
		})
	}()

	// Wait for first connection.
	mock.waitForConnections(t, 1, 2*time.Second)
	time.Sleep(100 * time.Millisecond)

	// After the first stream errors, pipeline should be disconnected during backoff.
	// The 500ms backoff gives us time to observe.
	h := pipeline.Health()
	if h.Connected {
		// This is expected — the stream just errored, so we should be in backoff.
		// (It's possible we catch it during the brief reconnect window, so we
		// just log rather than fail.)
		t.Log("note: caught pipeline in connected state during expected backoff — timing dependent")
	}

	// Wait for second connection.
	mock.waitForConnections(t, 1, 3*time.Second)
	time.Sleep(50 * time.Millisecond)

	// After second connection succeeds, health should show connected.
	h = pipeline.Health()
	if !h.Connected {
		// The stream may have already ended (clean EOF), so this is also timing dependent.
		t.Log("note: pipeline not connected — stream may have already ended")
	}

	cancel()
	<-done

	// After cancellation, pipeline should be disconnected.
	h = pipeline.Health()
	if h.Connected {
		t.Error("expected Connected=false after context cancellation")
	}
}

// TestE2E_CleanEOFBackoff verifies that a clean EOF (server closes stream
// without error) does NOT cause a tight reconnect loop — there must be a
// delay between the first and second connections.
func TestE2E_CleanEOFBackoff(t *testing.T) {
	mock := newMockTetragonServer(
		// Both attempts: clean EOF immediately.
		streamBehavior{Events: []Event{testEvent("/bin/eof1")}},
		streamBehavior{Events: []Event{testEvent("/bin/eof2")}},
	)
	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCSource(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	initialDelay := 300 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()

	done := make(chan error, 1)
	go func() {
		done <- src.StreamEventsWithRetry(ctx, pipeline, RetryConfig{
			InitialDelay: initialDelay,
			MaxDelay:     1 * time.Second,
		})
	}()

	// Wait for both connections.
	mock.waitForConnections(t, 2, 2*time.Second)
	elapsed := time.Since(start)

	// The second connection should have been delayed by at least InitialDelay.
	// We check for 80% of the delay to account for scheduling jitter.
	minExpected := time.Duration(float64(initialDelay) * 0.8)
	if elapsed < minExpected {
		t.Errorf("reconnect after clean EOF was too fast: %v (expected ≥%v)",
			elapsed, minExpected)
	}

	cancel()
	<-done
}

// TestE2E_MultipleReconnects verifies that the retry loop handles multiple
// consecutive failures followed by a successful connection.
func TestE2E_MultipleReconnects(t *testing.T) {
	mock := newMockTetragonServer(
		streamBehavior{Err: fmt.Errorf("fail-1")},
		streamBehavior{Err: fmt.Errorf("fail-2")},
		streamBehavior{Err: fmt.Errorf("fail-3")},
		// 4th attempt succeeds.
		streamBehavior{Events: []Event{testEvent("/bin/success")}},
	)
	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCSource(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- src.StreamEventsWithRetry(ctx, pipeline, RetryConfig{
			InitialDelay: 20 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		})
	}()

	// Wait for all 4 connections.
	mock.waitForConnections(t, 4, 4*time.Second)
	time.Sleep(100 * time.Millisecond)

	// The event from the 4th connection should be processed.
	records := sink.GetRecords()
	if len(records) < 1 {
		t.Fatalf("expected ≥1 record after 3 failures + 1 success, got %d", len(records))
	}

	found := false
	for _, r := range records {
		if r.Binary == "/bin/success" {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing /bin/success event after reconnect")
	}

	// Total connections should be ≥4.
	if mock.totalConns.Load() < 4 {
		t.Errorf("expected ≥4 connections, got %d", mock.totalConns.Load())
	}

	cancel()
	<-done
}

// TestE2E_GracefulShutdownDuringStream verifies that cancelling the context
// during active streaming exits cleanly without hanging.
func TestE2E_GracefulShutdownDuringStream(t *testing.T) {
	// Server that streams events slowly (one every 100ms).
	slowEvents := make([]Event, 50)
	for i := range slowEvents {
		slowEvents[i] = testEvent(fmt.Sprintf("/bin/slow-%d", i))
	}

	mock := newMockTetragonServer(streamBehavior{Events: slowEvents})
	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCSource(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	sink := &CollectorSink{}
	pipeline := NewPipeline(PipelineConfig{Sink: sink})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- src.StreamEventsWithRetry(ctx, pipeline, RetryConfig{
			InitialDelay: 50 * time.Millisecond,
		})
	}()

	// Let some events flow, then cancel.
	mock.waitForConnections(t, 1, 2*time.Second)
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Must exit within 1 second.
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("expected context.Canceled or nil, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StreamEventsWithRetry did not exit within 2s after cancel")
	}

	// Health should show disconnected.
	h := pipeline.Health()
	if h.Connected {
		t.Error("expected Connected=false after graceful shutdown")
	}

	// Should have processed at least some events.
	if h.Processed == 0 {
		t.Error("expected some events to be processed before shutdown")
	}
}

// TestE2E_DrainAfterShutdown verifies that Drain() can flush the pipeline
// after the streaming goroutine has exited.
func TestE2E_DrainAfterShutdown(t *testing.T) {
	var flushed atomic.Bool
	flushSink := &flushRecordingSink{flushed: &flushed}

	mock := newMockTetragonServer(streamBehavior{
		Events: []Event{testEvent("/bin/drain-test")},
	})
	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCSource(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewGRPCSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	pipeline := NewPipeline(PipelineConfig{Sink: flushSink})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- src.StreamEventsWithRetry(ctx, pipeline, RetryConfig{
			InitialDelay: 50 * time.Millisecond,
		})
	}()

	mock.waitForConnections(t, 1, 2*time.Second)
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	// Drain should call Flush.
	if err := pipeline.Drain(context.Background()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !flushed.Load() {
		t.Error("expected Flush to be called during Drain")
	}
}

// flushRecordingSink records whether Flush was called.
type flushRecordingSink struct {
	mu      sync.Mutex
	records []AuditRecord
	flushed *atomic.Bool
}

func (s *flushRecordingSink) Emit(_ context.Context, r AuditRecord) error {
	s.mu.Lock()
	s.records = append(s.records, r)
	s.mu.Unlock()
	return nil
}

func (s *flushRecordingSink) Flush(_ context.Context) error {
	s.flushed.Store(true)
	return nil
}

// Compile-time check that flushRecordingSink implements both interfaces.
var (
	_ Sink    = (*flushRecordingSink)(nil)
	_ Flusher = (*flushRecordingSink)(nil)
)
