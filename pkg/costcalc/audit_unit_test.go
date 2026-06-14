package costcalc

import (
	"math"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: clampDiscount tests — Issue #2 (NaN passthrough)
// ──────────────────────────────────────────────────────────────────────────────

func TestClampDiscount_NaN(t *testing.T) {
	got := clampDiscount(math.NaN())
	if got != 0 {
		t.Errorf("clampDiscount(NaN) = %v, want 0", got)
	}
}

func TestClampDiscount_NegativeInfinity(t *testing.T) {
	got := clampDiscount(math.Inf(-1))
	if got != 0 {
		t.Errorf("clampDiscount(-Inf) = %v, want 0", got)
	}
}

func TestClampDiscount_PositiveInfinity(t *testing.T) {
	got := clampDiscount(math.Inf(1))
	if got != 1 {
		t.Errorf("clampDiscount(+Inf) = %v, want 1", got)
	}
}

func TestClampDiscount_Normal(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"negative", -0.5, 0},
		{"zero", 0, 0},
		{"half", 0.5, 0.5},
		{"one", 1.0, 1.0},
		{"over_one", 1.5, 1.0},
		{"tiny_positive", 0.001, 0.001},
		{"just_below_one", 0.9999, 0.9999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampDiscount(tt.in)
			if got != tt.want {
				t.Errorf("clampDiscount(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: normalizeModelID edge cases
// ──────────────────────────────────────────────────────────────────────────────

func TestNormalizeModelID_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"version_suffix", "claude-opus-4-7", "claude-opus-4.7"},
		{"no_version", "claude-3-opus", "claude-3-opus"},
		{"double_digit_no_change", "claude-3-5-sonnet", "claude-3-5-sonnet"},
		{"empty_string", "", ""},
		{"no_hyphen", "model", "model"},
		{"single_char", "x", "x"},
		{"trailing_hyphen_digit", "gpt-4-1", "gpt-4.1"},
		{"letter_before_hyphen", "opus-7", "opus-7"},
		{"digit_suffix_not_version", "model-20250514", "model-20250514"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeModelID(tt.input)
			if got != tt.want {
				t.Errorf("normalizeModelID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: extractBaseModel edge cases — Issue #3 (empty ft: parts)
// ──────────────────────────────────────────────────────────────────────────────

func TestExtractBaseModel_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Fine-tune format
		{"ft_normal", "ft:gpt-4.1:org:custom:id", "gpt-4.1"},
		{"ft_bare", "ft:", ""},        // Issue #3: bare ft: prefix should return ""
		{"ft_empty_base", "ft::", ""}, // ft: with empty base model

		// Vertex AI publisher prefix
		{"vertex_publisher", "deepseek-ai/deepseek-v3.2-maas", "deepseek-v3.2-maas"},

		// MaaS suffix
		{"maas_suffix", "deepseek-v3.2-maas", "deepseek-v3.2"},

		// Date suffixes
		{"date_8digit", "claude-sonnet-4-20250514", "claude-sonnet-4"},
		{"date_iso", "gpt-4.1-2025-04-14", "gpt-4.1"},

		// Trailing tags
		{"latest", "gpt-4.1-latest", "gpt-4.1"},
		{"stable", "gpt-4.1-stable", "gpt-4.1"},

		// Preview/exp
		{"preview", "gemini-2.5-pro-preview-05-06", "gemini-2.5-pro"},
		{"exp", "gemini-1.5-pro-exp-0827", "gemini-1.5-pro"},

		// No transformation
		{"no_transform", "gpt-4o", ""},
		{"empty_string", "", ""},
		{"simple_model", "claude-sonnet-4", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBaseModel(tt.input)
			if got != tt.want {
				t.Errorf("extractBaseModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: Cost calculator boundary conditions
// ──────────────────────────────────────────────────────────────────────────────

func TestCalculate_ZeroTokens(t *testing.T) {
	calc := New()
	cost := calc.Calculate("google", "gemini-2.5-pro", 0, 0)
	if cost != 0 {
		t.Errorf("Calculate with 0 tokens = %v, want 0", cost)
	}
}

func TestCalculate_NegativeTokens(t *testing.T) {
	calc := New()
	cost := calc.Calculate("google", "gemini-2.5-pro", -100, -50)
	// Calculator does not clamp negative tokens — that's the caller's
	// responsibility. Verify the function doesn't panic and returns a
	// finite value (negative cost is the expected arithmetic result).
	if math.IsNaN(cost) || math.IsInf(cost, 0) {
		t.Errorf("Calculate with negative tokens = %v, want finite", cost)
	}
}

func TestCalculate_VeryLargeTokens(t *testing.T) {
	calc := New()
	cost := calc.Calculate("google", "gemini-2.5-pro", math.MaxInt64/2, math.MaxInt64/2)
	if math.IsInf(cost, 0) || math.IsNaN(cost) {
		t.Errorf("Calculate with large tokens produced %v, want finite number", cost)
	}
	if cost <= 0 {
		t.Errorf("Calculate with large tokens = %v, want positive", cost)
	}
}

func TestCalculate_DiscountApplied(t *testing.T) {
	calc := New()
	// Set pricing with 50% discount.
	calc.SetPricing(ModelPricing{
		Provider:         "test",
		Model:            "test-model",
		InputPerMillion:  10.0,
		OutputPerMillion: 20.0,
		DiscountPercent:  0.5,
	})
	cost := calc.Calculate("test", "test-model", 1_000_000, 0)
	// $10/M input * 1M tokens * (1 - 0.5 discount) = $5.00
	if cost < 4.99 || cost > 5.01 {
		t.Errorf("Calculate with 50%% discount = %v, want ~5.0", cost)
	}
}

func TestCalculate_NaNDiscountSafe(t *testing.T) {
	calc := New()
	calc.SetPricing(ModelPricing{
		Provider:         "test",
		Model:            "nan-model",
		InputPerMillion:  10.0,
		OutputPerMillion: 20.0,
		DiscountPercent:  math.NaN(),
	})
	cost := calc.Calculate("test", "nan-model", 1_000_000, 0)
	// NaN discount should be clamped to 0, so cost = $10.00/M * 1M = $10.00
	if math.IsNaN(cost) {
		t.Fatal("Calculate with NaN discount produced NaN — billing corruption!")
	}
	if cost < 9.99 || cost > 10.01 {
		t.Errorf("Calculate with NaN discount = %v, want ~10.0 (no discount)", cost)
	}
}
