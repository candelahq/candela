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

// ── Mock stores ─────────────────────────────────────────────────────────────

type combinedStore struct {
	storage.SpanReader
	summary *storage.UsageSummary
	models  []storage.ModelUsage
}

func (s *combinedStore) GetUsageWithModelBreakdown(_ context.Context, _ storage.UsageQuery) (*storage.UsageSummary, []storage.ModelUsage, error) {
	return s.summary, s.models, nil
}

type fallbackStore struct {
	summary *storage.UsageSummary
	models  []storage.ModelUsage
}

func (s *fallbackStore) GetTrace(context.Context, string) (*storage.Trace, error) { return nil, nil }
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

type mockSpanReader struct {
	storage.SpanReader
	summary *storage.UsageSummary
	models  []storage.ModelUsage
}

func (m *mockSpanReader) GetUsageSummary(_ context.Context, _ storage.UsageQuery) (*storage.UsageSummary, error) {
	if m.summary == nil {
		return &storage.UsageSummary{}, nil
	}
	return m.summary, nil
}

func (m *mockSpanReader) GetModelBreakdown(_ context.Context, _ storage.UsageQuery) ([]storage.ModelUsage, error) {
	return m.models, nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func startDashboardServer(t *testing.T, store storage.SpanReader) candelav1connect.DashboardServiceClient {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := candelav1connect.NewDashboardServiceHandler(connecthandlers.NewDashboardHandler(store, nil))
	mux.Handle(path, handler)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{ID: "test-user", Email: "test@test.com"})
		mux.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(server.Close)
	return candelav1connect.NewDashboardServiceClient(http.DefaultClient, server.URL)
}

func startDashboardTestServer(t *testing.T, reader storage.SpanReader, users storage.UserStore) candelav1connect.DashboardServiceClient {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := candelav1connect.NewDashboardServiceHandler(connecthandlers.NewDashboardHandler(reader, users))
	mux.Handle(path, handler)
	server := httptest.NewServer(testAuthMiddleware(mux))
	t.Cleanup(server.Close)
	return candelav1connect.NewDashboardServiceClient(http.DefaultClient, server.URL,
		connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("X-Test-Email", "user@test.com")
				return next(ctx, req)
			}
		})),
	)
}

var _ storage.CombinedUsageReader = (*combinedStore)(nil)
var _ = time.Now

// ── GetMyUsage Tests ────────────────────────────────────────────────────────

func TestGetMyUsage_CombinedReaderBranch(t *testing.T) {
	store := &combinedStore{
		summary: &storage.UsageSummary{TotalLLMCalls: 42, TotalInputTokens: 10000, TotalCostUSD: 1.23},
		models: []storage.ModelUsage{
			{Model: "claude-sonnet-4-20250514", Provider: "anthropic", CallCount: 30, CostUSD: 0.90, CacheReadTokens: 5000, CacheCreationTokens: 200},
			{Model: "gpt-4o", Provider: "openai", CallCount: 12, CostUSD: 0.33},
		},
	}
	client := startDashboardServer(t, store)
	resp, err := client.GetMyUsage(context.Background(), connect.NewRequest(&v1.GetMyUsageRequest{})) //nolint:staticcheck
	if err != nil {
		t.Fatalf("GetMyUsage failed: %v", err)
	}
	if resp.Msg.TotalCalls != 42 {
		t.Errorf("TotalCalls = %d, want 42", resp.Msg.TotalCalls)
	}
	if len(resp.Msg.Models) != 2 {
		t.Fatalf("Models count = %d, want 2", len(resp.Msg.Models))
	}
}

func TestGetMyUsage_FallbackBranch(t *testing.T) {
	store := &fallbackStore{
		summary: &storage.UsageSummary{TotalLLMCalls: 10, TotalCostUSD: 0.50},
		models:  []storage.ModelUsage{{Model: "gpt-4o", Provider: "openai", CallCount: 10, CostUSD: 0.50}},
	}
	if _, ok := storage.SpanReader(store).(storage.CombinedUsageReader); ok {
		t.Fatal("fallbackStore should NOT implement CombinedUsageReader")
	}
	client := startDashboardServer(t, store)
	resp, err := client.GetMyUsage(context.Background(), connect.NewRequest(&v1.GetMyUsageRequest{})) //nolint:staticcheck
	if err != nil {
		t.Fatalf("GetMyUsage fallback failed: %v", err)
	}
	if resp.Msg.TotalCalls != 10 {
		t.Errorf("TotalCalls = %d, want 10", resp.Msg.TotalCalls)
	}
}

