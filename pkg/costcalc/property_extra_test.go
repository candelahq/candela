package costcalc

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Property: Unknown model always returns zero cost.
func TestProperty_UnknownModelZeroCost(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "provider")
		model := rapid.StringMatching(`unknown-[a-z]{3,10}`).Draw(t, "model")
		input := rapid.Int64Range(0, 1_000_000).Draw(t, "input")
		output := rapid.Int64Range(0, 1_000_000).Draw(t, "output")

		calc := New()
		cost := calc.Calculate(provider, model, input, output)
		if cost != 0 {
			t.Fatalf("expected 0 cost for unknown model %s/%s, got %f", provider, model, cost)
		}
	})
}

// Property: Swapping provider/model args always returns >= 0 (never panics).
func TestProperty_NeverPanics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.String().Draw(t, "provider")
		model := rapid.String().Draw(t, "model")
		input := rapid.Int64Range(0, 1_000_000).Draw(t, "input")
		output := rapid.Int64Range(0, 1_000_000).Draw(t, "output")

		calc := New()
		cost := calc.Calculate(provider, model, input, output)
		if cost < 0 {
			t.Fatalf("negative cost: %f", cost)
		}
	})
}

// Property: CalculateTimeBased is non-negative and monotonic with duration.
func TestProperty_TimeBasedNonNegativeMonotonic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.SampledFrom([]string{"openai", "anthropic", "google"}).Draw(t, "provider")
		model := rapid.SampledFrom([]string{"gpt-4o-realtime-preview", "unknown"}).Draw(t, "model")
		dur1 := rapid.Int64Range(0, 3600).Draw(t, "dur1_secs")
		dur2 := rapid.Int64Range(dur1, 7200).Draw(t, "dur2_secs")

		calc := New()
		cost1 := calc.CalculateTimeBased(provider, model, time.Duration(dur1)*time.Second)
		cost2 := calc.CalculateTimeBased(provider, model, time.Duration(dur2)*time.Second)

		if cost1 < 0 {
			t.Fatalf("negative time-based cost: %f", cost1)
		}
		if cost2 < cost1 {
			t.Fatalf("time-based cost not monotonic: %f > %f for %ds < %ds", cost1, cost2, dur1, dur2)
		}
	})
}

// Property: NormalizeCachedInput always returns a non-negative value.
func TestProperty_CacheNormalizationNonNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.SampledFrom([]string{"openai", "anthropic", "google"}).Draw(t, "provider")
		model := rapid.SampledFrom([]string{"gpt-4o", "claude-sonnet-4-20250514", "gemini-2.5-pro"}).Draw(t, "model")
		rawInput := rapid.Int64Range(100, 100_000).Draw(t, "raw_input")
		cacheRead := rapid.Int64Range(0, rawInput).Draw(t, "cache_read")

		calc := New()
		normalized := calc.NormalizeCachedInput(provider, model, rawInput, cacheRead, 0)

		if normalized < 0 {
			t.Fatalf("negative normalized input: %d for %s/%s", normalized, provider, model)
		}
	})
}

// Property: SetPricing then Calculate uses new pricing.
func TestProperty_SetPricingOverrides(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		inputPer1M := rapid.Float64Range(0.01, 100.0).Draw(t, "input_per_1m")
		outputPer1M := rapid.Float64Range(0.01, 100.0).Draw(t, "output_per_1m")
		input := rapid.Int64Range(1, 100_000).Draw(t, "input")
		output := rapid.Int64Range(1, 100_000).Draw(t, "output")

		calc := New()
		calc.SetPricing(ModelPricing{
			Provider:         "custom",
			Model:            "custom-model",
			InputPerMillion:  inputPer1M,
			OutputPerMillion: outputPer1M,
		})

		cost := calc.Calculate("custom", "custom-model", input, output)
		expected := (float64(input)/1_000_000)*inputPer1M + (float64(output)/1_000_000)*outputPer1M

		// Allow small floating point tolerance.
		diff := cost - expected
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.0001 {
			t.Fatalf("cost %f != expected %f (diff %f)", cost, expected, diff)
		}
	})
}

// Property: NewEmpty calculator returns 0 for all models.
func TestProperty_EmptyCalculatorZero(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		provider := rapid.String().Draw(t, "provider")
		model := rapid.String().Draw(t, "model")
		input := rapid.Int64Range(0, 1_000_000).Draw(t, "input")
		output := rapid.Int64Range(0, 1_000_000).Draw(t, "output")

		calc := NewEmpty()
		cost := calc.Calculate(provider, model, input, output)
		if cost != 0 {
			t.Fatalf("empty calc returned %f, want 0", cost)
		}
	})
}
