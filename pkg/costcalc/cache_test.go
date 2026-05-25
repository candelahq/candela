package costcalc

import "testing"

// ── NormalizeCachedInput — All providers ─────────────────────────────────────

func TestNormalizeCachedInput_Anthropic(t *testing.T) {
	c := New()
	// Anthropic: input_tokens is FRESH only (additive, not inclusive).
	// rawInput=100 (fresh), cacheRead=80, cacheCreate=10
	// result = 100 + round(80*0.1) + round(10*1.25) = 100 + 8 + 13 = 121
	result := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 100, 80, 10)
	if result != 121 {
		t.Errorf("Anthropic = %d, want 121 (100 fresh + 8 read@0.1× + 13 create@1.25×)", result)
	}
}

func TestNormalizeCachedInput_AnthropicVertex(t *testing.T) {
	c := New()
	// anthropic-vertex should resolve to anthropic via alias and get same additive rates.
	result := c.NormalizeCachedInput("anthropic-vertex", "claude-sonnet-4-20250514", 100, 80, 10)
	if result != 121 {
		t.Errorf("anthropic-vertex = %d, want 121 (same as anthropic)", result)
	}
}

func TestNormalizeCachedInput_AnthropicDirect(t *testing.T) {
	c := New()
	// anthropic-direct should also resolve to anthropic (additive).
	result := c.NormalizeCachedInput("anthropic-direct", "claude-sonnet-4-20250514", 100, 80, 10)
	if result != 121 {
		t.Errorf("anthropic-direct = %d, want 121 (same as anthropic)", result)
	}
}

func TestNormalizeCachedInput_AnthropicReadOnly(t *testing.T) {
	c := New()
	// Based on Anthropic docs example: 50 fresh tokens, 100000 cache_read.
	// input_tokens=50, cache_read=100000, cache_creation=0
	// result = 50 + round(100000*0.1) + 0 = 50 + 10000 = 10050
	result := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 50, 100000, 0)
	if result != 10050 {
		t.Errorf("Anthropic read-only = %d, want 10050", result)
	}
}

func TestNormalizeCachedInput_AnthropicNoCaching(t *testing.T) {
	c := New()
	result := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 100, 0, 0)
	if result != 100 {
		t.Errorf("Anthropic no cache = %d, want 100 (unchanged)", result)
	}
}

func TestNormalizeCachedInput_OpenAI(t *testing.T) {
	c := New()
	// 100 total, 90 cache_read, 0 cache_creation
	// nonCached = 100 - 90 = 10
	// result = 10 + round(90*0.5) = 10 + 45 = 55
	result := c.NormalizeCachedInput("openai", "gpt-4o", 100, 90, 0)
	if result != 55 {
		t.Errorf("OpenAI = %d, want 55 (10 new + 45 cached@0.5×)", result)
	}
}

func TestNormalizeCachedInput_OpenAI_NoCaching(t *testing.T) {
	c := New()
	result := c.NormalizeCachedInput("openai", "gpt-4o", 100, 0, 0)
	if result != 100 {
		t.Errorf("OpenAI no cache = %d, want 100 (unchanged)", result)
	}
}

func TestNormalizeCachedInput_Google_Gemini25(t *testing.T) {
	c := New()
	// Gemini 2.5: 90% off → 0.10 multiplier
	// 1000 total, 800 cache_read, 0 cache_creation
	// nonCached = 200, cached_eq = round(800 * 0.10) = 80
	// result = 200 + 80 = 280
	result := c.NormalizeCachedInput("google", "gemini-2.5-pro", 1000, 800, 0)
	if result != 280 {
		t.Errorf("Google Gemini 2.5 = %d, want 280 (200 new + 80 cached@0.10×)", result)
	}
}

func TestNormalizeCachedInput_Google_Gemini25Flash(t *testing.T) {
	c := New()
	result := c.NormalizeCachedInput("google", "gemini-2.5-flash", 1000, 800, 0)
	if result != 280 {
		t.Errorf("Google Gemini 2.5 Flash = %d, want 280", result)
	}
}

