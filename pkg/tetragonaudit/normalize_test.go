package tetragonaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestNormalize_ProcessExec exercises the ProcessExec branch of normalize()
// which is separate from the ProcessKprobe path.
func TestNormalize_ProcessExec(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	event := Event{
		Time:     time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		NodeName: "worker-3",
		ProcessExec: &ProcessExec{
			Process: &Process{
				Binary:    "/usr/bin/python3",
				Arguments: "serve.py --port 8080",
				UID:       1001,
				Pod: &PodInfo{
					Namespace: "default",
					Name:      "web-pod-abc",
					Container: &ContainerInfo{
						Name:    "app",
						ImageID: "sha256:abc123",
					},
				},
			},
		},
	}

	err := p.ProcessEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	records := sink.GetRecords()
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	r := records[0]

	if r.Binary != "/usr/bin/python3" {
		t.Errorf("binary = %q, want /usr/bin/python3", r.Binary)
	}
	if r.Arguments != "serve.py --port 8080" {
		t.Errorf("arguments = %q", r.Arguments)
	}
	if r.UID != 1001 {
		t.Errorf("uid = %d, want 1001", r.UID)
	}
	if r.PodName != "web-pod-abc" {
		t.Errorf("pod = %q, want web-pod-abc", r.PodName)
	}
	if r.Namespace != "default" {
		t.Errorf("namespace = %q, want default", r.Namespace)
	}
	if r.Container != "app" {
		t.Errorf("container = %q, want app", r.Container)
	}
	// Exec events should default to INFO severity.
	if r.Severity != "INFO" {
		t.Errorf("severity = %q, want INFO", r.Severity)
	}
}

// TestNormalize_KprobeWithPodMetadata verifies that Pod/Container metadata
// is extracted from kprobe events.
func TestNormalize_KprobeWithPodMetadata(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	event := Event{
		Time:     time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
		NodeName: "node-2",
		ProcessKprobe: &ProcessKprobe{
			Process: &Process{
				Binary: "/usr/bin/curl",
				UID:    0,
				Pod: &PodInfo{
					Namespace: "prod",
					Name:      "api-server-xyz",
					Container: &ContainerInfo{
						Name:    "proxy",
						ImageID: "sha256:def456",
					},
				},
			},
			FunctionName: "tcp_connect",
			Action:       "SIGKILL",
			PolicyName:   "candela-enforce",
			Args: []KprobeArg{
				{SockArg: &SockArg{
					Daddr: "10.0.1.5",
					Dport: 6379,
					Saddr: "10.0.0.3",
					Sport: 45678,
				}},
			},
		},
	}

	err := p.ProcessEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	records := sink.GetRecords()
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	r := records[0]

	if r.Namespace != "prod" {
		t.Errorf("namespace = %q, want prod", r.Namespace)
	}
	if r.PodName != "api-server-xyz" {
		t.Errorf("pod = %q, want api-server-xyz", r.PodName)
	}
	if r.Container != "proxy" {
		t.Errorf("container = %q, want proxy", r.Container)
	}
	if r.Severity != "CRITICAL" {
		t.Errorf("severity = %q, want CRITICAL for SIGKILL", r.Severity)
	}
	if r.DstAddr != "10.0.1.5" || r.DstPort != 6379 {
		t.Errorf("dst = %s:%d, want 10.0.1.5:6379", r.DstAddr, r.DstPort)
	}
	if r.SrcAddr != "10.0.0.3" || r.SrcPort != 45678 {
		t.Errorf("src = %s:%d, want 10.0.0.3:45678", r.SrcAddr, r.SrcPort)
	}
}

// TestNormalize_NilProcess verifies graceful handling of kprobe with nil Process.
func TestNormalize_NilProcess(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	event := Event{
		Time:     time.Now(),
		NodeName: "node-1",
		ProcessKprobe: &ProcessKprobe{
			FunctionName: "sys_openat",
			Action:       "post",
			PolicyName:   "test-policy",
			// Process is nil — should not panic.
		},
	}

	err := p.ProcessEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	records := sink.GetRecords()
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Binary != "" {
		t.Errorf("binary should be empty for nil Process, got %q", records[0].Binary)
	}
	if records[0].FunctionName != "sys_openat" {
		t.Errorf("function = %q, want sys_openat", records[0].FunctionName)
	}
}

// TestNormalize_ProcessExecNilProcess verifies nil Process in ProcessExec.
func TestNormalize_ProcessExecNilProcess(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	event := Event{
		Time:        time.Now(),
		NodeName:    "node-1",
		ProcessExec: &ProcessExec{Process: nil},
	}

	err := p.ProcessEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	records := sink.GetRecords()
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Binary != "" {
		t.Errorf("binary should be empty for nil Process")
	}
}

// TestLogSink_EmitNoError verifies the LogSink implementation emits without error.
func TestLogSink_EmitNoError(t *testing.T) {
	sink := &LogSink{}
	err := sink.Emit(context.Background(), AuditRecord{
		Severity:     "CRITICAL",
		Action:       "SIGKILL",
		Binary:       "/bin/evil",
		PolicyName:   "block-policy",
		NodeName:     "node-5",
		FunctionName: "tcp_connect",
		DstAddr:      "10.0.0.1",
		DstPort:      443,
	})
	if err != nil {
		t.Errorf("LogSink.Emit returned error: %v", err)
	}
}

// errSink is a test sink that always returns an error.
type errSink struct{}

