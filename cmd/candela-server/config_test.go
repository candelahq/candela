package main

import (
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/proxy"
	"gopkg.in/yaml.v3"
)

// ── Config Parsing Tests ─────────────────────────────────────────────────────

func TestProviderOverride_YAMLParsing(t *testing.T) {
	raw := `
proxy:
  vertex_ai:
    project_id: "my-project"
    region: "us-east5"
    provider_overrides:
      mistral:
        region: "us-central1"
      deepseek:
        region: "global"
        endpoint: "https://custom-endpoint.example.com"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if cfg.Proxy.VertexAI.ProjectID != "my-project" {
		t.Errorf("project_id = %q, want %q", cfg.Proxy.VertexAI.ProjectID, "my-project")
	}
	if cfg.Proxy.VertexAI.Region != "us-east5" {
		t.Errorf("region = %q, want %q", cfg.Proxy.VertexAI.Region, "us-east5")
	}

	// Mistral override — region only.
	mistral, ok := cfg.Proxy.VertexAI.ProviderOverrides["mistral"]
	if !ok {
		t.Fatal("mistral override not found")
	}
	if mistral.Region != "us-central1" {
		t.Errorf("mistral.Region = %q, want %q", mistral.Region, "us-central1")
	}
	if mistral.Endpoint != "" {
		t.Errorf("mistral.Endpoint = %q, want empty", mistral.Endpoint)
	}

	// DeepSeek override — region + endpoint.
	deepseek, ok := cfg.Proxy.VertexAI.ProviderOverrides["deepseek"]
	if !ok {
		t.Fatal("deepseek override not found")
	}
	if deepseek.Region != "global" {
		t.Errorf("deepseek.Region = %q, want %q", deepseek.Region, "global")
	}
	if deepseek.Endpoint != "https://custom-endpoint.example.com" {
		t.Errorf("deepseek.Endpoint = %q, want %q", deepseek.Endpoint, "https://custom-endpoint.example.com")
	}
}

func TestProviderOverride_EmptyMap(t *testing.T) {
	raw := `
proxy:
  vertex_ai:
    project_id: "my-project"
    region: "us-central1"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if cfg.Proxy.VertexAI.ProviderOverrides != nil {
		t.Errorf("ProviderOverrides should be nil when not specified, got %v",
			cfg.Proxy.VertexAI.ProviderOverrides)
	}
}

func TestProviderOverride_EmptyOverrideBlock(t *testing.T) {
	raw := `
proxy:
  vertex_ai:
    project_id: "my-project"
    region: "us-east5"
    provider_overrides:
      mistral: {}
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	mistral, ok := cfg.Proxy.VertexAI.ProviderOverrides["mistral"]
	if !ok {
		t.Fatal("mistral override key not found")
	}
	if mistral.Region != "" {
		t.Errorf("empty block should have empty region, got %q", mistral.Region)
	}
	if mistral.Endpoint != "" {
		t.Errorf("empty block should have empty endpoint, got %q", mistral.Endpoint)
	}
}

func TestProviderOverride_OnlyEndpoint(t *testing.T) {
	raw := `
proxy:
  vertex_ai:
    project_id: "my-project"
    region: "us-east5"
    provider_overrides:
      mistral:
        endpoint: "https://my-mistral-proxy.example.com"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	mistral, ok := cfg.Proxy.VertexAI.ProviderOverrides["mistral"]
	if !ok {
		t.Fatal("mistral override not found")
	}
	if mistral.Region != "" {
		t.Errorf("region should be empty, got %q", mistral.Region)
	}
	if mistral.Endpoint != "https://my-mistral-proxy.example.com" {
		t.Errorf("endpoint = %q, want %q", mistral.Endpoint, "https://my-mistral-proxy.example.com")
	}
}

