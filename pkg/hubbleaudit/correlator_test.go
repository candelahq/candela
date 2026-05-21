package hubbleaudit

import (
	"context"
	"testing"
	"time"
)

func TestCorrelate_ByPodName(t *testing.T) {
	c := NewWindowCorrelator(CorrelatorConfig{MaxFlows: 100, TTL: time.Minute})

	flow1 := Flow{
		Time:        time.Now().Add(-10 * time.Second),
		Source:      Endpoint{Namespace: "default", PodName: "api-server", IP: "10.0.0.5", Port: 38472},
		Destination: Endpoint{Namespace: "default", PodName: "db", IP: "10.0.1.10", Port: 5432},
		Verdict:     "FORWARDED",
		Type:        "L3_L4",
	}
	flow2 := Flow{
		Time:        time.Now().Add(-5 * time.Second),
		Source:      Endpoint{Namespace: "default", PodName: "api-server", IP: "10.0.0.5", Port: 38473},
		Destination: Endpoint{Namespace: "kube-system", PodName: "dns", IP: "10.96.0.10", Port: 53},
		Verdict:     "FORWARDED",
		Type:        "L3_L4",
	}
	// Unrelated flow.
	flow3 := Flow{
		Time:        time.Now().Add(-3 * time.Second),
		Source:      Endpoint{Namespace: "monitoring", PodName: "prometheus", IP: "10.0.2.1", Port: 9090},
		Destination: Endpoint{Namespace: "default", PodName: "nginx", IP: "10.0.3.1", Port: 80},
		Verdict:     "FORWARDED",
		Type:        "L3_L4",
	}

	c.Ingest(flow1)
	c.Ingest(flow2)
	c.Ingest(flow3)

	// Correlate by source pod.
	flows, err := c.Correlate(context.Background(), "default/api-server", 30*time.Second)
	if err != nil {
		t.Fatalf("Correlate error: %v", err)
	}
	if len(flows) != 2 {
		t.Fatalf("expected 2 flows for default/api-server, got %d", len(flows))
	}
}

func TestCorrelate_ByDestPod(t *testing.T) {
	c := NewWindowCorrelator(CorrelatorConfig{MaxFlows: 100, TTL: time.Minute})

	flow := Flow{
		Time:        time.Now().Add(-5 * time.Second),
		Source:      Endpoint{Namespace: "default", PodName: "client", IP: "10.0.0.1", Port: 40000},
		Destination: Endpoint{Namespace: "default", PodName: "api-server", IP: "10.0.0.5", Port: 8080},
		Verdict:     "FORWARDED",
		Type:        "L3_L4",
	}
	c.Ingest(flow)

	flows, err := c.Correlate(context.Background(), "default/api-server", 30*time.Second)
	if err != nil {
		t.Fatalf("Correlate error: %v", err)
	}
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow for default/api-server (as dest), got %d", len(flows))
	}
}

func TestCorrelate_ByNetworkTuple(t *testing.T) {
	c := NewWindowCorrelator(CorrelatorConfig{MaxFlows: 100, TTL: time.Minute})

	flow := Flow{
		Time:        time.Now().Add(-2 * time.Second),
		Source:      Endpoint{IP: "10.0.0.5", Port: 38472},
		Destination: Endpoint{IP: "1.2.3.4", Port: 443},
		Verdict:     "DROPPED",
		Type:        "L3_L4",
	}
	c.Ingest(flow)

	flows, err := c.Correlate(context.Background(), "10.0.0.5:38472→1.2.3.4:443", 30*time.Second)
	if err != nil {
		t.Fatalf("Correlate error: %v", err)
	}
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
	if flows[0].Verdict != "DROPPED" {
		t.Errorf("verdict = %q, want DROPPED", flows[0].Verdict)
	}
}

func TestCorrelate_WindowExpiry(t *testing.T) {
	c := NewWindowCorrelator(CorrelatorConfig{MaxFlows: 100, TTL: time.Minute})

	// Flow from 2 minutes ago — outside a 30s window.
	flow := Flow{
		Time:        time.Now().Add(-2 * time.Minute),
		Source:      Endpoint{Namespace: "default", PodName: "old-pod", IP: "10.0.0.1", Port: 80},
		Destination: Endpoint{Namespace: "default", PodName: "other", IP: "10.0.0.2", Port: 443},
	}
	c.Ingest(flow)

	flows, err := c.Correlate(context.Background(), "default/old-pod", 30*time.Second)
	if err != nil {
		t.Fatalf("Correlate error: %v", err)
	}
	if len(flows) != 0 {
		t.Fatalf("expected 0 flows (expired), got %d", len(flows))
	}
}

