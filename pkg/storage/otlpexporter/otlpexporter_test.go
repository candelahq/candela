package otlpexporter

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// --- Writer unit tests ---

func TestIngestSpans_EmptyBatch(t *testing.T) {
	w := &Writer{
		client:  &noopClient{},
		timeout: 30 * time.Second,
	}

	err := w.IngestSpans(context.Background(), nil)
	if err != nil {
		t.Errorf("empty batch should not error, got %v", err)
	}

	err = w.IngestSpans(context.Background(), []storage.Span{})
	if err != nil {
		t.Errorf("zero-length batch should not error, got %v", err)
	}
}

func TestClose_Idempotent(t *testing.T) {
	w := &Writer{
		client:  &noopClient{},
		timeout: 30 * time.Second,
	}

	if err := w.Close(); err != nil {
		t.Errorf("first Close() error: %v", err)
	}
	// Second close should not panic.
	if err := w.Close(); err != nil {
		t.Errorf("second Close() error: %v", err)
	}
}

func TestIngestSpans_ExportError(t *testing.T) {
	w := &Writer{
		client:  &errorClient{err: context.DeadlineExceeded},
		timeout: 30 * time.Second,
	}

	spans := []storage.Span{
		{SpanID: "aaaa", TraceID: "aaaa", Name: "test", StartTime: time.Now(), EndTime: time.Now()},
	}

	err := w.IngestSpans(context.Background(), spans)
	if err == nil {
		t.Error("expected error from failing client, got nil")
	}
}

// --- Config validation ---

func TestNew_EmptyEndpoint(t *testing.T) {
	_, err := New(context.Background(), Config{})
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
}

func TestNew_BadProtocol(t *testing.T) {
	_, err := New(context.Background(), Config{Endpoint: "http://localhost:4318", Protocol: "websocket"})
	if err == nil {
		t.Error("expected error for unsupported protocol")
	}
}

// --- Capturing client for unit tests ---

type capturingClient struct {
	mu    sync.Mutex
	calls [][]*tracepb.ResourceSpans
}

func (c *capturingClient) Start(context.Context) error { return nil }
func (c *capturingClient) Stop(context.Context) error  { return nil }
func (c *capturingClient) UploadTraces(_ context.Context, rs []*tracepb.ResourceSpans) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, rs)
	return nil
}

func TestIngestSpans_NonLLMSpan(t *testing.T) {
	cc := &capturingClient{}
	w := &Writer{client: cc, timeout: 30 * time.Second}

	now := time.Now()
	spans := []storage.Span{
		{SpanID: "aaaa", TraceID: "bbbb", Name: "agent.plan", Kind: storage.SpanKindAgent,
			StartTime: now, EndTime: now, ProjectID: "proj-1"},
	}

	if err := w.IngestSpans(context.Background(), spans); err != nil {
		t.Fatalf("IngestSpans() error: %v", err)
	}

	if len(cc.calls) != 1 {
		t.Fatalf("expected 1 UploadTraces call, got %d", len(cc.calls))
	}
	span := cc.calls[0][0].ScopeSpans[0].Spans[0]
	if span.Name != "agent.plan" {
		t.Errorf("span name = %q, want %q", span.Name, "agent.plan")
	}
	// Should have no gen_ai.* attributes.
	for _, a := range span.Attributes {
		if len(a.Key) >= 6 && a.Key[:6] == "gen_ai" {
			t.Errorf("unexpected gen_ai attr %q on non-LLM span", a.Key)
		}
	}
}

func TestIngestSpans_LargeBatch(t *testing.T) {
	cc := &capturingClient{}
	w := &Writer{client: cc, timeout: 30 * time.Second}

	now := time.Now()
	spans := make([]storage.Span, 150)
	for i := range spans {
		spans[i] = storage.Span{
			SpanID: fmt.Sprintf("%016x", i), TraceID: "aaaa",
			Name: "llm.call", Kind: storage.SpanKindLLM,
			StartTime: now, EndTime: now, ProjectID: "proj-1",
			GenAI: &storage.GenAIAttributes{Model: "gpt-4o", Provider: "openai", InputTokens: 10},
		}
	}

	if err := w.IngestSpans(context.Background(), spans); err != nil {
		t.Fatalf("IngestSpans() error: %v", err)
	}

	if len(cc.calls) != 1 {
		t.Fatalf("expected 1 UploadTraces call, got %d", len(cc.calls))
	}
	totalSpans := len(cc.calls[0][0].ScopeSpans[0].Spans)
	if totalSpans != 150 {
		t.Errorf("got %d spans, want 150", totalSpans)
	}
}

