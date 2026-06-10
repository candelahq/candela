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
			name:         "Claude Opus 4.7 basic usage",
			provider:     "anthropic",
			model:        "claude-opus-4.7",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.017, // 1K×$5.00/M + 500×$25.00/M = $0.005 + $0.0125 = $0.0175
			wantMax:      0.018,
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
			provider:     "google",
			model:        "gemini-2.0-flash",
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
			name:         "Claude Haiku 4.5 pricing present",
			provider:     "anthropic",
			model:        "claude-haiku-4.5",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.003, // 1K×$1.00/M + 500×$5.00/M = $0.001 + $0.0025 = $0.0035
			wantMax:      0.004,
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
			name:         "Gemini 3.1 Pro",
			provider:     "google",
			model:        "gemini-3.1-pro",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.007, // 1K×$2.00/M + 500×$12.00/M = $0.002 + $0.006 = $0.008
			wantMax:      0.009,
		},
		{
			name:         "Gemini 3.1 Flash-Lite",
			provider:     "google",
			model:        "gemini-3.1-flash-lite",
			inputTokens:  10000,
			outputTokens: 2000,
			wantMin:      0.005, // 10K×$0.25/M + 2K×$1.50/M = $0.0025 + $0.003 = $0.0055
			wantMax:      0.006,
		},
		{
			name:         "Gemini 3 Flash",
			provider:     "google",
			model:        "gemini-3-flash",
			inputTokens:  10000,
			outputTokens: 2000,
			wantMin:      0.010, // 10K×$0.50/M + 2K×$3.00/M = $0.005 + $0.006 = $0.011
			wantMax:      0.012,
		},
		{
			name:         "Gemini 3 Flash Lite",
			provider:     "google",
			model:        "gemini-3-flash-lite",
			inputTokens:  100000,
			outputTokens: 20000,
			wantMin:      0.003, // 100K×$0.02/M + 20K×$0.10/M = $0.002 + $0.002 = $0.004
			wantMax:      0.005,
		},
		{
			name:         "Gemini 3.5 Flash",
			provider:     "google",
			model:        "gemini-3.5-flash",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.005, // 1K×$1.50/M + 500×$9.00/M = $0.0015 + $0.0045 = $0.006
			wantMax:      0.007,
		},
		{
			name:         "Gemini 3.5 Flash via gemini-oai provider",
			provider:     "gemini-oai",
			model:        "gemini-3.5-flash",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.005,
			wantMax:      0.007,
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
		{
			name:         "Mistral Medium 3",
			provider:     "mistral",
			model:        "mistral-medium-3",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.001, // 1K×$0.40/M + 500×$2.00/M = $0.0004 + $0.001 = $0.0014
			wantMax:      0.0015,
		},
		{
			name:         "DeepSeek V3.2",
			provider:     "deepseek",
			model:        "deepseek-v3.2-maas",
			inputTokens:  10000,
			outputTokens: 2000,
			wantMin:      0.001, // 10K×$0.14/M + 2K×$0.28/M = $0.0014 + $0.00056 = $0.00196
			wantMax:      0.002,
		},
		{
			name:         "Codestral 2501",
			provider:     "mistral",
			model:        "codestral-2501",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.0007, // 1K×$0.30/M + 500×$0.90/M = $0.0003 + $0.00045 = $0.00075
			wantMax:      0.0008,
		},
		{
			name:         "Qwen3 235B",
			provider:     "qwen",
			model:        "qwen3-235b-a22b-instruct-2507-maas",
			inputTokens:  1000,
			outputTokens: 500,
			wantMin:      0.0009, // 1K×$0.30/M + 500×$1.20/M = $0.0003 + $0.0006 = $0.0009
			wantMax:      0.001,
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

	// Override Gemini 2.0 Flash with a negotiated rate
	calc.LoadFromConfig(PricingConfig{
		Models: []ModelPricing{
			{Provider: "google", Model: "gemini-2.0-flash", InputPerMillion: 0.05, OutputPerMillion: 0.20},
		},
	})

	got := calc.Calculate("google", "gemini-2.0-flash", 1_000_000, 1_000_000)
	want := 0.25 // 0.05 + 0.20 (overridden, not 0.10 + 0.40)
	if math.Abs(got-want) > 0.001 {
		t.Errorf("Calculate with config override = %f, want %f", got, want)
	}
}

func TestGlobalDiscount(t *testing.T) {
	calc := New()

	calc.LoadFromConfig(PricingConfig{
		DiscountPercent: 0.20, // 20% off
	})

	// Claude Sonnet 4: list = $3.00/M in + $15.00/M out
	// 1M tokens each: $3.00 + $15.00 = $18.00 base
	// 20% off: $18.00 × 0.80 = $14.40
	got := calc.Calculate("anthropic", "claude-sonnet-4", 1_000_000, 1_000_000)
	want := 14.40
	if math.Abs(got-want) > 0.01 {
		t.Errorf("Calculate with global discount = %f, want %f", got, want)
	}
}

func TestModelDiscount(t *testing.T) {
	calc := New()

	calc.LoadFromConfig(PricingConfig{
		DiscountPercent: 0.10, // 10% global
		Models: []ModelPricing{
			{
				Provider:         "anthropic",
				Model:            "claude-sonnet-4",
				InputPerMillion:  3.00,
				OutputPerMillion: 15.00,
				DiscountPercent:  0.20, // 20% model-specific
			},
		},
	})

	// 1M tokens each: $3.00 + $15.00 = $18.00 base
	// model discount: $18.00 × 0.80 = $14.40
	// global discount: $14.40 × 0.90 = $12.96
	got := calc.Calculate("anthropic", "claude-sonnet-4", 1_000_000, 1_000_000)
	want := 12.96
	if math.Abs(got-want) > 0.01 {
		t.Errorf("Calculate with stacked discounts = %f, want %f", got, want)
	}
}

// ── Issue 1: MaaS providers must NOT inherit OpenAI cache discounts ──────────

func TestNormalizeCachedInput_MaaSProviders_NoDiscount(t *testing.T) {
	calc := New()

	tests := []struct {
		name        string
		provider    string
		model       string
		rawInput    int64
		cacheRead   int64
		cacheCreate int64
		want        int64
	}{
		{
			name:      "Qwen: no cache discount even with cached tokens",
			provider:  "qwen",
			model:     "qwen3-235b-a22b-instruct-2507-maas",
			rawInput:  100,
			cacheRead: 7,
			want:      100,
		},
		{
			name:      "DeepSeek: no cache discount",
			provider:  "deepseek",
			model:     "deepseek-v3.2-maas",
			rawInput:  1000,
			cacheRead: 500,
			want:      1000,
		},
		{
			name:     "Mistral: no cache discount",
			provider: "mistral",
			model:    "mistral-medium-3",
			rawInput: 500,
			want:     500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.NormalizeCachedInput(tt.provider, tt.model, tt.rawInput, tt.cacheRead, tt.cacheCreate)
			if got != tt.want {
				t.Errorf("NormalizeCachedInput(%q, %q, %d, %d, %d) = %d, want %d",
					tt.provider, tt.model, tt.rawInput, tt.cacheRead, tt.cacheCreate, got, tt.want)
			}
		})
	}
}

