package connecthandlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

// DashboardHandler implements the DashboardService ConnectRPC handler.
type DashboardHandler struct {
	store storage.SpanReader
	users storage.UserStore // optional, nil in local dev
}

// NewDashboardHandler creates a new DashboardHandler.
func NewDashboardHandler(store storage.SpanReader, users storage.UserStore) *DashboardHandler {
	return &DashboardHandler{store: store, users: users}
}

func (h *DashboardHandler) GetUsageSummary(
	ctx context.Context,
	req *connect.Request[v1.GetUsageSummaryRequest],
) (*connect.Response[v1.GetUsageSummaryResponse], error) {
	msg := req.Msg

	q := storage.UsageQuery{
		ProjectID:   msg.ProjectId,
		Environment: msg.Environment,
		UserID:      scopeUserID(ctx, h.users),
	}
	if msg.TimeRange != nil {
		if msg.TimeRange.Start != nil {
			q.StartTime = msg.TimeRange.Start.AsTime()
		}
		if msg.TimeRange.End != nil {
			q.EndTime = msg.TimeRange.End.AsTime()
		}
	}
	if q.StartTime.IsZero() {
		q.StartTime = time.Now().Add(-24 * time.Hour)
	}
	if q.EndTime.IsZero() {
		q.EndTime = time.Now()
	}

	summary, err := h.store.GetUsageSummary(ctx, q)
	if err != nil {
		return nil, internalError("failed to get usage summary", err)
	}

	return connect.NewResponse(&v1.GetUsageSummaryResponse{
		TotalTraces:       summary.TotalTraces,
		TotalSpans:        summary.TotalSpans,
		TotalLlmCalls:     summary.TotalLLMCalls,
		TotalInputTokens:  summary.TotalInputTokens,
		TotalOutputTokens: summary.TotalOutputTokens,
		TotalCostUsd:      summary.TotalCostUSD,
		AvgLatencyMs:      summary.AvgLatencyMs,
		ErrorRate:         summary.ErrorRate,
	}), nil
}

func (h *DashboardHandler) GetModelBreakdown(
	ctx context.Context,
	req *connect.Request[v1.GetModelBreakdownRequest],
) (*connect.Response[v1.GetModelBreakdownResponse], error) {
	msg := req.Msg

	q := storage.UsageQuery{ProjectID: msg.ProjectId, UserID: scopeUserID(ctx, h.users)}
	if msg.TimeRange != nil {
		if msg.TimeRange.Start != nil {
			q.StartTime = msg.TimeRange.Start.AsTime()
		}
		if msg.TimeRange.End != nil {
			q.EndTime = msg.TimeRange.End.AsTime()
		}
	}

	models, err := h.store.GetModelBreakdown(ctx, q)
	if err != nil {
		return nil, internalError("failed to get model breakdown", err)
	}

	var pbModels []*v1.ModelUsage
	for _, m := range models {
		pbModels = append(pbModels, &v1.ModelUsage{
			Model:        m.Model,
			Provider:     m.Provider,
			CallCount:    m.CallCount,
			InputTokens:  m.InputTokens,
			OutputTokens: m.OutputTokens,
			CostUsd:      m.CostUSD,
			AvgLatencyMs: m.AvgLatencyMs,
		})
	}

	return connect.NewResponse(&v1.GetModelBreakdownResponse{
		Models: pbModels,
	}), nil
}

func (h *DashboardHandler) GetLatencyPercentiles(
	ctx context.Context,
	req *connect.Request[v1.GetLatencyPercentilesRequest],
) (*connect.Response[v1.GetLatencyPercentilesResponse], error) {
	// TODO: implement ClickHouse quantile queries
	return connect.NewResponse(&v1.GetLatencyPercentilesResponse{}), nil
}

