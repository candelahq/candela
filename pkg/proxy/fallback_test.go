package proxy

import (
	"testing"
)

func makeFallbackProviders(names ...string) []*Provider {
	providers := make([]*Provider, len(names))
	for i, n := range names {
		providers[i] = &Provider{Name: n}
	}
	return providers
}

func TestFallbackDisabledReturnsNil(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: false,
		Chains: []FallbackChain{
			{ModelPrefix: "gpt-4", Providers: []string{"openai", "azure"}},
		},
	}, makeFallbackProviders("openai", "azure"))

	if got := r.FallbackProviders("gpt-4-turbo", "openai"); got != nil {
		t.Errorf("disabled resolver returned %v, want nil", got)
	}
}

func TestFallbackNoChainsReturnsNil(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{Enabled: true}, makeFallbackProviders("openai"))

	if got := r.FallbackProviders("gpt-4", "openai"); got != nil {
		t.Errorf("no-chains resolver returned %v, want nil", got)
	}
}

func TestFallbackMatchingChainReturnsFallbacks(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: true,
		Chains: []FallbackChain{
			{ModelPrefix: "gpt-4", Providers: []string{"openai", "azure", "anyscale"}},
		},
	}, makeFallbackProviders("openai", "azure", "anyscale"))

	got := r.FallbackProviders("gpt-4-turbo", "openai")
	if len(got) != 2 {
		t.Fatalf("expected 2 fallbacks, got %d", len(got))
	}
	if got[0].Name != "azure" || got[1].Name != "anyscale" {
		t.Errorf("expected [azure, anyscale], got [%s, %s]", got[0].Name, got[1].Name)
	}
}

func TestFallbackCaseInsensitive(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: true,
		Chains: []FallbackChain{
			{ModelPrefix: "GPT-4", Providers: []string{"OpenAI", "Azure"}},
		},
	}, makeFallbackProviders("openai", "azure"))

	got := r.FallbackProviders("gpt-4-turbo", "OPENAI")
	if len(got) != 1 || got[0].Name != "azure" {
		t.Errorf("case-insensitive match failed, got %v", got)
	}
}

func TestFallbackPrimaryNotInChain(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: true,
		Chains: []FallbackChain{
			{ModelPrefix: "gpt-4", Providers: []string{"azure", "anyscale"}},
		},
	}, makeFallbackProviders("openai", "azure", "anyscale"))

	// "openai" is not in the chain — should return all chain providers.
	got := r.FallbackProviders("gpt-4-turbo", "openai")
	if len(got) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(got))
	}
	if got[0].Name != "azure" || got[1].Name != "anyscale" {
		t.Errorf("expected [azure, anyscale], got [%s, %s]", got[0].Name, got[1].Name)
	}
}

func TestFallbackPrimaryIsLast(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: true,
		Chains: []FallbackChain{
			{ModelPrefix: "gpt-4", Providers: []string{"azure", "openai"}},
		},
	}, makeFallbackProviders("openai", "azure"))

	got := r.FallbackProviders("gpt-4-turbo", "openai")
	if got != nil {
		t.Errorf("primary-is-last should return nil, got %v", got)
	}
}

func TestFallbackMissingProviderSkipped(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: true,
		Chains: []FallbackChain{
			{ModelPrefix: "gpt-4", Providers: []string{"openai", "ghost", "azure"}},
		},
	}, makeFallbackProviders("openai", "azure")) // "ghost" not registered

	got := r.FallbackProviders("gpt-4-turbo", "openai")
	if len(got) != 1 || got[0].Name != "azure" {
		t.Errorf("expected [azure] (ghost skipped), got %v", got)
	}
}

func TestFallbackNoMatchReturnsNil(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: true,
		Chains: []FallbackChain{
			{ModelPrefix: "gpt-4", Providers: []string{"openai", "azure"}},
		},
	}, makeFallbackProviders("openai", "azure"))

	if got := r.FallbackProviders("claude-3-opus", "openai"); got != nil {
		t.Errorf("no-match should return nil, got %v", got)
	}
}

func TestFallbackMultipleChainsFirstMatch(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: true,
		Chains: []FallbackChain{
			{ModelPrefix: "gpt-4", Providers: []string{"openai", "azure"}},
			{ModelPrefix: "gpt-4", Providers: []string{"openai", "anyscale"}},
		},
	}, makeFallbackProviders("openai", "azure", "anyscale"))

	got := r.FallbackProviders("gpt-4-turbo", "openai")
	if len(got) != 1 || got[0].Name != "azure" {
		t.Errorf("expected first chain match [azure], got %v", got)
	}
}

func TestFallbackEmptyProviders(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: true,
		Chains: []FallbackChain{
			{ModelPrefix: "gpt-4", Providers: []string{}},
		},
	}, makeFallbackProviders("openai"))

	if got := r.FallbackProviders("gpt-4-turbo", "openai"); got != nil {
		t.Errorf("empty providers should return nil, got %v", got)
	}
}

// TestFallbackPrefixLengthSorting verifies that more-specific prefixes match
// before less-specific ones when chains overlap (e.g. "claude-sonnet-4" vs "claude-").
func TestFallbackPrefixLengthSorting(t *testing.T) {
	r := NewFallbackResolver(FallbackConfig{
		Enabled: true,
		Chains: []FallbackChain{
			// Less-specific chain listed first in config.
			{ModelPrefix: "claude-", Providers: []string{"anthropic", "generic-fallback"}},
			// More-specific chain listed second.
			{ModelPrefix: "claude-sonnet-4", Providers: []string{"anthropic", "anthropic-vertex"}},
		},
	}, makeFallbackProviders("anthropic", "anthropic-vertex", "generic-fallback"))

	// "claude-sonnet-4-1234" should match the more-specific "claude-sonnet-4" chain.
	got := r.FallbackProviders("claude-sonnet-4-1234", "anthropic")
	if len(got) != 1 || got[0].Name != "anthropic-vertex" {
		names := make([]string, len(got))
		for i, p := range got {
			names[i] = p.Name
		}
		t.Errorf("expected [anthropic-vertex] from specific chain, got %v", names)
	}

	// "claude-haiku" should match the less-specific "claude-" chain.
	got2 := r.FallbackProviders("claude-haiku", "anthropic")
	if len(got2) != 1 || got2[0].Name != "generic-fallback" {
		names := make([]string, len(got2))
		for i, p := range got2 {
			names[i] = p.Name
		}
		t.Errorf("expected [generic-fallback] from generic chain, got %v", names)
	}
}
