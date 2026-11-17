package costcalc

import (
	"math"
	"testing"
)

// costTolerance is tight enough to catch a wrong rate and loose enough to
// absorb the float rounding in the share numerator.
const costTolerance = 1e-15

func TestCacheCostUSD(t *testing.T) {
	c := New()

	tests := []struct {
		name        string
		provider    string
		model       string
		rawInput    int64
		cacheRead   int64
		cacheCreate int64
		extendedTTL bool
		want        float64
	}{
		{
			// N = 100 + round(80*0.1) + round(10*1.25) = 121.
			// C_in = 121 × $3/M. share = (8 + 12.5)/121. Cost = 20.5 × $3/M.
			name:        "anthropic 5m TTL",
			provider:    "anthropic",
			model:       "claude-sonnet-4-20250514",
			rawInput:    100,
			cacheRead:   80,
			cacheCreate: 10,
			want:        6.15e-5,
		},
		{
			// 1-hour TTL doubles the creation rate: share = (8 + 20)/128.
			name:        "anthropic 1h TTL",
			provider:    "anthropic",
			model:       "claude-sonnet-4-20250514",
			rawInput:    100,
			cacheRead:   80,
			cacheCreate: 10,
			extendedTTL: true,
			want:        8.4e-5,
		},
		{
			name:        "anthropic-direct alias",
			provider:    "anthropic-direct",
			model:       "claude-sonnet-4-20250514",
			rawInput:    100,
			cacheRead:   80,
			cacheCreate: 10,
			want:        6.15e-5,
		},
		{
			name:        "anthropic-vertex alias",
			provider:    "anthropic-vertex",
			model:       "claude-sonnet-4-20250514",
			rawInput:    100,
			cacheRead:   80,
			cacheCreate: 10,
			want:        6.15e-5,
		},
		{
			// Inclusive: N = (100-90) + round(90*0.5) = 55. Cost = 45 × $2.50/M.
			name:      "openai inclusive",
			provider:  "openai",
			model:     "gpt-4o",
			rawInput:  100,
			cacheRead: 90,
			want:      1.125e-4,
		},
		{
			// Cache writes are already inside the prompt for inclusive
			// providers, so they add nothing to the cache charge.
			name:        "openai ignores cache writes",
			provider:    "openai",
			model:       "gpt-4o",
			rawInput:    100,
			cacheRead:   90,
			cacheCreate: 5,
			want:        1.125e-4,
		},
		{
			// N = (1000-800) + round(800*0.10) = 280. Cost = 80 × $1.25/M.
			name:      "google 2.5 pro",
			provider:  "google",
			model:     "gemini-2.5-pro",
			rawInput:  1000,
			cacheRead: 800,
			want:      1.0e-4,
		},
		{
			name:      "gemini-oai alias",
			provider:  "gemini-oai",
			model:     "gemini-2.5-pro",
			rawInput:  1000,
			cacheRead: 800,
			want:      1.0e-4,
		},
		{
			// N = 200 + 80 = 280. Cost = 80 × $0.30/M.
			name:      "google 2.5 flash",
			provider:  "google",
			model:     "gemini-2.5-flash",
			rawInput:  1000,
			cacheRead: 800,
			want:      2.4e-5,
		},
		{
			name:      "no cache discount config",
			provider:  "mistral",
			model:     "mistral-large-latest",
			rawInput:  100,
			cacheRead: 50,
			want:      0,
		},
		{
			name:      "no cache tokens",
			provider:  "anthropic",
			model:     "claude-sonnet-4-20250514",
			rawInput:  100,
			cacheRead: 0,
			want:      0,
		},
		{
			name:      "model without pricing",
			provider:  "anthropic",
			model:     "claude-not-a-real-model",
			rawInput:  100,
			cacheRead: 50,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.CacheCostUSD(tt.provider, tt.model, tt.rawInput, tt.cacheRead, tt.cacheCreate, tt.extendedTTL)
			if math.Abs(got-tt.want) > costTolerance {
				t.Errorf("CacheCostUSD = %.12g, want %.12g", got, tt.want)
			}
		})
	}
}

