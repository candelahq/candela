package hubbleaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ── Mock Hubble Server ──

// streamBehavior defines what a mock Hubble server does on each connection.
type streamBehavior struct {
	flows []Flow // flows to stream
	err   error  // error to return after streaming (nil = clean EOF)
}

// mockHubbleService is the interface required by grpc.ServiceDesc.HandlerType.
type mockHubbleService interface {
	GetFlows()
}

// mockHubbleServiceImpl implements the mock service interface.
type mockHubbleServiceImpl struct{}

func (mockHubbleServiceImpl) GetFlows() {}

// mockHubbleServer is a configurable mock Hubble relay server.
type mockHubbleServer struct {
	behaviors  []streamBehavior
	connectCh  chan struct{}
	totalConns atomic.Int32
}

func (m *mockHubbleServer) handleGetFlows(srv any, stream grpc.ServerStream) error {
	// Receive the initial request.
	var req json.RawMessage
	if err := stream.RecvMsg(&req); err != nil {
		return err
	}

	connIdx := int(m.totalConns.Add(1)) - 1
	if m.connectCh != nil {
		select {
		case m.connectCh <- struct{}{}:
		default:
		}
	}

	if connIdx >= len(m.behaviors) {
		return fmt.Errorf("no behavior configured for connection %d", connIdx)
	}
	b := m.behaviors[connIdx]

	for _, flow := range b.flows {
		data, err := json.Marshal(flow)
		if err != nil {
			return err
		}
		if err := stream.SendMsg(data); err != nil {
			return err
		}
	}

	return b.err
}

func (m *mockHubbleServer) serve(t *testing.T) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := grpc.NewServer()
	sd := grpc.ServiceDesc{
		ServiceName: "observer.Observer",
		HandlerType: (*mockHubbleService)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "GetFlows",
				Handler:       m.handleGetFlows,
				ServerStreams: true,
				ClientStreams: false,
			},
		},
		Metadata: "observer.proto",
	}
	srv.RegisterService(&sd, &mockHubbleServiceImpl{})

	go func() {
		_ = srv.Serve(lis) // errors expected on GracefulStop
	}()

	return lis.Addr().String(), func() { srv.GracefulStop() }
}

// ── E2E Tests ──