// ── GetDashboardData Tests ──────────────────────────────────────────────────

func TestGetDashboardData_EmptyBody_Returns200(t *testing.T) {
	client := startDashboardTestServer(t, &mockSpanReader{}, nil)
	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{}))
	if err != nil {
		t.Fatalf("GetDashboardData with empty body should return 200: %v", err)
	}
	if resp.Msg.Summary.TotalLlmCalls != 0 {
		t.Errorf("TotalLlmCalls = %d, want 0", resp.Msg.Summary.TotalLlmCalls)
	}
}

func TestGetDashboardData_WithTimeRange_ReturnsSummary(t *testing.T) {
	reader := &mockSpanReader{summary: &storage.UsageSummary{TotalLLMCalls: 42, TotalCostUSD: 1.23}}
	client := startDashboardTestServer(t, reader, nil)
	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{}))
	if err != nil {
		t.Fatalf("GetDashboardData should succeed: %v", err)
	}
	if resp.Msg.Summary.TotalLlmCalls != 42 {
		t.Errorf("TotalLlmCalls = %d, want 42", resp.Msg.Summary.TotalLlmCalls)
	}
	if resp.Msg.Summary.TotalCostUsd != 1.23 {
		t.Errorf("TotalCostUsd = %f, want 1.23", resp.Msg.Summary.TotalCostUsd)
	}
}

func TestGetDashboardData_CombinedReader_SingleScan(t *testing.T) {
	reader := &combinedStore{
		summary: &storage.UsageSummary{TotalLLMCalls: 100, TotalInputTokens: 50000, TotalCostUSD: 5.00},
		models: []storage.ModelUsage{
			{Model: "gpt-4o", Provider: "openai", CallCount: 60, CostUSD: 3.00},
			{Model: "claude-sonnet", Provider: "anthropic", CallCount: 40, CostUSD: 2.00},
		},
	}
	client := startDashboardTestServer(t, reader, nil)
	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{}))
	if err != nil {
		t.Fatalf("GetDashboardData (combined) should succeed: %v", err)
	}
	if resp.Msg.Summary.TotalLlmCalls != 100 {
		t.Errorf("TotalLlmCalls = %d, want 100", resp.Msg.Summary.TotalLlmCalls)
	}
	if len(resp.Msg.Models) != 2 {
		t.Errorf("Models count = %d, want 2", len(resp.Msg.Models))
	}
}

func TestGetDashboardData_WithBudget(t *testing.T) {
	reader := &mockSpanReader{summary: &storage.UsageSummary{TotalLLMCalls: 5}}
	store := newIntegrationStore()
	store.users["uid-1"] = &storage.UserRecord{ID: "uid-1", Email: "user@test.com"}
	store.budgets["uid-1"] = &storage.BudgetRecord{LimitUSD: 50.0, SpentUSD: 12.50}
	client := startDashboardTestServer(t, reader, store)
	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{IncludeBudget: true}))
	if err != nil {
		t.Fatalf("GetDashboardData with budget should succeed: %v", err)
	}
	if resp.Msg.BudgetContext == nil || resp.Msg.BudgetContext.Budget == nil {
		t.Fatal("BudgetContext.Budget should be populated when include_budget=true")
	}
	if resp.Msg.BudgetContext.Budget.LimitUsd != 50.0 {
		t.Errorf("Budget.LimitUsd = %f, want 50.0", resp.Msg.BudgetContext.Budget.LimitUsd)
	}
}

func TestGetDashboardData_NoBudget_WhenFlagFalse(t *testing.T) {
	reader := &mockSpanReader{summary: &storage.UsageSummary{TotalLLMCalls: 5}}
	store := newIntegrationStore()
	store.users["uid-1"] = &storage.UserRecord{ID: "uid-1", Email: "user@test.com"}
	store.budgets["uid-1"] = &storage.BudgetRecord{LimitUSD: 50.0, SpentUSD: 12.50}
	client := startDashboardTestServer(t, reader, store)
	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{IncludeBudget: false}))
	if err != nil {
		t.Fatalf("GetDashboardData should succeed: %v", err)
	}
	if resp.Msg.BudgetContext != nil {
		t.Error("BudgetContext should be nil when include_budget=false")
	}
}

