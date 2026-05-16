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
// When a provider includes cached tokens in its total input count, these
// rates control how we normalize to cost-equivalent tokens.
type CacheDiscountConfig struct {
	ReadDiscount     float64 `yaml:"read_discount"`     // Multiplier for cache read tokens (e.g. 0.1 = 90% off)
	CreateMultiplier float64 `yaml:"create_multiplier"` // Multiplier for cache creation tokens (e.g. 1.25 = 25% surcharge)
}

// defaultCacheDiscounts defines cache pricing per canonical provider.
// Anthropic: 90% off reads, 25% surcharge on creation.
// Google/Gemini 2.5+: 90% off (model-aware, see googleCacheReadDiscount).
// Google/Gemini 2.0:  75% off (model-aware).
// OpenAI: 50% off reads, no creation concept.
var defaultCacheDiscounts = map[string]CacheDiscountConfig{
	"anthropic": {ReadDiscount: 0.1, CreateMultiplier: 1.25},
	"google":    {ReadDiscount: 0.10, CreateMultiplier: 1.0}, // Base rate, overridden by model-aware logic
	"openai":    {ReadDiscount: 0.5, CreateMultiplier: 1.0},
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
	"anthropic-direct": "anthropic",
	"anthropic-vertex": "anthropic",
	"gemini-oai":       "google", // Gemini via OpenAI-compat shares Google cache pricing
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

// NormalizeCachedInput returns cost-equivalent input tokens by applying
// provider-specific and model-specific cache discounts to raw token counts.
//
// All parsers return raw input token counts (including cached tokens at full
// price). This method adjusts them:
//   - Subtracts cached tokens from the total
//   - Adds them back at the discounted rate (e.g. 0.1× for 90% off)
//   - Applies cache creation surcharges (e.g. Anthropic's 1.25× creation cost)
//
// Provider aliases are resolved (e.g. "anthropic-vertex" → "anthropic",
// "gemini-oai" → "google"), and Google models get model-aware rates
// (Gemini 2.5+: 90% off, Gemini 2.0: 75% off).
//
// Returns rawInput unchanged when both cacheRead and cacheCreate are 0.
func (c *Calculator) NormalizeCachedInput(provider, model string, rawInput, cacheRead, cacheCreate int64) int64 {
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

	nonCached := rawInput - cacheRead - cacheCreate
	if nonCached < 0 {
		nonCached = 0
	}

	return nonCached +
		int64(math.Round(float64(cacheRead)*readDiscount)) +
		int64(math.Round(float64(cacheCreate)*cfg.CreateMultiplier))
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
// then precomputed provider-agnostic fallback.
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

	return ModelPricing{}, false
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
// can call through OpenAI, Google, or Anthropic must have a price here.
//
// Prices are list prices in USD per 1 million tokens (as of April 2026).
// For negotiated or discounted rates, use config overrides.
func (c *Calculator) loadDefaults() {
	defaults := []ModelPricing{
		// ── Google Gemini ─────────────────────────────────────────
		// Gemini 3.1 (latest)
		{Provider: "google", Model: "gemini-3.1-pro", InputPerMillion: 2.00, OutputPerMillion: 12.00},
		// Gemini 2.5
		{Provider: "google", Model: "gemini-2.5-pro", InputPerMillion: 1.25, OutputPerMillion: 10.00,
			InputPerMillionHigh: 2.50, OutputPerMillionHigh: 15.00, TierThresholdTokens: 200_000},
		{Provider: "google", Model: "gemini-2.5-flash", InputPerMillion: 0.30, OutputPerMillion: 2.50},
		{Provider: "google", Model: "gemini-2.5-flash-lite", InputPerMillion: 0.10, OutputPerMillion: 0.40},
		// Gemini 2.0
		{Provider: "google", Model: "gemini-2.0-flash", InputPerMillion: 0.10, OutputPerMillion: 0.40},
		{Provider: "google", Model: "gemini-2.0-pro", InputPerMillion: 1.25, OutputPerMillion: 10.00},
		// Gemini 1.5 (legacy)
		{Provider: "google", Model: "gemini-1.5-flash", InputPerMillion: 0.075, OutputPerMillion: 0.30},
		{Provider: "google", Model: "gemini-1.5-pro", InputPerMillion: 1.25, OutputPerMillion: 5.00},

		// ── OpenAI ───────────────────────────────────────────────
		// GPT-5.4 (latest, March 2026)
		{Provider: "openai", Model: "gpt-5.4-pro", InputPerMillion: 30.00, OutputPerMillion: 180.00},
		{Provider: "openai", Model: "gpt-5.4", InputPerMillion: 2.50, OutputPerMillion: 15.00},
		{Provider: "openai", Model: "gpt-5.4-mini", InputPerMillion: 0.75, OutputPerMillion: 4.50},
		{Provider: "openai", Model: "gpt-5.4-nano", InputPerMillion: 0.20, OutputPerMillion: 1.25},
		// GPT-4o
		{Provider: "openai", Model: "gpt-4o", InputPerMillion: 2.50, OutputPerMillion: 10.00},
		{Provider: "openai", Model: "gpt-4o-mini", InputPerMillion: 0.15, OutputPerMillion: 0.60},
		// GPT-4 (legacy)
		{Provider: "openai", Model: "gpt-4-turbo", InputPerMillion: 10.00, OutputPerMillion: 30.00},
		{Provider: "openai", Model: "gpt-3.5-turbo", InputPerMillion: 0.50, OutputPerMillion: 1.50},
		// Reasoning models
		{Provider: "openai", Model: "o3", InputPerMillion: 10.00, OutputPerMillion: 40.00},
		{Provider: "openai", Model: "o3-mini", InputPerMillion: 1.10, OutputPerMillion: 4.40},
		{Provider: "openai", Model: "o1", InputPerMillion: 15.00, OutputPerMillion: 60.00},
		{Provider: "openai", Model: "o1-mini", InputPerMillion: 3.00, OutputPerMillion: 12.00},

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
	}
	for _, p := range defaults {
		c.defaults[c.key(p.Provider, p.Model)] = p
	}
}