func TestIngestSpans_MixedResourceGrouping(t *testing.T) {
	cc := &capturingClient{}
	w := &Writer{client: cc, timeout: 30 * time.Second}

	now := time.Now()
	spans := []storage.Span{
		{SpanID: "aa", TraceID: "aa", Name: "s1", ProjectID: "proj-A", ServiceName: "svc-1", StartTime: now, EndTime: now},
		{SpanID: "bb", TraceID: "bb", Name: "s2", ProjectID: "proj-B", ServiceName: "svc-2", StartTime: now, EndTime: now},
		{SpanID: "cc", TraceID: "cc", Name: "s3", ProjectID: "proj-A", ServiceName: "svc-1", StartTime: now, EndTime: now},
	}

	if err := w.IngestSpans(context.Background(), spans); err != nil {
		t.Fatalf("IngestSpans() error: %v", err)
	}

	rs := cc.calls[0]
	if len(rs) != 2 {
		t.Fatalf("expected 2 ResourceSpans (2 projects), got %d", len(rs))
	}
	if len(rs[0].ScopeSpans[0].Spans) != 2 {
		t.Errorf("proj-A group: got %d spans, want 2", len(rs[0].ScopeSpans[0].Spans))
	}
	if len(rs[1].ScopeSpans[0].Spans) != 1 {
		t.Errorf("proj-B group: got %d spans, want 1", len(rs[1].ScopeSpans[0].Spans))
	}
}

// --- Integration tests ---

// newOTLPTestServer starts an httptest.Server that captures OTLP trace exports.
func newOTLPTestServer(t *testing.T) (*httptest.Server, func() *coltracepb.ExportTraceServiceRequest) {
	t.Helper()
	var (
		mu       sync.Mutex
		received *coltracepb.ExportTraceServiceRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			http.NotFound(w, r)
			return
		}
		var body []byte
		var err error
		if r.Header.Get("Content-Encoding") == "gzip" {
			gr, gerr := gzip.NewReader(r.Body)
			if gerr != nil {
				http.Error(w, "gzip error", 400)
				return
			}
			defer func() { _ = gr.Close() }()
			body, err = io.ReadAll(gr)
		} else {
			body, err = io.ReadAll(r.Body)
		}
		if err != nil {
			http.Error(w, "read error", 500)
			return
		}
		req := &coltracepb.ExportTraceServiceRequest{}
		if err := proto.Unmarshal(body, req); err != nil {
			http.Error(w, "unmarshal error", 400)
			return
		}
		mu.Lock()
		received = req
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	get := func() *coltracepb.ExportTraceServiceRequest {
		mu.Lock()
		defer mu.Unlock()
		return received
	}
	return srv, get
}

