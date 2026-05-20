package tetragonaudit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
)

func TestBuildLogPayload_Structure(t *testing.T) {
	record := AuditRecord{
		Timestamp:    time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Severity:     "CRITICAL",
		NodeName:     "node-1",
		PolicyName:   "deny-openai",
		Action:       "SIGKILL",
		Binary:       "/usr/bin/curl",
		Arguments:    "https://api.openai.com/v1/chat/completions",
		PodName:      "app-pod-abc",
		Namespace:    "default",
		Container:    "app",
		UID:          1000,
		DstAddr:      "104.18.6.192",
		DstPort:      443,
		SrcAddr:      "10.0.0.5",
		SrcPort:      54321,
		FunctionName: "tcp_connect",
	}

	logs := BuildLogPayload(record)

	// Verify top-level structure.
	if logs.ResourceLogs().Len() != 1 {
		t.Fatalf("expected 1 resource log, got %d", logs.ResourceLogs().Len())
	}

	rl := logs.ResourceLogs().At(0)

	// Resource attributes.
	res := rl.Resource()
	assertAttr(t, res.Attributes(), "service.name", "candela-sidecar")
	assertAttr(t, res.Attributes(), "k8s.node.name", "node-1")
	assertAttr(t, res.Attributes(), "k8s.namespace.name", "default")
	assertAttr(t, res.Attributes(), "k8s.pod.name", "app-pod-abc")
	assertAttr(t, res.Attributes(), "k8s.container.name", "app")

	// Scope.
	sl := rl.ScopeLogs().At(0)
	if sl.Scope().Name() != "candela.tetragon.audit" {
		t.Errorf("expected scope name 'candela.tetragon.audit', got %q", sl.Scope().Name())
	}

	// Log record.
	lr := sl.LogRecords().At(0)

	if lr.SeverityNumber() != plog.SeverityNumberFatal {
		t.Errorf("expected FATAL severity, got %v", lr.SeverityNumber())
	}
	if lr.SeverityText() != "CRITICAL" {
		t.Errorf("expected severity text 'CRITICAL', got %q", lr.SeverityText())
	}

	// Body should contain the summary.
	body := lr.Body().Str()
	if body == "" {
		t.Error("expected non-empty body")
	}

	// Attributes.
	attrs := lr.Attributes()
	assertAttr(t, attrs, "candela.audit.action", "SIGKILL")
	assertAttr(t, attrs, "candela.audit.policy", "deny-openai")
	assertAttr(t, attrs, "process.binary", "/usr/bin/curl")
	assertAttr(t, attrs, "net.dst.addr", "104.18.6.192")
	assertAttr(t, attrs, "tetragon.function", "tcp_connect")
}

func TestBuildLogPayload_Severities(t *testing.T) {
	tests := []struct {
		input    string
		wantNum  plog.SeverityNumber
		wantText string
	}{
		{"CRITICAL", plog.SeverityNumberFatal, "CRITICAL"},
		{"WARN", plog.SeverityNumberWarn, "WARN"},
		{"WARNING", plog.SeverityNumberWarn, "WARN"},
		{"INFO", plog.SeverityNumberInfo, "INFO"},
		{"", plog.SeverityNumberInfo, "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			logs := BuildLogPayload(AuditRecord{Severity: tt.input})
			lr := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
			if lr.SeverityNumber() != tt.wantNum {
				t.Errorf("severity %q: got number %v, want %v", tt.input, lr.SeverityNumber(), tt.wantNum)
			}
			if lr.SeverityText() != tt.wantText {
				t.Errorf("severity %q: got text %q, want %q", tt.input, lr.SeverityText(), tt.wantText)
			}
		})
	}
}

func TestBuildLogPayload_OmitsEmptyK8sAttrs(t *testing.T) {
	record := AuditRecord{
		Severity: "INFO",
		Binary:   "/bin/test",
	}
	logs := BuildLogPayload(record)
	res := logs.ResourceLogs().At(0).Resource()

	// Empty fields should not appear.
	if _, ok := res.Attributes().Get("k8s.node.name"); ok {
		t.Error("expected k8s.node.name to be absent when empty")
	}
	if _, ok := res.Attributes().Get("k8s.namespace.name"); ok {
		t.Error("expected k8s.namespace.name to be absent when empty")
	}
}

func TestNewOTelSink_RequiresEndpoint(t *testing.T) {
	_, err := NewOTelSink(OTelSinkConfig{})
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
}

