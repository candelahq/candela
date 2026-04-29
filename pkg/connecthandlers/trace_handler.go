// Package connecthandlers implements the ConnectRPC service handlers.
package connecthandlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TraceHandler implements the TraceService ConnectRPC handler.
type TraceHandler struct {
	store storage.SpanReader
	users storage.UserStore // optional, nil in local dev
}

// NewTraceHandler creates a new TraceHandler.
func NewTraceHandler(store storage.SpanReader, users storage.UserStore) *TraceHandler {
	return &TraceHandler{store: store, users: users}
}

func (h *TraceHandler) GetTrace(
	ctx context.Context,
	req *connect.Request[v1.GetTraceRequest],
) (*connect.Response[v1.GetTraceResponse], error) {
	trace, err := h.store.GetTrace(ctx, req.Msg.TraceId)
	if err != nil {
		slog.Error("failed to get trace", "trace_id", req.Msg.TraceId, "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("trace not found"))
	}

	// Authorization gate: non-admin users may only view their own traces.
	// The trace's user_id is set by the proxy at ingestion time and matches
	// the Firestore doc ID (sanitized email) used by scopeUserID.
	if ownerID := scopeUserID(ctx, h.users); ownerID != "" {
		// Determine the trace's owner from its root span (or first span).
		traceOwner := traceUserID(trace)
		if traceOwner != "" && traceOwner != ownerID {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("access denied"))
		}
		// If the trace has no user_id at all (legacy data), allow access
		// but log for backfill visibility.
		if traceOwner == "" {
			caller := auth.FromContext(ctx)
			if caller != nil {
				slog.Debug("legacy trace access: no user_id on trace",
					"trace_id", req.Msg.TraceId,
					"caller", caller.Email)
			}
		}
	}

	return connect.NewResponse(&v1.GetTraceResponse{
		Trace: traceToProto(trace),
	}), nil
}