// GetMyUsage returns the calling user's personal usage summary + budget context.
func (h *DashboardHandler) GetMyUsage(
	ctx context.Context,
	req *connect.Request[v1.GetMyUsageRequest],
) (*connect.Response[v1.GetMyUsageResponse], error) {
	authUser := auth.FromContext(ctx)
	if authUser == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Resolve the user's store ID (Firestore doc ID = sanitized email).
	// The proxy also writes this same ID into span.user_id, so BQ queries match.
	var userID string
	if h.users != nil {
		user, err := h.users.GetUserByEmail(ctx, authUser.Email)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
			}
			return nil, internalError("failed to look up user", err)
		}
		userID = user.ID
	} else {
		// No Firestore — use EffectiveID to match the proxy's user_id convention.
		userID = authUser.EffectiveID()
	}

	msg := req.Msg
	q := storage.UsageQuery{
		ProjectID: msg.ProjectId,
		UserID:    userID,
	}
	if msg.TimeRange != nil {
		if msg.TimeRange.Start != nil {
			q.StartTime = msg.TimeRange.Start.AsTime()
		}
		if msg.TimeRange.End != nil {
			q.EndTime = msg.TimeRange.End.AsTime()
		}
	}
	if q.StartTime.IsZero() {
		q.StartTime = time.Now().Add(-24 * time.Hour)
	}
	if q.EndTime.IsZero() {
		q.EndTime = time.Now()
	}

	summary, err := h.store.GetUsageSummary(ctx, q)
	if err != nil {
		return nil, internalError("failed to get usage summary", err)
	}

	models, err := h.store.GetModelBreakdown(ctx, q)
	if err != nil {
		return nil, internalError("failed to get model breakdown", err)
	}

	resp := &v1.GetMyUsageResponse{
		TotalCalls:        summary.TotalLLMCalls,
		TotalInputTokens:  summary.TotalInputTokens,
		TotalOutputTokens: summary.TotalOutputTokens,
		TotalCostUsd:      summary.TotalCostUSD,
		AvgLatencyMs:      summary.AvgLatencyMs,
	}

	for _, m := range models {
		resp.Models = append(resp.Models, &v1.ModelUsage{
			Model:        m.Model,
			Provider:     m.Provider,
			CallCount:    m.CallCount,
			InputTokens:  m.InputTokens,
			OutputTokens: m.OutputTokens,
			CostUsd:      m.CostUSD,
			AvgLatencyMs: m.AvgLatencyMs,
		})
	}

	// Attach budget context if Firestore is available.
	if h.users != nil {
		// Fetch budget and grants in parallel-safe single-pass:
		// one GetBudget + one ListGrants (instead of the previous
		// GetBudget + CheckBudget + ListGrants which called ListGrants twice).
		budget, err := h.users.GetBudget(ctx, userID)
		if err == nil && budget != nil {
			resp.Budget = &typespb.UserBudget{
				UserId:     budget.UserID,
				LimitUsd:   budget.LimitUSD,
				SpentUsd:   budget.SpentUSD,
				TokensUsed: budget.TokensUsed,
			}
		}

		// Fetch active grants (single call — also used to compute remaining).
		grants, grantErr := h.users.ListGrants(ctx, userID, true)
		if grantErr != nil {
			slog.Warn("failed to fetch grants for GetMyUsage", "user_id", userID, "error", grantErr)
		} else {
			for _, g := range grants {
				resp.ActiveGrants = append(resp.ActiveGrants, grantToProto(g))
			}
		}

		// Compute total remaining from budget + grants (avoids calling CheckBudget
		// which internally calls ListGrants again).
		var totalRemaining float64
		if budget != nil {
			budgetRemaining := budget.LimitUSD - budget.SpentUSD
			if budgetRemaining < 0 {
				budgetRemaining = 0
			}
			totalRemaining += budgetRemaining
		}
		for _, g := range grants {
			totalRemaining += g.Remaining()
		}
		resp.TotalRemainingUsd = totalRemaining
	}

	return connect.NewResponse(resp), nil
}

// GetTeamLeaderboard returns per-user usage ranked by cost (admin only).
func (h *DashboardHandler) GetTeamLeaderboard(
	ctx context.Context,
	req *connect.Request[v1.GetTeamLeaderboardRequest],
) (*connect.Response[v1.GetTeamLeaderboardResponse], error) {
	// Admin-only guard: scopeUserID returns "" for admins.
	if uid := scopeUserID(ctx, h.users); uid != "" {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("team leaderboard is admin-only"))
	}

	msg := req.Msg
	q := storage.UsageQuery{ProjectID: msg.ProjectId}
	if msg.TimeRange != nil {
		if msg.TimeRange.Start != nil {
			q.StartTime = msg.TimeRange.Start.AsTime()
		}
		if msg.TimeRange.End != nil {
			q.EndTime = msg.TimeRange.End.AsTime()
		}
	}
	if q.StartTime.IsZero() {
		q.StartTime = time.Now().Add(-30 * 24 * time.Hour) // default: last 30 days
	}
	if q.EndTime.IsZero() {
		q.EndTime = time.Now()
	}

	limit := int(msg.Limit)
	if limit <= 0 {
		limit = 20
	}

	users, err := h.store.GetUserLeaderboard(ctx, q, limit)
	if err != nil {
		return nil, internalError("failed to get user leaderboard", err)
	}

	// Batch enrichment: collect IDs to avoid N+1 query pattern.
	var userIDs []string
	for _, u := range users {
		userIDs = append(userIDs, u.UserID)
	}

	userMap := make(map[string]*storage.UserRecord)
	if h.users != nil && len(userIDs) > 0 {
		userMap, err = h.users.GetUsers(ctx, userIDs)
		if err != nil {
			slog.Warn("batch user enrichment failed", "error", err)
			// Continue with partial data rather than failing the whole request.
		}
	}

	var pbUsers []*v1.UserUsage
	for _, u := range users {
		pu := &v1.UserUsage{
			UserId:       u.UserID,
			CallCount:    u.CallCount,
			TotalTokens:  u.TotalTokens,
			CostUsd:      u.CostUSD,
			AvgLatencyMs: u.AvgLatencyMs,
			TopModel:     u.TopModel,
		}

		// Enrich from batch-fetched map.
		if rec, ok := userMap[u.UserID]; ok {
			pu.Email = rec.Email
			pu.DisplayName = rec.DisplayName
		}

		pbUsers = append(pbUsers, pu)
	}

	return connect.NewResponse(&v1.GetTeamLeaderboardResponse{
		Users: pbUsers,
	}), nil
}