func TestOTelSink_EmitRoundTrip(t *testing.T) {
	var received []byte
	var contentType string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		received = body
		w.WriteHeader(http.StatusOK)
		// Return a valid OTLP response.
		_, _ = w.Write([]byte("{}"))
	}))
	defer ts.Close()

	sink, err := NewOTelSink(OTelSinkConfig{
		Endpoint: ts.URL,
		Headers:  map[string]string{"X-Custom": "test-val"},
	})
	if err != nil {
		t.Fatalf("NewOTelSink: %v", err)
	}

	record := AuditRecord{
		Timestamp:    time.Now(),
		Severity:     "INFO",
		NodeName:     "node-2",
		PolicyName:   "allow-anthropic",
		Action:       "post",
		Binary:       "/usr/bin/python3",
		DstAddr:      "104.18.7.192",
		DstPort:      443,
		FunctionName: "tcp_connect",
	}

	if err := sink.Emit(context.Background(), record); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// Verify Content-Type.
	if contentType != "application/x-protobuf" {
		t.Errorf("expected Content-Type 'application/x-protobuf', got %q", contentType)
	}

	// Verify we can unmarshal the protobuf payload.
	if len(received) == 0 {
		t.Fatal("expected non-empty body")
	}
	exportReq := plogotlp.NewExportRequest()
	if err := exportReq.UnmarshalProto(received); err != nil {
		t.Fatalf("unmarshal OTLP export request: %v", err)
	}

	logs := exportReq.Logs()
	if logs.ResourceLogs().Len() != 1 {
		t.Fatalf("expected 1 resource log, got %d", logs.ResourceLogs().Len())
	}

	lr := logs.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)
	action, _ := lr.Attributes().Get("candela.audit.action")
	if action.Str() != "post" {
		t.Errorf("expected action 'post', got %q", action.Str())
	}
}

func TestOTelSink_EmitHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	sink, err := NewOTelSink(OTelSinkConfig{Endpoint: ts.URL})
	if err != nil {
		t.Fatalf("NewOTelSink: %v", err)
	}

	err = sink.Emit(context.Background(), AuditRecord{Severity: "INFO"})
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

func TestOTelSink_CustomHeaders(t *testing.T) {
	var authHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer ts.Close()

	sink, err := NewOTelSink(OTelSinkConfig{
		Endpoint: ts.URL,
		Headers:  map[string]string{"Authorization": "Bearer test-token"},
	})
	if err != nil {
		t.Fatalf("NewOTelSink: %v", err)
	}

	_ = sink.Emit(context.Background(), AuditRecord{Severity: "INFO"})

	if authHeader != "Bearer test-token" {
		t.Errorf("expected auth header 'Bearer test-token', got %q", authHeader)
	}
}

func TestOTelSink_ProtobufPayloadDeserializable(t *testing.T) {
	// Verify the full OTLP payload round-trips to JSON for debugging.
	record := AuditRecord{
		Timestamp:    time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		Severity:     "CRITICAL",
		NodeName:     "node-1",
		PolicyName:   "deny-all",
		Action:       "SIGKILL",
		Binary:       "/bin/curl",
		DstAddr:      "1.2.3.4",
		DstPort:      443,
		FunctionName: "tcp_connect",
	}

	logs := BuildLogPayload(record)

	// Marshal to proto → unmarshal → marshal to JSON.
	req := plogotlp.NewExportRequestFromLogs(logs)
	data, err := req.MarshalProto()
	if err != nil {
		t.Fatalf("marshal proto: %v", err)
	}

	req2 := plogotlp.NewExportRequest()
	if err := req2.UnmarshalProto(data); err != nil {
		t.Fatalf("unmarshal proto: %v", err)
	}

	jsonData, err := req2.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}

	// Should be valid JSON.
	if !json.Valid(jsonData) {
		t.Error("expected valid JSON from OTLP payload")
	}
}

// assertAttr checks a string attribute value.
func assertAttr(t *testing.T, attrs pcommon.Map, key, want string) {
	t.Helper()
	v, ok := attrs.Get(key)
	if !ok {
		t.Errorf("attribute %q not found", key)
		return
	}
	if v.Str() != want {
		t.Errorf("attribute %q = %q, want %q", key, v.Str(), want)
	}
}

func TestOTelSink_TrailingSlashEndpoint(t *testing.T) {
	var requestURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURL = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Endpoint with trailing slash — should NOT produce double-slash.
	sink, err := NewOTelSink(OTelSinkConfig{
		Endpoint: ts.URL + "/",
	})
	if err != nil {
		t.Fatalf("NewOTelSink: %v", err)
	}

	if err := sink.Emit(context.Background(), AuditRecord{Severity: "INFO"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if requestURL != "/v1/logs" {
		t.Errorf("expected path /v1/logs, got %q (double-slash bug?)", requestURL)
	}
}

func TestGRPCEventStreamAdapter_NilStream(t *testing.T) {
	adapter := &GRPCEventStreamAdapter{stream: nil}
	_, err := adapter.Recv()
	if err == nil {
		t.Fatal("expected error from nil stream adapter")
	}
}
