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

// ─── Provider alias: anthropic-direct → anthropic ────────────────────────────

func TestCalculator_AnthropicDirect_HasPricing(t *testing.T) {
	c := New()
	// anthropic-direct should resolve to anthropic pricing via alias.
	if !c.HasPricing("anthropic-direct", "claude-sonnet-4-20250514") {
		t.Error("HasPricing(anthropic-direct, claude-sonnet-4-20250514) should be true via alias")
	}
	if !c.HasPricing("anthropic-direct", "claude-opus-4-20250514") {
		t.Error("HasPricing(anthropic-direct, claude-opus-4-20250514) should be true via alias")
	}
}

func TestCalculator_AnthropicDirect_CalculateParity(t *testing.T) {
	c := New()
	// Cost must be identical to anthropic — same models, same pricing.
	direct := c.Calculate("anthropic-direct", "claude-sonnet-4-20250514", 100_000, 50_000)
	canonical := c.Calculate("anthropic", "claude-sonnet-4-20250514", 100_000, 50_000)
	if direct != canonical {
		t.Errorf("anthropic-direct cost = %f, anthropic cost = %f — must be identical", direct, canonical)
	}
	if direct == 0 {
		t.Error("cost should be non-zero for a known model")
	}
}

func TestCalculator_AnthropicDirect_ConfigOverrideInherited(t *testing.T) {
	c := New()
	// Override anthropic pricing — anthropic-direct should inherit it.
	c.LoadFromConfig(PricingConfig{
		Models: []ModelPricing{
			{Provider: "anthropic", Model: "claude-sonnet-4-20250514", InputPerMillion: 1.00, OutputPerMillion: 5.00},
		},
	})

	directCost := c.Calculate("anthropic-direct", "claude-sonnet-4-20250514", 1_000_000, 1_000_000)
	want := 6.0 // 1.00 + 5.00 (overridden rate)
	if directCost < want-0.001 || directCost > want+0.001 {
		t.Errorf("anthropic-direct cost = %f, want %f (should inherit anthropic override)", directCost, want)
	}
}

func TestCalculator_AnthropicDirect_UnknownModelStillBlocked(t *testing.T) {
	c := New()
	// A model that doesn't exist under anthropic should also not exist under anthropic-direct.
	if c.HasPricing("anthropic-direct", "claude-nonexistent-99") {
		t.Error("HasPricing should be false for unknown model even via alias")
	}
}