func (h *TraceHandler) ListTraces(
	ctx context.Context,
	req *connect.Request[v1.ListTracesRequest],
) (*connect.Response[v1.ListTracesResponse], error) {
	msg := req.Msg

	q := storage.TraceQuery{
		ProjectID:   msg.ProjectId,
		Environment: msg.Environment,
		Model:       msg.Model,
		Provider:    msg.Provider,
		Search:      msg.Search,
		OrderBy:     msg.OrderBy,
		Descending:  msg.Descending,
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

	if msg.Pagination != nil {
		q.PageSize = int(msg.Pagination.PageSize)
		q.PageToken = msg.Pagination.PageToken
	}
	if q.PageSize == 0 {
		q.PageSize = 50
	}

	result, err := h.store.QueryTraces(ctx, q)
	if err != nil {
		return nil, internalError("failed to query traces", err)
	}

	var summaries []*typespb.TraceSummary
	for _, t := range result.Traces {
		summaries = append(summaries, traceSummaryToProto(&t))
	}

	return connect.NewResponse(&v1.ListTracesResponse{
		Traces: summaries,
		Pagination: &typespb.PaginationResponse{
			NextPageToken: result.NextPageToken,
			TotalCount:    int32(result.TotalCount),
		},
	}), nil
}

func (h *TraceHandler) SearchSpans(
	ctx context.Context,
	req *connect.Request[v1.SearchSpansRequest],
) (*connect.Response[v1.SearchSpansResponse], error) {
	msg := req.Msg

	q := storage.SpanQuery{
		ProjectID:    msg.ProjectId,
		Kind:         storage.SpanKind(msg.Kind),
		Model:        msg.Model,
		NameContains: msg.NameContains,
		UserID:       scopeUserID(ctx, h.users),
	}

	if msg.TimeRange != nil {
		if msg.TimeRange.Start != nil {
			q.StartTime = msg.TimeRange.Start.AsTime()
		}
		if msg.TimeRange.End != nil {
			q.EndTime = msg.TimeRange.End.AsTime()
		}
	}

	if msg.Pagination != nil {
		q.PageSize = int(msg.Pagination.PageSize)
		q.PageToken = msg.Pagination.PageToken
	}

	result, err := h.store.SearchSpans(ctx, q)
	if err != nil {
		return nil, internalError("failed to search spans", err)
	}

	var spans []*typespb.Span
	for _, s := range result.Spans {
		spans = append(spans, spanToProto(&s))
	}

	return connect.NewResponse(&v1.SearchSpansResponse{
		Spans: spans,
		Pagination: &typespb.PaginationResponse{
			NextPageToken: result.NextPageToken,
			TotalCount:    int32(result.TotalCount),
		},
	}), nil
}

// --- Proto conversion helpers ---

func traceToProto(t *storage.Trace) *typespb.Trace {
	pt := &typespb.Trace{
		TraceId:      t.TraceID,
		StartTime:    timestamppb.New(t.StartTime),
		EndTime:      timestamppb.New(t.EndTime),
		Duration:     durationpb.New(t.Duration),
		ProjectId:    t.ProjectID,
		Environment:  t.Environment,
		SpanCount:    int32(t.SpanCount),
		TotalTokens:  t.TotalTokens,
		TotalCostUsd: t.TotalCostUSD,
		RootSpanName: t.RootSpanName,
	}

	for _, s := range t.Spans {
		pt.Spans = append(pt.Spans, spanToProto(&s))
	}

	return pt
}

func spanToProto(s *storage.Span) *typespb.Span {
	ps := &typespb.Span{
		SpanId:        s.SpanID,
		TraceId:       s.TraceID,
		ParentSpanId:  s.ParentSpanID,
		Name:          s.Name,
		Kind:          typespb.SpanKind(s.Kind),
		Status:        typespb.SpanStatus(s.Status),
		StatusMessage: s.StatusMessage,
		StartTime:     timestamppb.New(s.StartTime),
		EndTime:       timestamppb.New(s.EndTime),
		Duration:      durationpb.New(s.Duration),
		ProjectId:     s.ProjectID,
		Environment:   s.Environment,
		ServiceName:   s.ServiceName,
	}

	if s.GenAI != nil {
		ps.GenAi = &typespb.GenAIAttributes{
			Model:         s.GenAI.Model,
			Provider:      s.GenAI.Provider,
			InputTokens:   s.GenAI.InputTokens,
			OutputTokens:  s.GenAI.OutputTokens,
			TotalTokens:   s.GenAI.TotalTokens,
			CostUsd:       s.GenAI.CostUSD,
			Temperature:   s.GenAI.Temperature,
			MaxTokens:     s.GenAI.MaxTokens,
			InputContent:  s.GenAI.InputContent,
			OutputContent: s.GenAI.OutputContent,
		}
	}

	for k, v := range s.Attributes {
		ps.Attributes = append(ps.Attributes, &typespb.Attribute{
			Key:   k,
			Value: &typespb.Attribute_StringValue{StringValue: v},
		})
	}

	return ps
}

func traceSummaryToProto(t *storage.TraceSummary) *typespb.TraceSummary {
	return &typespb.TraceSummary{
		TraceId:         t.TraceID,
		StartTime:       timestamppb.New(t.StartTime),
		Duration:        durationpb.New(t.Duration),
		RootSpanName:    t.RootSpanName,
		ProjectId:       t.ProjectID,
		Environment:     t.Environment,
		SpanCount:       int32(t.SpanCount),
		LlmCallCount:    int32(t.LLMCallCount),
		TotalTokens:     t.TotalTokens,
		TotalCostUsd:    t.TotalCostUSD,
		Status:          typespb.SpanStatus(t.Status),
		PrimaryModel:    t.PrimaryModel,
		PrimaryProvider: t.PrimaryProvider,
	}
}

// traceUserID extracts the owner user_id from a trace.
// It checks the root span first (parent_span_id == ""), then falls back to the
// first span that has a user_id set. Returns "" if no span has a user_id
// (legacy data ingested before per-user attribution was added).
func traceUserID(t *storage.Trace) string {
	var fallback string
	for _, sp := range t.Spans {
		if sp.UserID == "" {
			continue
		}
		// Prefer the root span's user_id (most authoritative).
		if sp.ParentSpanID == "" {
			return sp.UserID
		}
		// Fallback: the first span in the trace with a user_id.
		if fallback == "" {
			fallback = sp.UserID
		}
	}
	return fallback
}
