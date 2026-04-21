// Package costcalc provides token-to-cost calculations for LLM API calls.
// It maintains a pricing table for common models and calculates costs from
// token counts. Pricing can be overridden via config for negotiated rates
// or enterprise discounts.
package costcalc

import (
	"log/slog"
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
}

// PricingConfig holds pricing configuration loaded from config.yaml.
type PricingConfig struct {
	DiscountPercent float64        `yaml:"discount_percent"` // Global discount (0.0–1.0)
	Models          []ModelPricing `yaml:"models"`           // Per-model overrides
}

// Calculator computes costs from token usage and model pricing.
type Calculator struct {
	mu             sync.RWMutex
	defaults       map[string]ModelPricing // key: "provider/model" — built-in list prices
	overrides      map[string]ModelPricing // key: "provider/model" — config overrides
	fallback       map[string]ModelPricing // key: "model" — deterministic name-only match
	globalDiscount float64                 // 0.0–1.0
	loggedUnknown  sync.Map                // key: "provider/model" — track logged warnings
}

// New creates a Calculator with default pricing for all supported cloud models.
func New() *Calculator {
	c := &Calculator{
		defaults:  make(map[string]ModelPricing),
		overrides: make(map[string]ModelPricing),
		fallback:  make(map[string]ModelPricing),
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

	inputCost := float64(inputTokens) / 1_000_000 * p.InputPerMillion
	outputCost := float64(outputTokens) / 1_000_000 * p.OutputPerMillion
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

	c.globalDiscount = cfg.DiscountPercent

	for _, p := range cfg.Models {
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

// SetGlobalDiscount sets the global discount percentage (0.0–1.0).
func (c *Calculator) SetGlobalDiscount(discount float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.globalDiscount = discount
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
func (c *Calculator) resolve(provider, model string) (ModelPricing, bool) {
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
		{Provider: "google", Model: "gemini-2.5-pro", InputPerMillion: 1.25, OutputPerMillion: 10.00},
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
		// Claude 4 (Vertex AI model IDs)
		{Provider: "anthropic", Model: "claude-sonnet-4-20250514", InputPerMillion: 3.00, OutputPerMillion: 15.00},
		{Provider: "anthropic", Model: "claude-opus-4-20250514", InputPerMillion: 15.00, OutputPerMillion: 75.00},
		// Claude 3.5 (legacy)
		{Provider: "anthropic", Model: "claude-3-5-sonnet-20241022", InputPerMillion: 3.00, OutputPerMillion: 15.00},
		{Provider: "anthropic", Model: "claude-haiku-3-5-20241022", InputPerMillion: 0.80, OutputPerMillion: 4.00},
		{Provider: "anthropic", Model: "claude-3-opus-20240229", InputPerMillion: 15.00, OutputPerMillion: 75.00},
	}
	for _, p := range defaults {
		c.defaults[c.key(p.Provider, p.Model)] = p
	}
}
