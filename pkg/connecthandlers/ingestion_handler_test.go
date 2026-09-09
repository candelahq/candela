package connecthandlers

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

type mockSpanSubmitter struct {
	spans []storage.Span
}

func (m *mockSpanSubmitter) SubmitBatch(spans []storage.Span) {
	m.spans = append(m.spans, spans...)
}

func TestIngestionHandler_IngestSpans(t *testing.T) {
	submitter := &mockSpanSubmitter{}
	handler := NewIngestionHandlerDirect(submitter)

	req := connect.NewRequest(&v1.IngestSpansRequest{
		Spans: []*typespb.Span{
			{
				SpanId:  "span1",
				TraceId: "trace1",
				Name:    "test-span",
			},
			{
				// Invalid span (no IDs)
				Name: "invalid-span",
			},
		},
	})

	ctx := context.Background()
	ctx = auth.NewContext(ctx, &auth.User{Email: "dev@example.com", ID: "dev-id"})

	resp, err := handler.IngestSpans(ctx, req)
	if err != nil {
		t.Fatalf("IngestSpans() error = %v", err)
	}

	if resp.Msg.AcceptedCount != 1 {
		t.Errorf("AcceptedCount = %v, want 1", resp.Msg.AcceptedCount)
	}
	if resp.Msg.RejectedCount != 1 {
		t.Errorf("RejectedCount = %v, want 1", resp.Msg.RejectedCount)
	}
	if len(submitter.spans) != 1 {
		t.Fatalf("SubmitBatch called with %v spans, want 1", len(submitter.spans))
	}
	if submitter.spans[0].UserID != "dev@example.com" {
		t.Errorf("span.UserID = %v, want 'dev@example.com'", submitter.spans[0].UserID)
	}
}

func TestIngestionHandler_IngestSpans_MaxBatchSizeExceeded(t *testing.T) {
	submitter := &mockSpanSubmitter{}
	// Default limit is 10,000; configure with small limit of 3 for testing
	handler := NewIngestionHandlerDirect(submitter, WithMaxBatchSize(3))

	spans := make([]*typespb.Span, 4)
	for i := range spans {
		spans[i] = &typespb.Span{
			SpanId:  "span",
			TraceId: "trace",
			Name:    "test",
		}
	}

	req := connect.NewRequest(&v1.IngestSpansRequest{
		Spans: spans,
	})

	_, err := handler.IngestSpans(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for batch exceeding maxBatchSize, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected connect.Error, got %T: %v", err, err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("got code %v, want %v", connectErr.Code(), connect.CodeInvalidArgument)
	}
	if len(submitter.spans) != 0 {
		t.Errorf("expected 0 spans submitted on rejection, got %d", len(submitter.spans))
	}
}

func TestIngestionHandler_IngestSpans_NilMessage(t *testing.T) {
	submitter := &mockSpanSubmitter{}
	handler := NewIngestionHandlerDirect(submitter)

	req := &connect.Request[v1.IngestSpansRequest]{} // req.Msg is nil

	_, err := handler.IngestSpans(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for nil message, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected connect.Error, got %T: %v", err, err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("got code %v, want %v", connectErr.Code(), connect.CodeInvalidArgument)
	}
}

func TestIngestionHandler_IngestSpans_EmptyBatch(t *testing.T) {
	submitter := &mockSpanSubmitter{}
	handler := NewIngestionHandlerDirect(submitter)

	req := connect.NewRequest(&v1.IngestSpansRequest{
		Spans: []*typespb.Span{},
	})

	resp, err := handler.IngestSpans(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error for empty batch: %v", err)
	}

	if resp.Msg.AcceptedCount != 0 {
		t.Errorf("AcceptedCount = %d, want 0", resp.Msg.AcceptedCount)
	}
	if resp.Msg.RejectedCount != 0 {
		t.Errorf("RejectedCount = %d, want 0", resp.Msg.RejectedCount)
	}
	if len(submitter.spans) != 0 {
		t.Errorf("expected 0 spans submitted, got %d", len(submitter.spans))
	}
}
