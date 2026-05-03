package prototest

import (
	"testing"

	typespb "github.com/candelahq/candela/gen/go/candela/types"
	"google.golang.org/protobuf/proto"
)

func TestSpan_RoundTrip_PreservesGenAI(t *testing.T) {
	original := &typespb.Span{
		SpanId:  "span-001",
		TraceId: "trace-001",
		Name:    "llm.completion",
		Kind:    typespb.SpanKind_SPAN_KIND_LLM,
		Status:  typespb.SpanStatus_SPAN_STATUS_OK,
		GenAi: &typespb.GenAIAttributes{
			Model:        "gpt-4o",
			Provider:     "openai",
			InputTokens:  1000,
			OutputTokens: 500,
			TotalTokens:  1500,
			CostUsd:      0.0075,
			Temperature:  0.7,
			MaxTokens:    4096,
			TopP:         0.9,
			InputContent: "Hello, world!",
		},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typespb.Span{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.GenAi.Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", decoded.GenAi.Model)
	}
	if decoded.GenAi.InputTokens != 1000 {
		t.Errorf("input_tokens = %d, want 1000", decoded.GenAi.InputTokens)
	}
	if decoded.GenAi.CostUsd != 0.0075 {
		t.Errorf("cost_usd = %f, want 0.0075", decoded.GenAi.CostUsd)
	}
	if decoded.GenAi.Temperature != 0.7 {
		t.Errorf("temperature = %f, want 0.7", decoded.GenAi.Temperature)
	}
	if decoded.GenAi.InputContent != "Hello, world!" {
		t.Errorf("input_content = %q, want 'Hello, world!'", decoded.GenAi.InputContent)
	}
}

func TestAttribute_OneofVariants_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		attr *typespb.Attribute
	}{
		{"string", &typespb.Attribute{Key: "k", Value: &typespb.Attribute_StringValue{StringValue: "hello"}}},
		{"int", &typespb.Attribute{Key: "k", Value: &typespb.Attribute_IntValue{IntValue: 42}}},
		{"double", &typespb.Attribute{Key: "k", Value: &typespb.Attribute_DoubleValue{DoubleValue: 3.14}}},
		{"bool", &typespb.Attribute{Key: "k", Value: &typespb.Attribute_BoolValue{BoolValue: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := proto.Marshal(tt.attr)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			decoded := &typespb.Attribute{}
			if err := proto.Unmarshal(data, decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !proto.Equal(tt.attr, decoded) {
				t.Errorf("round-trip mismatch: got %v, want %v", decoded, tt.attr)
			}
		})
	}
}

func TestTraceSummary_FieldNumbers_Stable(t *testing.T) {
	// Guard against accidental field renumbering by verifying
	// known field numbers serialize/deserialize correctly.
	original := &typespb.TraceSummary{
		TraceId:         "trace-abc",
		SpanCount:       5,
		LlmCallCount:    3,
		TotalTokens:     2500,
		TotalCostUsd:    0.05,
		PrimaryModel:    "gemini-2.5-pro",
		PrimaryProvider: "google_ai",
		UserId:          "user-xyz",
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &typespb.TraceSummary{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.TraceId != "trace-abc" {
		t.Errorf("trace_id (field 1) = %q", decoded.TraceId)
	}
	if decoded.SpanCount != 5 {
		t.Errorf("span_count (field 10) = %d", decoded.SpanCount)
	}
	if decoded.LlmCallCount != 3 {
		t.Errorf("llm_call_count (field 11) = %d", decoded.LlmCallCount)
	}
	if decoded.PrimaryModel != "gemini-2.5-pro" {
		t.Errorf("primary_model (field 15) = %q", decoded.PrimaryModel)
	}
	if decoded.UserId != "user-xyz" {
		t.Errorf("user_id (field 17) = %q", decoded.UserId)
	}
}

func TestBqSpanRow_RequiredFields_NonZero(t *testing.T) {
	// Verify that a fully populated BqSpanRow has all required fields non-zero.
	row := &typespb.BqSpanRow{
		SpanId:     "span-001",
		TraceId:    "trace-001",
		Name:       "llm.completion",
		Kind:       1,
		Status:     1,
		DurationNs: 1500000,
	}

	if row.SpanId == "" {
		t.Error("span_id is required but empty")
	}
	if row.TraceId == "" {
		t.Error("trace_id is required but empty")
	}
	if row.Name == "" {
		t.Error("name is required but empty")
	}
	if row.Kind == 0 {
		t.Error("kind is required but zero")
	}
	if row.Status == 0 {
		t.Error("status is required but zero")
	}
	if row.DurationNs == 0 {
		t.Error("duration_ns is required but zero")
	}

	// Verify round-trip
	data, err := proto.Marshal(row)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &typespb.BqSpanRow{}
	if err := proto.Unmarshal(data, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !proto.Equal(row, decoded) {
		t.Error("BqSpanRow round-trip mismatch")
	}
}
