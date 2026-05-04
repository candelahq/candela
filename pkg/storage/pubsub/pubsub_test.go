package pubsub

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	typespb "github.com/candelahq/candela/gen/go/candela/types"
	"github.com/candelahq/candela/pkg/storage"
)

func TestWriter_MarshalProto(t *testing.T) {
	w := &Writer{format: "proto"}
	span := testSpan()

	data, err := w.marshal(span)
	if err != nil {
		t.Fatalf("marshalProto failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("marshalProto returned empty data")
	}

	// Verify round-trip.
	var pb typespb.Span
	if err := proto.Unmarshal(data, &pb); err != nil {
		t.Fatalf("proto.Unmarshal failed: %v", err)
	}
	if pb.SpanId != span.SpanID {
		t.Errorf("SpanId = %q, want %q", pb.SpanId, span.SpanID)
	}
	if pb.TraceId != span.TraceID {
		t.Errorf("TraceId = %q, want %q", pb.TraceId, span.TraceID)
	}
	if pb.GetGenAi() == nil {
		t.Fatal("GenAi attributes missing after round-trip")
	}
	if pb.GetGenAi().Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", pb.GetGenAi().Model, "gpt-4")
	}
	if pb.GetGenAi().CostUsd != 0.05 {
		t.Errorf("CostUsd = %f, want 0.05", pb.GetGenAi().CostUsd)
	}
}

func TestWriter_MarshalJSON(t *testing.T) {
	w := &Writer{format: "json"}
	span := testSpan()

	data, err := w.marshal(span)
	if err != nil {
		t.Fatalf("marshalJSON failed: %v", err)
	}

	var decoded storage.Span
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded.SpanID != span.SpanID {
		t.Errorf("SpanID = %q, want %q", decoded.SpanID, span.SpanID)
	}
	if decoded.GenAI == nil || decoded.GenAI.Model != "gpt-4" {
		t.Error("GenAI attributes lost in JSON round-trip")
	}
}

func TestWriter_MarshalInvalidFormat(t *testing.T) {
	w := &Writer{format: "xml"}
	_, err := w.marshal(testSpan())
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestWriter_ProtoPreservesDuration(t *testing.T) {
	w := &Writer{format: "proto"}
	span := testSpan()
	span.Duration = 1500 * time.Millisecond

	data, err := w.marshal(span)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var pb typespb.Span
	if err := proto.Unmarshal(data, &pb); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	want := durationpb.New(1500 * time.Millisecond)
	if pb.Duration.Seconds != want.Seconds || pb.Duration.Nanos != want.Nanos {
		t.Errorf("Duration = %v, want %v", pb.Duration, want)
	}
}

func TestWriter_ProtoAttributes(t *testing.T) {
	w := &Writer{format: "proto"}
	span := testSpan()
	span.Attributes = map[string]string{
		"http.status":  "200",
		"proxy.stream": "true",
	}

	data, err := w.marshal(span)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var pb typespb.Span
	if err := proto.Unmarshal(data, &pb); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(pb.Attributes) != 2 {
		t.Fatalf("got %d attributes, want 2", len(pb.Attributes))
	}

	attrsMap := make(map[string]string)
	for _, attr := range pb.Attributes {
		if sv, ok := attr.Value.(*typespb.Attribute_StringValue); ok {
			attrsMap[attr.Key] = sv.StringValue
		}
	}

	if attrsMap["http.status"] != "200" {
		t.Errorf(`expected attribute "http.status" to be "200", got %q`, attrsMap["http.status"])
	}
	if attrsMap["proxy.stream"] != "true" {
		t.Errorf(`expected attribute "proxy.stream" to be "true", got %q`, attrsMap["proxy.stream"])
	}

	// Verify deterministic ordering (keys sorted alphabetically).
	if pb.Attributes[0].Key != "http.status" {
		t.Errorf("first attribute key = %q, want http.status (sorted)", pb.Attributes[0].Key)
	}
}

func TestNew_InvalidFormat(t *testing.T) {
	_, err := New(context.TODO(), Config{
		ProjectID: "project",
		TopicID:   "topic",
		Format:    "xml",
	})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestWriter_NilGenAI(t *testing.T) {
	w := &Writer{format: "proto"}
	span := testSpan()
	span.GenAI = nil

	data, err := w.marshal(span)
	if err != nil {
		t.Fatalf("marshal with nil GenAI failed: %v", err)
	}

	var pb typespb.Span
	if err := proto.Unmarshal(data, &pb); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if pb.GetGenAi() != nil {
		t.Error("expected nil GenAi in proto")
	}
}

func testSpan() storage.Span {
	now := time.Now()
	return storage.Span{
		SpanID:    "abc123",
		TraceID:   "trace456",
		Name:      "openai.chat",
		Kind:      storage.SpanKindLLM,
		Status:    storage.SpanStatusOK,
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		Duration:  100 * time.Millisecond,
		ProjectID: "test-project",
		UserID:    "user@example.com",
		GenAI: &storage.GenAIAttributes{
			Model:        "gpt-4",
			Provider:     "openai",
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			CostUSD:      0.05,
		},
	}
}
