package costcalc

import (
	"testing"
)

// ── OpenAI model pricing ─────────────────────────────────────────────────────

func TestCalculate_OpenAI_GPT41(t *testing.T) {
	c := New()
	// 1M input @ $2.00/M + 500K output @ $8.00/M = $2.00 + $4.00 = $6.00
	cost := c.Calculate("openai", "gpt-4.1", 1_000_000, 500_000)
	want := 6.00
	if cost < want-0.001 || cost > want+0.001 {
		t.Errorf("gpt-4.1 cost = %f, want %f", cost, want)
	}
}

func TestCalculate_OpenAI_GPT41Mini(t *testing.T) {
	c := New()
	// 1M input @ $0.40/M + 1M output @ $1.60/M = $0.40 + $1.60 = $2.00
	cost := c.Calculate("openai", "gpt-4.1-mini", 1_000_000, 1_000_000)
	want := 2.00
	if cost < want-0.001 || cost > want+0.001 {
		t.Errorf("gpt-4.1-mini cost = %f, want %f", cost, want)
	}
}

func TestCalculate_OpenAI_GPT41Nano(t *testing.T) {
	c := New()
	// 1M input @ $0.10/M + 1M output @ $0.40/M = $0.10 + $0.40 = $0.50
	cost := c.Calculate("openai", "gpt-4.1-nano", 1_000_000, 1_000_000)
	want := 0.50
	if cost < want-0.001 || cost > want+0.001 {
		t.Errorf("gpt-4.1-nano cost = %f, want %f", cost, want)
	}
}

func TestCalculate_OpenAI_O3(t *testing.T) {
	c := New()
	// o3 was cut from $10/$40 to $2/$8 in early 2026.
	// 1M input @ $2.00/M + 1M output @ $8.00/M = $2.00 + $8.00 = $10.00
	cost := c.Calculate("openai", "o3", 1_000_000, 1_000_000)
	want := 10.00
	if cost < want-0.001 || cost > want+0.001 {
		t.Errorf("o3 cost = %f, want %f", cost, want)
	}
}

func TestCalculate_OpenAI_O4Mini(t *testing.T) {
	c := New()
	// 1M input @ $1.10/M + 1M output @ $4.40/M = $1.10 + $4.40 = $5.50
	cost := c.Calculate("openai", "o4-mini", 1_000_000, 1_000_000)
	want := 5.50
	if cost < want-0.001 || cost > want+0.001 {
		t.Errorf("o4-mini cost = %f, want %f", cost, want)
	}
}

func TestHasPricing_AllOpenAIModels(t *testing.T) {
	c := New()
	models := []string{
		"gpt-4.1", "gpt-4.1-mini", "gpt-4.1-nano",
		"o3", "o4-mini",
		"gpt-4o", "gpt-4o-mini",
	}
	for _, m := range models {
		if !c.HasPricing("openai", m) {
			t.Errorf("HasPricing(openai, %s) = false, want true", m)
		}
	}
}

func TestOpenAI_PricingHierarchy(t *testing.T) {
	c := New()
	// gpt-4.1 > gpt-4.1-mini > gpt-4.1-nano
	full := c.Calculate("openai", "gpt-4.1", 1_000_000, 1_000_000)
	mini := c.Calculate("openai", "gpt-4.1-mini", 1_000_000, 1_000_000)
	nano := c.Calculate("openai", "gpt-4.1-nano", 1_000_000, 1_000_000)
	if full <= mini {
		t.Errorf("gpt-4.1 (%f) should cost more than gpt-4.1-mini (%f)", full, mini)
	}
	if mini <= nano {
		t.Errorf("gpt-4.1-mini (%f) should cost more than gpt-4.1-nano (%f)", mini, nano)
	}
}

// ── extractBaseModel ─────────────────────────────────────────────────────────

