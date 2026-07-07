package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigWarnings_DeprecatedFields(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		wantWarnings int
		wantModes    []string // expected warning substrings
	}{
		{
			name: "no deprecated fields — no warnings",
			yaml: `
vertex_ai:
  project: test
  anthropic:
    caching_mode: auto
    cache_ttl: 1h
`,
			wantWarnings: 0,
		},
		{
			name: "deprecated caching_mode only",
			yaml: `
vertex_ai:
  project: test
  caching_mode: auto
`,
			wantWarnings: 1,
			wantModes:    []string{"caching_mode has moved"},
		},
		{
			name: "deprecated cache_ttl only",
			yaml: `
vertex_ai:
  project: test
  cache_ttl: 1h
`,
			wantWarnings: 1,
			wantModes:    []string{"cache_ttl has moved"},
		},
		{
			name: "deprecated prompt_caching bool",
			yaml: `
vertex_ai:
  project: test
  prompt_caching: true
`,
			wantWarnings: 1,
			wantModes:    []string{"prompt_caching has been removed"},
		},
		{
			name: "all three deprecated fields",
			yaml: `
vertex_ai:
  project: test
  caching_mode: off
  cache_ttl: 5m
  prompt_caching: false
`,
			wantWarnings: 3,
		},
		{
			name: "new + deprecated coexist — both parsed independently",
			yaml: `
vertex_ai:
  project: test
  caching_mode: off
  anthropic:
    caching_mode: auto
    cache_ttl: 1h
`,
			wantWarnings: 1,
			wantModes:    []string{"caching_mode has moved"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			if err := yaml.Unmarshal([]byte(tt.yaml), &cfg); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			warnings := configWarnings(cfg)
			if len(warnings) != tt.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(warnings), tt.wantWarnings, warnings)
			}

			for _, substr := range tt.wantModes {
				found := false
				for _, w := range warnings {
					if strings.Contains(w, substr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected warning containing %q, got %v", substr, warnings)
				}
			}
		})
	}
}

func TestConfigWarnings_NewFieldsNotAffected(t *testing.T) {
	// Verify that using the new structure populates Anthropic fields
	// and does NOT trigger deprecated field warnings.
	yamlInput := `
vertex_ai:
  project: test
  anthropic:
    caching_mode: system-only
    cache_ttl: 1h
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlInput), &cfg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if cfg.VertexAI.Anthropic.CachingMode != "system-only" {
		t.Errorf("Anthropic.CachingMode = %q, want %q", cfg.VertexAI.Anthropic.CachingMode, "system-only")
	}
	if cfg.VertexAI.Anthropic.CacheTTL != "1h" {
		t.Errorf("Anthropic.CacheTTL = %q, want %q", cfg.VertexAI.Anthropic.CacheTTL, "1h")
	}

	warnings := configWarnings(cfg)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for new config, got %d: %v", len(warnings), warnings)
	}
}
