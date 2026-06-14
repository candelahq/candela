package proxy

import "testing"

func TestModelPolicy_NoPolicy(t *testing.T) {
	p := NewModelPolicy(nil)
	if !p.IsAllowed("openai", "gpt-4o") {
		t.Error("nil policy should allow all")
	}
}

func TestModelPolicy_AllowedModel(t *testing.T) {
	p := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "openai",
			Models:   []string{"gpt-4o", "gpt-4o-mini"},
		}},
	})
	if !p.IsAllowed("openai", "gpt-4o") {
		t.Error("should be allowed")
	}
	if p.IsAllowed("openai", "gpt-3.5-turbo") {
		t.Error("should be blocked")
	}
}

func TestModelPolicy_GlobPattern(t *testing.T) {
	p := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "anthropic",
			Models:   []string{"claude-sonnet-4-*"},
		}},
	})
	if !p.IsAllowed("anthropic", "claude-sonnet-4-20250514") {
		t.Error("glob should match")
	}
	if p.IsAllowed("anthropic", "claude-opus-4") {
		t.Error("should not match")
	}
}

func TestModelPolicy_UnknownProvider(t *testing.T) {
	p := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "openai",
			Models:   []string{"*"},
		}},
	})
	if p.IsAllowed("anthropic", "claude-sonnet-4") {
		t.Error("unlisted provider should be blocked")
	}
}

func TestModelPolicy_EmptyRules(t *testing.T) {
	p := NewModelPolicy(&PolicyConfig{})
	if !p.IsAllowed("openai", "gpt-4o") {
		t.Error("empty rules should allow all")
	}
}

func TestModelPolicy_CaseInsensitiveProvider(t *testing.T) {
	p := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "OpenAI",
			Models:   []string{"gpt-4o"},
		}},
	})
	if !p.IsAllowed("openai", "gpt-4o") {
		t.Error("provider match should be case-insensitive")
	}
	if !p.IsAllowed("OPENAI", "gpt-4o") {
		t.Error("provider match should be case-insensitive")
	}
}
