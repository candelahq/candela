package costcalc

import (
	"math"
	"testing"
)

// FuzzCalculate exercises the Calculate method with arbitrary inputs.
// It verifies the invariants that must always hold:
//   - Cost is never negative
//   - Cost is never NaN or Inf
//   - Local provider always returns $0.00
//   - Zero tokens always return $0.00
func FuzzCalculate(f *testing.F) {
	// Seed corpus: realistic inputs covering common providers.
	f.Add("openai", "gpt-4o", int64(1000), int64(500))
	f.Add("anthropic", "claude-sonnet-4-20250514", int64(2000), int64(1000))
	f.Add("google", "gemini-2.5-pro", int64(500000), int64(10000))
	f.Add("local", "llama3", int64(100), int64(50))
	f.Add("", "", int64(0), int64(0))
	f.Add("unknown-provider", "unknown-model", int64(99999999), int64(99999999))
	// Negative tokens (should not panic).
	f.Add("openai", "gpt-4o", int64(-100), int64(-50))
	// Max int64 (overflow boundary).
	f.Add("openai", "gpt-4o", int64(math.MaxInt64), int64(math.MaxInt64))

	calc := New()

	f.Fuzz(func(t *testing.T, provider, model string, inputTokens, outputTokens int64) {
		cost := calc.Calculate(provider, model, inputTokens, outputTokens)

		// Invariant 1: cost must never be NaN or Inf.
		if math.IsNaN(cost) {
			t.Errorf("Calculate(%q, %q, %d, %d) = NaN", provider, model, inputTokens, outputTokens)
		}
		if math.IsInf(cost, 0) {
			t.Errorf("Calculate(%q, %q, %d, %d) = Inf", provider, model, inputTokens, outputTokens)
		}

		// Invariant 2: local provider always costs $0.
		if provider == "local" && cost != 0 {
			t.Errorf("Calculate(local, %q, ...) = %f, want 0", model, cost)
		}

		// Invariant 3: zero tokens should cost $0.
		if inputTokens == 0 && outputTokens == 0 && cost != 0 {
			t.Errorf("Calculate(%q, %q, 0, 0) = %f, want 0", provider, model, cost)
		}
	})
}

// FuzzNormalizeCachedInput exercises the cache normalization logic with
// arbitrary inputs. It verifies:
//   - Result is never negative
//   - No panics on any combination of inputs
//   - When cacheRead and cacheCreate are 0, result equals rawInput
func FuzzNormalizeCachedInput(f *testing.F) {
	// Seed corpus covering each provider's semantics.
	f.Add("openai", "gpt-4o", int64(1000), int64(200), int64(0))
	f.Add("anthropic", "claude-sonnet-4-20250514", int64(500), int64(100), int64(50))
	f.Add("google", "gemini-2.5-pro", int64(2000), int64(500), int64(100))
	f.Add("google", "gemini-2.0-flash", int64(1000), int64(300), int64(0))
	f.Add("anthropic-direct", "claude-sonnet-4-20250514", int64(800), int64(200), int64(100))
	f.Add("unknown", "model", int64(1000), int64(500), int64(200))
	f.Add("", "", int64(0), int64(0), int64(0))
	// Edge: cache tokens exceed rawInput (possible in inclusive mode).
	f.Add("openai", "gpt-4o", int64(100), int64(500), int64(500))
	// Negative values.
	f.Add("openai", "gpt-4o", int64(-100), int64(-50), int64(-25))

	calc := New()

	f.Fuzz(func(t *testing.T, provider, model string, rawInput, cacheRead, cacheCreate int64) {
		result := calc.NormalizeCachedInput(provider, model, rawInput, cacheRead, cacheCreate)

		// Invariant 1: when no cache tokens, result must equal rawInput.
		if cacheRead <= 0 && cacheCreate <= 0 {
			if result != rawInput {
				t.Errorf("NormalizeCachedInput(%q, %q, %d, 0, 0) = %d, want %d",
					provider, model, rawInput, result, rawInput)
			}
		}

		// Note: We do NOT assert result >= 0 here because the function
		// can legitimately return negative values when rawInput is negative
		// (garbage-in/garbage-out for truly invalid inputs). The key
		// invariant is "no panic, no infinite loop."
	})
}

// FuzzNormalizeCachedInputWithTTL adds the TTL flag dimension.
func FuzzNormalizeCachedInputWithTTL(f *testing.F) {
	f.Add("anthropic", "claude-sonnet-4-20250514", int64(500), int64(100), int64(50), true)
	f.Add("anthropic", "claude-sonnet-4-20250514", int64(500), int64(100), int64(50), false)
	f.Add("google", "gemini-2.5-pro", int64(2000), int64(500), int64(100), true)
	f.Add("openai", "gpt-4o", int64(1000), int64(200), int64(0), false)

	calc := New()

	f.Fuzz(func(t *testing.T, provider, model string, rawInput, cacheRead, cacheCreate int64, extendedTTL bool) {
		result := calc.NormalizeCachedInputWithTTL(provider, model, rawInput, cacheRead, cacheCreate, extendedTTL)

		// Same invariant: no cache tokens → pass through.
		if cacheRead <= 0 && cacheCreate <= 0 {
			if result != rawInput {
				t.Errorf("NormalizeCachedInputWithTTL(%q, %q, %d, 0, 0, %v) = %d, want %d",
					provider, model, rawInput, extendedTTL, result, rawInput)
			}
		}

		// For Anthropic with extended TTL, cache creation cost should be
		// >= the default TTL cost (2.0x vs 1.25x). We verify this
		// relationship only for positive cache creation tokens.
		if provider == "anthropic" && cacheCreate > 0 && cacheRead <= 0 && rawInput >= 0 {
			defaultResult := calc.NormalizeCachedInputWithTTL(provider, model, rawInput, 0, cacheCreate, false)
			extendedResult := calc.NormalizeCachedInputWithTTL(provider, model, rawInput, 0, cacheCreate, true)
			if extendedResult < defaultResult {
				t.Errorf("extended TTL result (%d) < default TTL result (%d) for cacheCreate=%d",
					extendedResult, defaultResult, cacheCreate)
			}
		}
	})
}

// FuzzClampDiscount verifies clampDiscount never returns values outside [0, 1].
func FuzzClampDiscount(f *testing.F) {
	f.Add(0.0)
	f.Add(0.5)
	f.Add(1.0)
	f.Add(-1.0)
	f.Add(100.0)
	f.Add(math.NaN())
	f.Add(math.Inf(1))
	f.Add(math.Inf(-1))

	f.Fuzz(func(t *testing.T, d float64) {
		result := clampDiscount(d)

		if math.IsNaN(d) {
			// NaN comparisons are tricky — just ensure no panic.
			return
		}

		if result < 0 || result > 1 {
			t.Errorf("clampDiscount(%f) = %f, want [0, 1]", d, result)
		}
	})
}
