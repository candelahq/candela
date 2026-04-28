package bigquery

import (
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

func TestQuoteTable(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "project.dataset.table", "`project.dataset.table`"},
		{"already safe", "my_project.spans.traces", "`my_project.spans.traces`"},
		{"embedded backtick", "proj`.evil", "`proj\\`.evil`"},
		{"empty", "", "``"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteTable(tt.input)
			if got != tt.want {
				t.Errorf("quoteTable(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildTrace_BasicAggregation(t *testing.T) {
	now := time.Now().UTC()
	spans := []storage.Span{
		{
			SpanID:       "root-1",
			TraceID:      "trace-abc",
			ParentSpanID: "",
			Name:         "openai.chat",
			StartTime:    now,
			EndTime:      now.Add(2 * time.Second),
			Environment:  "production",
			GenAI: &storage.GenAIAttributes{
				TotalTokens: 500,
				CostUSD:     0.05,
			},
		},
		{
			SpanID:       "child-1",
			TraceID:      "trace-abc",
			ParentSpanID: "root-1",
			Name:         "anthropic.chat",
			StartTime:    now.Add(100 * time.Millisecond),
			EndTime:      now.Add(1500 * time.Millisecond),
			Environment:  "production",
			GenAI: &storage.GenAIAttributes{
				TotalTokens: 300,
				CostUSD:     0.03,
			},
		},
	}

	trace := buildTrace("trace-abc", spans)

	if trace.TraceID != "trace-abc" {
		t.Errorf("TraceID = %q, want trace-abc", trace.TraceID)
	}
	if trace.RootSpanName != "openai.chat" {
		t.Errorf("RootSpanName = %q, want openai.chat", trace.RootSpanName)
	}
	if trace.SpanCount != 2 {
		t.Errorf("SpanCount = %d, want 2", trace.SpanCount)
	}
	if trace.TotalTokens != 800 {
		t.Errorf("TotalTokens = %d, want 800", trace.TotalTokens)
	}
	if trace.TotalCostUSD != 0.08 {
		t.Errorf("TotalCostUSD = %f, want 0.08", trace.TotalCostUSD)
	}
	if trace.Duration != 2*time.Second {
		t.Errorf("Duration = %v, want 2s", trace.Duration)
	}
	if trace.Environment != "production" {
		t.Errorf("Environment = %q, want production", trace.Environment)
	}
}

func TestBuildTrace_NoGenAI(t *testing.T) {
	now := time.Now().UTC()
	spans := []storage.Span{
		{
			SpanID:       "span-1",
			TraceID:      "trace-xyz",
			ParentSpanID: "",
			Name:         "http.request",
			StartTime:    now,
			EndTime:      now.Add(500 * time.Millisecond),
			GenAI:        nil, // non-LLM span
		},
	}

	trace := buildTrace("trace-xyz", spans)

	if trace.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0 for non-GenAI span", trace.TotalTokens)
	}
	if trace.TotalCostUSD != 0 {
		t.Errorf("TotalCostUSD = %f, want 0", trace.TotalCostUSD)
	}
}

func TestBuildTrace_EmptySpans(t *testing.T) {
	trace := buildTrace("trace-empty", nil)

	if trace.SpanCount != 0 {
		t.Errorf("SpanCount = %d, want 0", trace.SpanCount)
	}
	if trace.RootSpanName != "" {
		t.Errorf("RootSpanName = %q, want empty", trace.RootSpanName)
	}
}

func TestSpanRowRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	original := storage.Span{
		SpanID:        "span-001",
		TraceID:       "trace-abc",
		ParentSpanID:  "span-000",
		Name:          "openai.chat",
		Kind:          storage.SpanKindLLM,
		Status:        storage.SpanStatusOK,
		StatusMessage: "success",
		StartTime:     now,
		EndTime:       now.Add(2 * time.Second),
		Duration:      2 * time.Second,
		ProjectID:     "my-project",
		Environment:   "production",
		ServiceName:   "candela-proxy",
		UserID:        "user-abc123",
		SessionID:     "session-xyz789",
		GenAI: &storage.GenAIAttributes{
			Model:         "gpt-4o",
			Provider:      "openai",
			InputTokens:   100,
			OutputTokens:  50,
			TotalTokens:   150,
			CostUSD:       0.0075,
			Temperature:   0.7,
			MaxTokens:     4096,
			InputContent:  "Hello world",
			OutputContent: "Hi there!",
		},
		Attributes: map[string]string{
			"http.request_id": "req-123",
			"proxy.upstream":  "https://api.openai.com",
		},
	}

	// Convert to BQ row and back.
	row := spanToRow(original)
	roundtripped := rowToSpan(row)

	// Core fields.
	if roundtripped.SpanID != original.SpanID {
		t.Errorf("SpanID = %q, want %q", roundtripped.SpanID, original.SpanID)
	}
	if roundtripped.TraceID != original.TraceID {
		t.Errorf("TraceID = %q, want %q", roundtripped.TraceID, original.TraceID)
	}
	if roundtripped.ParentSpanID != original.ParentSpanID {
		t.Errorf("ParentSpanID = %q, want %q", roundtripped.ParentSpanID, original.ParentSpanID)
	}
	if roundtripped.Name != original.Name {
		t.Errorf("Name = %q, want %q", roundtripped.Name, original.Name)
	}
	if roundtripped.ProjectID != original.ProjectID {
		t.Errorf("ProjectID = %q, want %q", roundtripped.ProjectID, original.ProjectID)
	}

	// User context fields (critical for budget + observability).
	if roundtripped.UserID != original.UserID {
		t.Errorf("UserID = %q, want %q", roundtripped.UserID, original.UserID)
	}
	if roundtripped.SessionID != original.SessionID {
		t.Errorf("SessionID = %q, want %q", roundtripped.SessionID, original.SessionID)
	}

	// GenAI fields.
	if roundtripped.GenAI == nil {
		t.Fatal("GenAI is nil after roundtrip")
	}
	if roundtripped.GenAI.Model != original.GenAI.Model {
		t.Errorf("GenAI.Model = %q, want %q", roundtripped.GenAI.Model, original.GenAI.Model)
	}
	if roundtripped.GenAI.InputTokens != original.GenAI.InputTokens {
		t.Errorf("GenAI.InputTokens = %d, want %d", roundtripped.GenAI.InputTokens, original.GenAI.InputTokens)
	}
	if roundtripped.GenAI.CostUSD != original.GenAI.CostUSD {
		t.Errorf("GenAI.CostUSD = %f, want %f", roundtripped.GenAI.CostUSD, original.GenAI.CostUSD)
	}

	// Attributes.
	if len(roundtripped.Attributes) != len(original.Attributes) {
		t.Errorf("Attributes count = %d, want %d", len(roundtripped.Attributes), len(original.Attributes))
	}
	if roundtripped.Attributes["http.request_id"] != "req-123" {
		t.Errorf("Attributes[http.request_id] = %q, want req-123", roundtripped.Attributes["http.request_id"])
	}
}
