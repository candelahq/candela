package connecthandlers

import (
	"testing"
	"time"

	typespb "github.com/candelahq/candela/gen/go/candela/types"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProtoToSpan_Valid(t *testing.T) {
	now := time.Now().UTC()
	ps := &typespb.Span{
		SpanId:        "s1",
		TraceId:       "t1",
		ParentSpanId:  "ps1",
		Name:          "test",
		Kind:          typespb.SpanKind(2),
		Status:        typespb.SpanStatus(2),
		StatusMessage: "boom",
		ProjectId:     "p1",
		Environment:   "env1",
		ServiceName:   "svc1",
		StartTime:     timestamppb.New(now),
		EndTime:       timestamppb.New(now.Add(time.Second)),
		Duration:      durationpb.New(time.Second),
		GenAi: &typespb.GenAIAttributes{
			Model:        "m1",
			Provider:     "pr1",
			InputTokens:  10,
			OutputTokens: 20,
		},
		Attributes: []*typespb.Attribute{
			{Key: "str", Value: &typespb.Attribute_StringValue{StringValue: "val"}},
			{Key: "int", Value: &typespb.Attribute_IntValue{IntValue: 42}},
			{Key: "dbl", Value: &typespb.Attribute_DoubleValue{DoubleValue: 3.14}},
			{Key: "bl", Value: &typespb.Attribute_BoolValue{BoolValue: true}},
		},
	}

	span, err := protoToSpan(ps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if span.SpanID != "s1" || span.TraceID != "t1" {
		t.Errorf("IDs mismatch")
	}
	if span.GenAI.TotalTokens != 30 {
		t.Errorf("TotalTokens = %d, want 30", span.GenAI.TotalTokens)
	}
	if span.Attributes["str"] != "val" || span.Attributes["int"] != "42" || span.Attributes["dbl"] != "3.140000" || span.Attributes["bl"] != "true" {
		t.Errorf("Attributes mismatch: %v", span.Attributes)
	}
}

func TestProtoToSpan_MissingIDs(t *testing.T) {
	_, err := protoToSpan(&typespb.Span{})
	if err == nil {
		t.Error("expected error for missing IDs")
	}
}

func TestProtoToSpan_AutoDuration(t *testing.T) {
	now := time.Now().UTC()
	ps := &typespb.Span{
		SpanId:    "s1",
		TraceId:   "t1",
		StartTime: timestamppb.New(now),
		EndTime:   timestamppb.New(now.Add(time.Second)),
	}

	span, err := protoToSpan(ps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if span.Duration != time.Second {
		t.Errorf("expected auto-calculated duration of 1s, got %v", span.Duration)
	}
}

func TestProtoToSpan_NegativeDurationClamped(t *testing.T) {
	now := time.Now().UTC()
	ps := &typespb.Span{
		SpanId:    "s1",
		TraceId:   "t1",
		StartTime: timestamppb.New(now),
		EndTime:   timestamppb.New(now.Add(-time.Second)),
	}

	span, err := protoToSpan(ps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if span.Duration != 0 {
		t.Errorf("expected clamped duration of 0, got %v", span.Duration)
	}
}
