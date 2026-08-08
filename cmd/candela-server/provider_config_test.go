package main

import (
	"testing"

	"github.com/candelahq/candela/pkg/proxy"
)

// TestAllDefaultProviders_HaveServerConfig verifies that every provider returned
// by proxy.DefaultProviders() is accounted for in the server's declarative
// provider configuration (provider_config.go). This prevents providers from
// being added to DefaultProviders() without the corresponding server wiring.
//
// The source of truth is allKnownProviders() which is composed from
// vertexAIProviders and nonVertexProviders — the same maps that main.go uses
// for PathRewriter setup, project-ID fallback filtering, and override
// validation. Adding a provider to the test map alone won't help — you must
// add it to the actual config that main.go consumes.
func TestAllDefaultProviders_HaveServerConfig(t *testing.T) {
	known := allKnownProviders()

	for _, p := range proxy.DefaultProviders() {
		if !known[p.Name] {
			t.Errorf("provider %q is in DefaultProviders() but not in server config (provider_config.go) — "+
				"add it to vertexAIProviders, maaSProviderRegion (if MaaS), or nonVertexProviders", p.Name)
		}
	}

	// Reverse check: ensure server config doesn't have stale entries.
	defaultNames := make(map[string]bool)
	for _, p := range proxy.DefaultProviders() {
		defaultNames[p.Name] = true
	}
	for name := range known {
		if !defaultNames[name] {
			t.Errorf("provider %q is in server config (provider_config.go) but no longer in DefaultProviders() — remove it", name)
		}
	}
}

// TestMaaSProviders_InVertexAIProviders verifies that every MaaS provider
// (from maaSProviderRegion) is also in vertexAIProviders, ensuring the
// no-project-ID filter correctly disables them.
func TestMaaSProviders_InVertexAIProviders(t *testing.T) {
	for name := range maaSProviderRegion {
		if !vertexAIProviders[name] {
			t.Errorf("MaaS provider %q is in maaSProviderRegion but not in vertexAIProviders — it won't be disabled when project ID is missing", name)
		}
	}
}
