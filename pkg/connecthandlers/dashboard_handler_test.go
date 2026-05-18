package connecthandlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/gen/go/candela/v1/candelav1connect"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/connecthandlers"
	"github.com/candelahq/candela/pkg/storage"
)

// ── Mock store that implements CombinedUsageReader ───────────────────────────

// combinedStore embeds a minimal SpanReader and adds CombinedUsageReader.
type combinedStore struct {
	storage.SpanReader // nil — only GetUsageWithModelBreakdown is called
	summary            *storage.UsageSummary
	models             []storage.ModelUsage
}

func (s *combinedStore) GetUsageWithModelBreakdown(_ context.Context, _ storage.UsageQuery) (*storage.UsageSummary, []storage.ModelUsage, error) {
	return s.summary, s.models, nil
}

// ── Mock store that does NOT implement CombinedUsageReader ───────────────────

type fallbackStore struct {
	summary *storage.UsageSummary
	models  []storage.ModelUsage
}

func (s *fallbackStore) GetTrace(context.Context, string) (*storage.Trace, error) {
	return nil, nil
}
func (s *fallbackStore) QueryTraces(context.Context, storage.TraceQuery) (*storage.TraceResult, error) {
	return nil, nil
}
func (s *fallbackStore) SearchSpans(context.Context, storage.SpanQuery) (*storage.SpanResult, error) {
	return nil, nil
}
func (s *fallbackStore) GetUsageSummary(_ context.Context, _ storage.UsageQuery) (*storage.UsageSummary, error) {
	return s.summary, nil
}
func (s *fallbackStore) GetModelBreakdown(_ context.Context, _ storage.UsageQuery) ([]storage.ModelUsage, error) {
	return s.models, nil
}
func (s *fallbackStore) GetUserLeaderboard(context.Context, storage.UsageQuery, int) ([]storage.UserUsageSummary, error) {
	return nil, nil
}
func (s *fallbackStore) GetTenantLeaderboard(context.Context, storage.UsageQuery, int) ([]storage.TenantUsageSummary, error) {
	return nil, nil
}
func (s *fallbackStore) GetJobLeaderboard(context.Context, storage.UsageQuery, int) ([]storage.JobUsageSummary, error) {
	return nil, nil
}
func (s *fallbackStore) Ping(context.Context) error { return nil }
func (s *fallbackStore) Close() error               { return nil }

// ── Test helpers ────────────────────────────────────────────────────────────

