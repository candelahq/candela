package connecthandlers

import (
	"context"

	connect "connectrpc.com/connect"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/storage"
)

// SpanSubmitter is the interface for submitting spans to the processing pipeline.
// This decouples the handler from the concrete processor implementation.
type SpanSubmitter interface {
	SubmitBatch(spans []storage.Span)
}

// IngestionHandler implements the IngestionService ConnectRPC/gRPC handler.
type IngestionHandler struct {
	submitter SpanSubmitter
}

// NewIngestionHandlerDirect creates a handler that submits spans to an in-process processor.
func NewIngestionHandlerDirect(submitter SpanSubmitter) *IngestionHandler {
	return &IngestionHandler{submitter: submitter}
}

func (h *IngestionHandler) IngestSpans(
	ctx context.Context,
	req *connect.Request[v1.IngestSpansRequest],
) (*connect.Response[v1.IngestSpansResponse], error) {
	var spans []storage.Span
	var errors []string

	for _, s := range req.Msg.Spans {
		span, err := protoToSpan(s)
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		spans = append(spans, *span)
	}

	if len(spans) > 0 {
		h.submitter.SubmitBatch(spans)
	}

	return connect.NewResponse(&v1.IngestSpansResponse{
		AcceptedCount: int32(len(spans)),
		RejectedCount: int32(len(errors)),
		Errors:        errors,
	}), nil
}