func TestNormalizeCachedInput_Google_Gemini20(t *testing.T) {
	c := New()
	// Gemini 2.0: 75% off → 0.25 multiplier
	// 1000 total, 800 cache_read, 0 cache_creation
	// nonCached = 200, cached_eq = round(800 * 0.25) = 200
	// result = 200 + 200 = 400
	result := c.NormalizeCachedInput("google", "gemini-2.0-flash", 1000, 800, 0)
	if result != 400 {
		t.Errorf("Google Gemini 2.0 = %d, want 400 (200 new + 200 cached@0.25×)", result)
	}
}

func TestNormalizeCachedInput_Google_Gemini15(t *testing.T) {
	c := New()
	result := c.NormalizeCachedInput("google", "gemini-1.5-pro", 1000, 800, 0)
	if result != 400 {
		t.Errorf("Google Gemini 1.5 = %d, want 400 (75%% off)", result)
	}
}

func TestNormalizeCachedInput_Google_Gemini3(t *testing.T) {
	c := New()
	result := c.NormalizeCachedInput("google", "gemini-3-flash", 1000, 800, 0)
	if result != 280 {
		t.Errorf("Google Gemini 3 = %d, want 280 (90%% off)", result)
	}
}

func TestNormalizeCachedInput_Google_NoCaching(t *testing.T) {
	c := New()
	result := c.NormalizeCachedInput("google", "gemini-2.5-pro", 500, 0, 0)
	if result != 500 {
		t.Errorf("Google no cache = %d, want 500 (unchanged)", result)
	}
}

// ── Issue 1 fix: gemini-oai aliases to google cache rates ────────────────────

func TestNormalizeCachedInput_GeminiOAI_UsesGoogleRates(t *testing.T) {
	c := New()
	// gemini-oai should use Google cache rates (90% off for 2.5+), NOT OpenAI 50%.
	result := c.NormalizeCachedInput("gemini-oai", "gemini-2.5-pro", 1000, 800, 0)
	if result != 280 {
		t.Errorf("gemini-oai Gemini 2.5 = %d, want 280 (should use Google 90%% off, not OpenAI 50%%)", result)
	}
}

func TestNormalizeCachedInput_GeminiOAI_Gemini20(t *testing.T) {
	c := New()
	result := c.NormalizeCachedInput("gemini-oai", "gemini-2.0-flash", 1000, 800, 0)
	if result != 400 {
		t.Errorf("gemini-oai Gemini 2.0 = %d, want 400 (75%% off)", result)
	}
}

// ── Issue 5 fix: config override support ─────────────────────────────────────

func TestNormalizeCachedInput_CustomOverride(t *testing.T) {
	c := New()
	// Override anthropic to use different cache rates.
	// Note: InputIncludesCache defaults to false (additive), matching Anthropic.
	c.SetCacheDiscount("anthropic", CacheDiscountConfig{
		ReadDiscount:       0.25,  // hypothetical: only 75% off on Vertex
		CreateMultiplier:   1.0,   // no creation surcharge
		InputIncludesCache: false, // Anthropic is additive
	})

	// rawInput=100 (fresh), cacheRead=80, cacheCreate=10
	// result = 100 + round(80*0.25) + round(10*1.0) = 100 + 20 + 10 = 130
	result := c.NormalizeCachedInput("anthropic-vertex", "claude-sonnet-4-20250514", 100, 80, 10)
	if result != 130 {
		t.Errorf("custom anthropic-vertex = %d, want 130 (overridden rates, additive)", result)
	}
}

func TestNormalizeCachedInput_UnknownProvider(t *testing.T) {
	c := New()
	// Unknown provider — return raw input unchanged.
	result := c.NormalizeCachedInput("bedrock", "titan-v2", 1000, 800, 0)
	if result != 1000 {
		t.Errorf("unknown provider = %d, want 1000 (raw, no discount)", result)
	}
}

func TestNormalizeCachedInput_LocalProvider(t *testing.T) {
	c := New()
	// Local models — no cache discount defined, return raw.
	result := c.NormalizeCachedInput("local", "llama-3", 500, 400, 0)
	if result != 500 {
		t.Errorf("local = %d, want 500 (no cache config for local)", result)
	}
}

