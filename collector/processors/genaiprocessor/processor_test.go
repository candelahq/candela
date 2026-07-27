package genaiprocessor

import (
	"context"
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap/zaptest"
)

func TestConsumeTraces(t *testing.T) {
	tests := []struct {
		name         string
		setupTraces  func() ptrace.Traces
		validateSpan func(t *testing.T, span ptrace.Span)
	}{
		{
			name: "processes GenAI spans correctly",
			setupTraces: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()
				span := ss.Spans().AppendEmpty()
				span.Attributes().PutStr(attrGenAISystem, "google")
				span.Attributes().PutStr(attrGenAIRequestModel, "gemini-2.5-pro")
				span.Attributes().PutInt(attrGenAIUsageInput, 100)
				span.Attributes().PutInt(attrGenAIUsageOutput, 50)
				return traces
			},
			validateSpan: func(t *testing.T, span ptrace.Span) {
				costVal, hasCost := span.Attributes().Get(attrGenAICostUSD)
				assert.True(t, hasCost, "expected cost attribute to be set")
				// just check it's > 0
				assert.Greater(t, costVal.Double(), 0.0)
			},
		},
		{
			name: "passes through non-GenAI spans unchanged",
			setupTraces: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()
				span := ss.Spans().AppendEmpty()
				span.Attributes().PutStr("http.method", "GET")
				return traces
			},
			validateSpan: func(t *testing.T, span ptrace.Span) {
				_, hasCost := span.Attributes().Get(attrGenAICostUSD)
				assert.False(t, hasCost, "did not expect cost on non-GenAI span")
			},
		},
		{
			name: "handles missing attributes gracefully",
			setupTraces: func() ptrace.Traces {
				traces := ptrace.NewTraces()
				rs := traces.ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()
				span := ss.Spans().AppendEmpty()
				// Only system set, missing model and tokens
				span.Attributes().PutStr(attrGenAISystem, "openai")
				return traces
			},
			validateSpan: func(t *testing.T, span ptrace.Span) {
				_, hasCost := span.Attributes().Get(attrGenAICostUSD)
				assert.False(t, hasCost, "did not expect cost without tokens")
			},
		},
		{
			name: "handles empty trace data",
			setupTraces: func() ptrace.Traces {
				return ptrace.NewTraces()
			},
			validateSpan: func(t *testing.T, span ptrace.Span) {
				// No spans to validate
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			next := new(consumertest.TracesSink)

			processor := &genaiProcessor{
				logger: logger,
				next:   next,
				calc:   costcalc.New(),
			}

			traces := tt.setupTraces()

			err := processor.ConsumeTraces(context.Background(), traces)
			require.NoError(t, err)

			allTraces := next.AllTraces()
			require.Len(t, allTraces, 1)

			processedTraces := allTraces[0]

			if processedTraces.ResourceSpans().Len() > 0 {
				require.GreaterOrEqual(t, processedTraces.ResourceSpans().At(0).ScopeSpans().Len(), 1, "expected at least one scope span")
				require.GreaterOrEqual(t, processedTraces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().Len(), 1, "expected at least one span")
				span := processedTraces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
				tt.validateSpan(t, span)
			}
		})
	}
}