func TestProviderOverride_UnknownProviderIgnored(t *testing.T) {
	raw := `
proxy:
  vertex_ai:
    project_id: "my-project"
    provider_overrides:
      llama:
        region: "mars-west1"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	// Unknown provider keys parse fine — they're just unused.
	llama, ok := cfg.Proxy.VertexAI.ProviderOverrides["llama"]
	if !ok {
		t.Fatal("llama override not found in map")
	}
	if llama.Region != "mars-west1" {
		t.Errorf("llama.Region = %q, want %q", llama.Region, "mars-west1")
	}
}

func TestProviderOverride_MultipleProviders(t *testing.T) {
	raw := `
proxy:
  vertex_ai:
    project_id: "my-project"
    region: "us-east5"
    provider_overrides:
      mistral:
        region: "us-central1"
      deepseek:
        region: "global"
      qwen:
        region: "global"
        endpoint: "https://qwen-endpoint.example.com"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("YAML unmarshal failed: %v", err)
	}

	if len(cfg.Proxy.VertexAI.ProviderOverrides) != 3 {
		t.Fatalf("expected 3 overrides, got %d", len(cfg.Proxy.VertexAI.ProviderOverrides))
	}

	cases := []struct {
		provider string
		region   string
		endpoint string
	}{
		{"mistral", "us-central1", ""},
		{"deepseek", "global", ""},
		{"qwen", "global", "https://qwen-endpoint.example.com"},
	}
	for _, tc := range cases {
		ov, ok := cfg.Proxy.VertexAI.ProviderOverrides[tc.provider]
		if !ok {
			t.Errorf("%s override not found", tc.provider)
			continue
		}
		if ov.Region != tc.region {
			t.Errorf("%s.Region = %q, want %q", tc.provider, ov.Region, tc.region)
		}
		if ov.Endpoint != tc.endpoint {
			t.Errorf("%s.Endpoint = %q, want %q", tc.provider, ov.Endpoint, tc.endpoint)
		}
	}
}

// ── getProviderOverride Tests ────────────────────────────────────────────────
// These test the real getProviderOverride() helper extracted in main.go.

func TestGetProviderOverride_MistralDefault(t *testing.T) {
	region, endpoint := getProviderOverride(nil, "mistral", "us-central1")
	if region != "us-central1" {
		t.Errorf("default region = %q, want %q", region, "us-central1")
	}
	if endpoint != "" {
		t.Errorf("default endpoint = %q, want empty", endpoint)
	}
}

func TestGetProviderOverride_EmptyMap(t *testing.T) {
	region, endpoint := getProviderOverride(map[string]ProviderOverride{}, "mistral", "us-central1")
	if region != "us-central1" {
		t.Errorf("empty map region = %q, want %q", region, "us-central1")
	}
	if endpoint != "" {
		t.Errorf("empty map endpoint = %q, want empty", endpoint)
	}
}

func TestGetProviderOverride_RegionOverride(t *testing.T) {
	region, _ := getProviderOverride(map[string]ProviderOverride{
		"mistral": {Region: "europe-west1"},
	}, "mistral", "us-central1")
	if region != "europe-west1" {
		t.Errorf("override region = %q, want %q", region, "europe-west1")
	}
}

func TestGetProviderOverride_EmptyRegionFallsToDefault(t *testing.T) {
	region, _ := getProviderOverride(map[string]ProviderOverride{
		"mistral": {Region: ""},
	}, "mistral", "us-central1")
	if region != "us-central1" {
		t.Errorf("empty region = %q, want %q (default)", region, "us-central1")
	}
}

func TestGetProviderOverride_OtherProviderDoesNotAffect(t *testing.T) {
	region, _ := getProviderOverride(map[string]ProviderOverride{
		"deepseek": {Region: "us-east5"},
	}, "mistral", "us-central1")
	if region != "us-central1" {
		t.Errorf("unrelated override = %q, want %q (default)", region, "us-central1")
	}
}

func TestGetProviderOverride_EndpointOnly(t *testing.T) {
	region, endpoint := getProviderOverride(map[string]ProviderOverride{
		"mistral": {Endpoint: "https://custom.example.com"},
	}, "mistral", "us-central1")
	if region != "us-central1" {
		t.Errorf("region = %q, want %q (default when no region override)", region, "us-central1")
	}
	if endpoint != "https://custom.example.com" {
		t.Errorf("endpoint = %q, want %q", endpoint, "https://custom.example.com")
	}
}