// TestCacheCostUSD_TieredInput pins the tiered branch: the share applies to the
// tiered input cost, not to the marginal cost of the cached tokens.
func TestCacheCostUSD_TieredInput(t *testing.T) {
	c := New()

	// N = (500000-100000) + round(100000*0.10) = 410000, above the 200K tier.
	// C_in = 200000 × $1.25/M + 210000 × $2.50/M = $0.775.
	// share = 10000/410000.
	const want = 0.775 * 10000.0 / 410000.0
	got := c.CacheCostUSD("google", "gemini-2.5-pro", 500000, 100000, 0, false)
	if math.Abs(got-want) > costTolerance {
		t.Errorf("CacheCostUSD = %.12g, want %.12g", got, want)
	}
}

// TestCacheCostUSD_SubTokenRounding documents the one place the split does not
// reconcile exactly: the normalized input rounds each cache term to whole
// tokens, so a fractional equivalent leaves a gap under one input token.
func TestCacheCostUSD_SubTokenRounding(t *testing.T) {
	c := New()

	// 10 cache-write tokens at 1.25× are 12.5 equivalent tokens. The
	// normalized input rounds that to 13, while the share keeps the exact 12.5.
	const provider, model = "anthropic", "claude-sonnet-4-20250514"
	normalized := c.NormalizeCachedInputWithTTL(provider, model, 100, 80, 10, false)
	total := c.Calculate(provider, model, normalized, 0)
	fresh := c.Calculate(provider, model, 100, 0)
	cacheCost := c.CacheCostUSD(provider, model, 100, 80, 10, false)

	gap := total - cacheCost - fresh
	if gap <= 0 {
		t.Errorf("gap = %g, want the half-token rounding gap", gap)
	}
	if maxGap := 3.0 / 1_000_000; gap > maxGap {
		t.Errorf("gap = %g, want at most one input token (%g)", gap, maxGap)
	}
}

// TestCacheCostUSD_ReconcilesWithCalculate asserts the invariant that makes the
// cache cost meaningful: the cached share plus the fresh share equal exactly
// what Calculate charges for the normalized input, for a model without tiering.
func TestCacheCostUSD_ReconcilesWithCalculate(t *testing.T) {
	c := New()

	tests := []struct {
		name        string
		provider    string
		model       string
		rawInput    int64
		cacheRead   int64
		cacheCreate int64
		extendedTTL bool
		fresh       int64 // tokens Calculate bills at the full input rate
	}{
		{
			// Additive: rawInput is the fresh count. cacheCreate=8 keeps both
			// cache equivalents integral so the split reconciles exactly.
			name:        "anthropic additive",
			provider:    "anthropic",
			model:       "claude-sonnet-4-20250514",
			rawInput:    100,
			cacheRead:   80,
			cacheCreate: 8,
			fresh:       100,
		},
		{
			name:        "anthropic additive 1h",
			provider:    "anthropic",
			model:       "claude-sonnet-4-20250514",
			rawInput:    100,
			cacheRead:   80,
			cacheCreate: 10,
			extendedTTL: true,
			fresh:       100,
		},
		{
			// Inclusive: the fresh count is rawInput minus the cached reads.
			name:      "openai inclusive",
			provider:  "openai",
			model:     "gpt-4o",
			rawInput:  100,
			cacheRead: 90,
			fresh:     10,
		},
		{
			name:      "google inclusive",
			provider:  "google",
			model:     "gemini-2.5-flash",
			rawInput:  1000,
			cacheRead: 800,
			fresh:     200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := c.NormalizeCachedInputWithTTL(tt.provider, tt.model, tt.rawInput, tt.cacheRead, tt.cacheCreate, tt.extendedTTL)
			total := c.Calculate(tt.provider, tt.model, normalized, 0)
			freshCost := c.Calculate(tt.provider, tt.model, tt.fresh, 0)
			cacheCost := c.CacheCostUSD(tt.provider, tt.model, tt.rawInput, tt.cacheRead, tt.cacheCreate, tt.extendedTTL)

			if diff := cacheCost + freshCost - total; math.Abs(diff) > costTolerance {
				t.Errorf("cache %.12g + fresh %.12g = %.12g, want %.12g (diff %.3g)",
					cacheCost, freshCost, cacheCost+freshCost, total, diff)
			}
			if cacheCost > total+costTolerance {
				t.Errorf("cache cost %.12g exceeds total input cost %.12g", cacheCost, total)
			}
		})
	}
}
