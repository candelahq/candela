package proxy

import "testing"

func TestModelPolicy_NoPolicy(t *testing.T) {
	p, err := NewModelPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsAllowed("openai", "gpt-4o") {
		t.Error("nil policy should allow all")
	}
}

func TestModelPolicy_AllowedModel(t *testing.T) {
	p, err := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "openai",
			Models:   []string{"gpt-4o", "gpt-4o-mini"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsAllowed("openai", "gpt-4o") {
		t.Error("should be allowed")
	}
	if p.IsAllowed("openai", "gpt-3.5-turbo") {
		t.Error("should be blocked")
	}
}

func TestModelPolicy_GlobPattern(t *testing.T) {
	p, err := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "anthropic",
			Models:   []string{"claude-sonnet-4-*"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsAllowed("anthropic", "claude-sonnet-4-20250514") {
		t.Error("glob should match")
	}
	if p.IsAllowed("anthropic", "claude-opus-4") {
		t.Error("should not match")
	}
}

func TestModelPolicy_UnknownProvider(t *testing.T) {
	p, err := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "openai",
			Models:   []string{"*"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.IsAllowed("anthropic", "claude-sonnet-4") {
		t.Error("unlisted provider should be blocked")
	}
}

func TestModelPolicy_EmptyRules(t *testing.T) {
	p, err := NewModelPolicy(&PolicyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsAllowed("openai", "gpt-4o") {
		t.Error("empty rules should allow all")
	}
}

func TestModelPolicy_CaseInsensitiveProvider(t *testing.T) {
	p, err := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "OpenAI",
			Models:   []string{"gpt-4o"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsAllowed("openai", "gpt-4o") {
		t.Error("provider match should be case-insensitive")
	}
	if !p.IsAllowed("OPENAI", "gpt-4o") {
		t.Error("provider match should be case-insensitive")
	}
}

func TestModelPolicy_SlashedModelID(t *testing.T) {
	p, err := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "huggingface",
			Models:   []string{"deepseek-ai/*"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsAllowed("huggingface", "deepseek-ai/deepseek-chat") {
		t.Error("glob should match across slashes")
	}
}

func TestModelPolicy_CaseInsensitive(t *testing.T) {
	p, err := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "OpenAI",
			Models:   []string{"GPT-4o"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsAllowed("openai", "gpt-4o") {
		t.Error("should be case-insensitive")
	}
}

func TestModelPolicy_EmptyModelsErrors(t *testing.T) {
	_, err := NewModelPolicy(&PolicyConfig{
		AllowedModels: []ProviderModels{{
			Provider: "openai",
			Models:   []string{},
		}},
	})
	if err == nil {
		t.Error("empty models list should error")
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"gpt-4o", "gpt-4o", true},
		{"gpt-4o", "gpt-4o-mini", false},
		{"claude-sonnet-4-*", "claude-sonnet-4-20250514", true},
		{"claude-sonnet-4-*", "claude-opus-4", false},
		{"deepseek-ai/*", "deepseek-ai/deepseek-chat", true},
		{"*", "anything", true},
		{"*", "org/model", true},
		{"prefix-*-suffix", "prefix-middle-suffix", true},
		{"prefix-*-suffix", "prefix-middle-other", false},
	}
	for _, tt := range tests {
		got := matchGlob(tt.pattern, tt.name)
		if got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}
