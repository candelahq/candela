package costcalc

import (
	"math"
	"math/rand"
	"testing"
)

// The property: for a request that uses a provider with cache pricing, the
// cache charge plus the charge for the fresh tokens equals what Calculate
// charges for the whole normalized prompt, and the cache charge never exceeds
// that total. A break here means the dashboard attributes to cache dollars the
// provider never charged, or hides dollars it did charge.
//
// Scope: non-tiered models. For a model with tiered pricing the cache charge
// is a proportional share of the tiered input cost, so the fresh-token charge
// computed at base rates cannot add back to the total by construction.

type cacheCostCase struct {
	provider string
	model    string
	// canonical is the key cache pricing is stored under.
	canonical string
	// inclusive is true when the provider's prompt count already contains the
	// cached tokens (OpenAI, Google). Anthropic reports fresh tokens only.
	inclusive bool
}

func cacheCostCases() []cacheCostCase {
	return []cacheCostCase{
		{"anthropic", "claude-sonnet-4-20250514", "anthropic", false},
		{"anthropic-direct", "claude-sonnet-4-20250514", "anthropic", false},
		{"anthropic-vertex", "claude-sonnet-4-20250514", "anthropic", false},
		{"anthropic-bedrock", "claude-sonnet-4-20250514", "anthropic", false},
		{"openai", "gpt-4o", "openai", true},
		{"google", "gemini-2.5-flash", "google", true},
		{"gemini-oai", "gemini-2.5-flash", "google", true},
		{"gemini-vertex", "gemini-2.5-flash", "google", true},
		// No cache pricing configured, so every prompt token bills at the full
		// input rate and nothing is attributable to cache.
		{"xai", "grok-4.6", "", true},
	}
}

// cacheCostTolerance allows the rounding NormalizeCachedInputWithTTL applies to
// the discounted equivalents, which is under one token per direction.
func cacheCostTolerance(p ModelPricing) float64 {
	return 2 * p.InputPerMillion / 1e6
}

func checkCacheCost(
	t *testing.T,
	c *Calculator,
	tc cacheCostCase,
	rawInput, cacheRead, cacheCreate int64,
	extendedTTL bool,
) {
	t.Helper()

	p, ok := c.Resolve(tc.provider, tc.model)
	if !ok {
		t.Fatalf("%s/%s has no pricing", tc.provider, tc.model)
	}
	tol := cacheCostTolerance(p)

	normalized := c.NormalizeCachedInputWithTTL(
		tc.provider, tc.model, rawInput, cacheRead, cacheCreate, extendedTTL)
	inputCost := c.Calculate(tc.provider, tc.model, normalized, 0)
	cacheCost := c.CacheCostUSD(
		tc.provider, tc.model, rawInput, cacheRead, cacheCreate, extendedTTL)

	if cacheCost < 0 {
		t.Fatalf("%s/%s raw=%d read=%d create=%d ttl1h=%v: cache cost %v is negative",
			tc.provider, tc.model, rawInput, cacheRead, cacheCreate, extendedTTL, cacheCost)
	}
	if cacheCost > inputCost+tol {
		t.Fatalf("%s/%s raw=%d read=%d create=%d ttl1h=%v: cache cost %v exceeds input cost %v",
			tc.provider, tc.model, rawInput, cacheRead, cacheCreate, extendedTTL, cacheCost, inputCost)
	}

	cfg, priced := c.GetCacheDiscount(tc.canonical)
	if !priced {
		if cacheCost != 0 {
			t.Fatalf("%s/%s has no cache pricing but reports cache cost %v",
				tc.provider, tc.model, cacheCost)
		}
		return
	}

	if cacheRead == 0 && cacheCreate == 0 {
		if cacheCost != 0 {
			t.Fatalf("%s/%s has no cache tokens but reports cache cost %v",
				tc.provider, tc.model, cacheCost)
		}
		return
	}

	freshTokens := rawInput
	if cfg.InputIncludesCache {
		// The prompt count already contains the cached reads. Cache writes are
		// billed at the full input rate for these providers, and the parsers
		// never report them, so only the reads move out of the fresh charge.
		freshTokens = rawInput - cacheRead
		if freshTokens < 0 {
			freshTokens = 0
		}
	}
	freshCost := c.Calculate(tc.provider, tc.model, freshTokens, 0)

	if p.TierThresholdTokens > 0 && normalized > p.TierThresholdTokens {
		return
	}
	if diff := math.Abs(cacheCost + freshCost - inputCost); diff > tol {
		t.Fatalf("%s/%s raw=%d read=%d create=%d ttl1h=%v: cache %v + fresh %v = %v, input cost %v, off by %v",
			tc.provider, tc.model, rawInput, cacheRead, cacheCreate, extendedTTL,
			cacheCost, freshCost, cacheCost+freshCost, inputCost, diff)
	}
}

func TestCacheCostReconcilesWithInputCost(t *testing.T) {
	c := New()
	rng := rand.New(rand.NewSource(1))

	for _, tc := range cacheCostCases() {
		for i := 0; i < 2000; i++ {
			rawInput := int64(rng.Intn(2_000_000))
			cacheRead := int64(rng.Intn(1_000_000))
			cacheCreate := int64(rng.Intn(1_000_000))
			if rng.Intn(4) == 0 {
				cacheRead = 0
			}
			if rng.Intn(4) == 0 {
				cacheCreate = 0
			}
			if tc.inclusive {
				// A prompt count that already contains the cached tokens cannot
				// report more cached tokens than it has, and these providers do
				// not report cache writes at all.
				cacheCreate = 0
				if cacheRead > rawInput {
					cacheRead = rawInput / 2
				}
			}
			checkCacheCost(t, c, tc, rawInput, cacheRead, cacheCreate, rng.Intn(2) == 1)
		}
	}
}

func FuzzCacheCostReconciles(f *testing.F) {
	for _, seed := range [][4]int64{
		{0, 0, 0, 0},
		{1000, 800, 0, 0},
		{100, 80, 10, 0},
		{5000, 4096, 512, 1},
		{200_000, 199_999, 1, 0},
	} {
		f.Add(seed[0], seed[1], seed[2], seed[3])
	}

	c := New()
	f.Fuzz(func(t *testing.T, rawInput, cacheRead, cacheCreate, ttl int64) {
		const bound = 5_000_000
		rawInput = min(max(rawInput, 0), bound)
		cacheRead = min(max(cacheRead, 0), bound)
		cacheCreate = min(max(cacheCreate, 0), bound)

		for _, tc := range cacheCostCases() {
			read, create := cacheRead, cacheCreate
			if tc.inclusive {
				create = 0
				if read > rawInput {
					read = rawInput / 2
				}
			}
			checkCacheCost(t, c, tc, rawInput, read, create, ttl%2 == 1)
		}
	})
}