func TestE2E_HubbleStreamFlows(t *testing.T) {
	mock := &mockHubbleServer{
		behaviors: []streamBehavior{
			{
				flows: []Flow{
					{
						Time:        time.Now(),
						Source:      Endpoint{Namespace: "default", PodName: "client", IP: "10.0.0.1", Port: 40000},
						Destination: Endpoint{Namespace: "default", PodName: "api", IP: "10.0.0.5", Port: 8080},
						Verdict:     "FORWARDED",
						Type:        "L3_L4",
						Summary:     "TCP SYN",
					},
					{
						Time:        time.Now(),
						Source:      Endpoint{Namespace: "default", PodName: "client", IP: "10.0.0.1", Port: 40001},
						Destination: Endpoint{Namespace: "kube-system", PodName: "dns", IP: "10.96.0.10", Port: 53},
						Verdict:     "FORWARDED",
						Type:        "L3_L4",
						Summary:     "UDP",
					},
				},
			},
		},
	}

	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCFlowSource(GRPCFlowSourceConfig{
		Addr:     addr,
		DialOpts: []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	})
	if err != nil {
		t.Fatalf("NewGRPCFlowSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := src.Observe(ctx, FlowFilter{})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	var received []Flow
	for flow := range ch {
		received = append(received, flow)
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 flows, got %d", len(received))
	}
	if received[0].Verdict != "FORWARDED" {
		t.Errorf("flow[0].Verdict = %q", received[0].Verdict)
	}
	if received[1].Destination.Port != 53 {
		t.Errorf("flow[1].Destination.Port = %d, want 53", received[1].Destination.Port)
	}
}

func TestE2E_HubbleReconnectAfterError(t *testing.T) {
	mock := &mockHubbleServer{
		connectCh: make(chan struct{}, 4),
		behaviors: []streamBehavior{
			{err: fmt.Errorf("transient error")},
			{
				flows: []Flow{
					{
						Time:        time.Now(),
						Source:      Endpoint{Namespace: "default", PodName: "app", IP: "10.0.0.1", Port: 80},
						Destination: Endpoint{IP: "10.0.0.2", Port: 443},
						Verdict:     "DROPPED",
					},
				},
			},
		},
	}

	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCFlowSource(GRPCFlowSourceConfig{
		Addr:     addr,
		DialOpts: []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	})
	if err != nil {
		t.Fatalf("NewGRPCFlowSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var received []Flow
	handler := func(_ context.Context, flow Flow) error {
		received = append(received, flow)
		cancel() // Signal done after first flow.
		return nil
	}

	_ = src.StreamWithRetry(ctx, handler, RetryConfig{
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     200 * time.Millisecond,
	})

	if len(received) < 1 {
		t.Fatalf("expected at least 1 flow after reconnect, got %d", len(received))
	}
	if received[0].Verdict != "DROPPED" {
		t.Errorf("flow verdict = %q, want DROPPED", received[0].Verdict)
	}
	if mock.totalConns.Load() < 2 {
		t.Errorf("expected >= 2 connections, got %d", mock.totalConns.Load())
	}
}

func TestE2E_HubbleHealthToggles(t *testing.T) {
	mock := &mockHubbleServer{
		connectCh: make(chan struct{}, 4),
		behaviors: []streamBehavior{
			{err: fmt.Errorf("fail 1")},
			{
				flows: []Flow{
					{
						Time:        time.Now(),
						Source:      Endpoint{Namespace: "ns", PodName: "p", IP: "10.0.0.1", Port: 80},
						Destination: Endpoint{IP: "10.0.0.2", Port: 443},
						Verdict:     "FORWARDED",
					},
				},
			},
		},
	}

	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCFlowSource(GRPCFlowSourceConfig{
		Addr:     addr,
		DialOpts: []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	})
	if err != nil {
		t.Fatalf("NewGRPCFlowSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var healthStates []bool
	handler := func(_ context.Context, _ Flow) error {
		healthStates = append(healthStates, src.Health().Connected)
		cancel()
		return nil
	}

	// Check initial state.
	if src.Health().Connected {
		t.Error("expected disconnected initially")
	}

	_ = src.StreamWithRetry(ctx, handler, RetryConfig{
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     200 * time.Millisecond,
	})

	// During flow processing, we should have been connected.
	if len(healthStates) == 0 {
		t.Fatal("no health states recorded")
	}
	if !healthStates[0] {
		t.Error("expected connected=true during flow processing")
	}
}

func TestE2E_HubbleGracefulShutdown(t *testing.T) {
	mock := &mockHubbleServer{
		behaviors: []streamBehavior{
			{
				flows: func() []Flow {
					// Large number of flows — we'll cancel before they finish.
					flows := make([]Flow, 100)
					for i := range flows {
						flows[i] = Flow{
							Time:        time.Now(),
							Source:      Endpoint{IP: "10.0.0.1", Port: uint32(i)},
							Destination: Endpoint{IP: "10.0.0.2", Port: 443},
							Verdict:     "FORWARDED",
						}
					}
					return flows
				}(),
			},
		},
	}

	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCFlowSource(GRPCFlowSourceConfig{
		Addr:     addr,
		DialOpts: []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	})
	if err != nil {
		t.Fatalf("NewGRPCFlowSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	handler := func(_ context.Context, _ Flow) error {
		count++
		if count >= 3 {
			cancel()
		}
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- src.StreamWithRetry(ctx, handler, RetryConfig{
			InitialDelay: 50 * time.Millisecond,
		})
	}()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("StreamWithRetry did not exit within 5s")
	}
}

func TestE2E_HubbleFlowsToCorrelator(t *testing.T) {
	// Integration test: flows from mock Hubble server → correlator → lookup.
	mock := &mockHubbleServer{
		behaviors: []streamBehavior{
			{
				flows: []Flow{
					{
						Time:        time.Now(),
						Source:      Endpoint{Namespace: "prod", PodName: "api", IP: "10.0.0.5", Port: 38472},
						Destination: Endpoint{Namespace: "prod", PodName: "db", IP: "10.0.1.10", Port: 5432},
						Verdict:     "FORWARDED",
						Type:        "L3_L4",
					},
					{
						Time:        time.Now(),
						Source:      Endpoint{Namespace: "prod", PodName: "api", IP: "10.0.0.5", Port: 38473},
						Destination: Endpoint{IP: "1.2.3.4", Port: 443},
						Verdict:     "DROPPED",
						Type:        "L3_L4",
					},
				},
			},
		},
	}

	addr, cleanup := mock.serve(t)
	defer cleanup()

	src, err := NewGRPCFlowSource(GRPCFlowSourceConfig{
		Addr:     addr,
		DialOpts: []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	})
	if err != nil {
		t.Fatalf("NewGRPCFlowSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	correlator := NewWindowCorrelator(CorrelatorConfig{MaxFlows: 100, TTL: time.Minute})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handler := func(_ context.Context, flow Flow) error {
		correlator.Ingest(flow)
		return nil
	}

	_ = src.StreamWithRetry(ctx, handler, RetryConfig{
		InitialDelay: 50 * time.Millisecond,
	})

	// Verify correlator has the flows.
	stats := correlator.Stats()
	if stats.FlowCount != 2 {
		t.Fatalf("correlator has %d flows, want 2", stats.FlowCount)
	}

	// Lookup by pod.
	flows, err := correlator.Correlate(context.Background(), "prod/api", time.Minute)
	if err != nil {
		t.Fatalf("Correlate error: %v", err)
	}
	if len(flows) != 2 {
		t.Fatalf("expected 2 flows for prod/api, got %d", len(flows))
	}

	// Lookup by tuple.
	flows, err = correlator.Correlate(context.Background(), "10.0.0.5:38473→1.2.3.4:443", time.Minute)
	if err != nil {
		t.Fatalf("Correlate error: %v", err)
	}
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow for tuple, got %d", len(flows))
	}
	if flows[0].Verdict != "DROPPED" {
		t.Errorf("verdict = %q, want DROPPED", flows[0].Verdict)
	}
}
