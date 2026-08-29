// Package costcalc provides token-to-cost calculations for LLM API calls.
// It maintains a pricing table for common models and calculates costs from
// token counts. Pricing can be overridden via config for negotiated rates
// or enterprise discounts.
package costcalc

import (
	_ "embed"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/catalog"
	"gopkg.in/yaml.v3"
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

	// Time-based pricing for self-hosted models (e.g. Cloud Run GPU).
	// When set, cost = request_duration_seconds × PerSecondUSD.
	// Used instead of per-token pricing for infrastructure-cost models.
	PerSecondUSD float64 `yaml:"per_second_usd,omitempty" json:"per_second_usd,omitempty"`
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
	unknownMu      sync.Mutex                     // guards loggedUnknown
	loggedUnknown  map[string]bool                // key: "provider/model" — bounded to maxLoggedUnknown (#654)
}

// maxLoggedUnknown caps the number of unique unknown model keys tracked
// to prevent unbounded memory growth under enumeration attacks (#654).
const maxLoggedUnknown = 1000

//go:embed pricing.yaml
var defaultPricingYAML []byte

// pricingFile is the schema for pricing.yaml, used only during YAML unmarshalling.
type pricingFile struct {
	Models []struct {
		Provider             string  `yaml:"provider"`
		Model                string  `yaml:"model"`
		InputPerMillion      float64 `yaml:"input_per_million"`
		OutputPerMillion     float64 `yaml:"output_per_million"`
		InputPerMillionHigh  float64 `yaml:"input_per_million_high,omitempty"`
		OutputPerMillionHigh float64 `yaml:"output_per_million_high,omitempty"`
		TierThresholdTokens  int64   `yaml:"tier_threshold_tokens,omitempty"`
		DiscountPercent      float64 `yaml:"discount_percent,omitempty"`
	} `yaml:"models"`
}

// providerAliases maps proxy route names to their canonical pricing provider.
// This ensures that passthrough routes (e.g. anthropic-direct) share pricing
// with their canonical provider, including config overrides and cache discounts.
var providerAliases = map[string]string{
	"anthropic-direct":  "anthropic",
	"anthropic-vertex":  "anthropic",
	"anthropic-bedrock": "anthropic",
	"gemini-oai":        "google", // Gemini via OpenAI-compat shares Google cache pricing
	"gemini-vertex":     "google", // Native Gemini via Vertex AI shares Google pricing
}

// New creates a Calculator with default pricing for all supported cloud models.
func New() *Calculator {
	c := newBase()
	c.loadDefaults()
	c.rebuildFallback()
	return c
}

// NewEmpty creates a Calculator with no built-in model pricing.
// Use this when pricing will be loaded from a database or catalog at runtime
// rather than from the embedded pricing.yaml. Cache discount defaults and
// provider aliases are still initialized.
func NewEmpty() *Calculator {
	return newBase()
}

// LoadDefaults loads embedded pricing from pricing.yaml into the calculator.
// This is useful when a database-backed catalog is unavailable and the caller
// needs to fall back to built-in list prices. Safe to call on a Calculator
// created with NewEmpty(). Existing config overrides are preserved.
func (c *Calculator) LoadDefaults() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadDefaults()
	c.rebuildFallback()
}

