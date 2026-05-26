// Package costcalc provides token-to-cost calculations for LLM API calls.
// It maintains a pricing table for common models and calculates costs from
// token counts. Pricing can be overridden via config for negotiated rates
// or enterprise discounts.
package costcalc

import (
	"log/slog"
	"math"
	"strings"
	"sync"
)

// ModelPricing defines the per-token pricing for a model.
type ModelPricing struct {
	Model            string  `yaml:"model" json:"model"`
	Provider         string  `yaml:"provider" json:"provider"`
	InputPerMillion  float64 `yaml:"input_per_million" json:"input_per_million"`                   // USD per 1M input tokens
	OutputPerMillion float64 `yaml:"output_per_million" json:"output_per_million"`                 // USD per 1M output tokens
	DiscountPercent  float64 `yaml:"discount_percent,omitempty" json:"discount_percent,omitempty"` // 0.0–1.0, model-specific discount

	// Tiered pricing: some models (e.g. Gemini 2.5 Pro) charge higher rates
	// when the input context exceeds a threshold. If TierThresholdTokens > 0
	// and inputTokens > TierThresholdTokens, the high-tier rates are used.
	// Zero values mean "no tiered pricing — use base rates for all contexts."
	InputPerMillionHigh  float64 `yaml:"input_per_million_high,omitempty" json:"input_per_million_high,omitempty"`
	OutputPerMillionHigh float64 `yaml:"output_per_million_high,omitempty" json:"output_per_million_high,omitempty"`
	TierThresholdTokens  int64   `yaml:"tier_threshold_tokens,omitempty" json:"tier_threshold_tokens,omitempty"`
}

// PricingConfig holds pricing configuration loaded from config.yaml.
type PricingConfig struct {
	DiscountPercent float64        `yaml:"discount_percent"` // Global discount (0.0–1.0)
	Models          []ModelPricing `yaml:"models"`           // Per-model overrides
}

// CacheDiscountConfig defines the cache token discount rates for a provider.
// These rates control how we normalize cached tokens to cost-equivalent tokens.
type CacheDiscountConfig struct {
	ReadDiscount       float64 `yaml:"read_discount"`        // Multiplier for cache read tokens (e.g. 0.1 = 90% off)
	CreateMultiplier   float64 `yaml:"create_multiplier"`    // Multiplier for cache creation tokens (e.g. 1.25 = 25% surcharge)
	InputIncludesCache bool    `yaml:"input_includes_cache"` // True if rawInput already includes cache tokens (OpenAI/Google), false if they're additive (Anthropic)
}

// Anthropic cache creation multipliers by TTL tier.
// See: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching#pricing
const (
	// anthropicDefaultCacheCreateMultiplier is the cache creation rate for
	// the default 5-minute TTL: 1.25× base input price.
	anthropicDefaultCacheCreateMultiplier = 1.25

	// anthropicExtendedCacheCreateMultiplier is the cache creation rate for
	// the extended 1-hour TTL: 2.0× base input price.
	anthropicExtendedCacheCreateMultiplier = 2.0
)

// defaultCacheDiscounts defines cache pricing per canonical provider.
//
// Token reporting semantics differ by provider:
//   - OpenAI:    prompt_tokens includes cached_tokens (inclusive)
//   - Google:    promptTokenCount includes cachedContentTokenCount (inclusive)
//   - Anthropic: input_tokens is ONLY fresh tokens; cache_read/creation are additive
//
// Anthropic cache creation pricing is TTL-dependent:
//   - 5-minute TTL (default): 1.25× base input price
//   - 1-hour TTL (opt-in):    2.0×  base input price
//
// The default multiplier here is for 5-minute TTL. Use
// NormalizeCachedInputWithTTL with extendedTTL=true for 1-hour TTL.
//
// See: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching#tracking-cache-performance
//
//	"total_input_tokens = cache_read_input_tokens + cache_creation_input_tokens + input_tokens"
var defaultCacheDiscounts = map[string]CacheDiscountConfig{
	"anthropic": {ReadDiscount: 0.1, CreateMultiplier: anthropicDefaultCacheCreateMultiplier, InputIncludesCache: false},
	"google":    {ReadDiscount: 0.10, CreateMultiplier: 1.0, InputIncludesCache: true}, // Base rate, overridden by model-aware logic
	"openai":    {ReadDiscount: 0.5, CreateMultiplier: 1.0, InputIncludesCache: true},
}

