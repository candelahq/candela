package connecthandlers_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/connecthandlers"
	"github.com/candelahq/candela/pkg/storage"
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

	// Currently a stub returning empty
	resp, err := client.GetLatencyPercentiles(context.Background(), connect.NewRequest(&v1.GetLatencyPercentilesRequest{}))
	if err != nil {
		t.Fatalf("GetLatencyPercentiles failed: %v", err)
	}
	if resp.Msg == nil {
		t.Fatal("Expected non-nil response")
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
