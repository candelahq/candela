package proxy

import (
	"path"
	"strings"
)

// PolicyConfig defines model access rules.
type PolicyConfig struct {
	AllowedModels []ProviderModels `yaml:"allowed_models" json:"allowed_models"`
}

// ProviderModels defines allowed model patterns for a specific provider.
type ProviderModels struct {
	Provider string   `yaml:"provider" json:"provider"`
	Models   []string `yaml:"models" json:"models"`
}

// ModelPolicy evaluates model access rules against an allowlist.
// A nil ModelPolicy allows all models (no restrictions).
type ModelPolicy struct {
	rules map[string][]string // provider (lowercase) → model glob patterns
}

// NewModelPolicy creates a policy from config.
// Returns nil if config is nil or has no rules (all models allowed).
func NewModelPolicy(cfg *PolicyConfig) *ModelPolicy {
	if cfg == nil || len(cfg.AllowedModels) == 0 {
		return nil // no restrictions
	}
	p := &ModelPolicy{rules: make(map[string][]string)}
	for _, pm := range cfg.AllowedModels {
		p.rules[strings.ToLower(pm.Provider)] = pm.Models
	}
	return p
}

// IsAllowed checks if a model is permitted by the policy.
// Returns true if no policy is set (nil receiver).
// Uses path.Match for glob pattern support (e.g. "claude-sonnet-4-*").
func (p *ModelPolicy) IsAllowed(provider, model string) bool {
	if p == nil {
		return true // no policy = all allowed
	}
	patterns, ok := p.rules[strings.ToLower(provider)]
	if !ok {
		return false // provider not in allowlist
	}
	for _, pattern := range patterns {
		if matched, _ := path.Match(pattern, model); matched {
			return true
		}
	}
	return false
}