// Calculator computes costs from token usage and model pricing.
type Calculator struct {
	mu             sync.RWMutex
	defaults       map[string]ModelPricing        // key: "provider/model" — built-in list prices
	overrides      map[string]ModelPricing        // key: "provider/model" — config overrides
	fallback       map[string]ModelPricing        // key: "model" — deterministic name-only match
	aliases        map[string]string              // provider name aliases (e.g. "anthropic-direct" → "anthropic")
	cacheDiscounts map[string]CacheDiscountConfig // key: canonical provider name
	globalDiscount float64                        // 0.0–1.0
	loggedUnknown  sync.Map                       // key: "provider/model" — track logged warnings
}

// providerAliases maps proxy route names to their canonical pricing provider.
// This ensures that passthrough routes (e.g. anthropic-direct) share pricing
// with their canonical provider, including config overrides and cache discounts.
var providerAliases = map[string]string{
	"anthropic-direct":  "anthropic",
	"anthropic-vertex":  "anthropic",
	"anthropic-bedrock": "anthropic",
	"gemini-oai":        "google", // Gemini via OpenAI-compat shares Google cache pricing
}

// New creates a Calculator with default pricing for all supported cloud models.
func New() *Calculator {
	c := &Calculator{
		defaults:       make(map[string]ModelPricing),
		overrides:      make(map[string]ModelPricing),
		fallback:       make(map[string]ModelPricing),
		aliases:        providerAliases,
		cacheDiscounts: make(map[string]CacheDiscountConfig),
	}
	// Copy default cache discounts.
	for k, v := range defaultCacheDiscounts {
		c.cacheDiscounts[k] = v
	}
	c.loadDefaults()
	c.rebuildFallback()
	return c
}

// Calculate returns the estimated cost in USD for the given model and token counts.
// Local models always return $0.00. Unknown cloud models log a warning (once) and return $0.00.
func (c *Calculator) Calculate(provider, model string, inputTokens, outputTokens int64) float64 {
	// Local models run on your hardware — no API cost.
	if strings.ToLower(provider) == "local" {
		return 0
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	p, ok := c.resolve(provider, model)
	if !ok {
		key := c.key(provider, model)
		if _, alreadyLogged := c.loggedUnknown.LoadOrStore(key, true); !alreadyLogged {
			slog.Warn("⚠️ missing pricing for cloud model — cost will be $0.00 (inaccurate)",
				"provider", provider,
				"model", model,
				"input_tokens", inputTokens,
				"output_tokens", outputTokens,
			)
		}
		return 0 // Unknown model — this is a gap, not a feature
	}

	// Select pricing tier. Models with TierThresholdTokens > 0 charge higher
	// rates when the prompt exceeds that threshold (e.g. Gemini 2.5 Pro >200K).
	inputRate := p.InputPerMillion
	outputRate := p.OutputPerMillion
	if p.TierThresholdTokens > 0 && inputTokens > p.TierThresholdTokens {
		if p.InputPerMillionHigh > 0 {
			inputRate = p.InputPerMillionHigh
		}
		if p.OutputPerMillionHigh > 0 {
			outputRate = p.OutputPerMillionHigh
		}
	}

	inputCost := float64(inputTokens) / 1_000_000 * inputRate
	outputCost := float64(outputTokens) / 1_000_000 * outputRate
	baseCost := inputCost + outputCost

	// Apply model-level discount, then global discount.
	if p.DiscountPercent > 0 {
		baseCost *= (1 - p.DiscountPercent)
	}
	if c.globalDiscount > 0 {
		baseCost *= (1 - c.globalDiscount)
	}

	return baseCost
}

// LoadFromConfig applies pricing overrides from configuration.
// Config overrides take priority over built-in defaults.
func (c *Calculator) LoadFromConfig(cfg PricingConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.globalDiscount = clampDiscount(cfg.DiscountPercent)

	for _, p := range cfg.Models {
		p.DiscountPercent = clampDiscount(p.DiscountPercent)
		c.overrides[c.key(p.Provider, p.Model)] = p
	}

	c.rebuildFallback()

	if cfg.DiscountPercent > 0 {
		slog.Info("💰 global pricing discount applied",
			"discount", cfg.DiscountPercent)
	}
	if len(cfg.Models) > 0 {
		slog.Info("💰 pricing overrides loaded",
			"count", len(cfg.Models))
	}
}

// SetPricing adds or updates pricing for a model (runtime override).
func (c *Calculator) SetPricing(p ModelPricing) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.overrides[c.key(p.Provider, p.Model)] = p
	c.rebuildFallback()
}

