package connecthandlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	connect "connectrpc.com/connect"
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

// GetMyUsage returns the calling user's personal usage summary (BigQuery).
//
// This is a pure BigQuery handler — ~800ms, authoritative token/cost split.
// For real-time budget progress bars and grant remaining, call
// UserService.GetMyBudget (~80ms, Firestore-only) in parallel.
//
// Client rendering pattern:
//
//	UserService.GetMyBudget()     → renders budget bar + grant bars (~80ms)
//	DashboardService.GetMyUsage() → renders token counts + model table (~800ms)
func (h *DashboardHandler) GetMyUsage(
	ctx context.Context,
	req *connect.Request[v1.GetMyUsageRequest],
) (*connect.Response[v1.GetMyUsageResponse], error) {
	authUser := auth.FromContext(ctx)
	if authUser == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Resolve the user's store ID via the in-process cache (60s TTL).
	// The proxy writes this same ID into span.user_id, so BQ queries match.
	var userID string
	if h.users != nil {
		id, err := resolveUserID(ctx, h.users, authUser.Email)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
			}
			return nil, internalError("failed to look up user", err)
		}
		userID = id
	} else {
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

	// ── BigQuery: usage summary + model breakdown (truly concurrent) ─────────
	// Both queries hit BigQuery independently (~800ms each). Run them in
	// separate goroutines so total wait is max(summary, breakdown) ≈ 800ms
	// instead of sum ≈ 1600ms.
	type summaryResult struct {
		v   *storage.UsageSummary
		err error
	}
	type modelsResult struct {
		v   []storage.ModelUsage
		err error
	}
	summaryCh := make(chan summaryResult, 1)
	modelsCh := make(chan modelsResult, 1)
	go func() {
		v, err := h.store.GetUsageSummary(ctx, q)
		summaryCh <- summaryResult{v, err}
	}()
	go func() {
		v, err := h.store.GetModelBreakdown(ctx, q)
		modelsCh <- modelsResult{v, err}
	}()
	sr := <-summaryCh
	mr := <-modelsCh
	if sr.err != nil {
		return nil, internalError("failed to get usage summary", sr.err)
	}
	if mr.err != nil {
		return nil, internalError("failed to get model breakdown", mr.err)
	}
	type bqResult struct {
		summary *storage.UsageSummary
		models  []storage.ModelUsage
	}
	bq := bqResult{summary: sr.v, models: mr.v}

	// Build response — token/cost figures are always from BigQuery.
	// Budget progress bars and grant remaining: call UserService.GetMyBudget.
	resp := &v1.GetMyUsageResponse{}
	if bq.summary != nil {
		resp.TotalCalls = bq.summary.TotalLLMCalls
		resp.TotalInputTokens = bq.summary.TotalInputTokens
		resp.TotalOutputTokens = bq.summary.TotalOutputTokens
		resp.TotalCostUsd = bq.summary.TotalCostUSD
		resp.AvgLatencyMs = bq.summary.AvgLatencyMs
	}
	for _, m := range bq.models {
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
			if rec.DisplayName != nil {
				pu.DisplayName = *rec.DisplayName
			}
		}

		pbUsers = append(pbUsers, pu)
	}

	return connect.NewResponse(&v1.GetTeamLeaderboardResponse{
		Users: pbUsers,
	}), nil
}

// GetTenantLeaderboard returns per-tenant LLM cost aggregations ranked by spend (admin only).
// Tenant identity is captured from X-Candela-Tenant-Id header or W3C Baggage (candela.tenant_id).
func (h *DashboardHandler) GetTenantLeaderboard(
	ctx context.Context,
	req *connect.Request[v1.GetTenantLeaderboardRequest],
) (*connect.Response[v1.GetTenantLeaderboardResponse], error) {
	// Admin-only: non-admin users (non-empty scopeUserID) are denied.
	if uid := scopeUserID(ctx, h.users); uid != "" {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("tenant leaderboard is admin-only"))
	}

	msg := req.Msg
	q := storage.UsageQuery{ProjectID: msg.GetProjectId()}
	if msg.GetTimeRange() != nil {
		if msg.GetTimeRange().Start != nil {
			q.StartTime = msg.GetTimeRange().Start.AsTime()
		}
		if msg.GetTimeRange().End != nil {
			q.EndTime = msg.GetTimeRange().End.AsTime()
		}
	}
	if q.StartTime.IsZero() {
		q.StartTime = time.Now().Add(-30 * 24 * time.Hour) // default: last 30 days
	}
	if q.EndTime.IsZero() {
		q.EndTime = time.Now()
	}

	limit := int(msg.GetLimit())
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	tenants, err := h.store.GetTenantLeaderboard(ctx, q, limit)
	if err != nil {
		return nil, internalError("failed to get tenant leaderboard", err)
	}

	var pbTenants []*v1.TenantUsage
	for _, t := range tenants {
		pbTenants = append(pbTenants, &v1.TenantUsage{
			TenantId:     t.TenantID,
			CallCount:    t.CallCount,
			TotalTokens:  t.TotalTokens,
			CostUsd:      t.CostUSD,
			AvgLatencyMs: t.AvgLatencyMs,
			TopModel:     t.TopModel,
		})
	}

	slog.Info("tenant leaderboard fetched",
		"project_id", q.ProjectID,
		"tenant_count", len(tenants),
		"limit", limit,
	)

	return connect.NewResponse(&v1.GetTenantLeaderboardResponse{
		Tenants: pbTenants,
	}), nil
}
