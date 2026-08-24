package connecthandlers

import (
	"context"
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