// SetCacheDiscount overrides the cache discount config for a canonical provider.
// Use this for providers with non-standard cache pricing (e.g. Anthropic on
// Vertex AI if Google charges different cache rates than direct Anthropic).
func (c *Calculator) SetCacheDiscount(provider string, cfg CacheDiscountConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheDiscounts[strings.ToLower(provider)] = cfg
}

// GetCacheDiscount returns the runtime-overridden cache discount config for a
// canonical provider, if one has been set via SetCacheDiscount. Returns false
// if only the default (model-aware) logic is active.
func (c *Calculator) GetCacheDiscount(provider string) (CacheDiscountConfig, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cfg, ok := c.cacheDiscounts[strings.ToLower(provider)]
	return cfg, ok
}

// NormalizeCachedInput returns cost-equivalent input tokens by applying
// provider-specific and model-specific cache discounts to raw token counts.
//
// Token reporting differs by provider:
//   - Inclusive (OpenAI/Google): rawInput includes cache tokens. We subtract
//     them and re-add at the discounted rate.
//   - Additive (Anthropic): rawInput is ONLY fresh tokens. Cache tokens are
//     separate. We add them at the discounted rate on top of rawInput.
//
// Provider aliases are resolved (e.g. "anthropic-vertex" → "anthropic",
// "gemini-oai" → "google"), and Google models get model-aware rates
// (Gemini 2.5+: 90% off, Gemini 2.0: 75% off).
//
// Returns rawInput unchanged when both cacheRead and cacheCreate are 0.
//
// NOTE: This method assumes the default 5-minute Anthropic cache TTL (1.25×
// creation rate). For 1-hour TTL (2.0× creation rate), use
// NormalizeCachedInputWithTTL with extendedTTL=true.
func (c *Calculator) NormalizeCachedInput(provider, model string, rawInput, cacheRead, cacheCreate int64) int64 {
	return c.NormalizeCachedInputWithTTL(provider, model, rawInput, cacheRead, cacheCreate, false)
}

