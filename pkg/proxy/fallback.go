package proxy

import (
	"sort"
	"strings"
)

// FallbackConfig controls automatic provider fallback for failed upstream
// requests. When enabled and a primary provider fails, the proxy tries
// alternate providers in the order defined by the matching chain.
type FallbackConfig struct {
	Enabled bool            `yaml:"enabled"`
	Chains  []FallbackChain `yaml:"chains"`
}

// FallbackChain maps a model-name prefix to an ordered list of provider names.
// When a request's model matches the prefix (case-insensitive), the chain
// defines which providers to try and in what order.
type FallbackChain struct {
	ModelPrefix string   `yaml:"model_prefix"` // e.g. "gpt-4", "claude-"
	Providers   []string `yaml:"providers"`    // ordered provider names
}

// FallbackResolver holds pre-indexed fallback chains and a provider lookup
// table. It is safe for concurrent use after construction (read-only).
type FallbackResolver struct {
	chains    []FallbackChain
	providers map[string]*Provider
	enabled   bool
}

// NewFallbackResolver creates a resolver from the given config and available
// providers. The provider slice is indexed by name for O(1) lookups.
// Chains are sorted by prefix length descending so more-specific prefixes
// (e.g. "claude-sonnet-4") match before less-specific ones (e.g. "claude-").
func NewFallbackResolver(cfg FallbackConfig, providers []*Provider) *FallbackResolver {
	pm := make(map[string]*Provider, len(providers))
	for _, p := range providers {
		pm[strings.ToLower(p.Name)] = p
	}

	// Sort chains by prefix length descending for most-specific-first matching.
	chains := make([]FallbackChain, len(cfg.Chains))
	copy(chains, cfg.Chains)
	sort.Slice(chains, func(i, j int) bool {
		return len(chains[i].ModelPrefix) > len(chains[j].ModelPrefix)
	})

	return &FallbackResolver{
		chains:    chains,
		providers: pm,
		enabled:   cfg.Enabled,
	}
}

// FallbackProviders returns the providers to try after the primary provider
// has failed. The returned slice preserves chain ordering and excludes the
// primary.
//
// If the primary is not found in the matching chain, all chain providers are
// returned (the caller already tried the primary outside the chain).
//
// Returns nil when fallback is disabled, no chains are configured, or no
// chain matches the model.
func (r *FallbackResolver) FallbackProviders(model, primaryName string) []*Provider {
	if !r.enabled {
		return nil
	}
	if len(r.chains) == 0 {
		return nil
	}

	modelLower := strings.ToLower(model)
	primaryLower := strings.ToLower(primaryName)

	for _, chain := range r.chains {
		prefix := strings.ToLower(chain.ModelPrefix)
		if !strings.HasPrefix(modelLower, prefix) {
			continue
		}

		// Find primary's position in the chain.
		primaryIdx := -1
		for i, name := range chain.Providers {
			if strings.ToLower(name) == primaryLower {
				primaryIdx = i
				break
			}
		}

		var result []*Provider
		if primaryIdx < 0 {
			// Primary not in chain — return all chain providers.
			for _, name := range chain.Providers {
				if p, ok := r.providers[strings.ToLower(name)]; ok {
					result = append(result, p)
				}
			}
		} else {
			// Return providers after primary in chain order.
			for i := primaryIdx + 1; i < len(chain.Providers); i++ {
				if p, ok := r.providers[strings.ToLower(chain.Providers[i])]; ok {
					result = append(result, p)
				}
			}
		}

		if len(result) == 0 {
			return nil
		}
		return result
	}

	return nil
}