func TestGetDashboardData_CacheTokenFields(t *testing.T) {
	reader := &mockSpanReader{summary: &storage.UsageSummary{TotalLLMCalls: 15, TotalCacheReadTokens: 50000, TotalCacheCreationTokens: 10000}}
	client := startDashboardTestServer(t, reader, nil)
	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{}))
	if err != nil {
		t.Fatalf("GetDashboardData should succeed: %v", err)
	}
	if resp.Msg.Summary.TotalCacheReadTokens != 50000 {
		t.Errorf("TotalCacheReadTokens = %d, want 50000", resp.Msg.Summary.TotalCacheReadTokens)
	}
	if resp.Msg.Summary.TotalCacheCreationTokens != 10000 {
		t.Errorf("TotalCacheCreationTokens = %d, want 10000", resp.Msg.Summary.TotalCacheCreationTokens)
	}
}

func TestLegacyRPCs_StillWork(t *testing.T) {
	reader := &mockSpanReader{
		summary: &storage.UsageSummary{TotalLLMCalls: 7},
		models:  []storage.ModelUsage{{Model: "test", Provider: "p", CallCount: 7}},
	}
	client := startDashboardTestServer(t, reader, nil)
	sumResp, err := client.GetUsageSummary(context.Background(), connect.NewRequest(&v1.GetUsageSummaryRequest{})) //nolint:staticcheck
	if err != nil {
		t.Fatalf("GetUsageSummary should still work: %v", err)
	}
	if sumResp.Msg.TotalLlmCalls != 7 {
		t.Errorf("TotalLlmCalls = %d, want 7", sumResp.Msg.TotalLlmCalls)
	}
	modResp, err := client.GetModelBreakdown(context.Background(), connect.NewRequest(&v1.GetModelBreakdownRequest{})) //nolint:staticcheck
	if err != nil {
		t.Fatalf("GetModelBreakdown should still work: %v", err)
	}
	if len(modResp.Msg.Models) != 1 {
		t.Errorf("len(Models) = %d, want 1", len(modResp.Msg.Models))
	}
}

func TestGetDashboardData_UnauthenticatedUser_StillReturnsData(t *testing.T) {
	reader := &mockSpanReader{summary: &storage.UsageSummary{TotalLLMCalls: 99}}
	mux := http.NewServeMux()
	path, handler := candelav1connect.NewDashboardServiceHandler(connecthandlers.NewDashboardHandler(reader, nil))
	mux.Handle(path, handler)
	server := httptest.NewServer(testAuthMiddleware(mux))
	t.Cleanup(server.Close)
	client := candelav1connect.NewDashboardServiceClient(http.DefaultClient, server.URL)
	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{}))
	if err != nil {
		t.Fatalf("GetDashboardData without auth should succeed: %v", err)
	}
	if resp.Msg.Summary.TotalLlmCalls != 99 {
		t.Errorf("TotalLlmCalls = %d, want 99", resp.Msg.Summary.TotalLlmCalls)
	}
}

func TestGetDashboardData_AuthUserPropagated(t *testing.T) {
	reader := &mockSpanReader{summary: &storage.UsageSummary{TotalLLMCalls: 5}}
	store := newIntegrationStore()
	connecthandlers.ResetUserIDCacheForTest()
	client := startDashboardTestServer(t, reader, store)
	resp, err := client.GetDashboardData(context.Background(), connect.NewRequest(&v1.GetDashboardDataRequest{IncludeBudget: true}))
	if err != nil {
		t.Fatalf("GetDashboardData should succeed even if user not in store: %v", err)
	}
	if resp.Msg.BudgetContext != nil {
		t.Error("BudgetContext should be nil when user not found in store")
	}
	if resp.Msg.Summary.TotalLlmCalls != 5 {
		t.Errorf("TotalLlmCalls = %d, want 5", resp.Msg.Summary.TotalLlmCalls)
	}
}