// NormalizeCachedInputWithTTL returns cost-equivalent input tokens, with
// TTL-aware cache creation pricing for Anthropic.
//
// When extendedTTL is true and the provider is Anthropic (including aliases
// like anthropic-direct, anthropic-vertex), the cache creation multiplier
// is 2.0× instead of the default 1.25×. This matches Anthropic's pricing
// for 1-hour cache TTL entries.
//
// For non-Anthropic providers, extendedTTL is ignored (they don't have
// TTL-dependent creation pricing).
//
// See: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching#pricing
func (c *Calculator) NormalizeCachedInputWithTTL(provider, model string, rawInput, cacheRead, cacheCreate int64, extendedTTL bool) int64 {
	if cacheRead <= 0 && cacheCreate <= 0 {
		return rawInput
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Resolve provider alias (e.g. "gemini-oai" → "google").
	canonical := strings.ToLower(provider)
	if alias, ok := c.aliases[canonical]; ok {
		canonical = alias
	}

	cfg, ok := c.cacheDiscounts[canonical]
	if !ok {
		// Unknown provider — no cache discount info, return raw.
		return rawInput
	}

	// Google/Gemini models have model-aware cache discount rates by default.
	// Only apply if the config hasn't been overridden via SetCacheDiscount.
	readDiscount := cfg.ReadDiscount
	defaultGoogleCfg := defaultCacheDiscounts["google"]
	if canonical == "google" && cfg.ReadDiscount == defaultGoogleCfg.ReadDiscount {
		readDiscount = googleCacheReadDiscount(model)
	}

	// Anthropic 1-hour TTL charges 2.0× for cache creation (vs 1.25× for 5m).
	// Override the multiplier when the caller signals extended TTL.
	createMultiplier := cfg.CreateMultiplier
	if extendedTTL && canonical == "anthropic" {
		createMultiplier = anthropicExtendedCacheCreateMultiplier
	}

	cachedReadEq := int64(math.Round(float64(cacheRead) * readDiscount))
	cachedCreateEq := int64(math.Round(float64(cacheCreate) * createMultiplier))

	if cfg.InputIncludesCache {
		// Inclusive mode (OpenAI, Google): rawInput already contains cache tokens.
		// Subtract them out, then re-add at discounted rate.
		nonCached := rawInput - cacheRead - cacheCreate
		if nonCached < 0 {
			nonCached = 0
		}
		return nonCached + cachedReadEq + cachedCreateEq
	}

	// Additive mode (Anthropic): rawInput is ONLY fresh tokens.
	// Cache tokens are separate — just add their discounted equivalents.
	return rawInput + cachedReadEq + cachedCreateEq
}

// googleCacheReadDiscount returns the cache read discount for a Google/Gemini
// model. Per GEAP pricing (May 2026):
//   - Gemini 2.5+ and 3.x models: 90% off (0.10×)
//   - Gemini 2.0 and older:       75% off (0.25×)
func googleCacheReadDiscount(model string) float64 {
	m := strings.ToLower(model)
	if strings.Contains(m, "gemini-2.0") ||
		strings.Contains(m, "gemini-1.5") ||
		strings.Contains(m, "gemini-1.0") {
		return 0.25
	}
	return 0.10
}

// SetGlobalDiscount sets the global discount percentage (0.0–1.0).
func (c *Calculator) SetGlobalDiscount(discount float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.globalDiscount = clampDiscount(discount)
}

// clampDiscount ensures a discount is within [0.0, 1.0].
func clampDiscount(d float64) float64 {
	if d < 0 {
		return 0
	}
	if d > 1 {
		return 1
	}
	return d
}

// HasPricing returns true if a provider/model has pricing configured
// (either via config override or built-in default). Local models always
// return true since they are free by definition.
func (c *Calculator) HasPricing(provider, model string) bool {
	if strings.ToLower(provider) == "local" {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.resolve(provider, model)
	return ok
}

// resolve looks up pricing: config overrides first, then built-in defaults,
// then precomputed provider-agnostic fallback, then prefix-based fuzzy match.
// Provider aliases (e.g. "anthropic-direct" → "anthropic") are resolved before
// lookup so passthrough routes inherit canonical pricing and config overrides.
func (c *Calculator) resolve(provider, model string) (ModelPricing, bool) {
	// Resolve provider alias (e.g. "anthropic-direct" → "anthropic").
	if canonical, ok := c.aliases[strings.ToLower(provider)]; ok {
		provider = canonical
	}

	key := c.key(provider, model)

	// 1. Config override (exact match)
	if p, ok := c.overrides[key]; ok {
		return p, true
	}

	// 2. Built-in default (exact match)
	if p, ok := c.defaults[key]; ok {
		return p, true
	}

	// 3. Precomputed provider-agnostic fallback (deterministic)
	if p, ok := c.fallback[strings.ToLower(model)]; ok {
		return p, true
	}

	// 4. Prefix-based fuzzy match for model variants.
	// Handles: date suffixes (gpt-4.1-2025-04-14), preview tags
	// (gemini-2.5-pro-preview-05-06), and OpenAI fine-tunes (ft:gpt-4.1:org:name:id).
	if base := extractBaseModel(model); base != "" && base != strings.ToLower(model) {
		baseKey := c.key(provider, base)
		if p, ok := c.overrides[baseKey]; ok {
			return p, true
		}
		if p, ok := c.defaults[baseKey]; ok {
			return p, true
		}
		if p, ok := c.fallback[base]; ok {
			return p, true
		}
	}

	return ModelPricing{}, false
}

// extractBaseModel strips common suffixes and prefixes from model names to find
// the canonical base model for pricing lookup. Returns lowercase base or empty
// string if no transformation applies.
//
// Handles:
//   - Date suffixes: "gpt-4.1-2025-04-14" → "gpt-4.1"
//   - Preview/exp tags: "gemini-2.5-pro-preview-05-06" → "gemini-2.5-pro"
//   - Snapshot suffixes: "claude-sonnet-4-20250514" → "claude-sonnet-4"
//   - OpenAI fine-tunes: "ft:gpt-4.1:org:custom:id" → "gpt-4.1"
//   - Latest/stable tags: "gpt-4.1-latest" → "gpt-4.1"
func extractBaseModel(model string) string {
	m := strings.ToLower(model)

	// OpenAI fine-tune format: ft:{base_model}:{org}:{name}:{id}
	if strings.HasPrefix(m, "ft:") {
		parts := strings.SplitN(m, ":", 3)
		if len(parts) >= 2 {
			return parts[1]
		}
	}

	// Strip common trailing tags.
	for _, suffix := range []string{"-latest", "-stable"} {
		if strings.HasSuffix(m, suffix) {
			return strings.TrimSuffix(m, suffix)
		}
	}

	// Strip -preview* suffix (e.g. "-preview-05-06", "-preview")
	if idx := strings.Index(m, "-preview"); idx > 0 {
		return m[:idx]
	}

	// Strip -exp* suffix (e.g. "-exp-0827")
	if idx := strings.Index(m, "-exp"); idx > 0 {
		return m[:idx]
	}

	// Strip date suffixes: 8-digit (YYYYMMDD) or ISO-like (YYYY-MM-DD).
	// Match "-20250514" or "-2025-04-14" at the end.
	if len(m) > 9 {
		// "-YYYYMMDD" (9 chars)
		tail := m[len(m)-9:]
		if tail[0] == '-' && isAllDigits(tail[1:]) {
			return m[:len(m)-9]
		}
	}
	if len(m) > 11 {
		// "-YYYY-MM-DD" (11 chars)
		tail := m[len(m)-11:]
		if tail[0] == '-' && len(tail) == 11 && tail[5] == '-' && tail[8] == '-' {
			stripped := strings.ReplaceAll(tail[1:], "-", "")
			if isAllDigits(stripped) {
				return m[:len(m)-11]
			}
		}
	}

	return ""
}

// isAllDigits returns true if s is non-empty and contains only ASCII digits.
func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// rebuildFallback creates a deterministic lookup for model names without providers.
// Priority: Overrides > Defaults. Tie-breaker: Alphabetical provider.
//
// IMPORTANT: This MUST be called while holding a write lock on c.mu.
func (c *Calculator) rebuildFallback() {
	c.fallback = make(map[string]ModelPricing)

	// Sort providers alphabetically to ensure deterministic selection when multiple
	// providers offer the same model name.
	process := func(source map[string]ModelPricing) {
		// Group by model name
		grouped := make(map[string][]ModelPricing)
		for _, p := range source {
			m := strings.ToLower(p.Model)
			grouped[m] = append(grouped[m], p)
		}

		// For each model, select the first provider alphabetically
		for m, ps := range grouped {
			best := ps[0]
			for _, p := range ps[1:] {
				if strings.ToLower(p.Provider) < strings.ToLower(best.Provider) {
					best = p
				}
			}
			// Don't overwrite if a higher-priority source (e.g. Overrides) already set it.
			if _, exists := c.fallback[m]; !exists {
				c.fallback[m] = best
			}
		}
	}

	// Process Overrides first, then Defaults.
	process(c.overrides)
	process(c.defaults)
}

func (c *Calculator) key(provider, model string) string {
	return strings.ToLower(provider + "/" + model)
}

// loadDefaults populates built-in list prices for all cloud models reachable
// through the Candela proxy. This should be exhaustive — every model a user
// can call through Google or Anthropic must have a price here.
//
// Prices are list prices in USD per 1 million tokens (as of May 2026).
// For negotiated or discounted rates, use config overrides.
func (c *Calculator) loadDefaults() {
	defaults := []ModelPricing{
		// ── Google Gemini ─────────────────────────────────────────
		// Gemini 3.5 (latest, May 2026)
		{Provider: "google", Model: "gemini-3.5-flash", InputPerMillion: 1.50, OutputPerMillion: 9.00},
		// Gemini 3.1
		{Provider: "google", Model: "gemini-3.1-pro", InputPerMillion: 2.00, OutputPerMillion: 12.00,
			InputPerMillionHigh: 4.00, OutputPerMillionHigh: 18.00, TierThresholdTokens: 200_000},
		{Provider: "google", Model: "gemini-3.1-flash-lite", InputPerMillion: 0.25, OutputPerMillion: 1.50},
		// Gemini 3
		{Provider: "google", Model: "gemini-3-flash", InputPerMillion: 0.50, OutputPerMillion: 3.00},
		{Provider: "google", Model: "gemini-3-flash-lite", InputPerMillion: 0.02, OutputPerMillion: 0.10},
		// Gemini 2.5
		{Provider: "google", Model: "gemini-2.5-pro", InputPerMillion: 1.25, OutputPerMillion: 10.00,
			InputPerMillionHigh: 2.50, OutputPerMillionHigh: 15.00, TierThresholdTokens: 200_000},
		{Provider: "google", Model: "gemini-2.5-flash", InputPerMillion: 0.30, OutputPerMillion: 2.50},
		{Provider: "google", Model: "gemini-2.5-flash-lite", InputPerMillion: 0.10, OutputPerMillion: 0.40},
		// Gemini 2.0
		{Provider: "google", Model: "gemini-2.0-flash", InputPerMillion: 0.10, OutputPerMillion: 0.40},
		// Gemini 1.5 (legacy)
		{Provider: "google", Model: "gemini-1.5-flash", InputPerMillion: 0.075, OutputPerMillion: 0.30},
		{Provider: "google", Model: "gemini-1.5-pro", InputPerMillion: 1.25, OutputPerMillion: 5.00},

		// ── Anthropic (via Vertex AI or direct) ──────────────────
		// Claude 4.6/4.7 (latest)
		{Provider: "anthropic", Model: "claude-opus-4.7", InputPerMillion: 5.00, OutputPerMillion: 25.00},
		{Provider: "anthropic", Model: "claude-opus-4.6", InputPerMillion: 5.00, OutputPerMillion: 25.00},
		{Provider: "anthropic", Model: "claude-sonnet-4.6", InputPerMillion: 3.00, OutputPerMillion: 15.00},
		{Provider: "anthropic", Model: "claude-haiku-4.5", InputPerMillion: 1.00, OutputPerMillion: 5.00},
		// Claude 4 (short names — used by editors and Claude Code)
		{Provider: "anthropic", Model: "claude-sonnet-4", InputPerMillion: 3.00, OutputPerMillion: 15.00},
		{Provider: "anthropic", Model: "claude-opus-4", InputPerMillion: 5.00, OutputPerMillion: 25.00},
		// Claude 4 (Vertex AI model IDs with date suffix)
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514", InputPerMillion: 3.00, OutputPerMillion: 15.00},
		{Provider: "anthropic", Model: "claude-opus-4-20250514", InputPerMillion: 5.00, OutputPerMillion: 25.00},
		// Claude 3.5 (legacy)
		{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022", InputPerMillion: 3.00, OutputPerMillion: 15.00},
		{Provider: "anthropic", Model: "claude-haiku-3-5-20241022", InputPerMillion: 0.80, OutputPerMillion: 4.00},
		{Provider: "anthropic", Model: "claude-3-opus-20240229", InputPerMillion: 15.00, OutputPerMillion: 75.00},

		// ── OpenAI ───────────────────────────────────────────────
		// GPT-4.1 family (current flagship, 1M context)
		{Provider: "openai", Model: "gpt-4.1", InputPerMillion: 2.00, OutputPerMillion: 8.00},
		{Provider: "openai", Model: "gpt-4.1-mini", InputPerMillion: 0.40, OutputPerMillion: 1.60},
		{Provider: "openai", Model: "gpt-4.1-nano", InputPerMillion: 0.10, OutputPerMillion: 0.40},
		// o-series reasoning models
		{Provider: "openai", Model: "o3", InputPerMillion: 2.00, OutputPerMillion: 8.00},
		{Provider: "openai", Model: "o4-mini", InputPerMillion: 1.10, OutputPerMillion: 4.40},
		// GPT-4o (legacy)
		{Provider: "openai", Model: "gpt-4o", InputPerMillion: 2.50, OutputPerMillion: 10.00},
		{Provider: "openai", Model: "gpt-4o-mini", InputPerMillion: 0.15, OutputPerMillion: 0.60},
	}
	for _, p := range defaults {
		c.defaults[c.key(p.Provider, p.Model)] = p
	}
}
