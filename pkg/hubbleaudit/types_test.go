package hubbleaudit

import (
	"context"
	"testing"
	"time"
)

// TestFlowStructure verifies that Flow and Endpoint can be constructed
// with all their fields and that the struct layout is correct.
func TestFlowStructure(t *testing.T) {
	flow := Flow{
		Time: time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		Source: Endpoint{
			Namespace: "default",
			PodName:   "client-pod",
			IP:        "10.0.0.5",
			Port:      38472,
			Labels:    []string{"app=client"},
			Identity:  12345,
		},
		Destination: Endpoint{
			Namespace: "kube-system",
			PodName:   "dns-server",
			IP:        "10.96.0.10",
			Port:      53,
			Labels:    []string{"k8s:io.kubernetes.pod.namespace=kube-system"},
			Identity:  1,
		},
		Verdict:               "FORWARDED",
		Type:                  "L3_L4",
		Summary:               "TCP Flags: SYN",
		IsReply:               false,
		TraceObservationPoint: "to-endpoint",
	}

	if flow.Source.PodName != "client-pod" {
		t.Errorf("source pod = %q", flow.Source.PodName)
	}
	if flow.Destination.Port != 53 {
		t.Errorf("dst port = %d", flow.Destination.Port)
	}
	if flow.Verdict != "FORWARDED" {
		t.Errorf("verdict = %q", flow.Verdict)
	}
	if flow.TraceObservationPoint != "to-endpoint" {
		t.Errorf("trace = %q", flow.TraceObservationPoint)
	}
}

// TestFlowFilter verifies filter construction.
func TestFlowFilter(t *testing.T) {
	since := time.Now()
	filter := FlowFilter{
		Namespace: "production",
		PodName:   "api-server",
		Verdicts:  []string{"DROPPED", "ERROR"},
		Since:     &since,
	}

	if filter.Namespace != "production" {
		t.Errorf("namespace = %q", filter.Namespace)
	}
	if len(filter.Verdicts) != 2 {
		t.Errorf("verdicts len = %d", len(filter.Verdicts))
	}
	if filter.Since == nil || !filter.Since.Equal(since) {
		t.Errorf("since mismatch")
	}
}

// noopCorrelator is a no-op Correlator for interface compliance testing.
type noopCorrelator struct{}

func (n *noopCorrelator) Correlate(_ context.Context, _ string, _ time.Duration) ([]Flow, error) {
	return nil, nil
}

// TestCorrelatorInterface verifies that noopCorrelator satisfies the Correlator interface.
func TestCorrelatorInterface(t *testing.T) {
	var c Correlator = &noopCorrelator{}
	flows, err := c.Correlate(context.Background(), "10.0.0.5:38472→1.2.3.4:443", 5*time.Second)
	if err != nil {
		t.Fatalf("Correlate error: %v", err)
	}
	if flows != nil {
		t.Errorf("expected nil flows, got %v", flows)
	}
}

// noopFlowSource is a no-op FlowSource for interface compliance testing.
type noopFlowSource struct{}

func (n *noopFlowSource) Observe(ctx context.Context, _ FlowFilter) (<-chan Flow, error) {
	ch := make(chan Flow)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// TestFlowSourceInterface verifies that noopFlowSource satisfies FlowSource.
func TestFlowSourceInterface(t *testing.T) {
	var s FlowSource = &noopFlowSource{}
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := s.Observe(ctx, FlowFilter{Namespace: "default"})
	if err != nil {
		t.Fatalf("Observe error: %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	cancel()

	// Channel should close after context cancellation.
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after cancel")
	}
}
