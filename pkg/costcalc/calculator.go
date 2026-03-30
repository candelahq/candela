// Package costcalc provides token-to-cost calculations for LLM API calls.
// It maintains a pricing table for common models and calculates costs from
// token counts.
package costcalc

import (
	"strings"
	"sync"
)

// ModelPricing defines the per-token pricing for a model.
type ModelPricing struct {
	Model           string  `yaml:"model" json:"model"`
	Provider        string  `yaml:"provider" json:"provider"`
	InputPerMillion float64 `yaml:"input_per_million" json:"input_per_million"`   // USD per 1M input tokens
	OutputPerMillion float64 `yaml:"output_per_million" json:"output_per_million"` // USD per 1M output tokens
}

// Calculator computes costs from token usage and model pricing.
type Calculator struct {
	mu      sync.RWMutex
	pricing map[string]ModelPricing // key: "provider/model"
}

// New creates a Calculator with default pricing for common models.
func New() *Calculator {
	c := &Calculator{
		pricing: make(map[string]ModelPricing),
	}
	c.loadDefaults()
	return c
}

// Calculate returns the estimated cost in USD for the given model and token counts.
func (c *Calculator) Calculate(provider, model string, inputTokens, outputTokens int64) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.key(provider, model)
	p, ok := c.pricing[key]
	if !ok {
		// Try matching just by model name (provider-agnostic)
		for k, v := range c.pricing {
			if strings.HasSuffix(k, "/"+model) {
				p = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return 0 // Unknown model, can't calculate cost
	}

	inputCost := float64(inputTokens) / 1_000_000 * p.InputPerMillion
	outputCost := float64(outputTokens) / 1_000_000 * p.OutputPerMillion
	return inputCost + outputCost
}

// SetPricing adds or updates pricing for a model.
func (c *Calculator) SetPricing(p ModelPricing) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pricing[c.key(p.Provider, p.Model)] = p
}

func (c *Calculator) key(provider, model string) string {
	return strings.ToLower(provider + "/" + model)
}

func (c *Calculator) loadDefaults() {
	defaults := []ModelPricing{
		// Google
		{Provider: "google", Model: "gemini-2.0-flash", InputPerMillion: 0.10, OutputPerMillion: 0.40},
		{Provider: "google", Model: "gemini-2.0-pro", InputPerMillion: 1.25, OutputPerMillion: 10.00},
		{Provider: "google", Model: "gemini-1.5-flash", InputPerMillion: 0.075, OutputPerMillion: 0.30},
		{Provider: "google", Model: "gemini-1.5-pro", InputPerMillion: 1.25, OutputPerMillion: 5.00},

		// OpenAI
		{Provider: "openai", Model: "gpt-4o", InputPerMillion: 2.50, OutputPerMillion: 10.00},
		{Provider: "openai", Model: "gpt-4o-mini", InputPerMillion: 0.15, OutputPerMillion: 0.60},
		{Provider: "openai", Model: "gpt-4-turbo", InputPerMillion: 10.00, OutputPerMillion: 30.00},
		{Provider: "openai", Model: "gpt-3.5-turbo", InputPerMillion: 0.50, OutputPerMillion: 1.50},
		{Provider: "openai", Model: "o1", InputPerMillion: 15.00, OutputPerMillion: 60.00},
		{Provider: "openai", Model: "o1-mini", InputPerMillion: 3.00, OutputPerMillion: 12.00},

		// Anthropic
		{Provider: "anthropic", Model: "claude-3.5-sonnet", InputPerMillion: 3.00, OutputPerMillion: 15.00},
		{Provider: "anthropic", Model: "claude-3.5-haiku", InputPerMillion: 0.80, OutputPerMillion: 4.00},
		{Provider: "anthropic", Model: "claude-3-opus", InputPerMillion: 15.00, OutputPerMillion: 75.00},
	}
	for _, p := range defaults {
		c.pricing[c.key(p.Provider, p.Model)] = p
	}
}