func (s *errSink) Emit(_ context.Context, _ AuditRecord) error {
	return errors.New("sink failure")
}

// TestProcessJSONStream_SinkError exercises the sink-error stats path.
func TestProcessJSONStream_SinkError(t *testing.T) {
	p := NewPipeline(PipelineConfig{Sink: &errSink{}})

	input := `{"time":"2026-05-18T12:00:00Z","node_name":"n1","process_kprobe":{"process":{"binary":"/bin/test"},"function_name":"tcp_connect","action":"post","policy_name":"test"}}`
	_ = p.ProcessJSONStream(context.Background(), strings.NewReader(input))

	_, _, errs := p.Stats().Snapshot()
	if errs != 1 {
		t.Errorf("errors = %d, want 1 (sink failure should be counted)", errs)
	}
}

// TestProcessJSONStream_EmptyLines verifies empty lines are gracefully skipped.
func TestProcessJSONStream_EmptyLines(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	input := "\n\n{\"time\":\"2026-05-18T12:00:00Z\",\"node_name\":\"n1\",\"process_kprobe\":{\"process\":{\"binary\":\"/bin/test\"},\"function_name\":\"f\",\"action\":\"post\",\"policy_name\":\"p\"}}\n\n\n"
	err := p.ProcessJSONStream(context.Background(), strings.NewReader(input))
	if err != nil {
		t.Fatalf("ProcessJSONStream: %v", err)
	}

	if len(sink.GetRecords()) != 1 {
		t.Errorf("got %d records, want 1 (empty lines should be skipped)", len(sink.GetRecords()))
	}
	processed, _, errs := p.Stats().Snapshot()
	if processed != 1 || errs != 0 {
		t.Errorf("stats: processed=%d errors=%d, want 1/0", processed, errs)
	}
}

// TestProcessEvent_FilterDrop verifies that filtered events increment Dropped stat.
func TestProcessEvent_FilterDrop(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{
		Sink:    sink,
		Filters: []EventFilter{EnforcementOnly()},
	})

	// This is a "post" (allow) event — should be filtered out.
	event := Event{
		Time:     time.Now(),
		NodeName: "node-1",
		ProcessKprobe: &ProcessKprobe{
			Process: &Process{Binary: "/bin/test"},
			Action:  "post",
		},
	}

	err := p.ProcessEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}

	if len(sink.GetRecords()) != 0 {
		t.Error("expected 0 records for filtered event")
	}

	_, dropped, _ := p.Stats().Snapshot()
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
}

// TestNormalize_SigkillCaseVariant verifies "Sigkill" (mixed case) severity.
func TestNormalize_SigkillCaseVariant(t *testing.T) {
	sink := &CollectorSink{}
	p := NewPipeline(PipelineConfig{Sink: sink})

	event := Event{
		Time:     time.Now(),
		NodeName: "node-1",
		ProcessKprobe: &ProcessKprobe{
			Process: &Process{Binary: "/bin/test"},
			Action:  "Sigkill", // Mixed case variant.
		},
	}

	_ = p.ProcessEvent(context.Background(), event)

	records := sink.GetRecords()
	if len(records) != 1 {
		t.Fatalf("got %d records", len(records))
	}
	if records[0].Severity != "CRITICAL" {
		t.Errorf("severity = %q, want CRITICAL for Sigkill variant", records[0].Severity)
	}
}

// TestBuildLogPayload_PodResourceAttributes verifies OTel resource attributes
// include k8s metadata when present.
func TestBuildLogPayload_PodResourceAttributes(t *testing.T) {
	record := AuditRecord{
		Severity:  "INFO",
		NodeName:  "worker-2",
		Namespace: "production",
		PodName:   "api-server-abc",
		Container: "sidecar",
	}

	logs := BuildLogPayload(record)
	res := logs.ResourceLogs().At(0).Resource().Attributes()

	checks := map[string]string{
		"k8s.node.name":      "worker-2",
		"k8s.namespace.name": "production",
		"k8s.pod.name":       "api-server-abc",
		"k8s.container.name": "sidecar",
	}

	for key, want := range checks {
		v, ok := res.Get(key)
		if !ok {
			t.Errorf("resource attribute %q not found", key)
			continue
		}
		if v.Str() != want {
			t.Errorf("%q = %q, want %q", key, v.Str(), want)
		}
	}
}

// TestBuildLogPayload_OmitsEmptyK8sAttrs_Detailed verifies that all four k8s
// resource attributes are individually omitted when empty.
func TestBuildLogPayload_OmitsEmptyK8sAttrs_Detailed(t *testing.T) {
	// Only NodeName is set — the other three should be absent.
	record := AuditRecord{
		Severity: "INFO",
		NodeName: "n1",
	}
	logs := BuildLogPayload(record)
	res := logs.ResourceLogs().At(0).Resource().Attributes()

	for _, key := range []string{"k8s.namespace.name", "k8s.pod.name", "k8s.container.name"} {
		if _, ok := res.Get(key); ok {
			t.Errorf("resource attribute %q should be absent when field is empty", key)
		}
	}
}

// TestGRPCSource_CustomDialOptions verifies NewGRPCSource accepts custom opts.
func TestGRPCSource_CustomDialOptions(t *testing.T) {
	src, err := NewGRPCSource("localhost:12345",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewGRPCSource with custom opts: %v", err)
	}
	defer func() { _ = src.Close() }()

	if src.Conn() == nil {
		t.Error("Conn() should not be nil with custom opts")
	}
}
