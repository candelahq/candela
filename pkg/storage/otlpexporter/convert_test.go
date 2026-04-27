package otlpexporter

import (
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// --- hexToBytes tests ---

func TestHexToBytes_ValidTraceID(t *testing.T) {
	// 32-char hex → 16 bytes.
	h := "0123456789abcdef0123456789abcdef"
	b := hexToBytes(h, 16)
	if len(b) != 16 {
		t.Fatalf("got %d bytes, want 16", len(b))
	}
	if b[0] != 0x01 || b[15] != 0xef {
		t.Errorf("decoded bytes mismatch: got %x", b)
	}
}

func TestHexToBytes_ValidSpanID(t *testing.T) {
	h := "0123456789abcdef"
	b := hexToBytes(h, 8)
	if len(b) != 8 {
		t.Fatalf("got %d bytes, want 8", len(b))
	}
}

func TestHexToBytes_InvalidHex(t *testing.T) {
	b := hexToBytes("not-valid-hex!", 16)
	if len(b) != 16 {
		t.Fatalf("got %d bytes, want 16 (zero-filled)", len(b))
	}
	for i, v := range b {
		if v != 0 {
			t.Errorf("byte %d = %d, want 0", i, v)
		}
	}
}

func TestHexToBytes_Empty(t *testing.T) {
	b := hexToBytes("", 8)
	if len(b) != 8 {
		t.Fatalf("got %d bytes, want 8", len(b))
	}
}

// --- buildAttributes tests ---

func TestBuildAttributes_AllGenAIFields(t *testing.T) {
	s := storage.Span{
		GenAI: &storage.GenAIAttributes{
			Provider:      "openai",
			Model:         "gpt-4o",
			InputTokens:   100,
			OutputTokens:  50,
			TotalTokens:   150,
			CostUSD:       0.005,
			Temperature:   0.7,
			MaxTokens:     4096,
			InputContent:  "Hello",
			OutputContent: "Hi there!",
		},
		UserID: "user-123",
	}

	attrs := buildAttributes(s)
	attrMap := attrListToMap(attrs)

	tests := []struct {
		key      string
		wantType string // "string", "int", "double"
	}{
		{"gen_ai.system", "string"},
		{"gen_ai.request.model", "string"},
		{"gen_ai.usage.input_tokens", "int"},
		{"gen_ai.usage.output_tokens", "int"},
		{"gen_ai.usage.total_tokens", "int"},
		{"gen_ai.usage.cost", "double"},
		{"gen_ai.request.temperature", "double"},
		{"gen_ai.request.max_tokens", "int"},
		{"gen_ai.prompt", "string"},
		{"gen_ai.completion", "string"},
		{"enduser.id", "string"},
	}

	for _, tt := range tests {
		kv, ok := attrMap[tt.key]
		if !ok {
			t.Errorf("missing attribute %q", tt.key)
			continue
		}

		switch tt.wantType {
		case "string":
			if _, ok := kv.Value.Value.(*commonpb.AnyValue_StringValue); !ok {
				t.Errorf("attribute %q: want string, got %T", tt.key, kv.Value.Value)
			}
		case "int":
			if _, ok := kv.Value.Value.(*commonpb.AnyValue_IntValue); !ok {
				t.Errorf("attribute %q: want int, got %T", tt.key, kv.Value.Value)
			}
		case "double":
			if _, ok := kv.Value.Value.(*commonpb.AnyValue_DoubleValue); !ok {
				t.Errorf("attribute %q: want double, got %T", tt.key, kv.Value.Value)
			}
		}
	}
}

func TestBuildAttributes_NilGenAI(t *testing.T) {
	s := storage.Span{GenAI: nil}
	attrs := buildAttributes(s)

	for _, a := range attrs {
		if len(a.Key) >= 6 && a.Key[:6] == "gen_ai" {
			t.Errorf("unexpected gen_ai attribute %q with nil GenAI", a.Key)
		}
	}
}

func TestBuildAttributes_PassThrough(t *testing.T) {
	s := storage.Span{
		Attributes: map[string]string{
			"proxy.upstream":  "https://api.openai.com",
			"http.request_id": "req-abc",
		},
	}
	attrs := buildAttributes(s)
	attrMap := attrListToMap(attrs)

	if _, ok := attrMap["proxy.upstream"]; !ok {
		t.Error("missing pass-through attribute proxy.upstream")
	}
	if _, ok := attrMap["http.request_id"]; !ok {
		t.Error("missing pass-through attribute http.request_id")
	}
}

func TestBuildAttributes_UserID_Empty(t *testing.T) {
	s := storage.Span{UserID: ""}
	attrs := buildAttributes(s)
	attrMap := attrListToMap(attrs)

	if _, ok := attrMap["enduser.id"]; ok {
		t.Error("enduser.id should be absent when UserID is empty")
	}
}

// --- mapSpanKind tests ---

func TestMapSpanKind(t *testing.T) {
	tests := []struct {
		input storage.SpanKind
		want  tracepb.Span_SpanKind
	}{
		{storage.SpanKindLLM, tracepb.Span_SPAN_KIND_CLIENT},
		{storage.SpanKindAgent, tracepb.Span_SPAN_KIND_INTERNAL},
		{storage.SpanKindTool, tracepb.Span_SPAN_KIND_CLIENT},
		{storage.SpanKindChain, tracepb.Span_SPAN_KIND_INTERNAL},
		{storage.SpanKindUnspecified, tracepb.Span_SPAN_KIND_UNSPECIFIED},
	}

	for _, tt := range tests {
		got := mapSpanKind(tt.input)
		if got != tt.want {
			t.Errorf("mapSpanKind(%d) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// --- mapStatus tests ---

func TestMapStatus(t *testing.T) {
	tests := []struct {
		status  storage.SpanStatus
		message string
		want    tracepb.Status_StatusCode
	}{
		{storage.SpanStatusOK, "", tracepb.Status_STATUS_CODE_OK},
		{storage.SpanStatusError, "timeout", tracepb.Status_STATUS_CODE_ERROR},
		{storage.SpanStatusUnspecified, "", tracepb.Status_STATUS_CODE_UNSET},
	}

	for _, tt := range tests {
		got := mapStatus(tt.status, tt.message)
		if got.Code != tt.want {
			t.Errorf("mapStatus(%d) = %v, want %v", tt.status, got.Code, tt.want)
		}
		if tt.status == storage.SpanStatusError && got.Message != tt.message {
			t.Errorf("mapStatus error message = %q, want %q", got.Message, tt.message)
		}
	}
}

// --- convertSpan full round-trip ---

func TestConvertSpan_FullRoundTrip(t *testing.T) {
	now := time.Now()
	s := storage.Span{
		SpanID:        "0123456789abcdef",
		TraceID:       "0123456789abcdef0123456789abcdef",
		ParentSpanID:  "fedcba9876543210",
		Name:          "openai.chat",
		Kind:          storage.SpanKindLLM,
		Status:        storage.SpanStatusOK,
		StatusMessage: "",
		StartTime:     now,
		EndTime:       now.Add(500 * time.Millisecond),
		Duration:      500 * time.Millisecond,
		ProjectID:     "proj-1",
		ServiceName:   "my-app",
		UserID:        "user-42",
		GenAI: &storage.GenAIAttributes{
			Provider:     "openai",
			Model:        "gpt-4o",
			InputTokens:  200,
			OutputTokens: 100,
			TotalTokens:  300,
			CostUSD:      0.01,
		},
		Attributes: map[string]string{"http.status": "200"},
	}

	proto := convertSpan(s)

	if proto.Name != "openai.chat" {
		t.Errorf("Name = %q, want %q", proto.Name, "openai.chat")
	}
	if proto.Kind != tracepb.Span_SPAN_KIND_CLIENT {
		t.Errorf("Kind = %v, want CLIENT", proto.Kind)
	}
	if proto.Status.Code != tracepb.Status_STATUS_CODE_OK {
		t.Errorf("Status = %v, want OK", proto.Status.Code)
	}
	if len(proto.TraceId) != 16 {
		t.Errorf("TraceId length = %d, want 16", len(proto.TraceId))
	}
	if len(proto.SpanId) != 8 {
		t.Errorf("SpanId length = %d, want 8", len(proto.SpanId))
	}
	if len(proto.ParentSpanId) != 8 {
		t.Errorf("ParentSpanId length = %d, want 8", len(proto.ParentSpanId))
	}
	if proto.StartTimeUnixNano != uint64(now.UnixNano()) {
		t.Errorf("StartTime mismatch")
	}
	if len(proto.Attributes) == 0 {
		t.Error("expected attributes to be populated")
	}
}

// --- spansToResourceSpans grouping ---

func TestSpansToResourceSpans_Grouping(t *testing.T) {
	now := time.Now()
	spans := []storage.Span{
		{SpanID: "aaaa", TraceID: "aaaa", ProjectID: "proj-1", ServiceName: "svc-a", StartTime: now, EndTime: now, Name: "span-a"},
		{SpanID: "bbbb", TraceID: "bbbb", ProjectID: "proj-2", ServiceName: "svc-b", StartTime: now, EndTime: now, Name: "span-b"},
		{SpanID: "cccc", TraceID: "cccc", ProjectID: "proj-1", ServiceName: "svc-a", StartTime: now, EndTime: now, Name: "span-c"},
	}

	rs := spansToResourceSpans(spans)

	if len(rs) != 2 {
		t.Fatalf("got %d ResourceSpans, want 2 (grouped by ProjectID+ServiceName)", len(rs))
	}

	// First group: proj-1/svc-a should have 2 spans.
	if len(rs[0].ScopeSpans[0].Spans) != 2 {
		t.Errorf("group 0: got %d spans, want 2", len(rs[0].ScopeSpans[0].Spans))
	}
	// Second group: proj-2/svc-b should have 1 span.
	if len(rs[1].ScopeSpans[0].Spans) != 1 {
		t.Errorf("group 1: got %d spans, want 1", len(rs[1].ScopeSpans[0].Spans))
	}
}

func TestSpansToResourceSpans_DefaultServiceName(t *testing.T) {
	now := time.Now()
	spans := []storage.Span{
		{SpanID: "aaaa", TraceID: "aaaa", ProjectID: "proj-1", ServiceName: "", StartTime: now, EndTime: now, Name: "span-a"},
	}

	rs := spansToResourceSpans(spans)

	resAttrs := rs[0].Resource.Attributes
	found := false
	for _, a := range resAttrs {
		if a.Key == "service.name" {
			if sv, ok := a.Value.Value.(*commonpb.AnyValue_StringValue); ok && sv.StringValue == "candela" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected service.name=candela when ServiceName is empty")
	}
}

// --- PR review feedback tests ---

func TestBuildAttributes_ZeroValuesEmitted(t *testing.T) {
	// Temperature=0.0 and CostUSD=0.0 are valid settings — must be exported.
	s := storage.Span{
		GenAI: &storage.GenAIAttributes{
			Provider:    "openai",
			Model:       "gpt-4o",
			Temperature: 0.0,
			CostUSD:     0.0,
			MaxTokens:   0,
		},
	}

	attrs := buildAttributes(s)
	attrMap := attrListToMap(attrs)

	for _, key := range []string{
		"gen_ai.request.temperature",
		"gen_ai.usage.cost",
		"gen_ai.request.max_tokens",
	} {
		if _, ok := attrMap[key]; !ok {
			t.Errorf("zero-value attribute %q should be present when GenAI is non-nil", key)
		}
	}
}

func TestBuildAttributes_DuplicateKeyPrevention(t *testing.T) {
	// If Attributes map contains a key that collides with a GenAI semconv key,
	// the explicit GenAI value should win (pass-through is skipped).
	s := storage.Span{
		GenAI: &storage.GenAIAttributes{
			Provider: "openai",
		},
		Attributes: map[string]string{
			"gen_ai.system": "should-be-ignored",
			"custom.key":    "should-be-kept",
		},
	}

	attrs := buildAttributes(s)

	// Count occurrences of gen_ai.system — should be exactly 1.
	count := 0
	for _, a := range attrs {
		if a.Key == "gen_ai.system" {
			count++
			if sv, ok := a.Value.Value.(*commonpb.AnyValue_StringValue); ok {
				if sv.StringValue != "openai" {
					t.Errorf("gen_ai.system value = %q, want %q (explicit should win)", sv.StringValue, "openai")
				}
			}
		}
	}
	if count != 1 {
		t.Errorf("gen_ai.system appeared %d times, want exactly 1 (no duplicates)", count)
	}

	// custom.key should still be present.
	attrMap := attrListToMap(attrs)
	if _, ok := attrMap["custom.key"]; !ok {
		t.Error("non-colliding custom attribute should be preserved")
	}
}

// --- Test helpers ---

func attrListToMap(attrs []*commonpb.KeyValue) map[string]*commonpb.KeyValue {
	m := make(map[string]*commonpb.KeyValue, len(attrs))
	for _, a := range attrs {
		m[a.Key] = a
	}
	return m
}