func TestNormalizeCachedInput_EmptyModel(t *testing.T) {
	c := New()
	// Empty model on Google should default to 90% off (Gemini 2.5+ default).
	result := c.NormalizeCachedInput("google", "", 1000, 800, 0)
	if result != 280 {
		t.Errorf("empty model Google = %d, want 280 (default 90%% off)", result)
	}
}

func TestNormalizeCachedInput_CachedExceedsRaw(t *testing.T) {
	c := New()
	// API inconsistency: cached > raw. Should clamp nonCached to 0.
	result := c.NormalizeCachedInput("openai", "gpt-4o", 50, 100, 0)
	if result != 50 {
		t.Errorf("cached > raw = %d, want 50 (clamped)", result)
	}
}

// ── googleCacheReadDiscount model-awareness ──────────────────────────────────

func TestGoogleCacheReadDiscount(t *testing.T) {
	tests := []struct {
		model    string
		wantRate float64
	}{
		{"gemini-2.5-pro", 0.10},
		{"gemini-2.5-pro-preview-05-06", 0.10},
		{"gemini-2.5-flash", 0.10},
		{"gemini-2.5-flash-lite", 0.10},
		{"gemini-3-flash", 0.10},
		{"gemini-3.1-pro", 0.10},
		{"gemini-4-ultra", 0.10},
		{"some-future-model", 0.10},
		{"gemini-2.0-flash", 0.25},
		{"gemini-2.0-flash-001", 0.25},
		{"gemini-2.0-flash-lite", 0.25},
		{"gemini-1.5-pro", 0.25},
		{"gemini-1.5-flash", 0.25},
		{"gemini-1.0-pro", 0.25},
		{"Gemini-2.5-Pro", 0.10},
		{"GEMINI-2.0-FLASH", 0.25},
		{"", 0.10},
	}

	for _, tt := range tests {
		got := googleCacheReadDiscount(tt.model)
		if got != tt.wantRate {
			t.Errorf("googleCacheReadDiscount(%q) = %v, want %v", tt.model, got, tt.wantRate)
		}
	}
}

// ── Provider alias: gemini-oai → google for cache ────────────────────────────

func TestProviderAliases_GeminiOAI(t *testing.T) {
	// Verify gemini-oai is aliased to google in providerAliases.
	canonical, ok := providerAliases["gemini-oai"]
	if !ok {
		t.Fatal("gemini-oai not in providerAliases")
	}
	if canonical != "google" {
		t.Errorf("gemini-oai alias = %q, want %q", canonical, "google")
	}
}

// ── Gemini review fix: SetCacheDiscount overrides model-aware Google logic ────

func TestNormalizeCachedInput_GoogleOverride_TakesPrecedence(t *testing.T) {
	c := New()
	// Override Google's cache discount to a flat 30% off (0.70× multiplier).
	// This should bypass model-aware logic entirely.
	c.SetCacheDiscount("google", CacheDiscountConfig{
		ReadDiscount:       0.70,
		CreateMultiplier:   1.0,
		InputIncludesCache: true, // Google is inclusive
	})

	// Even for a Gemini 2.5 model that normally gets 90% off,
	// the override should apply 30% off instead.
	// 1000 total, 800 cache_read, 0 cache_creation
	// nonCached = 200, cached_eq = round(800 * 0.70) = 560
	// result = 200 + 560 = 760
	result := c.NormalizeCachedInput("google", "gemini-2.5-pro", 1000, 800, 0)
	if result != 760 {
		t.Errorf("Google override = %d, want 760 (custom 30%% off, not model-aware 90%%)", result)
	}

	// Same for a 2.0 model — should also use the override, not 75%.
	result2 := c.NormalizeCachedInput("google", "gemini-2.0-flash", 1000, 800, 0)
	if result2 != 760 {
		t.Errorf("Google override (2.0) = %d, want 760 (custom 30%% off, not legacy 75%%)", result2)
	}
}

// ── Anthropic additive semantics: real-world data from official docs ──────────

func TestNormalizeCachedInput_AnthropicDocsExample(t *testing.T) {
	// From https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching#tracking-cache-performance
	// "If you have a request with 100,000 tokens of cached content (read from cache),
	// 0 tokens of new content being cached, and 50 tokens in your user message"
	// → input_tokens: 50, cache_read: 100000, cache_creation: 0
	c := New()
	result := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 50, 100000, 0)
	// 50 + round(100000 * 0.1) = 50 + 10000 = 10050
	if result != 10050 {
		t.Errorf("docs example = %d, want 10050", result)
	}
}