func TestCorrelate_RingBufferOverflow(t *testing.T) {
	c := NewWindowCorrelator(CorrelatorConfig{MaxFlows: 3, TTL: time.Minute})

	// Insert 4 flows into a ring of size 3 — oldest should be evicted.
	for i := range 4 {
		c.Ingest(Flow{
			Time:        time.Now(),
			Source:      Endpoint{Namespace: "ns", PodName: "pod", IP: "10.0.0.1", Port: uint32(1000 + i)},
			Destination: Endpoint{IP: "10.0.0.2", Port: 80},
		})
	}

	stats := c.Stats()
	if stats.FlowCount != 3 {
		t.Errorf("flow count = %d, want 3", stats.FlowCount)
	}

	// The oldest flow (port 1000) should be evicted.
	flows, err := c.Correlate(context.Background(), "ns/pod", time.Minute)
	if err != nil {
		t.Fatalf("Correlate error: %v", err)
	}
	if len(flows) != 3 {
		t.Fatalf("expected 3 flows, got %d", len(flows))
	}
	for _, f := range flows {
		if f.Source.Port == 1000 {
			t.Error("evicted flow (port 1000) should not be returned")
		}
	}
}

func TestCorrelate_EmptyKey(t *testing.T) {
	c := NewWindowCorrelator(CorrelatorConfig{})

	_, err := c.Correlate(context.Background(), "", time.Minute)
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestCorrelate_NoMatch(t *testing.T) {
	c := NewWindowCorrelator(CorrelatorConfig{MaxFlows: 100, TTL: time.Minute})

	c.Ingest(Flow{
		Time:        time.Now(),
		Source:      Endpoint{Namespace: "ns-a", PodName: "pod-a", IP: "10.0.0.1", Port: 80},
		Destination: Endpoint{IP: "10.0.0.2", Port: 443},
	})

	flows, err := c.Correlate(context.Background(), "ns-b/pod-b", time.Minute)
	if err != nil {
		t.Fatalf("Correlate error: %v", err)
	}
	if len(flows) != 0 {
		t.Fatalf("expected 0 flows for non-matching key, got %d", len(flows))
	}
}

func TestCorrelate_ConcurrentSafety(t *testing.T) {
	c := NewWindowCorrelator(CorrelatorConfig{MaxFlows: 1000, TTL: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Writer goroutine.
	go func() {
		for i := 0; ctx.Err() == nil; i++ {
			c.Ingest(Flow{
				Time:        time.Now(),
				Source:      Endpoint{Namespace: "default", PodName: "stress", IP: "10.0.0.1", Port: uint32(i % 65535)},
				Destination: Endpoint{IP: "10.0.0.2", Port: 443},
			})
		}
	}()

	// Reader goroutine.
	go func() {
		for ctx.Err() == nil {
			_, _ = c.Correlate(ctx, "default/stress", time.Minute)
		}
	}()

	<-ctx.Done()
	// If we reach here without a race detector panic, concurrency is safe.
}

func TestCorrelatorStats(t *testing.T) {
	c := NewWindowCorrelator(CorrelatorConfig{MaxFlows: 100, TTL: time.Minute})

	c.Ingest(Flow{
		Time:        time.Now(),
		Source:      Endpoint{Namespace: "a", PodName: "b", IP: "10.0.0.1", Port: 80},
		Destination: Endpoint{Namespace: "c", PodName: "d", IP: "10.0.0.2", Port: 443},
	})

	stats := c.Stats()
	if stats.FlowCount != 1 {
		t.Errorf("FlowCount = %d, want 1", stats.FlowCount)
	}
	if stats.PodKeys < 2 {
		t.Errorf("PodKeys = %d, want >= 2 (src + dst)", stats.PodKeys)
	}
	if stats.TupleKeys != 1 {
		t.Errorf("TupleKeys = %d, want 1", stats.TupleKeys)
	}
	if stats.RingCapacity != 100 {
		t.Errorf("RingCapacity = %d, want 100", stats.RingCapacity)
	}
}

func TestPodKey(t *testing.T) {
	tests := []struct {
		name string
		ep   Endpoint
		want string
	}{
		{"full", Endpoint{Namespace: "default", PodName: "api"}, "default/api"},
		{"no namespace", Endpoint{PodName: "api"}, ""},
		{"no pod", Endpoint{Namespace: "default"}, ""},
		{"empty", Endpoint{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podKey(tt.ep)
			if got != tt.want {
				t.Errorf("podKey(%+v) = %q, want %q", tt.ep, got, tt.want)
			}
		})
	}
}

func TestTupleKey(t *testing.T) {
	tests := []struct {
		name string
		flow Flow
		want string
	}{
		{
			"full",
			Flow{Source: Endpoint{IP: "10.0.0.1", Port: 80}, Destination: Endpoint{IP: "10.0.0.2", Port: 443}},
			"10.0.0.1:80→10.0.0.2:443",
		},
		{
			"no src IP",
			Flow{Source: Endpoint{Port: 80}, Destination: Endpoint{IP: "10.0.0.2", Port: 443}},
			"",
		},
		{
			"no dst IP",
			Flow{Source: Endpoint{IP: "10.0.0.1", Port: 80}, Destination: Endpoint{Port: 443}},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tupleKey(tt.flow)
			if got != tt.want {
				t.Errorf("tupleKey = %q, want %q", got, tt.want)
			}
		})
	}
}
