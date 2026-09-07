package connecthandlers_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/connecthandlers"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGetUsageSummary_Success(t *testing.T) {
	store := &fallbackStore{
		summary: &storage.UsageSummary{
			TotalLLMCalls: 50,
			TotalCostUSD:  2.50,
		},
	}
	client := startDashboardServer(t, store)

	resp, err := client.GetUsageSummary(context.Background(), connect.NewRequest(&v1.GetUsageSummaryRequest{ //nolint:staticcheck // testing deprecated endpoint until removal
		ProjectId: "proj1",
	}))
	if err != nil {
		t.Fatalf("GetUsageSummary failed: %v", err)
	}

	if resp.Msg.TotalLlmCalls != 50 {
		t.Errorf("TotalLlmCalls = %d, want 50", resp.Msg.TotalLlmCalls)
	}
	if resp.Msg.TotalCostUsd != 2.50 {
		t.Errorf("TotalCostUsd = %f, want 2.50", resp.Msg.TotalCostUsd)
	}
}

func TestGetModelBreakdown_Success(t *testing.T) {
	store := &fallbackStore{
		models: []storage.ModelUsage{
			{Model: "gpt-4", Provider: "openai", CallCount: 20, CostUSD: 1.50},
		},
	}
	client := startDashboardServer(t, store)

	resp, err := client.GetModelBreakdown(context.Background(), connect.NewRequest(&v1.GetModelBreakdownRequest{ //nolint:staticcheck // testing deprecated endpoint until removal
		ProjectId: "proj1",
	}))
	if err != nil {
		t.Fatalf("GetModelBreakdown failed: %v", err)
	}

	if len(resp.Msg.Models) != 1 {
		t.Fatalf("Models count = %d, want 1", len(resp.Msg.Models))
	}
	if resp.Msg.Models[0].Model != "gpt-4" {
		t.Errorf("Model = %q, want gpt-4", resp.Msg.Models[0].Model)
	}
}

func TestGetLatencyPercentiles_Success(t *testing.T) {
	store := &fallbackStore{}
	client := startDashboardServer(t, store)

	// Fallback store returns nil spans → empty response
	resp, err := client.GetLatencyPercentiles(context.Background(), connect.NewRequest(&v1.GetLatencyPercentilesRequest{}))
	if err != nil {
		t.Fatalf("GetLatencyPercentiles failed: %v", err)
	}
	if resp.Msg == nil {
		t.Fatal("Expected non-nil response")
	}
}

type spanMockStore struct {
	fallbackStore
	spans []storage.Span
}

func (s *spanMockStore) SearchSpans(context.Context, storage.SpanQuery) (*storage.SpanResult, error) {
	return &storage.SpanResult{Spans: s.spans, TotalCount: len(s.spans)}, nil
}

func TestGetLatencyPercentiles_WithSpans(t *testing.T) {
	spans := make([]storage.Span, 100)
	for i := 0; i < 100; i++ {
		spans[i] = storage.Span{
			Duration: time.Duration(i+1) * time.Millisecond,
		}
	}
	store := &spanMockStore{spans: spans}
	client := startDashboardServer(t, store)

	resp, err := client.GetLatencyPercentiles(context.Background(), connect.NewRequest(&v1.GetLatencyPercentilesRequest{}))
	if err != nil {
		t.Fatalf("GetLatencyPercentiles failed: %v", err)
	}
	if resp.Msg.P50Ms != 50.0 {
		t.Errorf("P50Ms = %f, want 50.0", resp.Msg.P50Ms)
	}
	if resp.Msg.P90Ms != 90.0 {
		t.Errorf("P90Ms = %f, want 90.0", resp.Msg.P90Ms)
	}
	if resp.Msg.P95Ms != 95.0 {
		t.Errorf("P95Ms = %f, want 95.0", resp.Msg.P95Ms)
	}
	if resp.Msg.P99Ms != 99.0 {
		t.Errorf("P99Ms = %f, want 99.0", resp.Msg.P99Ms)
	}
}