func TestNormalizeCachedInput_AnthropicPreWarm(t *testing.T) {
	// From official docs — pre-warm response:
	// {"input_tokens": 8, "cache_creation_input_tokens": 5120, "cache_read_input_tokens": 0}
	c := New()
	result := c.NormalizeCachedInput("anthropic", "claude-opus-4-7-20251101", 8, 0, 5120)
	// 8 + round(0 * 0.1) + round(5120 * 1.25) = 8 + 0 + 6400 = 6408
	if result != 6408 {
		t.Errorf("pre-warm = %d, want 6408", result)
	}
}

func TestNormalizeCachedInput_AnthropicMixedTTL(t *testing.T) {
	// From official docs: mixed 5-min and 1-hour TTL response
	// {\"input_tokens\": 2048, \"cache_read\": 1800, \"cache_creation\": 248}
	// NormalizeCachedInput (no TTL arg) uses 5-min default: 1.25× for creation.
	// For 1-hour TTL, use NormalizeCachedInputWithTTL with extendedTTL=true.
	c := New()
	result := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 2048, 1800, 248)
	// Additive: 2048 + round(1800 * 0.1) + round(248 * 1.25)
	//         = 2048 + 180 + 310 = 2538
	if result != 2538 {
		t.Errorf("mixed TTL (5m default) = %d, want 2538", result)
	}
}

func TestNormalizeCachedInput_AnthropicZeroFresh(t *testing.T) {
	// Edge case: zero fresh tokens (all content is cached).
	c := New()
	result := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 0, 50000, 0)
	// 0 + round(50000 * 0.1) = 5000
	if result != 5000 {
		t.Errorf("zero fresh = %d, want 5000", result)
	}
}

// ── Inclusive vs Additive: same raw numbers, different results ────────────────

func TestNormalizeCachedInput_InclusiveVsAdditive(t *testing.T) {
	c := New()
	// Same raw numbers, different providers → different results.
	// This proves the mode matters.

	// OpenAI (inclusive): rawInput=1000 includes 800 cached.
	// nonCached = 1000 - 800 = 200, cached_eq = 400
	// result = 200 + 400 = 600
	oai := c.NormalizeCachedInput("openai", "gpt-4o", 1000, 800, 0)
	if oai != 600 {
		t.Errorf("OpenAI (inclusive) = %d, want 600", oai)
	}

	// Anthropic (additive): rawInput=1000 is FRESH ONLY, 800 cached is separate.
	// result = 1000 + round(800 * 0.1) = 1000 + 80 = 1080
	anth := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 1000, 800, 0)
	if anth != 1080 {
		t.Errorf("Anthropic (additive) = %d, want 1080", anth)
	}

	// The results MUST differ — same inputs, different modes.
	if oai == anth {
		t.Error("inclusive and additive modes must produce different results for same raw numbers")
	}
}

// ── Custom override: switching mode from additive to inclusive ────────────────

func TestNormalizeCachedInput_OverrideModeSwitch(t *testing.T) {
	c := New()
	// Hypothetical: a Bedrock-Claude provider that reports input_tokens inclusively
	// (AWS changes the reporting semantics). Override to inclusive mode.
	c.SetCacheDiscount("anthropic", CacheDiscountConfig{
		ReadDiscount:       0.1,
		CreateMultiplier:   1.25,
		InputIncludesCache: true, // override: inclusive, not additive
	})

	// rawInput=100 (now INCLUSIVE), cacheRead=80, cacheCreate=10
	// nonCached = 100 - 80 - 10 = 10
	// result = 10 + round(80*0.1) + round(10*1.25) = 10 + 8 + 13 = 31
	result := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 100, 80, 10)
	if result != 31 {
		t.Errorf("mode switch to inclusive = %d, want 31", result)
	}
}

// ── Issue #191: Anthropic 1-hour cache TTL at 2.0× creation rate ─────────────