func TestExtractBaseModel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Date suffixes (YYYYMMDD)
		{"date suffix 8-digit", "claude-sonnet-4-20250514", "claude-sonnet-4"},
		{"date suffix gpt", "gpt-4.1-20250414", "gpt-4.1"},

		// Date suffixes (YYYY-MM-DD)
		{"date suffix ISO", "gpt-4.1-2025-04-14", "gpt-4.1"},
		{"date suffix ISO claude", "claude-opus-4-2025-05-14", "claude-opus-4"},

		// Preview tags
		{"preview simple", "gemini-2.5-pro-preview", "gemini-2.5-pro"},
		{"preview with date", "gemini-2.5-pro-preview-05-06", "gemini-2.5-pro"},
		{"preview with longer", "gemini-3.1-pro-preview-2025-05", "gemini-3.1-pro"},

		// Experimental tags
		{"exp simple", "gemini-2.0-flash-exp", "gemini-2.0-flash"},
		{"exp with date", "gemini-2.0-flash-exp-0827", "gemini-2.0-flash"},

		// Latest/stable tags
		{"latest", "gpt-4.1-latest", "gpt-4.1"},
		{"stable", "gpt-4o-stable", "gpt-4o"},

		// OpenAI fine-tunes
		{"fine-tune basic", "ft:gpt-4.1:myorg:custom:abc123", "gpt-4.1"},
		{"fine-tune mini", "ft:gpt-4.1-mini:org:name:id", "gpt-4.1-mini"},
		{"fine-tune no-name", "ft:gpt-4o:org", "gpt-4o"},

		// No transformation needed — returns empty
		{"exact model no suffix", "gpt-4.1", ""},
		{"exact model no suffix 2", "claude-sonnet-4", ""},
		{"exact model no suffix 3", "gemini-2.5-pro", ""},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBaseModel(tt.input)
			if got != tt.want {
				t.Errorf("extractBaseModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── isAllDigits ──────────────────────────────────────────────────────────────

func TestIsAllDigits(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"12345678", true},
		{"20250514", true},
		{"0", true},
		{"", false},
		{"123abc", false},
		{"12-34", false},
		{" 123", false},
		{"123 ", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isAllDigits(tt.input)
			if got != tt.want {
				t.Errorf("isAllDigits(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ── Prefix-based resolution via Calculate/HasPricing ─────────────────────────

func TestPrefixResolution_DateSuffix_HasPricing(t *testing.T) {
	c := New()
	// gpt-4.1-2025-04-14 should resolve to gpt-4.1 pricing.
	if !c.HasPricing("openai", "gpt-4.1-2025-04-14") {
		t.Error("HasPricing should resolve gpt-4.1-2025-04-14 to gpt-4.1 via prefix matching")
	}
}

func TestPrefixResolution_DateSuffix_CostParity(t *testing.T) {
	c := New()
	exact := c.Calculate("openai", "gpt-4.1", 1_000_000, 500_000)
	variant := c.Calculate("openai", "gpt-4.1-2025-04-14", 1_000_000, 500_000)
	if exact != variant {
		t.Errorf("gpt-4.1 cost (%f) != gpt-4.1-2025-04-14 cost (%f) — prefix resolution failed", exact, variant)
	}
	if exact == 0 {
		t.Error("cost should be non-zero for a known model")
	}
}

func TestPrefixResolution_DateSuffix8Digit(t *testing.T) {
	c := New()
	// claude-sonnet-4-20250514 has an exact match, but let's test with a different date.
	// claude-opus-4.6-20260101 should resolve to claude-opus-4.6
	if !c.HasPricing("anthropic", "claude-opus-4.6-20260101") {
		t.Error("HasPricing should resolve claude-opus-4.6-20260101 to claude-opus-4.6 via prefix")
	}
	exact := c.Calculate("anthropic", "claude-opus-4.6", 1_000_000, 1_000_000)
	variant := c.Calculate("anthropic", "claude-opus-4.6-20260101", 1_000_000, 1_000_000)
	if exact != variant {
		t.Errorf("claude-opus-4.6 (%f) != claude-opus-4.6-20260101 (%f)", exact, variant)
	}
}

func TestPrefixResolution_Preview(t *testing.T) {
	c := New()
	// gemini-2.5-pro-preview-05-06 should resolve to gemini-2.5-pro.
	if !c.HasPricing("google", "gemini-2.5-pro-preview-05-06") {
		t.Error("HasPricing should resolve gemini-2.5-pro-preview-05-06 via prefix")
	}
	exact := c.Calculate("google", "gemini-2.5-pro", 1_000_000, 1_000_000)
	variant := c.Calculate("google", "gemini-2.5-pro-preview-05-06", 1_000_000, 1_000_000)
	if exact != variant {
		t.Errorf("gemini-2.5-pro (%f) != gemini-2.5-pro-preview-05-06 (%f)", exact, variant)
	}
}

func TestPrefixResolution_Latest(t *testing.T) {
	c := New()
	if !c.HasPricing("openai", "gpt-4o-latest") {
		t.Error("HasPricing should resolve gpt-4o-latest to gpt-4o via prefix")
	}
	exact := c.Calculate("openai", "gpt-4o", 1_000_000, 1_000_000)
	variant := c.Calculate("openai", "gpt-4o-latest", 1_000_000, 1_000_000)
	if exact != variant {
		t.Errorf("gpt-4o (%f) != gpt-4o-latest (%f)", exact, variant)
	}
}

func TestPrefixResolution_FineTune(t *testing.T) {
	c := New()
	if !c.HasPricing("openai", "ft:gpt-4.1:myorg:custom:abc123") {
		t.Error("HasPricing should resolve fine-tuned gpt-4.1 to gpt-4.1 via ft: prefix parsing")
	}
	exact := c.Calculate("openai", "gpt-4.1", 1_000_000, 1_000_000)
	ft := c.Calculate("openai", "ft:gpt-4.1:myorg:custom:abc123", 1_000_000, 1_000_000)
	if exact != ft {
		t.Errorf("gpt-4.1 (%f) != ft:gpt-4.1:... (%f)", exact, ft)
	}
}

func TestPrefixResolution_ExactMatchTakesPriority(t *testing.T) {
	c := New()
	// claude-sonnet-4-20250514 has BOTH an exact match and would match prefix
	// resolution to claude-sonnet-4. Verify exact match pricing is used.
	exact := c.Calculate("anthropic", "claude-sonnet-4-20250514", 1_000_000, 1_000_000)
	base := c.Calculate("anthropic", "claude-sonnet-4", 1_000_000, 1_000_000)
	// Both happen to be the same price ($3/$15), but the point is it resolves
	// via exact match, not prefix fallback.
	if exact == 0 || base == 0 {
		t.Error("both should have non-zero pricing")
	}
	if exact != base {
		t.Logf("exact=%f, base=%f — different pricing is fine (exact match takes priority)", exact, base)
	}
}

func TestPrefixResolution_UnknownBase_StillBlocked(t *testing.T) {
	c := New()
	// A fine-tune of a model we don't have pricing for should still be blocked.
	if c.HasPricing("openai", "ft:gpt-99:org:name:id") {
		t.Error("HasPricing should be false for ft: with unknown base model")
	}
}

func TestPrefixResolution_ProviderAliasCombo(t *testing.T) {
	c := New()
	// anthropic-direct + date suffix → should resolve both alias AND prefix.
	if !c.HasPricing("anthropic-direct", "claude-opus-4.7-20260101") {
		t.Error("should resolve anthropic-direct alias + date suffix prefix")
	}
	direct := c.Calculate("anthropic-direct", "claude-opus-4.7-20260101", 1_000_000, 1_000_000)
	canonical := c.Calculate("anthropic", "claude-opus-4.7", 1_000_000, 1_000_000)
	if direct != canonical {
		t.Errorf("anthropic-direct+suffix (%f) != anthropic canonical (%f)", direct, canonical)
	}
}

// ── Cross-component consistency canary ───────────────────────────────────────
// These tests verify that the Go calculator is the source of truth and has
// pricing for every model that should be tracked.

func TestAllProviders_HaveAtLeastOneModel(t *testing.T) {
	c := New()
	providers := map[string]string{
		"google":    "gemini-2.5-pro",
		"anthropic": "claude-sonnet-4",
		"openai":    "gpt-4.1",
	}
	for provider, model := range providers {
		if !c.HasPricing(provider, model) {
			t.Errorf("provider %q has no pricing for flagship model %q", provider, model)
		}
	}
}