type queryRecordingStore struct {
	fallbackStore
	lastQuery storage.SpanQuery
}

func (s *queryRecordingStore) SearchSpans(_ context.Context, sq storage.SpanQuery) (*storage.SpanResult, error) {
	s.lastQuery = sq
	return &storage.SpanResult{Spans: nil, TotalCount: 0}, nil
}

func TestGetLatencyPercentiles_EndOnlyTimeRange(t *testing.T) {
	historicalEnd := time.Now().Add(-48 * time.Hour)
	store := &queryRecordingStore{}
	client := startDashboardServer(t, store)

	req := &v1.GetLatencyPercentilesRequest{
		TimeRange: &typespb.TimeRange{
			End: timestamppb.New(historicalEnd),
		},
	}
	_, err := client.GetLatencyPercentiles(context.Background(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("GetLatencyPercentiles failed: %v", err)
	}

	if store.lastQuery.StartTime.After(store.lastQuery.EndTime) {
		t.Fatalf("StartTime (%v) is after EndTime (%v)", store.lastQuery.StartTime, store.lastQuery.EndTime)
	}
	expectedStart := historicalEnd.Add(-24 * time.Hour)
	diff := store.lastQuery.StartTime.Sub(expectedStart)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("StartTime = %v, want ~%v", store.lastQuery.StartTime, expectedStart)
	}
}

func TestGetTeamLeaderboard_DeveloperDenied(t *testing.T) {
	userStore := &mockUserStoreForDashboard{
		users: map[string]*storage.UserRecord{
			"dev@example.com": {
				ID:    "dev@example.com",
				Email: "dev@example.com",
				Role:  storage.RoleDeveloper,
			},
		},
	}
	client := startDashboardServerWithAuth(t, &fallbackStore{}, userStore, &auth.User{ID: "dev-uid", Email: "dev@example.com"})

	_, err := client.GetTeamLeaderboard(context.Background(), connect.NewRequest(&v1.GetTeamLeaderboardRequest{}))
	if err == nil {
		t.Fatal("expected permission denied for developer")
	}
	if connectErr, ok := err.(*connect.Error); ok && connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connectErr.Code())
	}
}

func TestGetTeamLeaderboard_AdminAllowed(t *testing.T) {
	userStore := &mockUserStoreForDashboard{
		users: map[string]*storage.UserRecord{
			"admin@example.com": {
				ID:    "admin@example.com",
				Email: "admin@example.com",
				Role:  storage.RoleAdmin,
			},
		},
	}
	client := startDashboardServerWithAuth(t, &fallbackStore{}, userStore, &auth.User{ID: "admin-uid", Email: "admin@example.com"})

	_, err := client.GetTeamLeaderboard(context.Background(), connect.NewRequest(&v1.GetTeamLeaderboardRequest{}))
	if err != nil {
		t.Fatalf("expected admin to access, got: %v", err)
	}
}

func TestGetTenantLeaderboard_DeveloperDenied(t *testing.T) {
	userStore := &mockUserStoreForDashboard{
		users: map[string]*storage.UserRecord{
			"dev@example.com": {
				ID:    "dev@example.com",
				Email: "dev@example.com",
				Role:  storage.RoleDeveloper,
			},
		},
	}
	handler := connecthandlers.NewDashboardHandler(&fallbackStore{}, userStore)
	ctx := auth.NewContext(context.Background(), &auth.User{ID: "dev-uid", Email: "dev@example.com"})

	_, err := handler.GetTenantLeaderboard(ctx, connect.NewRequest(&v1.GetTenantLeaderboardRequest{}))
	if err == nil {
		t.Fatal("expected permission denied for developer")
	}
	if connectErr, ok := err.(*connect.Error); ok && connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("expected CodePermissionDenied, got %v", connectErr.Code())
	}
}

