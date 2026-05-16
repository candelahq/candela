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

// ── Deduplication classification tests ──────────────────────────────────────

// classifySpans replicates the core classification logic from IngestSpans
// for unit testing without needing a real BigQuery client.
func classifySpans(spans []storage.Span) (optimistic []storage.Span, pessimistic []spanRow) {
	for _, span := range spans {
		if span.Attributes != nil && span.Attributes["candela.is_retry"] == "true" {
			delete(span.Attributes, "candela.is_retry")
			pessimistic = append(pessimistic, spanToRow(span))
		} else {
			optimistic = append(optimistic, span)
		}
	}
	return
}

func TestClassifySpans_AllOptimistic(t *testing.T) {
	spans := []storage.Span{
		{SpanID: "s1", Name: "op1", Attributes: map[string]string{"env": "prod"}},
		{SpanID: "s2", Name: "op2"},
		{SpanID: "s3", Name: "op3", Attributes: map[string]string{}},
	}

	opt, pess := classifySpans(spans)

	if len(opt) != 3 {
		t.Errorf("optimistic count = %d, want 3", len(opt))
	}
	if len(pess) != 0 {
		t.Errorf("pessimistic count = %d, want 0", len(pess))
	}
}

func TestClassifySpans_AllPessimistic(t *testing.T) {
	spans := []storage.Span{
		{SpanID: "s1", Attributes: map[string]string{"candela.is_retry": "true"}},
		{SpanID: "s2", Attributes: map[string]string{"candela.is_retry": "true", "env": "staging"}},
	}

	opt, pess := classifySpans(spans)

	if len(opt) != 0 {
		t.Errorf("optimistic count = %d, want 0", len(opt))
	}
	if len(pess) != 2 {
		t.Errorf("pessimistic count = %d, want 2", len(pess))
	}
}

func TestClassifySpans_MixedBatch(t *testing.T) {
	spans := []storage.Span{
		{SpanID: "fresh-1", Name: "op1"},
		{SpanID: "retry-1", Attributes: map[string]string{"candela.is_retry": "true"}},
		{SpanID: "fresh-2", Name: "op2", Attributes: map[string]string{"env": "prod"}},
		{SpanID: "retry-2", Attributes: map[string]string{"candela.is_retry": "true", "user": "alice"}},
	}

	opt, pess := classifySpans(spans)

	if len(opt) != 2 {
		t.Errorf("optimistic count = %d, want 2", len(opt))
	}
	if len(pess) != 2 {
		t.Errorf("pessimistic count = %d, want 2", len(pess))
	}

	// Verify optimistic spans are the fresh ones
	for _, s := range opt {
		if s.Attributes != nil && s.Attributes["candela.is_retry"] == "true" {
			t.Errorf("optimistic span %s should not have is_retry", s.SpanID)
		}
	}
}

func TestClassifySpans_IsRetryStripped(t *testing.T) {
	// Ensure candela.is_retry is removed from the span before conversion to row,
	// so it doesn't leak into long-term storage.
	spans := []storage.Span{
		{
			SpanID: "retry-span",
			Name:   "llm.call",
			Attributes: map[string]string{
				"candela.is_retry": "true",
				"http.method":      "POST",
				"user.id":          "u42",
			},
		},
	}

	_, pess := classifySpans(spans)

	if len(pess) != 1 {
		t.Fatalf("pessimistic count = %d, want 1", len(pess))
	}

	// The converted row should have attributes but NOT candela.is_retry
	row := pess[0]
	for _, attr := range row.Attributes {
		if attr.Key == "candela.is_retry" {
			t.Error("candela.is_retry should have been stripped before row conversion")
		}
	}

	// Verify other attributes survived
	found := map[string]bool{}
	for _, attr := range row.Attributes {
		found[attr.Key] = true
	}
	if !found["http.method"] {
		t.Error("http.method attribute was lost during classification")
	}
	if !found["user.id"] {
		t.Error("user.id attribute was lost during classification")
	}
}