func TestNormalizeCachedInputWithTTL_Anthropic_1hTTL(t *testing.T) {
	c := New()
	// 1-hour TTL: cache creation charged at 2.0× (not 1.25×).
	// rawInput=100 (fresh), cacheRead=80, cacheCreate=10
	// result = 100 + round(80*0.1) + round(10*2.0) = 100 + 8 + 20 = 128
	result := c.NormalizeCachedInputWithTTL("anthropic", "claude-sonnet-4-20250514", 100, 80, 10, true)
	if result != 128 {
		t.Errorf("Anthropic 1h TTL = %d, want 128 (100 fresh + 8 read@0.1× + 20 create@2.0×)", result)
	}
}

func TestNormalizeCachedInputWithTTL_Anthropic_5mTTL(t *testing.T) {
	c := New()
	// 5-minute TTL (extendedTTL=false): same as NormalizeCachedInput default.
	// rawInput=100 (fresh), cacheRead=80, cacheCreate=10
	// result = 100 + round(80*0.1) + round(10*1.25) = 100 + 8 + 13 = 121
	result := c.NormalizeCachedInputWithTTL("anthropic", "claude-sonnet-4-20250514", 100, 80, 10, false)
	if result != 121 {
		t.Errorf("Anthropic 5m TTL = %d, want 121 (same as default)", result)
	}
}

func TestNormalizeCachedInputWithTTL_AnthropicVertex_1hTTL(t *testing.T) {
	c := New()
	// anthropic-vertex alias should also get 2.0× for 1h TTL.
	result := c.NormalizeCachedInputWithTTL("anthropic-vertex", "claude-sonnet-4-20250514", 100, 80, 10, true)
	if result != 128 {
		t.Errorf("anthropic-vertex 1h TTL = %d, want 128", result)
	}
}

func TestNormalizeCachedInputWithTTL_AnthropicDirect_1hTTL(t *testing.T) {
	c := New()
	// anthropic-direct alias should also get 2.0× for 1h TTL.
	result := c.NormalizeCachedInputWithTTL("anthropic-direct", "claude-sonnet-4-20250514", 100, 80, 10, true)
	if result != 128 {
		t.Errorf("anthropic-direct 1h TTL = %d, want 128", result)
	}
}

func TestNormalizeCachedInputWithTTL_NonAnthropic_Ignored(t *testing.T) {
	c := New()
	// extendedTTL=true should be ignored for non-Anthropic providers.
	// OpenAI: 100 total, 90 cache_read, 0 cache_creation
	// nonCached = 10, cached_eq = round(90*0.5) = 45 → result = 55
	result := c.NormalizeCachedInputWithTTL("openai", "gpt-4o", 100, 90, 0, true)
	if result != 55 {
		t.Errorf("OpenAI with extendedTTL=true = %d, want 55 (TTL ignored for non-Anthropic)", result)
	}
}

func TestNormalizeCachedInputWithTTL_ReadOnly_TTLIrrelevant(t *testing.T) {
	c := New()
	// When there are no cache creation tokens, TTL doesn't matter.
	// Read discount is always 0.1× regardless of TTL.
	result5m := c.NormalizeCachedInputWithTTL("anthropic", "claude-sonnet-4-20250514", 50, 100000, 0, false)
	result1h := c.NormalizeCachedInputWithTTL("anthropic", "claude-sonnet-4-20250514", 50, 100000, 0, true)
	if result5m != result1h {
		t.Errorf("read-only: 5m=%d vs 1h=%d — should be identical (no creation tokens)", result5m, result1h)
	}
	if result5m != 10050 {
		t.Errorf("read-only = %d, want 10050", result5m)
	}
}

func TestNormalizeCachedInputWithTTL_PreWarm_1hTTL(t *testing.T) {
	c := New()
	// Pre-warm with 1-hour TTL: all creation, no reads.
	// {\"input_tokens\": 8, \"cache_creation_input_tokens\": 5120, \"cache_read_input_tokens\": 0}
	// result = 8 + round(0 * 0.1) + round(5120 * 2.0) = 8 + 0 + 10240 = 10248
	result := c.NormalizeCachedInputWithTTL("anthropic", "claude-opus-4.6", 8, 0, 5120, true)
	if result != 10248 {
		t.Errorf("pre-warm 1h TTL = %d, want 10248 (8 fresh + 10240 create@2.0×)", result)
	}
}

