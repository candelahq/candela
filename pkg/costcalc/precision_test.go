package costcalc

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

// Property: Cost is always finite, non-negative, and monotonic with respect to tokens
func TestProperty_CostPrecisionBounds(t *testing.T) {
	c := New()

	rapid.Check(t, func(t *rapid.T) {
		inTokens := rapid.Int64Range(0, 10_000_000).Draw(t, "inTokens")
		outTokens := rapid.Int64Range(0, 10_000_000).Draw(t, "outTokens")

		// Test across multiple providers and models including sub-cent models
		models := []struct {
			provider string
			model    string
		}{
			{"openai", "gpt-4.1-nano"},          // low-cost: $0.10 / $0.40 per 1M
			{"google", "gemini-2.5-flash-lite"}, // low-cost: $0.10 / $0.40 per 1M
			{"anthropic", "claude-haiku-4.5"},   // $1.00 / $5.00 per 1M
			{"anthropic", "claude-sonnet-5"},    // promotional / fallback
		}

		for _, m := range models {
			cost := c.Calculate(m.provider, m.model, inTokens, outTokens)

			// 1. Never NaN or infinite
			if math.IsNaN(cost) || math.IsInf(cost, 0) {
				t.Fatalf("cost for %s/%s returned NaN or Inf: %f", m.provider, m.model, cost)
			}

			// 2. Never negative
			if cost < 0 {
				t.Fatalf("cost for %s/%s returned negative value: %f", m.provider, m.model, cost)
			}

			// 3. Zero tokens must yield exactly zero cost
			if inTokens == 0 && outTokens == 0 && cost != 0 {
				t.Fatalf("zero tokens for %s/%s must cost $0.00, got %f", m.provider, m.model, cost)
			}
		}
	})
}

// Property: Accumulation error across multiple micro-transactions is bounded
func TestProperty_AccumulatedSpendPrecision(t *testing.T) {
	c := New()

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(10, 200).Draw(t, "transactions")
		provider := "google"
		model := "gemini-2.5-flash-lite" // $0.10 / $0.40 per 1M tokens

		var totalIn, totalOut int64
		var accumulatedCost float64

		for i := 0; i < n; i++ {
			// Small prompt batches (10 to 500 tokens)
			in := rapid.Int64Range(10, 500).Draw(t, "in")
			out := rapid.Int64Range(5, 200).Draw(t, "out")

			totalIn += in
			totalOut += out
			accumulatedCost += c.Calculate(provider, model, in, out)
		}

		bulkCost := c.Calculate(provider, model, totalIn, totalOut)

		// Floating point accumulation drift between sum(Calculate(x_i)) and Calculate(sum(x_i))
		// must be strictly bounded within 1e-9 (1 nano-USD)
		drift := math.Abs(accumulatedCost - bulkCost)
		if drift > 1e-9 {
			t.Fatalf("accumulated drift %e exceeded 1e-9 tolerance (sum=%f, bulk=%f, txs=%d)",
				drift, accumulatedCost, bulkCost, n)
		}
	})
}

// Property: Sub-cent models maintain accurate token cost down to individual tokens
func TestProperty_SubCentTokenGranularity(t *testing.T) {
	c := New()

	// Gemini 2.5 Flash Lite is $0.10 per 1M input tokens = $0.00000010 per token
	// Verify that fractional token costs are preserved without premature truncation to 0
	cost1Token := c.Calculate("google", "gemini-2.5-flash-lite", 1, 0)
	expected1Token := 0.10 / 1_000_000.0

	if math.Abs(cost1Token-expected1Token) > 1e-15 {
		t.Fatalf("1-token cost for gemini-2.5-flash-lite = %e, want %e", cost1Token, expected1Token)
	}

	// 10 tokens
	cost10Tokens := c.Calculate("google", "gemini-2.5-flash-lite", 10, 0)
	expected10Tokens := 10 * (0.10 / 1_000_000.0)
	if math.Abs(cost10Tokens-expected10Tokens) > 1e-15 {
		t.Fatalf("10-token cost for gemini-2.5-flash-lite = %e, want %e", cost10Tokens, expected10Tokens)
	}
}
