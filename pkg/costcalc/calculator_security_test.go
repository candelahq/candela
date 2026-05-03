package costcalc

import (
	"math"
	"testing"
)

// ── Tests for audit v2 fixes ──

func TestClampDiscount_AboveOne(t *testing.T) {
	got := clampDiscount(1.5)
	if got != 1.0 {
		t.Errorf("clampDiscount(1.5) = %f, want 1.0", got)
	}
}

func TestClampDiscount_Negative(t *testing.T) {
	got := clampDiscount(-0.3)
	if got != 0.0 {
		t.Errorf("clampDiscount(-0.3) = %f, want 0.0", got)
	}
}

func TestClampDiscount_ValidRange(t *testing.T) {
	for _, d := range []float64{0.0, 0.1, 0.5, 0.99, 1.0} {
		got := clampDiscount(d)
		if got != d {
			t.Errorf("clampDiscount(%f) = %f, want %f", d, got, d)
		}
	}
}

func TestSetGlobalDiscount_ClampsAboveOne(t *testing.T) {
	calc := New()
	calc.SetGlobalDiscount(2.0) // should clamp to 1.0

	// With 100% discount, cost should be zero.
	cost := calc.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000)
	if cost != 0 {
		t.Errorf("expected $0 with 100%% discount, got %f", cost)
	}
}

func TestSetGlobalDiscount_ClampsNegative(t *testing.T) {
	calc := New()
	calc.SetGlobalDiscount(-0.5) // should clamp to 0.0

	// With 0% discount, cost should be normal.
	cost := calc.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000)
	// GPT-4o: $2.50/M in + $10.00/M out = $12.50
	if math.Abs(cost-12.50) > 0.01 {
		t.Errorf("expected ~$12.50 with 0%% discount, got %f", cost)
	}
}

func TestLoadFromConfig_ClampsModelDiscount(t *testing.T) {
	calc := New()
	calc.LoadFromConfig(PricingConfig{
		Models: []ModelPricing{
			{
				Provider:         "openai",
				Model:            "gpt-4o",
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				DiscountPercent:  1.5, // invalid — should clamp to 1.0
			},
		},
	})

	cost := calc.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000)
	if cost != 0 {
		t.Errorf("expected $0 with clamped 100%% model discount, got %f", cost)
	}
}

func TestCalculate_NeverNegative(t *testing.T) {
	calc := New()
	// Even with model+global discount both at max valid value (1.0), cost must be ≥0.
	calc.LoadFromConfig(PricingConfig{
		DiscountPercent: 1.0,
		Models: []ModelPricing{
			{
				Provider:         "openai",
				Model:            "gpt-4o",
				InputPerMillion:  2.50,
				OutputPerMillion: 10.00,
				DiscountPercent:  1.0,
			},
		},
	})

	cost := calc.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000)
	if cost < 0 {
		t.Errorf("cost must never be negative, got %f", cost)
	}
}
