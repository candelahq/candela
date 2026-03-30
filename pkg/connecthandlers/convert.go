package connecthandlers

import (
	"fmt"
	"time"

	typespb "github.com/candelahq/candela/gen/go/candela/types"
	"github.com/candelahq/candela/pkg/storage"
)

// protoToSpan converts a proto Span to a storage Span.
func protoToSpan(ps *typespb.Span) (*storage.Span, error) {
	if ps.SpanId == "" || ps.TraceId == "" {
		return nil, fmt.Errorf("span_id and trace_id are required")
	}

	span := &storage.Span{
		SpanID:        ps.SpanId,
		TraceID:       ps.TraceId,
		ParentSpanID:  ps.ParentSpanId,
		Name:          ps.Name,
		Kind:          storage.SpanKind(ps.Kind),
		Status:        storage.SpanStatus(ps.Status),
		StatusMessage: ps.StatusMessage,
		ProjectID:     ps.ProjectId,
		Environment:   ps.Environment,
		ServiceName:   ps.ServiceName,
		Attributes:    make(map[string]string),
	}

	if ps.StartTime != nil {
		span.StartTime = ps.StartTime.AsTime()
	}
	if ps.EndTime != nil {
		span.EndTime = ps.EndTime.AsTime()
	}
	if ps.Duration != nil {
		span.Duration = ps.Duration.AsDuration()
	} else if !span.StartTime.IsZero() && !span.EndTime.IsZero() {
		span.Duration = span.EndTime.Sub(span.StartTime)
	}

	if ps.GenAi != nil {
		span.GenAI = &storage.GenAIAttributes{
			Model:         ps.GenAi.Model,
			Provider:      ps.GenAi.Provider,
			InputTokens:   ps.GenAi.InputTokens,
			OutputTokens:  ps.GenAi.OutputTokens,
			TotalTokens:   ps.GenAi.TotalTokens,
			CostUSD:       ps.GenAi.CostUsd,
			Temperature:   ps.GenAi.Temperature,
			MaxTokens:     ps.GenAi.MaxTokens,
			InputContent:  ps.GenAi.InputContent,
			OutputContent: ps.GenAi.OutputContent,
		}
		// Auto-compute total tokens if not set.
		if span.GenAI.TotalTokens == 0 {
			span.GenAI.TotalTokens = span.GenAI.InputTokens + span.GenAI.OutputTokens
		}
	}

	for _, attr := range ps.Attributes {
		switch v := attr.Value.(type) {
		case *typespb.Attribute_StringValue:
			span.Attributes[attr.Key] = v.StringValue
		case *typespb.Attribute_IntValue:
			span.Attributes[attr.Key] = fmt.Sprintf("%d", v.IntValue)
		case *typespb.Attribute_DoubleValue:
			span.Attributes[attr.Key] = fmt.Sprintf("%f", v.DoubleValue)
		case *typespb.Attribute_BoolValue:
			span.Attributes[attr.Key] = fmt.Sprintf("%t", v.BoolValue)
		}
	}

	// Set defaults.
	if span.StartTime.IsZero() {
		span.StartTime = time.Now()
	}

	return span, nil
}
