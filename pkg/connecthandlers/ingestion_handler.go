package connecthandlers

import (
	"context"
	"errors"
	"fmt"

	connect "connectrpc.com/connect"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

// DefaultMaxIngestBatchSize is the default upper bound on the number of spans
// that can be ingested in a single IngestSpans call to protect against unbounded
// memory allocation and pipeline buffer exhaustion (#644).
const DefaultMaxIngestBatchSize = 10000

// SpanSubmitter is the interface for submitting spans to the processing pipeline.
// This decouples the handler from the concrete processor implementation.
type SpanSubmitter interface {
	SubmitBatch(spans []storage.Span)
}

// IngestionOption configures an IngestionHandler.
type IngestionOption func(*IngestionHandler)

// WithMaxBatchSize overrides the maximum allowed spans per IngestSpans request.
func WithMaxBatchSize(size int) IngestionOption {
	return func(h *IngestionHandler) {
		if size > 0 {
			h.maxBatchSize = size
		}
	}
}

// IngestionHandler implements the IngestionService ConnectRPC/gRPC handler.
type IngestionHandler struct {
	submitter    SpanSubmitter
	maxBatchSize int
}

// NewIngestionHandlerDirect creates a handler that submits spans to an in-process processor.
func NewIngestionHandlerDirect(submitter SpanSubmitter, opts ...IngestionOption) *IngestionHandler {
	h := &IngestionHandler{
		submitter:    submitter,
		maxBatchSize: DefaultMaxIngestBatchSize,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *IngestionHandler) IngestSpans(
	ctx context.Context,
	req *connect.Request[v1.IngestSpansRequest],
) (*connect.Response[v1.IngestSpansResponse], error) {
	if req.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("request message cannot be nil"))
	}

	spanCount := len(req.Msg.Spans)
	if spanCount > h.maxBatchSize {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("batch size %d exceeds maximum allowed batch size %d", spanCount, h.maxBatchSize),
		)
	}

	if spanCount == 0 {
		return connect.NewResponse(&v1.IngestSpansResponse{
			AcceptedCount: 0,
			RejectedCount: 0,
		}), nil
	}

	// Resolve the caller's identity so we can attribute spans that arrive
	// without a user_id (e.g. from candela-local).
	var callerID string
	if caller := auth.FromContext(ctx); caller != nil {
		callerID = caller.EffectiveID()
	}

	spans := make([]storage.Span, 0, spanCount)
	var errMsgs []string

	for _, s := range req.Msg.Spans {
		span, err := protoToSpan(s)
		if err != nil {
			errMsgs = append(errMsgs, err.Error())
			continue
		}
		// Stamp the caller's identity on spans with no user_id.
		// This ensures candela-local spans are attributed to the
		// authenticated user for per-user views (Today page).
		if span.UserID == "" && callerID != "" {
			span.UserID = callerID
		}
		spans = append(spans, *span)
	}

	if len(spans) > 0 {
		h.submitter.SubmitBatch(spans)
	}

	return connect.NewResponse(&v1.IngestSpansResponse{
		AcceptedCount: int32(len(spans)),
		RejectedCount: int32(len(errMsgs)),
		Errors:        errMsgs,
	}), nil
}
