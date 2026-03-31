// Package connecthandlers implements the ConnectRPC service handlers.
package connecthandlers

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TraceHandler implements the TraceService ConnectRPC handler.
type TraceHandler struct {
	store storage.SpanReader
}

// NewTraceHandler creates a new TraceHandler.
func NewTraceHandler(store storage.SpanReader) *TraceHandler {
	return &TraceHandler{store: store}
}

func (h *TraceHandler) GetTrace(
	ctx context.Context,
	req *connect.Request[v1.GetTraceRequest],
) (*connect.Response[v1.GetTraceResponse], error) {
	trace, err := h.store.GetTrace(ctx, req.Msg.TraceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
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
		return nil, connect.NewError(connect.CodeInternal, err)
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
		ProjectID: msg.ProjectId,
		Kind:      storage.SpanKind(msg.Kind),
		Model:     msg.Model,
		NameContains: msg.NameContains,
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
		return nil, connect.NewError(connect.CodeInternal, err)
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
			Model:          s.GenAI.Model,
			Provider:       s.GenAI.Provider,
			InputTokens:    s.GenAI.InputTokens,
			OutputTokens:   s.GenAI.OutputTokens,
			TotalTokens:    s.GenAI.TotalTokens,
			CostUsd:        s.GenAI.CostUSD,
			Temperature:    s.GenAI.Temperature,
			MaxTokens:      s.GenAI.MaxTokens,
			InputContent:   s.GenAI.InputContent,
			OutputContent:  s.GenAI.OutputContent,
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