// newBase creates a Calculator with initialized maps, provider aliases, and
// default cache discounts, but no model pricing loaded.
func newBase() *Calculator {
	c := &Calculator{
		defaults:       make(map[string]ModelPricing),
		overrides:      make(map[string]ModelPricing),
		fallback:       make(map[string]ModelPricing),
		aliases:        providerAliases,
		cacheDiscounts: make(map[string]CacheDiscountConfig),
		loggedUnknown:  make(map[string]bool),
	}
	// Copy default cache discounts.
	for k, v := range defaultCacheDiscounts {
		c.cacheDiscounts[k] = v
	}
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
		c.unknownMu.Lock()
		if c.loggedUnknown == nil {
			c.loggedUnknown = make(map[string]bool)
		}
		if !c.loggedUnknown[key] && len(c.loggedUnknown) < maxLoggedUnknown {
			c.loggedUnknown[key] = true
			c.unknownMu.Unlock()
			slog.Warn("⚠️ missing pricing for cloud model — cost will be $0.00 (inaccurate)",
				"provider", provider,
				"model", model,
				"input_tokens", inputTokens,
				"output_tokens", outputTokens,
			)
		} else {
			c.unknownMu.Unlock()
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

// CalculateTimeBased returns cost based on request duration for self-hosted models.
// Used when PerSecondUSD is set (e.g. Cloud Run GPU at ~$0.000466/sec for L4).
func (c *Calculator) CalculateTimeBased(provider, model string, duration time.Duration) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.resolve(provider, model)
	if !ok || p.PerSecondUSD == 0 {
		return 0
	}
	// Clamp negative durations to zero (clock drift protection).
	if duration < 0 {
		duration = 0
	}
	cost := duration.Seconds() * p.PerSecondUSD
	if p.DiscountPercent > 0 {
		cost *= (1 - p.DiscountPercent)
	}
	if c.globalDiscount > 0 {
		cost *= (1 - c.globalDiscount)
	}
	return cost
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

// LoadFromCatalog loads pricing from a ModelCatalogStore.
// This replaces the built-in defaults with catalog entries,
// preserving any config overrides that were loaded separately.
func (c *Calculator) LoadFromCatalog(entries []catalog.Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear and rebuild defaults from catalog entries.
	c.defaults = make(map[string]ModelPricing)
	for _, e := range entries {
		if !e.Enabled {
			continue
		}
		c.defaults[c.key(e.Provider, e.ModelID)] = ModelPricing{
			Model:                e.ModelID,
			Provider:             e.Provider,
			InputPerMillion:      e.InputPerMillion,
			OutputPerMillion:     e.OutputPerMillion,
			DiscountPercent:      clampDiscount(e.DiscountPercent),
			InputPerMillionHigh:  e.InputPerMillionHigh,
			OutputPerMillionHigh: e.OutputPerMillionHigh,
			TierThresholdTokens:  e.TierThresholdTokens,
		}
	}
	c.rebuildFallback()
	slog.Info("📦 catalog pricing loaded", "models", len(c.defaults))
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

// Models returns all known model pricing entries (defaults merged with overrides).
// Overrides take priority over defaults on key conflicts. The returned slice is
// sorted by provider, then model name. This is used to populate the /v1/models
// endpoint so clients can discover available models.
func (c *Calculator) Models() []ModelPricing {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Merge defaults and overrides into a single map (overrides win).
	merged := make(map[string]ModelPricing, len(c.defaults)+len(c.overrides))
	for k, v := range c.defaults {
		merged[k] = v
	}
	for k, v := range c.overrides {
		merged[k] = v
	}

	// Collect into a slice.
	models := make([]ModelPricing, 0, len(merged))
	for _, m := range merged {
		models = append(models, m)
	}

	// Sort by provider, then model.
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].Model < models[j].Model
	})

	return models
}

// Defaults returns a copy of the built-in default pricing table.
// This does not include config overrides — only the compiled-in list prices.
// The returned slice is sorted by provider, then model name for deterministic output.
func (c *Calculator) Defaults() []ModelPricing {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]ModelPricing, 0, len(c.defaults))
	for _, p := range c.defaults {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Provider != result[j].Provider {
			return result[i].Provider < result[j].Provider
		}
		return result[i].Model < result[j].Model
	})
	return result
}