func TestIntegration_HTTPExport(t *testing.T) {
	srv, getReceived := newOTLPTestServer(t)
	defer srv.Close()

	ctx := context.Background()
	writer, err := New(ctx, Config{Endpoint: srv.URL, Protocol: "http", Insecure: true, TimeoutSec: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = writer.Close() }()

	now := time.Now()
	spans := []storage.Span{
		{
			SpanID: "0123456789abcdef", TraceID: "0123456789abcdef0123456789abcdef",
			Name: "openai.chat", Kind: storage.SpanKindLLM, Status: storage.SpanStatusOK,
			StartTime: now, EndTime: now.Add(200 * time.Millisecond),
			ProjectID: "test-project", ServiceName: "my-svc", Environment: "staging", UserID: "user-1",
			GenAI: &storage.GenAIAttributes{
				Provider: "openai", Model: "gpt-4o",
				InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CostUSD: 0.005,
			},
		},
	}

	if err := writer.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("IngestSpans() error: %v", err)
	}

	received := getReceived()
	if received == nil {
		t.Fatal("server did not receive any OTLP request")
	}
	if len(received.ResourceSpans) != 1 {
		t.Fatalf("got %d ResourceSpans, want 1", len(received.ResourceSpans))
	}

	rs := received.ResourceSpans[0]

	// Verify ALL resource attributes.
	resMap := make(map[string]string)
	for _, a := range rs.Resource.Attributes {
		if sv, ok := a.Value.Value.(*commonpb.AnyValue_StringValue); ok {
			resMap[a.Key] = sv.StringValue
		}
	}
	for _, tc := range []struct{ key, want string }{
		{"service.namespace", "test-project"},
		{"service.name", "my-svc"},
		{"deployment.environment", "staging"},
	} {
		if resMap[tc.key] != tc.want {
			t.Errorf("resource %s = %q, want %q", tc.key, resMap[tc.key], tc.want)
		}
	}

	// Verify scope.
	scope := rs.ScopeSpans[0].Scope
	if scope.Name != "candela" {
		t.Errorf("scope name = %q, want candela", scope.Name)
	}

	// Verify span fields.
	span := rs.ScopeSpans[0].Spans[0]
	if span.Name != "openai.chat" {
		t.Errorf("span name = %q, want %q", span.Name, "openai.chat")
	}
	if len(span.TraceId) != 16 {
		t.Errorf("TraceId length = %d, want 16", len(span.TraceId))
	}

	// Verify GenAI + user attributes.
	attrMap := make(map[string]bool)
	for _, a := range span.Attributes {
		attrMap[a.Key] = true
	}
	for _, key := range []string{
		"gen_ai.system", "gen_ai.request.model",
		"gen_ai.usage.input_tokens", "gen_ai.usage.output_tokens", "gen_ai.usage.cost",
		"enduser.id",
	} {
		if !attrMap[key] {
			t.Errorf("missing expected attribute %q", key)
		}
	}
}

func TestIntegration_HTTPExport_MultiBatch(t *testing.T) {
	srv, getReceived := newOTLPTestServer(t)
	defer srv.Close()

	ctx := context.Background()
	writer, err := New(ctx, Config{Endpoint: srv.URL, Protocol: "http", Insecure: true, TimeoutSec: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = writer.Close() }()

	now := time.Now()
	spans := make([]storage.Span, 25)
	for i := range spans {
		spans[i] = storage.Span{
			SpanID: fmt.Sprintf("%016x", i), TraceID: fmt.Sprintf("%032x", i),
			Name: fmt.Sprintf("call-%d", i), Kind: storage.SpanKindLLM,
			StartTime: now, EndTime: now.Add(time.Duration(i) * time.Millisecond),
			ProjectID: "proj-batch",
			GenAI:     &storage.GenAIAttributes{Provider: "openai", Model: "gpt-4o", InputTokens: int64(i * 10)},
		}
	}

	if err := writer.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("IngestSpans() error: %v", err)
	}

	received := getReceived()
	if received == nil {
		t.Fatal("server received nothing")
	}
	total := 0
	for _, rs := range received.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			total += len(ss.Spans)
		}
	}
	if total != 25 {
		t.Errorf("total exported spans = %d, want 25", total)
	}
}

// --- Mock clients ---

// noopClient implements otlptrace.Client with no-ops.
type noopClient struct{}

func (c *noopClient) Start(context.Context) error                                      { return nil }
func (c *noopClient) Stop(context.Context) error                                       { return nil }
func (c *noopClient) UploadTraces(_ context.Context, _ []*tracepb.ResourceSpans) error { return nil }

// errorClient implements otlptrace.Client that always fails on upload.
type errorClient struct {
	err error
}

func (c *errorClient) Start(context.Context) error                                      { return nil }
func (c *errorClient) Stop(context.Context) error                                       { return nil }
func (c *errorClient) UploadTraces(_ context.Context, _ []*tracepb.ResourceSpans) error { return c.err }
