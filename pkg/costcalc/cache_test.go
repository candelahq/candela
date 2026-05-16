package costcalc

import "testing"

// ── NormalizeCachedInput — All providers ─────────────────────────────────────

func TestNormalizeCachedInput_Anthropic(t *testing.T) {
	c := New()
	// 100 total, 80 cache_read, 10 cache_creation, 10 new
	// nonCached = 100 - 80 - 10 = 10
	// result = 10 + round(80*0.1) + round(10*1.25) = 10 + 8 + 13 = 31
	result := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 100, 80, 10)
	if result != 31 {
		t.Errorf("Anthropic = %d, want 31 (10 new + 8 read@0.1× + 13 create@1.25×)", result)
	}
}

func TestNormalizeCachedInput_AnthropicVertex(t *testing.T) {
	c := New()
	// anthropic-vertex should resolve to anthropic via alias and get same rates.
	result := c.NormalizeCachedInput("anthropic-vertex", "claude-sonnet-4-20250514", 100, 80, 10)
	if result != 31 {
		t.Errorf("anthropic-vertex = %d, want 31 (same as anthropic)", result)
	}
}

func TestNormalizeCachedInput_AnthropicDirect(t *testing.T) {
	c := New()
	// anthropic-direct should also resolve to anthropic.
	result := c.NormalizeCachedInput("anthropic-direct", "claude-sonnet-4-20250514", 100, 80, 10)
	if result != 31 {
		t.Errorf("anthropic-direct = %d, want 31 (same as anthropic)", result)
	}
}

func TestNormalizeCachedInput_AnthropicReadOnly(t *testing.T) {
	c := New()
	// 188607 total, 188086 cache_read, 0 cache_creation
	// nonCached = 188607 - 188086 = 521
	// result = 521 + round(188086*0.1) + 0 = 521 + 18809 = 19330
	result := c.NormalizeCachedInput("anthropic", "claude-sonnet-4-20250514", 188607, 188086, 0)
	if result != 19330 {
		t.Errorf("Anthropic read-only = %d, want 19330", result)
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
	// Override anthropic-vertex to use different cache rates (e.g. Google's Vertex pricing).
	c.SetCacheDiscount("anthropic", CacheDiscountConfig{
		ReadDiscount:     0.25, // hypothetical: only 75% off on Vertex
		CreateMultiplier: 1.0,  // no creation surcharge
	})

	// 100 total, 80 cache_read, 10 cache_creation
	// nonCached = 100 - 80 - 10 = 10
	// result = 10 + round(80*0.25) + round(10*1.0) = 10 + 20 + 10 = 40
	result := c.NormalizeCachedInput("anthropic-vertex", "claude-sonnet-4-20250514", 100, 80, 10)
	if result != 40 {
		t.Errorf("custom anthropic-vertex = %d, want 40 (overridden rates)", result)
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
		ReadDiscount:     0.70,
		CreateMultiplier: 1.0,
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
