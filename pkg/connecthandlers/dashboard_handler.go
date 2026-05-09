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
//
// Data source strategy (two-tier):
//   - TODAY's tokens/cost → Firestore budget doc (written synchronously by the
//     proxy on every span, zero BigQuery streaming lag)
//   - Model breakdown → BigQuery always (no Firestore equivalent; ~1s latency ok)
//   - Historical time ranges → BigQuery for everything
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

	// Detect "today" requests: default dashboard view within the last 25h.
	// We use Firestore for budget/grant remaining (real-time), and BigQuery
	// for token counts and cost (authoritative, fast with day partition + clustering).
	//
	// NOTE: We intentionally do NOT use Firestore budget.SpentUSD or
	// budget.TokensUsed as the primary cost/token source because:
	//   - budget.SpentUSD is only the budget-portion of cost (after grants absorb
	//     their share via the DeductSpend waterfall). A dev fully covered by grants
	//     would show $0 spent on the budget doc.
	//   - budget.TokensUsed is proportionally attributed (not total token count).
	// BigQuery always has the accurate total cost and token split per span.
	isToday := q.StartTime.After(time.Now().Add(-25*time.Hour)) &&
		q.EndTime.After(time.Now().Add(-1*time.Hour))

	// ── Concurrent fetch: BigQuery (spend/tokens/models) + Firestore (budget+grants) ──
	type budgetResult struct {
		budget *storage.BudgetRecord
		grants []*storage.GrantRecord
		err    error
	}
	type bqResult struct {
		summary *storage.UsageSummary
		models  []storage.ModelUsage
		err     error
	}

	budgetCh := make(chan budgetResult, 1)
	bqCh := make(chan bqResult, 1)

	// Goroutine 1: Firestore — budget limits + active grants.
	// These are real-time: the proxy writes them synchronously on every span.
	go func() {
		if h.users == nil {
			budgetCh <- budgetResult{}
			return
		}
		budget, err := h.users.GetBudget(ctx, userID)
		if err != nil {
			budgetCh <- budgetResult{err: err}
			return
		}
		grants, grantErr := h.users.ListGrants(ctx, userID, true)
		if grantErr != nil {
			slog.Warn("failed to fetch grants for GetMyUsage", "user_id", userID, "error", grantErr)
		}
		budgetCh <- budgetResult{budget: budget, grants: grants}
	}()

	// Goroutine 2: BigQuery — usage summary + model breakdown.
	// BigQuery is the authoritative source for token counts and cost because:
	// - It has the accurate input/output token split (Firestore only has totals)
	// - budget.SpentUSD only tracks the budget-portion, not grant-funded cost
	// - Day partition + (project_id, user_id) clustering makes today queries fast (~1s)
	go func() {
		models, err := h.store.GetModelBreakdown(ctx, q)
		if err != nil {
			bqCh <- bqResult{err: err}
			return
		}
		summary, err := h.store.GetUsageSummary(ctx, q)
		if err != nil {
			bqCh <- bqResult{err: err}
			return
		}
		bqCh <- bqResult{summary: summary, models: models}
	}()

	// Collect results.
	br := <-budgetCh
	bq := <-bqCh

	if bq.err != nil {
		return nil, internalError("failed to get usage data", bq.err)
	}
	if br.err != nil {
		slog.Warn("failed to fetch budget for GetMyUsage", "user_id", userID, "error", br.err)
		// Non-fatal: continue without budget context.
	}

	// Build response: token/cost figures always from BigQuery.
	resp := &v1.GetMyUsageResponse{}
	if bq.summary != nil {
		resp.TotalCalls = bq.summary.TotalLLMCalls
		resp.TotalInputTokens = bq.summary.TotalInputTokens
		resp.TotalOutputTokens = bq.summary.TotalOutputTokens
		resp.TotalCostUsd = bq.summary.TotalCostUSD
		resp.AvgLatencyMs = bq.summary.AvgLatencyMs
	}

	// "Today" Firestore fallback: if BQ has no data yet (streaming buffer lag,
	// first call within the last ~90s), use Firestore total tokens as a stopgap
	// so the dashboard doesn't show 0 immediately after the first API call.
	if isToday && bq.summary != nil &&
		bq.summary.TotalInputTokens == 0 && bq.summary.TotalOutputTokens == 0 &&
		br.budget != nil && br.budget.TokensUsed > 0 {
		// Firestore TokensUsed = budget-portion only (not grant-funded tokens),
		// so this is a lower bound, not the true total. Show it with the understanding
		// it may be partial for grant users. Better than showing 0.
		resp.TotalInputTokens = br.budget.TokensUsed
		if br.budget.SpentUSD > 0 {
			resp.TotalCostUsd = br.budget.SpentUSD
		}
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

	// Budget context from Firestore — real-time, always accurate.
	// budget.SpentUsd here is the budget-portion only (grants waterfall first).
	// This is correct for "budget remaining" computation: limit - budget_spent.
	if br.budget != nil {
		resp.Budget = &typespb.UserBudget{
			UserId:     br.budget.UserID,
			LimitUsd:   br.budget.LimitUSD,
			SpentUsd:   br.budget.SpentUSD,   // budget-portion spent
			TokensUsed: br.budget.TokensUsed, // budget-portion tokens
		}
	}
	for _, g := range br.grants {
		resp.ActiveGrants = append(resp.ActiveGrants, grantToProto(g))
	}

	// TotalRemainingUsd: budget headroom + sum of active grant remaining.
	// Correctly handles: budget-only, grants-only, and mixed cases.
	//   - If grants cover all cost: budget.SpentUSD=0, so budget headroom = LimitUSD
	//   - If grants exhausted: grants[i].Remaining()=0, budget absorbs remainder
	//   - Mixed: both contribute to remaining
	var totalRemaining float64
	if br.budget != nil {
		if rem := br.budget.LimitUSD - br.budget.SpentUSD; rem > 0 {
			totalRemaining += rem
		}
	}
	for _, g := range br.grants {
		totalRemaining += g.Remaining()
	}
	resp.TotalRemainingUsd = totalRemaining

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
