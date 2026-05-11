package duckdb

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// ── Integration Tests: GetTenantLeaderboard ───────────────────────────────
// These use a real in-memory DuckDB store (same as other duckdb tests).

// INTEG-5: GetTenantLeaderboard returns tenants ranked by cost, highest first.
func TestGetTenantLeaderboard_RanksByCost(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	spans := []storage.Span{
		tenantSpan("span-1", "trace-A", "acme-corp", 0.05),
		tenantSpan("span-2", "trace-B", "acme-corp", 0.10), // acme total = 0.15
		tenantSpan("span-3", "trace-C", "beta-inc", 0.20),  // beta total = 0.20
		tenantSpan("span-4", "trace-D", "gamma-co", 0.01),  // gamma total = 0.01
	}
	for i := range spans {
		spans[i].ProjectID = "proj-rank"
		spans[i].StartTime = now.Add(-time.Duration(i) * time.Minute)
		spans[i].EndTime = spans[i].StartTime.Add(100 * time.Millisecond)
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("AppendSpans: %v", err)
	}

	q := storage.UsageQuery{
		ProjectID: "proj-rank",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
	}
	results, err := s.GetTenantLeaderboard(ctx, q, 10)
	if err != nil {
		t.Fatalf("GetTenantLeaderboard: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	// Ranked by cost descending: beta (0.20) > acme (0.15) > gamma (0.01).
	wantOrder := []string{"beta-inc", "acme-corp", "gamma-co"}
	for i, want := range wantOrder {
		if results[i].TenantID != want {
			t.Errorf("results[%d].TenantID = %q, want %q", i, results[i].TenantID, want)
		}
	}
}

// INTEG-6: Spans with empty tenant_id are excluded from the leaderboard (CRIT-3 fix).
func TestGetTenantLeaderboard_ExcludesEmptyAndNullTenants(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	spans := []storage.Span{
		tenantSpan("span-a", "trace-A", "real-tenant", 0.50),
		tenantSpan("span-b", "trace-B", "", 99.99), // no tenant — must be excluded
	}
	for i := range spans {
		spans[i].ProjectID = "proj-empty"
		spans[i].StartTime = now.Add(-time.Duration(i) * time.Minute)
		spans[i].EndTime = spans[i].StartTime.Add(100 * time.Millisecond)
	}
	if err := s.IngestSpans(ctx, spans); err != nil {
		t.Fatalf("AppendSpans: %v", err)
	}

	q := storage.UsageQuery{
		ProjectID: "proj-empty",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
	}
	results, err := s.GetTenantLeaderboard(ctx, q, 10)
	if err != nil {
		t.Fatalf("GetTenantLeaderboard: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 (empty-tenant spans must be excluded)", len(results))
	}
	if results[0].TenantID != "real-tenant" {
		t.Errorf("TenantID = %q, want real-tenant", results[0].TenantID)
	}
}

// INTEG-7: limit parameter is respected.
func TestGetTenantLeaderboard_LimitRespected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Insert 5 distinct tenants.
	for i := 0; i < 5; i++ {
		sp := tenantSpan(
			"span-limit-"+string(rune('a'+i)),
			"trace-limit-"+string(rune('a'+i)),
			"tenant-"+string(rune('a'+i)),
			float64(i+1)*0.10,
		)
		sp.ProjectID = "proj-limit"
		sp.StartTime = now.Add(-time.Duration(i) * time.Minute)
		sp.EndTime = sp.StartTime.Add(100 * time.Millisecond)
		if err := s.IngestSpans(ctx, []storage.Span{sp}); err != nil {
			t.Fatalf("IngestSpans: %v", err)
		}
	}

	q := storage.UsageQuery{
		ProjectID: "proj-limit",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
	}
	results, err := s.GetTenantLeaderboard(ctx, q, 3)
	if err != nil {
		t.Fatalf("GetTenantLeaderboard: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3 (limit must be enforced)", len(results))
	}
}

// INTEG-8: Empty store returns empty slice, not an error.
func TestGetTenantLeaderboard_EmptyStore(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	q := storage.UsageQuery{
		ProjectID: "proj-empty-store",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
	}
	results, err := s.GetTenantLeaderboard(ctx, q, 10)
	if err != nil {
		t.Fatalf("GetTenantLeaderboard on empty store returned error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

// tenantSpan builds a minimal span with the given tenant_id and cost.
func tenantSpan(spanID, traceID, tenantID string, costUSD float64) storage.Span {
	now := time.Now().Truncate(time.Microsecond)
	const totalTokens = int64(100)
	return storage.Span{
		SpanID:    spanID,
		TraceID:   traceID,
		Name:      "test.llm.call",
		Kind:      storage.SpanKindLLM,
		Status:    storage.SpanStatusOK,
		StartTime: now,
		EndTime:   now.Add(100 * time.Millisecond),
		TenantID:  tenantID,
		GenAI: &storage.GenAIAttributes{
			Model:        "gpt-4o",
			Provider:     "openai",
			InputTokens:  60,
			OutputTokens: 40,
			TotalTokens:  totalTokens,
			CostUSD:      costUSD,
		},
	}
}