func TestGetProviderOverride_BothRegionAndEndpoint(t *testing.T) {
	region, endpoint := getProviderOverride(map[string]ProviderOverride{
		"mistral": {
			Region:   "europe-west1",
			Endpoint: "https://custom.example.com",
		},
	}, "mistral", "us-central1")
	if region != "europe-west1" {
		t.Errorf("region = %q, want %q", region, "europe-west1")
	}
	if endpoint != "https://custom.example.com" {
		t.Errorf("endpoint = %q, want %q", endpoint, "https://custom.example.com")
	}
}

func TestGetProviderOverride_DeepSeekDefaultGlobal(t *testing.T) {
	for _, provider := range []string{"deepseek", "qwen"} {
		region, _ := getProviderOverride(nil, provider, "global")
		if region != "global" {
			t.Errorf("%s default = %q, want %q", provider, region, "global")
		}
	}
}

func TestGetProviderOverride_DeepSeekOverride(t *testing.T) {
	region, _ := getProviderOverride(map[string]ProviderOverride{
		"deepseek": {Region: "us-central1"},
	}, "deepseek", "global")
	if region != "us-central1" {
		t.Errorf("deepseek override = %q, want %q", region, "us-central1")
	}
}

func TestGetProviderOverride_QwenEmptyFallsToDefault(t *testing.T) {
	region, _ := getProviderOverride(map[string]ProviderOverride{
		"qwen": {Region: ""},
	}, "qwen", "global")
	if region != "global" {
		t.Errorf("empty region = %q, want %q", region, "global")
	}
}

// ── Production Config File Test ──────────────────────────────────────────────

func TestProductionExampleConfig_HasMistralOverride(t *testing.T) {
	raw := `
proxy:
  enabled: true
  vertex_ai:
    project_id: "austin-azra-sandbox-project"
    region: "global"
    provider_overrides:
      mistral:
        region: "us-central1"
  providers:
    - mistral
    - deepseek
    - qwen
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("production example YAML failed to parse: %v", err)
	}

	mistral, ok := cfg.Proxy.VertexAI.ProviderOverrides["mistral"]
	if !ok {
		t.Fatal("mistral override not found")
	}
	if mistral.Region != "us-central1" {
		t.Errorf("production config mistral region = %q, want %q",
			mistral.Region, "us-central1")
	}
}

// ── ProviderOverride Struct Tests ────────────────────────────────────────────

func TestProviderOverride_ZeroValue(t *testing.T) {
	var ov ProviderOverride
	if ov.Region != "" {
		t.Errorf("zero value Region = %q, want empty", ov.Region)
	}
	if ov.Endpoint != "" {
		t.Errorf("zero value Endpoint = %q, want empty", ov.Endpoint)
	}
}

func TestProviderOverride_YAMLRoundTrip(t *testing.T) {
	original := ProviderOverride{
		Region:   "europe-west1",
		Endpoint: "https://vertex.example.com",
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ProviderOverride
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Region != original.Region {
		t.Errorf("round-trip Region = %q, want %q", decoded.Region, original.Region)
	}
	if decoded.Endpoint != original.Endpoint {
		t.Errorf("round-trip Endpoint = %q, want %q", decoded.Endpoint, original.Endpoint)
	}
}

// ── Custom Provider Config Tests ─────────────────────────────────────────────

func TestConfigParsesCustomProviders(t *testing.T) {
	yamlData := `
custom_providers:
  - name: my-provider
    upstream_url: https://api.example.com
    auth_env_var: MY_API_KEY
disabled_providers:
  - anthropic-direct
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.CustomProviders) != 1 {
		t.Fatalf("expected 1 custom provider, got %d", len(cfg.CustomProviders))
	}
	if cfg.CustomProviders[0].Name != "my-provider" {
		t.Errorf("expected 'my-provider', got %q", cfg.CustomProviders[0].Name)
	}
	if cfg.CustomProviders[0].UpstreamURL != "https://api.example.com" {
		t.Errorf("upstream_url = %q, want %q", cfg.CustomProviders[0].UpstreamURL, "https://api.example.com")
	}
	if cfg.CustomProviders[0].AuthEnvVar != "MY_API_KEY" {
		t.Errorf("auth_env_var = %q, want %q", cfg.CustomProviders[0].AuthEnvVar, "MY_API_KEY")
	}
	if cfg.CustomProviders[0].Enabled != nil {
		t.Errorf("enabled should be nil when not set, got %v", *cfg.CustomProviders[0].Enabled)
	}
	if len(cfg.DisabledProviders) != 1 {
		t.Fatalf("expected 1 disabled provider, got %d", len(cfg.DisabledProviders))
	}
	if cfg.DisabledProviders[0] != "anthropic-direct" {
		t.Errorf("disabled provider = %q, want %q", cfg.DisabledProviders[0], "anthropic-direct")
	}
}