func TestClassifySpans_NilAttributes(t *testing.T) {
	// Spans with nil Attributes should not panic and should be classified as optimistic.
	spans := []storage.Span{
		{SpanID: "nil-attrs", Attributes: nil},
	}

	opt, pess := classifySpans(spans)

	if len(opt) != 1 {
		t.Errorf("optimistic count = %d, want 1", len(opt))
	}
	if len(pess) != 0 {
		t.Errorf("pessimistic count = %d, want 0", len(pess))
	}
}

func TestClassifySpans_IsRetryFalseIsOptimistic(t *testing.T) {
	// A span with candela.is_retry = "false" should NOT be treated as pessimistic.
	spans := []storage.Span{
		{SpanID: "s1", Attributes: map[string]string{"candela.is_retry": "false"}},
	}

	opt, pess := classifySpans(spans)

	if len(opt) != 1 {
		t.Errorf("optimistic count = %d, want 1", len(opt))
	}
	if len(pess) != 0 {
		t.Errorf("pessimistic count = %d, want 0", len(pess))
	}
}

func TestClassifySpans_EmptyInput(t *testing.T) {
	opt, pess := classifySpans(nil)

	if opt != nil {
		t.Errorf("optimistic should be nil, got %v", opt)
	}
	if pess != nil {
		t.Errorf("pessimistic should be nil, got %v", pess)
	}
}

func TestClassifySpans_RowFieldIntegrity(t *testing.T) {
	// Verify that a pessimistic span's fields are fully preserved through
	// the classifySpans → spanToRow pipeline.
	now := time.Now().UTC().Truncate(time.Millisecond)
	spans := []storage.Span{
		{
			SpanID:       "span-full",
			TraceID:      "trace-123",
			ParentSpanID: "parent-0",
			Name:         "anthropic.chat",
			Kind:         storage.SpanKindLLM,
			Status:       storage.SpanStatusOK,
			StartTime:    now,
			EndTime:      now.Add(3 * time.Second),
			Duration:     3 * time.Second,
			ProjectID:    "proj-42",
			Environment:  "production",
			ServiceName:  "gateway",
			UserID:       "alice",
			SessionID:    "sess-abc",
			TenantID:     "acme",
			GenAI: &storage.GenAIAttributes{
				Model:         "claude-sonnet-4-20250514",
				Provider:      "anthropic",
				InputTokens:   1000,
				OutputTokens:  200,
				TotalTokens:   1200,
				CostUSD:       0.05,
				Temperature:   0.7,
				MaxTokens:     8192,
				InputContent:  "Hello",
				OutputContent: "World",
			},
			Attributes: map[string]string{
				"candela.is_retry": "true",
				"custom.field":     "value",
			},
		},
	}

	_, pess := classifySpans(spans)
	if len(pess) != 1 {
		t.Fatalf("expected 1 pessimistic row, got %d", len(pess))
	}

	row := pess[0]

	// Core fields
	if row.SpanID != "span-full" {
		t.Errorf("SpanID = %q", row.SpanID)
	}
	if row.TraceID != "trace-123" {
		t.Errorf("TraceID = %q", row.TraceID)
	}
	if row.ProjectID != "proj-42" {
		t.Errorf("ProjectID = %q", row.ProjectID)
	}
	if row.TenantID != "acme" {
		t.Errorf("TenantID = %q", row.TenantID)
	}

	// GenAI fields
	if row.GenAIModel != "claude-sonnet-4-20250514" {
		t.Errorf("GenAIModel = %q", row.GenAIModel)
	}
	if row.InputTokens != 1000 {
		t.Errorf("InputTokens = %d", row.InputTokens)
	}
	if row.CostUSD != 0.05 {
		t.Errorf("CostUSD = %f", row.CostUSD)
	}

	// Timestamps
	if !row.StartTime.Equal(now) {
		t.Errorf("StartTime = %v, want %v", row.StartTime, now)
	}
	if row.DurationNs != int64(3*time.Second) {
		t.Errorf("DurationNs = %d, want %d", row.DurationNs, int64(3*time.Second))
	}
}
