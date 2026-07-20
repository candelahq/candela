package connecthandlers

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/storage"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTraceUserID(t *testing.T) {
	tests := []struct {
		name   string
		trace  *storage.Trace
		wantID string
	}{
		{
			name: "root span has user_id",
			trace: &storage.Trace{
				TraceID: "t1",
				Spans: []storage.Span{
					{SpanID: "root", ParentSpanID: "", UserID: "alice@example.com"},
					{SpanID: "child", ParentSpanID: "root", UserID: "alice@example.com"},
				},
			},
			wantID: "alice@example.com",
		},
		{
			name: "root span empty, child has user_id",
			trace: &storage.Trace{
				TraceID: "t2",
				Spans: []storage.Span{
					{SpanID: "root", ParentSpanID: ""},
					{SpanID: "child", ParentSpanID: "root", UserID: "bob@example.com"},
				},
			},
			wantID: "bob@example.com",
		},
		{
			name: "no spans have user_id (legacy)",
			trace: &storage.Trace{
				TraceID: "t3",
				Spans: []storage.Span{
					{SpanID: "root", ParentSpanID: ""},
					{SpanID: "child", ParentSpanID: "root"},
				},
			},
			wantID: "",
		},
		{
			name: "empty trace",
			trace: &storage.Trace{
				TraceID: "t4",
				Spans:   nil,
			},
			wantID: "",
		},
		{
			name: "root has different user than child — root wins",
			trace: &storage.Trace{
				TraceID: "t5",
				Spans: []storage.Span{
					{SpanID: "root", ParentSpanID: "", UserID: "alice@example.com"},
					{SpanID: "child", ParentSpanID: "root", UserID: "bob@example.com"},
				},
			},
			wantID: "alice@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := traceUserID(tt.trace)
			if got != tt.wantID {
				t.Errorf("traceUserID() = %q, want %q", got, tt.wantID)
			}
		})
	}
}

// ── Mock SpanReader ─────────────────────────────────────────────────────────

type mockSpanReader struct {
	spans []storage.Span
}

func (m *mockSpanReader) GetTrace(ctx context.Context, traceID string) (*storage.Trace, error) {
	return nil, storage.ErrNotFound
}

func (m *mockSpanReader) QueryTraces(ctx context.Context, q storage.TraceQuery) (*storage.TraceResult, error) {
	return nil, nil
}

func (m *mockSpanReader) SearchSpans(ctx context.Context, q storage.SpanQuery) (*storage.SpanResult, error) {
	var filtered []storage.Span
	for _, s := range m.spans {
		if !q.StartTime.IsZero() && s.StartTime.Before(q.StartTime) {
			continue
		}
		if !q.EndTime.IsZero() && s.EndTime.After(q.EndTime) {
			continue
		}
		filtered = append(filtered, s)
	}

	total := len(filtered)
	if q.PageSize > 0 && len(filtered) > q.PageSize {
		filtered = filtered[:q.PageSize]
	}

	return &storage.SpanResult{
		Spans:      filtered,
		TotalCount: total,
	}, nil
}

func (m *mockSpanReader) GetUsageSummary(ctx context.Context, q storage.UsageQuery) (*storage.UsageSummary, error) {
	return nil, nil
}

func (m *mockSpanReader) GetModelBreakdown(ctx context.Context, q storage.UsageQuery) ([]storage.ModelUsage, error) {
	return nil, nil
}

func (m *mockSpanReader) GetUserLeaderboard(ctx context.Context, q storage.UsageQuery, limit int) ([]storage.UserUsageSummary, error) {
	return nil, nil
}

func (m *mockSpanReader) GetTenantLeaderboard(ctx context.Context, q storage.UsageQuery, limit int) ([]storage.TenantUsageSummary, error) {
	return nil, nil
}

func (m *mockSpanReader) GetJobLeaderboard(ctx context.Context, q storage.UsageQuery, limit int) ([]storage.JobUsageSummary, error) {
	return nil, nil
}

func (m *mockSpanReader) Ping(ctx context.Context) error {
	return nil
}

func (m *mockSpanReader) Close() error {
	return nil
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestSearchSpans_TimeRangeFiltering(t *testing.T) {
	now := time.Now().UTC()
	store := &mockSpanReader{
		spans: []storage.Span{
			{SpanID: "s1", StartTime: now.Add(-10 * time.Hour), EndTime: now.Add(-9 * time.Hour)},
			{SpanID: "s2", StartTime: now.Add(-5 * time.Hour), EndTime: now.Add(-4 * time.Hour)},
			{SpanID: "s3", StartTime: now.Add(-1 * time.Hour), EndTime: now},
		},
	}
	handler := NewTraceHandler(store, nil)

	req := connect.NewRequest(&v1.SearchSpansRequest{
		ProjectId: "proj1",
		TimeRange: &typespb.TimeRange{
			Start: timestamppb.New(now.Add(-6 * time.Hour)),
			End:   timestamppb.New(now.Add(-2 * time.Hour)),
		},
	})

	res, err := handler.SearchSpans(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchSpans failed: %v", err)
	}

	if len(res.Msg.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(res.Msg.Spans))
	}
	if res.Msg.Spans[0].SpanId != "s2" {
		t.Errorf("expected span s2, got %s", res.Msg.Spans[0].SpanId)
	}
}

func TestSearchSpans_EmptyTimeRange(t *testing.T) {
	now := time.Now().UTC()
	store := &mockSpanReader{
		spans: []storage.Span{
			{SpanID: "s1", StartTime: now.Add(-10 * time.Hour), EndTime: now.Add(-9 * time.Hour)},
		},
	}
	handler := NewTraceHandler(store, nil)

	req := connect.NewRequest(&v1.SearchSpansRequest{
		ProjectId: "proj1",
		TimeRange: &typespb.TimeRange{
			Start: timestamppb.New(now.Add(-2 * time.Hour)),
			End:   timestamppb.New(now.Add(-1 * time.Hour)),
		},
	})

	res, err := handler.SearchSpans(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchSpans failed: %v", err)
	}

	if len(res.Msg.Spans) != 0 {
		t.Fatalf("expected 0 spans, got %d", len(res.Msg.Spans))
	}
}

func TestSearchSpans_PageSizeLimit(t *testing.T) {
	now := time.Now().UTC()
	store := &mockSpanReader{
		spans: []storage.Span{
			{SpanID: "s1", StartTime: now.Add(-1 * time.Hour), EndTime: now},
			{SpanID: "s2", StartTime: now.Add(-1 * time.Hour), EndTime: now},
			{SpanID: "s3", StartTime: now.Add(-1 * time.Hour), EndTime: now},
			{SpanID: "s4", StartTime: now.Add(-1 * time.Hour), EndTime: now},
		},
	}
	handler := NewTraceHandler(store, nil)

	req := connect.NewRequest(&v1.SearchSpansRequest{
		ProjectId: "proj1",
		Pagination: &typespb.PaginationRequest{
			PageSize: 2,
		},
	})

	res, err := handler.SearchSpans(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchSpans failed: %v", err)
	}

	if len(res.Msg.Spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(res.Msg.Spans))
	}
	// The total count should still reflect the total matched spans without page size limit
	if res.Msg.Pagination.TotalCount != 4 {
		t.Errorf("expected total count 4, got %d", res.Msg.Pagination.TotalCount)
	}
}
