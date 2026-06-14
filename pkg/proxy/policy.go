package proxy

import (
	"fmt"
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
// Returns (nil, nil) if config is nil or has no rules (all models allowed).
// Returns an error if a provider has an empty models list.
func NewModelPolicy(cfg *PolicyConfig) (*ModelPolicy, error) {
	if cfg == nil || len(cfg.AllowedModels) == 0 {
		return nil, nil // no restrictions
	}
	p := &ModelPolicy{rules: make(map[string][]string)}
	for _, pm := range cfg.AllowedModels {
		if len(pm.Models) == 0 {
			return nil, fmt.Errorf("policy: provider %q has empty models list — use [\"*\"] to allow all", pm.Provider)
		}
		p.rules[strings.ToLower(pm.Provider)] = pm.Models
	}
	return p, nil
}

// IsAllowed checks if a model is permitted by the policy.
// Returns true if no policy is set (nil receiver).
// Uses matchGlob for glob pattern support (e.g. "claude-sonnet-4-*", "deepseek-ai/*").
func (p *ModelPolicy) IsAllowed(provider, model string) bool {
	if p == nil {
		return true // no policy = all allowed
	}
	patterns, ok := p.rules[strings.ToLower(provider)]
	if !ok {
		return false // provider not in allowlist
	}
	model = strings.ToLower(model)
	for _, pattern := range patterns {
		if matchGlob(strings.ToLower(pattern), model) {
			return true
		}
	}
	return false
}

// matchGlob performs glob matching where * matches any characters including /.
// This differs from path.Match which treats / as a separator.
func matchGlob(pattern, name string) bool {
	// Split pattern on * and check if name contains all parts in order.
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name // no wildcards, exact match
	}
	// Check prefix.
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	name = name[len(parts[0]):]
	// Check middle parts.
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(name, parts[i])
		if idx < 0 {
			return false
		}
		name = name[idx+len(parts[i]):]
	}
	// Check suffix.
	return strings.HasSuffix(name, parts[len(parts)-1])
}