func TestCustomProviderExplicitlyDisabled(t *testing.T) {
	yamlData := `
custom_providers:
  - name: skip-me
    upstream_url: https://api.skip.com
    enabled: false
  - name: keep-me
    upstream_url: https://api.keep.com
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.CustomProviders) != 2 {
		t.Fatalf("expected 2 custom providers parsed, got %d", len(cfg.CustomProviders))
	}
	if cfg.CustomProviders[0].Enabled == nil || *cfg.CustomProviders[0].Enabled {
		t.Error("first provider should be explicitly disabled")
	}
	if cfg.CustomProviders[1].Enabled != nil {
		t.Errorf("second provider enabled should be nil, got %v", *cfg.CustomProviders[1].Enabled)
	}
}

func TestCustomProviderWithAuthHeader(t *testing.T) {
	yamlData := `
custom_providers:
  - name: custom-llm
    upstream_url: https://api.custom-llm.com
    auth_header: x-api-key
    auth_env_var: CUSTOM_LLM_KEY
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.CustomProviders[0].AuthHeader != "x-api-key" {
		t.Errorf("auth_header = %q, want %q", cfg.CustomProviders[0].AuthHeader, "x-api-key")
	}
}

func TestDisabledProvidersFiltering(t *testing.T) {
	// Simulate the filtering logic from main.go.
	allProviders := proxy.DefaultProviders()
	disabledNames := []string{"anthropic-direct", "OpenAI"} // mixed case to test case-insensitivity

	disabled := make(map[string]bool, len(disabledNames))
	for _, name := range disabledNames {
		disabled[strings.ToLower(name)] = true
	}
	var filtered []proxy.Provider
	for _, p := range allProviders {
		if !disabled[strings.ToLower(p.Name)] {
			filtered = append(filtered, p)
		}
	}

	// Verify the disabled providers are removed.
	for _, p := range filtered {
		lower := strings.ToLower(p.Name)
		if lower == "anthropic-direct" || lower == "openai" {
			t.Errorf("provider %q should have been filtered out", p.Name)
		}
	}

	// Verify count is reduced by 2.
	if len(filtered) != len(allProviders)-2 {
		t.Errorf("expected %d providers after filtering, got %d",
			len(allProviders)-2, len(filtered))
	}
}

func TestDisabledAndCustomCombined(t *testing.T) {
	yamlData := `
custom_providers:
  - name: my-llm
    upstream_url: https://api.my-llm.com
    auth_env_var: MY_LLM_KEY
disabled_providers:
  - anthropic-direct
  - openai
proxy:
  enabled: true
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatal(err)
	}

	if len(cfg.CustomProviders) != 1 {
		t.Errorf("custom providers = %d, want 1", len(cfg.CustomProviders))
	}
	if len(cfg.DisabledProviders) != 2 {
		t.Errorf("disabled providers = %d, want 2", len(cfg.DisabledProviders))
	}
	if !cfg.Proxy.Enabled {
		t.Error("proxy should be enabled")
	}
}

func TestEmptyCustomAndDisabled(t *testing.T) {
	yamlData := `
proxy:
  enabled: true
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.CustomProviders) != 0 {
		t.Errorf("custom providers should be empty, got %d", len(cfg.CustomProviders))
	}
	if len(cfg.DisabledProviders) != 0 {
		t.Errorf("disabled providers should be empty, got %d", len(cfg.DisabledProviders))
	}
}
