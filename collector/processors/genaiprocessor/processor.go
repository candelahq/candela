// Package genaiprocessor implements an OpenTelemetry Collector processor that
// enriches spans with GenAI-specific metadata: cost calculations, token
// normalization, and span kind classification.
package genaiprocessor

import (
	"context"

	"github.com/candelahq/candela/pkg/costcalc"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

const (
	typeStr = "genai"

	// OTel GenAI semantic convention attribute keys.
	attrGenAISystem       = "gen_ai.system"
	attrGenAIRequestModel = "gen_ai.request.model"
	attrGenAIUsageInput   = "gen_ai.usage.input_tokens"
	attrGenAIUsageOutput  = "gen_ai.usage.output_tokens"
	attrGenAICostUSD      = "gen_ai.usage.cost_usd" // Candela-enriched
)

// Config holds processor configuration.
type Config struct{}

// Factory creates genai processor instances.
type Factory struct{}

// NewFactory returns a new processor factory.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	return &genaiProcessor{
		logger: set.Logger,
		next:   next,
		calc:   costcalc.New(),
	}, nil
}

type genaiProcessor struct {
	logger *zap.Logger
	next   consumer.Traces
	calc   *costcalc.Calculator
}

func (p *genaiProcessor) Start(_ context.Context, _ component.Host) error {
	return nil
}

func (p *genaiProcessor) Shutdown(_ context.Context) error {
	return nil
}

func (p *genaiProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (p *genaiProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				p.enrichSpan(span)
			}
		}
	}

	return p.next.ConsumeTraces(ctx, td)
}

func (p *genaiProcessor) enrichSpan(span ptrace.Span) {
	attrs := span.Attributes()

	// Check if this is a GenAI span.
	systemVal, hasSystem := attrs.Get(attrGenAISystem)
	modelVal, hasModel := attrs.Get(attrGenAIRequestModel)

	if !hasSystem && !hasModel {
		// Not a GenAI span — skip enrichment.
		// Tool/function spans could be classified by name in the future.
		return
	}

	provider := ""
	if hasSystem {
		provider = systemVal.Str()
	}
	model := ""
	if hasModel {
		model = modelVal.Str()
	}

	// Calculate cost if token counts are present and cost is not already set.
	_, hasCost := attrs.Get(attrGenAICostUSD)
	if !hasCost {
		var inputTokens, outputTokens int64
		if v, ok := attrs.Get(attrGenAIUsageInput); ok {
			inputTokens = v.Int()
		}
		if v, ok := attrs.Get(attrGenAIUsageOutput); ok {
			outputTokens = v.Int()
		}

		if inputTokens > 0 || outputTokens > 0 {
			cost := p.calc.Calculate(provider, model, inputTokens, outputTokens)
			if cost > 0 {
				attrs.PutDouble(attrGenAICostUSD, cost)
				p.logger.Debug("enriched span with cost",
					zap.String("model", model),
					zap.Float64("cost_usd", cost),
				)
			}
		}
	}
}
