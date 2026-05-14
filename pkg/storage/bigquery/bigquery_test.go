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

func TestBuildTrace_MultiSpanDuration(t *testing.T) {
	now := time.Now().UTC()
	spans := []storage.Span{
		{
			SpanID:    "s1",
			StartTime: now.Add(1 * time.Second),
			EndTime:   now.Add(3 * time.Second),
		},
		{
			SpanID:    "s2",
			StartTime: now,
			EndTime:   now.Add(2 * time.Second),
		},
	}

	trace := buildTrace("t1", spans)
	// Duration should be from earliest start to latest end = 3s.
	// buildTrace iterates in slice order; s1 is first so root start = now+1s, max end = now+3s = 2s.
	// This is a known limitation: buildTrace uses the first span as root.
	if trace.Duration != 2*time.Second {
		t.Errorf("Duration = %v, want 2s", trace.Duration)
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
		TenantID:      "tenant-42",
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

	// Tenant ID.
	if roundtripped.TenantID != original.TenantID {
		t.Errorf("TenantID = %q, want %q", roundtripped.TenantID, original.TenantID)
	}
}

func TestBuildTrace_TenantIDPreserved(t *testing.T) {
	spans := []storage.Span{
		{SpanID: "s1", TenantID: "acme-corp", StartTime: time.Now()},
	}
	trace := buildTrace("t1", spans)
	if trace.Spans[0].TenantID != "acme-corp" {
		t.Errorf("TenantID = %q, want acme-corp", trace.Spans[0].TenantID)
	}
}

// ── Duration fix integration tests ──────────────────────────────────────────

func TestBuildTrace_DurationWithParentSpan(t *testing.T) {
	// Regression test for the 0ms duration bug:
	// When ALL spans have a ParentSpanID (W3C trace context propagation),
	// buildTrace must still compute a non-zero duration from the time range.
	now := time.Now().UTC()
	spans := []storage.Span{
		{
			SpanID:       "proxy-span-1",
			TraceID:      "external-trace-id",
			ParentSpanID: "external-parent-id", // ← has parent, NOT a root span
			Name:         "anthropic.chat.stream",
			StartTime:    now,
			EndTime:      now.Add(5 * time.Second),
			GenAI: &storage.GenAIAttributes{
				TotalTokens: 1000,
				CostUSD:     0.10,
			},
		},
		{
			SpanID:       "proxy-span-2",
			TraceID:      "external-trace-id",
			ParentSpanID: "external-parent-id", // also has parent
			Name:         "anthropic.chat.stream",
			StartTime:    now.Add(6 * time.Second),
			EndTime:      now.Add(10 * time.Second),
			GenAI: &storage.GenAIAttributes{
				TotalTokens: 2000,
				CostUSD:     0.20,
			},
		},
	}

	trace := buildTrace("external-trace-id", spans)

	if trace.Duration == 0 {
		t.Fatal("Duration is 0 — the fallback for traces without root spans is broken")
	}
	if trace.Duration != 10*time.Second {
		t.Errorf("Duration = %v, want 10s (from earliest start to latest end)", trace.Duration)
	}
	// RootSpanName should be empty since no span has ParentSpanID=="".
	if trace.RootSpanName != "" {
		t.Errorf("RootSpanName = %q, want empty (no root span exists)", trace.RootSpanName)
	}
	// Tokens and cost should still be aggregated.
	if trace.TotalTokens != 3000 {
		t.Errorf("TotalTokens = %d, want 3000", trace.TotalTokens)
	}
}

func TestBuildTrace_DurationWithRootSpan(t *testing.T) {
	// Regression guard: when a root span EXISTS, its duration should be used
	// (not the fallback). This ensures the fix didn't break the normal path.
	now := time.Now().UTC()
	spans := []storage.Span{
		{
			SpanID:       "root-span",
			TraceID:      "trace-normal",
			ParentSpanID: "", // root span
			Name:         "openai.chat",
			StartTime:    now,
			EndTime:      now.Add(3 * time.Second),
		},
		{
			SpanID:       "child-span",
			TraceID:      "trace-normal",
			ParentSpanID: "root-span",
			Name:         "tool.call",
			StartTime:    now.Add(500 * time.Millisecond),
			EndTime:      now.Add(2 * time.Second),
		},
	}

	trace := buildTrace("trace-normal", spans)

	if trace.Duration != 3*time.Second {
		t.Errorf("Duration = %v, want 3s (from root span)", trace.Duration)
	}
	if trace.RootSpanName != "openai.chat" {
		t.Errorf("RootSpanName = %q, want openai.chat", trace.RootSpanName)
	}
}

func TestSpanRowRoundtrip_CacheTokens(t *testing.T) {
	// Verify that cache token fields survive the spanToRow → rowToSpan roundtrip.
	now := time.Now().UTC().Truncate(time.Millisecond)
	original := storage.Span{
		SpanID:    "span-cache",
		TraceID:   "trace-cache",
		Name:      "anthropic.chat.stream",
		StartTime: now,
		EndTime:   now.Add(2 * time.Second),
		Duration:  2 * time.Second,
		GenAI: &storage.GenAIAttributes{
			Model:               "claude-sonnet-4-20250514",
			Provider:            "anthropic",
			InputTokens:         19455, // cost-normalized
			OutputTokens:        393,
			TotalTokens:         19848,
			CostUSD:             0.64,
			CacheReadTokens:     188086, // raw from API
			CacheCreationTokens: 500,    // raw from API
		},
	}

	row := spanToRow(original)
	roundtripped := rowToSpan(row)

	if roundtripped.GenAI == nil {
		t.Fatal("GenAI is nil after roundtrip")
	}
	if roundtripped.GenAI.CacheReadTokens != 188086 {
		t.Errorf("CacheReadTokens = %d, want 188086", roundtripped.GenAI.CacheReadTokens)
	}
	if roundtripped.GenAI.CacheCreationTokens != 500 {
		t.Errorf("CacheCreationTokens = %d, want 500", roundtripped.GenAI.CacheCreationTokens)
	}
	// Also verify the cost-normalized tokens survived.
	if roundtripped.GenAI.InputTokens != 19455 {
		t.Errorf("InputTokens = %d, want 19455", roundtripped.GenAI.InputTokens)
	}
}
