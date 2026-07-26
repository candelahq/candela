package costcalc

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

// Property: Cost is always >= 0 for any valid input.
func TestProperty_CostNonNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.SampledFrom([]string{"openai", "anthropic", "google"}).Draw(t, "provider")
		model := rapid.SampledFrom([]string{"gpt-4o", "gpt-4o-mini", "claude-sonnet-4-20250514", "gemini-2.5-pro", "unknown-model"}).Draw(t, "model")
		inputTokens := rapid.IntRange(0, 1_000_000).Draw(t, "input_tokens")
		outputTokens := rapid.IntRange(0, 1_000_000).Draw(t, "output_tokens")

		calc := New()
		cost := calc.Calculate(provider, model, int64(inputTokens), int64(outputTokens))
		if cost < 0 {
			t.Fatalf("negative cost: %f for %s/%s (%d, %d)", cost, provider, model, inputTokens, outputTokens)
		}
	})
}

// Property: Cost increases monotonically with input tokens (output fixed).
func TestProperty_CostMonotonicInput(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.SampledFrom([]string{"openai", "anthropic"}).Draw(t, "provider")
		model := rapid.SampledFrom([]string{"gpt-4o", "claude-sonnet-4-20250514"}).Draw(t, "model")
		tokens1 := rapid.IntRange(0, 500_000).Draw(t, "tokens1")
		tokens2 := rapid.IntRange(tokens1, 1_000_000).Draw(t, "tokens2")
		outputTokens := rapid.IntRange(0, 100_000).Draw(t, "output_tokens")

		calc := New()
		cost1 := calc.Calculate(provider, model, int64(tokens1), int64(outputTokens))
		cost2 := calc.Calculate(provider, model, int64(tokens2), int64(outputTokens))
		if cost2 < cost1 {
			t.Fatalf("cost not monotonic: %f > %f for input %d > %d", cost1, cost2, tokens1, tokens2)
		}
	})
}

// Property: Cost increases monotonically with output tokens (input fixed).
func TestProperty_CostMonotonicOutput(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.SampledFrom([]string{"openai", "anthropic"}).Draw(t, "provider")
		model := rapid.SampledFrom([]string{"gpt-4o", "claude-sonnet-4-20250514"}).Draw(t, "model")
		inputTokens := rapid.IntRange(0, 100_000).Draw(t, "input_tokens")
		tokens1 := rapid.IntRange(0, 500_000).Draw(t, "tokens1")
		tokens2 := rapid.IntRange(tokens1, 1_000_000).Draw(t, "tokens2")

		calc := New()
		cost1 := calc.Calculate(provider, model, int64(inputTokens), int64(tokens1))
		cost2 := calc.Calculate(provider, model, int64(inputTokens), int64(tokens2))
		if cost2 < cost1 {
			t.Fatalf("cost not monotonic: %f > %f for output %d > %d", cost1, cost2, tokens1, tokens2)
		}
	})
}

// Property: Zero tokens always yields zero cost.
func TestProperty_ZeroTokensZeroCost(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.SampledFrom([]string{"openai", "anthropic", "google"}).Draw(t, "provider")
		model := rapid.SampledFrom([]string{"gpt-4o", "gpt-4o-mini", "claude-sonnet-4-20250514", "gemini-2.5-pro"}).Draw(t, "model")

		calc := New()
		cost := calc.Calculate(provider, model, 0, 0)
		if cost != 0 {
			t.Fatalf("non-zero cost %f for zero tokens: %s/%s", cost, provider, model)
		}
	})
}

// Property: Scaling tokens by N scales cost by N (linearity).
func TestProperty_CostLinearScaling(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.SampledFrom([]string{"openai", "anthropic"}).Draw(t, "provider")
		model := rapid.SampledFrom([]string{"gpt-4o", "claude-sonnet-4-20250514"}).Draw(t, "model")
		inputTokens := rapid.IntRange(0, 10_000).Draw(t, "input_tokens")
		outputTokens := rapid.IntRange(0, 10_000).Draw(t, "output_tokens")
		scale := rapid.IntRange(1, 100).Draw(t, "scale")

		calc := New()
		baseCost := calc.Calculate(provider, model, int64(inputTokens), int64(outputTokens))
		scaledCost := calc.Calculate(provider, model, int64(inputTokens*scale), int64(outputTokens*scale))

		expected := baseCost * float64(scale)
		if math.Abs(scaledCost-expected) > 1e-9 {
			t.Fatalf("cost not linear: %f * %d = %f, got %f", baseCost, scale, expected, scaledCost)
		}
	})
}
