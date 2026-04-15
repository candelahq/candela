package connecthandlers

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
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
		return nil, connect.NewError(connect.CodeInternal, err)
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
		return nil, connect.NewError(connect.CodeInternal, err)
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