func TestNormalizeCachedInputWithTTL_MixedTTL_1h(t *testing.T) {
	c := New()
	// Same scenario as TestNormalizeCachedInput_AnthropicMixedTTL, but with 1h TTL.
	// {\"input_tokens\": 2048, \"cache_read\": 1800, \"cache_creation\": 248}
	// Additive: 2048 + round(1800 * 0.1) + round(248 * 2.0)
	//         = 2048 + 180 + 496 = 2724
	result := c.NormalizeCachedInputWithTTL("anthropic", "claude-sonnet-4-20250514", 2048, 1800, 248, true)
	if result != 2724 {
		t.Errorf("mixed TTL (1h) = %d, want 2724", result)
	}
}

// ── End-to-end: normalize → Calculate → correct dollar amount ────────────────

func TestNormalizeThenCalculate_Anthropic_E2E(t *testing.T) {
	c := New()
	// Claude Sonnet 4: $3.00/M input, $15.00/M output (built-in pricing).
	// Anthropic response: 50 fresh + 100000 cached_read + 0 creation
	normalized := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 50, 100000, 0)
	// normalized = 50 + 10000 = 10050 cost-equivalent tokens

	cost := c.Calculate("anthropic", "claude-sonnet-4-20250514", normalized, 500)

	// Expected: input = 10050/1M * $3.00 = $0.030150
	//           output = 500/1M * $15.00 = $0.007500
	//           total = $0.037650
	wantCost := 0.037650
	if cost < wantCost-0.001 || cost > wantCost+0.001 {
		t.Errorf("E2E cost = %f, want ~%f", cost, wantCost)
	}
}

func TestNormalizeThenCalculate_Google_E2E(t *testing.T) {
	c := New()
	// Gemini 2.5 Flash: $0.30/M input, $2.50/M output (built-in pricing, below 200K tier).
	// Google response: 1000 total (800 cached, 200 fresh)
	normalized := c.NormalizeCachedInput("google", "gemini-2.5-flash", 1000, 800, 0)
	// inclusive: nonCached = 200, cached_eq = round(800 * 0.10) = 80
	// normalized = 280

	cost := c.Calculate("google", "gemini-2.5-flash", normalized, 100)
	// Expected: input = 280/1M * $0.30 = $0.000084
	//           output = 100/1M * $2.50 = $0.000250
	//           total ≈ $0.000334
	if cost < 0.0001 || cost > 0.001 {
		t.Errorf("Google E2E cost = %f, expected small positive value", cost)
	}
}

// Note: OpenAI E2E test removed — no built-in pricing for OpenAI models.
// Cache normalization tests for the "openai" provider (above) remain valid
// because they test token-reporting semantics, not dollar amounts.

func TestNormalizeThenCalculate_Anthropic_1hTTL_E2E(t *testing.T) {
	c := New()
	// Claude Sonnet 4: $3.00/M input, $15.00/M output.
	// 1-hour TTL pre-warm: 8 fresh + 0 read + 5120 creation
	normalized := c.NormalizeCachedInputWithTTL("anthropic", "claude-sonnet-4-20250514", 8, 0, 5120, true)
	// normalized = 8 + 0 + round(5120 * 2.0) = 8 + 10240 = 10248

	cost := c.Calculate("anthropic", "claude-sonnet-4-20250514", normalized, 0)
	// Expected: input = 10248/1M * $3.00 = $0.030744
	wantCost := 0.030744
	if cost < wantCost-0.001 || cost > wantCost+0.001 {
		t.Errorf("1h TTL E2E cost = %f, want ~%f", cost, wantCost)
	}

	// Compare with 5m TTL for same tokens — should be cheaper.
	normalized5m := c.NormalizeCachedInputWithTTL("anthropic", "claude-sonnet-4-20250514", 8, 0, 5120, false)
	cost5m := c.Calculate("anthropic", "claude-sonnet-4-20250514", normalized5m, 0)
	// normalized5m = 8 + round(5120 * 1.25) = 8 + 6400 = 6408
	// cost5m = 6408/1M * $3.00 = $0.019224
	if cost <= cost5m {
		t.Errorf("1h TTL cost ($%f) should be MORE than 5m TTL cost ($%f)", cost, cost5m)
	}
}
