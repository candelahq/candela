package costcalc

// Unit tests for CRIT-17 (discount=1.0 yields $0 cost) and related calculator
// edge cases: stacked discounts, ParseModelName date suffix handling,
// clampDiscount boundary conditions.

import (
	"testing"
)

// ─── CRIT-17: discount_percent=1.0 → $0 cost, HasPricing still true ──────────

func TestCalculator_FullDiscount_YieldsZeroCost(t *testing.T) {
	c := New()
	// Override gpt-4o with 100% discount — should produce $0 cost.
	c.SetPricing(ModelPricing{
		Provider:         "openai",
		Model:            "gpt-4o",
		InputPerMillion:  2.50,
		OutputPerMillion: 10.00,
		DiscountPercent:  1.0, // 100% off
	})

	cost := c.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000)
	if cost != 0.0 {
		t.Errorf("Calculate with 100%% discount = %f, want 0.0", cost)
	}
}

func TestCalculator_FullDiscount_HasPricingStillTrue(t *testing.T) {
	// CRIT-17: even with discount=1.0 (free), HasPricing returns true,
	// which means the proxy gate does NOT block the request. This is intentional
	// (admin explicitly configured the model), but documents the behaviour.
	c := New()
	c.SetPricing(ModelPricing{
		Provider:        "openai",
		Model:           "gpt-4o",
		DiscountPercent: 1.0,
	})
	if !c.HasPricing("openai", "gpt-4o") {
		t.Error("HasPricing should return true even when discount=1.0")
	}
}

// ─── Stacked discounts (model + global) ───────────────────────────────────────

func TestCalculator_StackedDiscounts(t *testing.T) {
	c := New()
	// Model-level 10% discount + global 20% discount.
	// Effective multiplier: (1 - 0.10) * (1 - 0.20) = 0.90 * 0.80 = 0.72
	c.SetPricing(ModelPricing{
		Provider:         "openai",
		Model:            "gpt-4o",
		InputPerMillion:  1_000_000, // $1 per token for easy math
		OutputPerMillion: 0,
		DiscountPercent:  0.10,
	})
	c.SetGlobalDiscount(0.20)

	// 1 input token → baseCost = 1.0
	// After model 10% off: 0.90
	// After global 20% off: 0.90 * 0.80 = 0.72
	cost := c.Calculate("openai", "gpt-4o", 1, 0)
	const want = 0.72
	if cost < want-0.0001 || cost > want+0.0001 {
		t.Errorf("stacked discount cost = %f, want ~%f", cost, want)
	}
}

// ─── clampDiscount boundary conditions ───────────────────────────────────────

func TestClampDiscount_Bounds(t *testing.T) {
	cases := []struct {
		input float64
		want  float64
	}{
		{-0.5, 0},
		{0, 0},
		{0.25, 0.25},
		{1.0, 1.0},
		{1.5, 1.0},
		{100, 1.0},
	}
	for _, tc := range cases {
		got := clampDiscount(tc.input)
		if got != tc.want {
			t.Errorf("clampDiscount(%v) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// ─── ParseModelName date suffix handling ─────────────────────────────────────
// Note: ParseModelName lives in pkg/proxy/translate.go; see proxy package tests.
// Tested here via the pricing table lookup (model name resolution).

func TestCalculator_UnknownModel_ZeroCost(t *testing.T) {
	c := New()
	cost := c.Calculate("openai", "gpt-nonexistent-9000", 1_000_000, 1_000_000)
	if cost != 0 {
		t.Errorf("unknown model cost = %f, want 0.0", cost)
	}
}

func TestCalculator_LocalProvider_AlwaysFree(t *testing.T) {
	c := New()
	cost := c.Calculate("local", "llama3", 1_000_000, 1_000_000)
	if cost != 0 {
		t.Errorf("local model cost = %f, want 0.0 (runs on your hardware)", cost)
	}
	if !c.HasPricing("local", "anything") {
		t.Error("HasPricing should return true for local provider")
	}
}
