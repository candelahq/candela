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