// clampDiscount ensures a discount is within [0.0, 1.0].
// NaN is treated as 0 (no discount) — NaN compares false for all ordered
// comparisons, so without this check it would pass through and corrupt
// every cost calculation downstream (baseCost *= (1 - NaN) → NaN).
func clampDiscount(d float64) float64 {
	if math.IsNaN(d) || d < 0 {
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

// Resolve returns the resolved pricing for a model without calculating cost.
// Returns the resolved ModelPricing and true if found, or zero value and false.
// This is used to snapshot pricing at request time for cost auditing.
func (c *Calculator) Resolve(provider, model string) (ModelPricing, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.resolve(provider, model)
}

// ResolveEffective returns the pricing that would be used for the given token counts,
// accounting for tiered pricing. This captures the actual rates for cost auditing.
// When the input context exceeds the tier threshold, the high-tier rates are returned
// in InputPerMillion/OutputPerMillion so the snapshot reflects what was actually charged.
func (c *Calculator) ResolveEffective(provider, model string, inputTokens int64) (ModelPricing, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.resolve(provider, model)
	if !ok {
		return p, false
	}
	// Apply tiered pricing selection (same logic as Calculate).
	if p.TierThresholdTokens > 0 && inputTokens > p.TierThresholdTokens {
		if p.InputPerMillionHigh > 0 {
			p.InputPerMillion = p.InputPerMillionHigh
		}
		if p.OutputPerMillionHigh > 0 {
			p.OutputPerMillion = p.OutputPerMillionHigh
		}
	}
	return p, true
}

// GlobalDiscount returns the current global discount percentage (0.0–1.0).
func (c *Calculator) GlobalDiscount() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.globalDiscount
}

// lookupCandidate performs the three-tier pricing lookup (overrides → defaults
// → provider-agnostic fallback) for a single candidate model name. Extracted
// to avoid repeating this pattern across every resolution step.
func (c *Calculator) lookupCandidate(provider, candidate string) (ModelPricing, bool) {
	key := c.key(provider, candidate)
	if p, ok := c.overrides[key]; ok {
		return p, true
	}
	if p, ok := c.defaults[key]; ok {
		return p, true
	}
	if p, ok := c.fallback[strings.ToLower(candidate)]; ok {
		return p, true
	}
	return ModelPricing{}, false
}

// resolve looks up pricing: config overrides first, then built-in defaults,
// then precomputed provider-agnostic fallback, then model ID normalization
// (hyphen→dot version suffix), then prefix-based fuzzy match.
// Provider aliases (e.g. "anthropic-direct" → "anthropic") are resolved before
// lookup so passthrough routes inherit canonical pricing and config overrides.
func (c *Calculator) resolve(provider, model string) (ModelPricing, bool) {
	// Resolve provider alias (e.g. "anthropic-direct" → "anthropic").
	if canonical, ok := c.aliases[strings.ToLower(provider)]; ok {
		provider = canonical
	}

	// 1-3. Exact match: config override → built-in default → fallback.
	if p, ok := c.lookupCandidate(provider, model); ok {
		return p, true
	}

	// 4. Model ID normalization: try replacing the last hyphen-digit with
	// dot-digit. This handles Vertex AI format (claude-opus-4-7) mapping
	// to our canonical dotted entries (claude-opus-4.7) without duplicating
	// every pricing entry.
	if normalized := normalizeModelID(model); normalized != model {
		if p, ok := c.lookupCandidate(provider, normalized); ok {
			return p, true
		}
	}

	// 5. Prefix-based fuzzy match for model variants.
	// Handles: date suffixes (gpt-4.1-2025-04-14), preview tags
	// (gemini-2.5-pro-preview-05-06), and OpenAI fine-tunes (ft:gpt-4.1:org:name:id).
	if base := extractBaseModel(model); base != "" && base != strings.ToLower(model) {
		if p, ok := c.lookupCandidate(provider, base); ok {
			return p, true
		}

		// 5b. Normalize the extracted base (hyphen→dot) for Vertex AI
		// dated IDs. Example chain:
		//   "claude-haiku-4-5-20251001" → strip date → "claude-haiku-4-5"
		//   → normalize → "claude-haiku-4.5" → matches pricing.yaml.
		if normBase := normalizeModelID(base); normBase != base {
			if p, ok := c.lookupCandidate(provider, normBase); ok {
				return p, true
			}
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

	// Strip Vertex AI MaaS publisher prefixes (e.g. "deepseek-ai/deepseek-v3.2-maas")
	if i := strings.LastIndex(m, "/"); i >= 0 {
		// Return prefix-stripped only. Don't also strip -maas here,
		// since the pricing key may include -maas (e.g. "deepseek-v3.2-maas").
		return m[i+1:]
	}

	// Strip Vertex AI MaaS "-maas" suffix (e.g. "deepseek-v3.2-maas" → "deepseek-v3.2")
	if strings.HasSuffix(m, "-maas") {
		return strings.TrimSuffix(m, "-maas")
	}

	// OpenAI fine-tune format: ft:{base_model}:{org}:{name}:{id}
	if strings.HasPrefix(m, "ft:") {
		parts := strings.SplitN(m, ":", 3)
		if len(parts) >= 2 && parts[1] != "" {
			return parts[1]
		}
	}

	// Strip common trailing tags.
	for _, suffix := range []string{"-latest", "-stable"} {
		if strings.HasSuffix(m, suffix) {
			return strings.TrimSuffix(m, suffix)
		}
	}

	// Strip -preview* suffix (e.g. "-preview-05-06", "-preview").
	// Use hasSuffixTag to avoid false positives on model names that
	// contain "-preview" mid-name (e.g. a hypothetical "preview-model").
	if idx := hasSuffixTag(m, "-preview"); idx > 0 {
		return m[:idx]
	}

	// Strip -exp* suffix (e.g. "-exp-0827", "-exp").
	// Same word-boundary guard as -preview above.
	if idx := hasSuffixTag(m, "-exp"); idx > 0 {
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

// hasSuffixTag finds a tag like "-preview" or "-exp" in model name m and returns
// the index of the tag, but only if the tag appears as a word-boundary suffix
// (i.e. it's at the end of the string or followed by a hyphen). This prevents
// false positives on model names that contain the tag mid-name (e.g. a
// hypothetical "text-expander-3b" should NOT match "-exp").
//
// Scans right-to-left so that "text-expander-flash-exp" correctly matches the
// trailing "-exp" rather than the mid-word "-exp" in "expander".
//
// Returns the index of the tag in m, or -1 if not found in a valid position.
func hasSuffixTag(m, tag string) int {
	// Scan backwards through all occurrences.
	for end := len(m); end > 0; {
		idx := strings.LastIndex(m[:end], tag)
		if idx <= 0 {
			return -1
		}
		rest := m[idx+len(tag):]
		// Valid positions: tag is at end of string, or followed by "-" (sub-suffix).
		if rest == "" || rest[0] == '-' {
			return idx
		}
		// This occurrence is mid-word; keep scanning left.
		end = idx
	}
	return -1
}

// normalizeModelID converts trailing hyphen-digit version suffixes to dot-digit.
// This handles Vertex AI model IDs (claude-opus-4-7) mapping to our canonical
// dotted format (claude-opus-4.7) without breaking legitimate hyphens like
// claude-3-opus or claude-3-5-sonnet.
//
// Strategy: replace only the last hyphen when it's followed by a single digit,
// which indicates a minor version number rather than a model family prefix.
func normalizeModelID(model string) string {
	idx := strings.LastIndex(model, "-")
	if idx > 0 && idx < len(model)-1 {
		suffix := model[idx+1:]
		// Single digit version suffix (not a date like 20250514)
		// AND the character before the hyphen is also a digit,
		// so we only convert "4-7" → "4.7", not "opus-4" → "opus.4".
		if len(suffix) == 1 && suffix[0] >= '0' && suffix[0] <= '9' &&
			model[idx-1] >= '0' && model[idx-1] <= '9' {
			return model[:idx] + "." + suffix
		}
	}
	return model
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

// loadDefaults populates built-in list prices from the embedded pricing.yaml.
// This should be exhaustive — every model a user can call through the Candela
// proxy must have a price in pricing.yaml.
//
// Prices are list prices in USD per 1 million tokens (as of May 2026).
// For negotiated or discounted rates, use config overrides.
func (c *Calculator) loadDefaults() {
	var pf pricingFile
	if err := yaml.Unmarshal(defaultPricingYAML, &pf); err != nil {
		panic(fmt.Sprintf("costcalc: failed to parse embedded pricing.yaml: %v", err))
	}
	for i, m := range pf.Models {
		if m.Provider == "" || m.Model == "" {
			panic(fmt.Sprintf("costcalc: pricing.yaml entry %d: provider and model are required", i))
		}
		if m.InputPerMillion <= 0 || m.OutputPerMillion < 0 {
			panic(fmt.Sprintf("costcalc: pricing.yaml entry %d (%s/%s): input price must be > 0, output price must be >= 0", i, m.Provider, m.Model))
		}
	}
	for _, m := range pf.Models {
		c.defaults[c.key(m.Provider, m.Model)] = ModelPricing{
			Provider:             m.Provider,
			Model:                m.Model,
			InputPerMillion:      m.InputPerMillion,
			OutputPerMillion:     m.OutputPerMillion,
			InputPerMillionHigh:  m.InputPerMillionHigh,
			OutputPerMillionHigh: m.OutputPerMillionHigh,
			TierThresholdTokens:  m.TierThresholdTokens,
			DiscountPercent:      m.DiscountPercent,
		}
	}
}