func TestGetTenantLeaderboard_AdminAllowed(t *testing.T) {
	userStore := &mockUserStoreForDashboard{
		users: map[string]*storage.UserRecord{
			"admin@example.com": {
				ID:    "admin@example.com",
				Email: "admin@example.com",
				Role:  storage.RoleAdmin,
			},
		},
	}
	handler := connecthandlers.NewDashboardHandler(&fallbackStore{}, userStore)
	ctx := auth.NewContext(context.Background(), &auth.User{ID: "admin-uid", Email: "admin@example.com"})

	_, err := handler.GetTenantLeaderboard(ctx, connect.NewRequest(&v1.GetTenantLeaderboardRequest{}))
	if err != nil {
		t.Fatalf("expected admin to access, got: %v", err)
	}
}

type paginatedSpanMockStore struct {
	fallbackStore
	page1 []storage.Span
	page2 []storage.Span
}

func (s *paginatedSpanMockStore) SearchSpans(_ context.Context, sq storage.SpanQuery) (*storage.SpanResult, error) {
	if sq.PageToken == "" {
		return &storage.SpanResult{
			Spans:         s.page1,
			NextPageToken: "token-page-2",
			TotalCount:    len(s.page1) + len(s.page2),
		}, nil
	}
	if sq.PageToken == "token-page-2" {
		return &storage.SpanResult{
			Spans:         s.page2,
			NextPageToken: "",
			TotalCount:    len(s.page1) + len(s.page2),
		}, nil
	}
	return &storage.SpanResult{}, nil
}

func TestGetLatencyPercentiles_PaginatedSpans(t *testing.T) {
	// Page 1 has 1000 spans with latencies 1..1000ms
	// Page 2 has 1000 spans with latencies 1001..2000ms
	// Without pagination: P50 would be 500ms, P90 would be 900ms.
	// With pagination across all 2000 spans: P50 = 1000ms, P90 = 1800ms.
	page1 := make([]storage.Span, 1000)
	for i := 0; i < 1000; i++ {
		page1[i] = storage.Span{Duration: time.Duration(i+1) * time.Millisecond}
	}
	page2 := make([]storage.Span, 1000)
	for i := 0; i < 1000; i++ {
		page2[i] = storage.Span{Duration: time.Duration(1001+i) * time.Millisecond}
	}

	store := &paginatedSpanMockStore{page1: page1, page2: page2}
	client := startDashboardServer(t, store)

	resp, err := client.GetLatencyPercentiles(context.Background(), connect.NewRequest(&v1.GetLatencyPercentilesRequest{}))
	if err != nil {
		t.Fatalf("GetLatencyPercentiles failed: %v", err)
	}
	if resp.Msg.P50Ms != 1000.0 {
		t.Errorf("P50Ms = %f, want 1000.0", resp.Msg.P50Ms)
	}
	if resp.Msg.P90Ms != 1800.0 {
		t.Errorf("P90Ms = %f, want 1800.0", resp.Msg.P90Ms)
	}
}

func TestGetLatencyPercentiles_UserScoping(t *testing.T) {
	store := &queryRecordingStore{}
	userStore := &mockUserStoreForDashboard{
		users: map[string]*storage.UserRecord{
			"dev@example.com": {
				ID:    "dev@example.com",
				Email: "dev@example.com",
				Role:  storage.RoleDeveloper,
			},
		},
	}
	client := startDashboardServerWithAuth(t, store, userStore, &auth.User{ID: "dev-uid", Email: "dev@example.com"})

	_, err := client.GetLatencyPercentiles(context.Background(), connect.NewRequest(&v1.GetLatencyPercentilesRequest{}))
	if err != nil {
		t.Fatalf("GetLatencyPercentiles failed: %v", err)
	}

	if store.lastQuery.UserID != "dev@example.com" {
		t.Errorf("expected UserID to be scoped to 'dev@example.com', got %q", store.lastQuery.UserID)
	}
}
