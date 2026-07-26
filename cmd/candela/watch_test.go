package main

import (
	"testing"

	"github.com/candelahq/candela/pkg/processor"
)

func TestFormatTraceRow(t *testing.T) {
	// Just verify it doesn't panic
	tb := processor.TraceBroadcast{
		TraceID:      "abc-123",
		RootSpanName: "chat/completions",
		Model:        "gpt-4o",
		Provider:     "openai",
		CostUSD:      0.0034,
		DurationMs:   120,
		SpanCount:    3,
		Status:       "ok",
		Timestamp:    "14:32:05",
	}
	printTraceRow(tb)
}