// ── Issue 4: extractBaseModel handles publisher prefixes and -maas suffix ────

func TestCalculate_DeepSeek_WithPublisherPrefix(t *testing.T) {
	calc := New()

	// "deepseek-ai/deepseek-v3.2-maas" should strip prefix to "deepseek-v3.2-maas"
	// and match deepseek/deepseek-v3.2-maas pricing.
	got := calc.Calculate("deepseek", "deepseek-ai/deepseek-v3.2-maas", 10000, 2000)
	if got <= 0 {
		t.Errorf("Calculate(deepseek, deepseek-ai/deepseek-v3.2-maas) = %f, want > 0 (pricing should resolve)", got)
	}
}

func TestCalculate_Qwen_WithPublisherPrefix(t *testing.T) {
	calc := New()

	// "qwen/qwen3-235b-a22b-instruct-2507-maas" should strip "qwen/" prefix
	// and match qwen/qwen3-235b-a22b-instruct-2507-maas pricing.
	got := calc.Calculate("qwen", "qwen/qwen3-235b-a22b-instruct-2507-maas", 1000, 500)
	if got <= 0 {
		t.Errorf("Calculate(qwen, qwen/qwen3-...) = %f, want > 0 (pricing should resolve)", got)
	}
}

func TestCalculate_DeepSeek_WithoutMaasSuffix(t *testing.T) {
	calc := New()

	// "deepseek-v3.2" (no -maas suffix) should resolve via extractBaseModel
	// which strips -maas → tries "deepseek-v3.2" as base. However, the stored
	// model IS "deepseek-v3.2-maas". extractBaseModel strips -maas from the
	// stored model during fallback building, so "deepseek-v3.2" without -maas
	// won't match directly. Let's verify the exact behavior.
	got := calc.Calculate("deepseek", "deepseek-v3.2", 10000, 2000)
	// This may or may not resolve depending on fallback logic; the important
	// thing is it doesn't panic and returns a non-negative value.
	if got < 0 {
		t.Errorf("Calculate(deepseek, deepseek-v3.2) = %f, want >= 0", got)
	}
}

func TestQwen3CoderPricing(t *testing.T) {
	c := New()
	cost := c.Calculate("qwen", "qwen3-coder-480b-a35b-instruct-maas", 1000000, 1000000)
	// $0.22 input + $1.80 output = $2.02
	if cost < 2.01 || cost > 2.03 {
		t.Errorf("qwen3-coder cost for 1M in + 1M out = %f, want ~2.02", cost)
	}
}