func startDashboardServer(t *testing.T, store storage.SpanReader) candelav1connect.DashboardServiceClient {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := candelav1connect.NewDashboardServiceHandler(
		connecthandlers.NewDashboardHandler(store, nil),
	)
	mux.Handle(path, handler)

	// Inject an authenticated user into every request so GetMyUsage passes auth.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{ID: "test-user", Email: "test@test.com"})
		mux.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(server.Close)

	return candelav1connect.NewDashboardServiceClient(http.DefaultClient, server.URL)
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestGetMyUsage_CombinedReaderBranch(t *testing.T) {
	// When the store implements CombinedUsageReader, the handler should use it
	// instead of the two-concurrent-query fallback.
	store := &combinedStore{
		summary: &storage.UsageSummary{
			TotalLLMCalls:    42,
			TotalInputTokens: 10000,
			TotalCostUSD:     1.23,
		},
		models: []storage.ModelUsage{
			{Model: "claude-sonnet-4-20250514", Provider: "anthropic", CallCount: 30, CostUSD: 0.90,
				CacheReadTokens: 5000, CacheCreationTokens: 200},
			{Model: "gpt-4o", Provider: "openai", CallCount: 12, CostUSD: 0.33},
		},
	}

	client := startDashboardServer(t, store)

	resp, err := client.GetMyUsage(context.Background(), connect.NewRequest(&v1.GetMyUsageRequest{})) //nolint:staticcheck // testing deprecated method
	if err != nil {
		t.Fatalf("GetMyUsage failed: %v", err)
	}

	if resp.Msg.TotalCalls != 42 {
		t.Errorf("TotalCalls = %d, want 42", resp.Msg.TotalCalls)
	}
	if resp.Msg.TotalCostUsd != 1.23 {
		t.Errorf("TotalCostUsd = %f, want 1.23", resp.Msg.TotalCostUsd)
	}
	if len(resp.Msg.Models) != 2 {
		t.Fatalf("Models count = %d, want 2", len(resp.Msg.Models))
	}
	if resp.Msg.Models[0].Model != "claude-sonnet-4-20250514" {
		t.Errorf("Models[0].Model = %q, want claude-sonnet-4-20250514", resp.Msg.Models[0].Model)
	}
}

func TestGetMyUsage_FallbackBranch(t *testing.T) {
	// When the store does NOT implement CombinedUsageReader, the handler should
	// fall back to concurrent GetUsageSummary + GetModelBreakdown.
	store := &fallbackStore{
		summary: &storage.UsageSummary{
			TotalLLMCalls: 10,
			TotalCostUSD:  0.50,
		},
		models: []storage.ModelUsage{
			{Model: "gpt-4o", Provider: "openai", CallCount: 10, CostUSD: 0.50},
		},
	}

	// Verify the store does NOT satisfy CombinedUsageReader.
	if _, ok := storage.SpanReader(store).(storage.CombinedUsageReader); ok {
		t.Fatal("fallbackStore should NOT implement CombinedUsageReader")
	}

	client := startDashboardServer(t, store)

	resp, err := client.GetMyUsage(context.Background(), connect.NewRequest(&v1.GetMyUsageRequest{})) //nolint:staticcheck // testing deprecated method
	if err != nil {
		t.Fatalf("GetMyUsage fallback failed: %v", err)
	}
	if resp.Msg.TotalCalls != 10 {
		t.Errorf("TotalCalls = %d, want 10", resp.Msg.TotalCalls)
	}
	if len(resp.Msg.Models) != 1 {
		t.Errorf("Models count = %d, want 1", len(resp.Msg.Models))
	}
}

// Compile-time assertion: combinedStore satisfies CombinedUsageReader.
var _ storage.CombinedUsageReader = (*combinedStore)(nil)

// Compile-time assertion: fallbackStore does NOT satisfy CombinedUsageReader.
// (Verified at runtime in TestGetMyUsage_FallbackBranch above — can't
// assert "does not implement" at compile time.)

// Suppress unused-import lint for time (used by mock stubs if needed).
var _ = time.Now

// ── GetDashboardData tests ──────────────────────────────────────────────────

func TestGetDashboardData_CombinedReader(t *testing.T) {
	store := &combinedStore{
		summary: &storage.UsageSummary{
			TotalTraces:              100,
			TotalLLMCalls:            42,
			TotalInputTokens:         10000,
			TotalOutputTokens:        5000,
			TotalCostUSD:             1.23,
			TotalCacheReadTokens:     8000,
			TotalCacheCreationTokens: 200,
		},
		models: []storage.ModelUsage{
			{Model: "claude-sonnet-4-20250514", Provider: "anthropic", CallCount: 30, CostUSD: 0.90,
				CacheReadTokens: 5000, CacheCreationTokens: 200},
			{Model: "gpt-4o", Provider: "openai", CallCount: 12, CostUSD: 0.33},
		},
	}

	client := startDashboardServer(t, store)

	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{}))
	if err != nil {
		t.Fatalf("GetDashboardData failed: %v", err)
	}

	// Verify summary is populated.
	if resp.Msg.Summary == nil {
		t.Fatal("Summary is nil")
	}
	if resp.Msg.Summary.TotalLlmCalls != 42 {
		t.Errorf("TotalLlmCalls = %d, want 42", resp.Msg.Summary.TotalLlmCalls)
	}
	if resp.Msg.Summary.TotalCacheReadTokens != 8000 {
		t.Errorf("TotalCacheReadTokens = %d, want 8000", resp.Msg.Summary.TotalCacheReadTokens)
	}
	if resp.Msg.Summary.TotalCacheCreationTokens != 200 {
		t.Errorf("TotalCacheCreationTokens = %d, want 200", resp.Msg.Summary.TotalCacheCreationTokens)
	}

	// Verify models.
	if len(resp.Msg.Models) != 2 {
		t.Fatalf("Models count = %d, want 2", len(resp.Msg.Models))
	}
	if resp.Msg.Models[0].CacheReadTokens != 5000 {
		t.Errorf("Models[0].CacheReadTokens = %d, want 5000", resp.Msg.Models[0].CacheReadTokens)
	}

	// Budget context should be nil (no include_budget).
	if resp.Msg.BudgetContext != nil {
		t.Error("BudgetContext should be nil when include_budget is false")
	}
}

func TestGetDashboardData_FallbackReader(t *testing.T) {
	store := &fallbackStore{
		summary: &storage.UsageSummary{
			TotalLLMCalls: 10,
			TotalCostUSD:  0.50,
		},
		models: []storage.ModelUsage{
			{Model: "gpt-4o", Provider: "openai", CallCount: 10, CostUSD: 0.50},
		},
	}

	client := startDashboardServer(t, store)

	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{}))
	if err != nil {
		t.Fatalf("GetDashboardData fallback failed: %v", err)
	}
	if resp.Msg.Summary == nil {
		t.Fatal("Summary is nil")
	}
	if resp.Msg.Summary.TotalLlmCalls != 10 {
		t.Errorf("TotalLlmCalls = %d, want 10", resp.Msg.Summary.TotalLlmCalls)
	}
	if len(resp.Msg.Models) != 1 {
		t.Errorf("Models count = %d, want 1", len(resp.Msg.Models))
	}
}
