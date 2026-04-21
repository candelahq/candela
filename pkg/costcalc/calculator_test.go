package costcalc

import (
	"math"
	"testing"
)

func TestCalculate(t *testing.T) {
	calc := New()

	tests := []struct {
		name         string
		provider     string
		model        string
		inputTokens  int64
		outputTokens int64
		wantMin      float64
		wantMax      float64
	}{
		{
			name:         "GPT-4o basic usage",
			provider:     "openai",
			model:        "gpt-4o",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.007,
			wantMax:      0.008,
		},
		{
			name:         "Gemini 2.0 Flash",
			provider:     "google",
			model:        "gemini-2.0-flash",
			inputTokens:  10000,
			outputTokens: 2000,
			wantMin:      0.001,
			wantMax:      0.002,
		},
		{
			name:         "Claude Sonnet 4",
			provider:     "anthropic",
			model:        "claude-sonnet-4-20250514",
			inputTokens:  5000,
			outputTokens: 1000,
			wantMin:      0.029,
			wantMax:      0.031,
		},
		{
			name:         "Unknown model returns zero",
			provider:     "unknown",
			model:        "mystery-model",
			inputTokens:  1000,
			outputTokens: 1000,
			wantMin:      0,
			wantMax:      0,
		},
		{
			name:         "Zero tokens returns zero cost",
			provider:     "openai",
			model:        "gpt-4o",
			inputTokens:  0,
			outputTokens: 0,
			wantMin:      0,
			wantMax:      0,
		},
		{
			name:         "Local provider always zero cost",
			provider:     "local",
			model:        "llama3.2:8b",
			inputTokens:  100000,
			outputTokens: 50000,
			wantMin:      0,
			wantMax:      0,
		},
		{
			name:         "Local provider case-insensitive",
			provider:     "Local",
			model:        "codellama:13b",
			inputTokens:  1000000,
			outputTokens: 1000000,
			wantMin:      0,
			wantMax:      0,
		},
		{
			name:         "GPT-5.4 pricing present",
			provider:     "openai",
			model:        "gpt-5.4",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.009,
			wantMax:      0.011,
		},
		{
			name:         "Gemini 2.5 Flash pricing present",
			provider:     "google",
			model:        "gemini-2.5-flash",
			inputTokens:  10000,
			outputTokens: 2000,
			wantMin:      0.007,
			wantMax:      0.009,
		},
		{
			name:         "Provider-agnostic fallback",
			provider:     "gemini-oai",
			model:        "gemini-2.5-pro",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.006,
			wantMax:      0.007,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.Calculate(tt.provider, tt.model, tt.inputTokens, tt.outputTokens)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("Calculate(%s/%s, %d, %d) = %f, want between %f and %f",
					tt.provider, tt.model, tt.inputTokens, tt.outputTokens,
					got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSetPricing(t *testing.T) {
	calc := New()

	calc.SetPricing(ModelPricing{
		Provider:         "custom",
		Model:            "my-model",
		InputPerMillion:  1.0,
		OutputPerMillion: 2.0,
	})

	got := calc.Calculate("custom", "my-model", 1_000_000, 1_000_000)
	want := 3.0 // 1.0 + 2.0
	if math.Abs(got-want) > 0.001 {
		t.Errorf("Calculate with custom pricing = %f, want %f", got, want)
	}
}

func TestLoadFromConfig(t *testing.T) {
	calc := New()

	// Override GPT-4o with a negotiated rate
	calc.LoadFromConfig(PricingConfig{
		Models: []ModelPricing{
			{Provider: "openai", Model: "gpt-4o", InputPerMillion: 2.00, OutputPerMillion: 8.00},
		},
	})

	got := calc.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000)
	want := 10.0 // 2.00 + 8.00 (overridden, not 2.50 + 10.00)
	if math.Abs(got-want) > 0.001 {
		t.Errorf("Calculate with config override = %f, want %f", got, want)
	}
}

func TestGlobalDiscount(t *testing.T) {
	calc := New()

	calc.LoadFromConfig(PricingConfig{
		DiscountPercent: 0.20, // 20% off
	})

	// GPT-4o: list = $2.50/M in + $10.00/M out
	// 1M tokens each: $2.50 + $10.00 = $12.50 base
	// 20% off: $12.50 × 0.80 = $10.00
	got := calc.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000)
	want := 10.0
	if math.Abs(got-want) > 0.001 {
		t.Errorf("Calculate with global discount = %f, want %f", got, want)
	}
}

func TestModelDiscount(t *testing.T) {
	calc := New()

	calc.LoadFromConfig(PricingConfig{
		DiscountPercent: 0.10, // 10% global
		Models: []ModelPricing{
			{
				Provider:         "openai",
				Model:            "gpt-4o",
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				DiscountPercent:  0.20, // 20% model-specific
			},
		},
	})

	// 1M tokens each: $2.50 + $10.00 = $12.50 base
	// model discount: $12.50 × 0.80 = $10.00
	// global discount: $10.00 × 0.90 = $9.00
	got := calc.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000)
	want := 9.0
	if math.Abs(got-want) > 0.001 {
		t.Errorf("Calculate with stacked discounts = %f, want %f", got, want)
	}
}
