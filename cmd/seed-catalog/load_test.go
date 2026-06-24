package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalogFileYAML(t *testing.T) {
	content := `models:
  - provider: test-provider
    model: test-model
    input_per_million: 1.50
    output_per_million: 9.00
  - provider: test-provider
    model: test-model-tiered
    input_per_million: 1.25
    output_per_million: 10.00
    input_per_million_high: 2.50
    output_per_million_high: 15.00
    tier_threshold_tokens: 200000
`
	path := writeTestFile(t, "catalog.yaml", content)
	entries, err := loadCatalogFile(path)
	if err != nil {
		t.Fatalf("loadCatalogFile(%q): %v", path, err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify first entry.
	e := entries[0]
	if e.Provider != "test-provider" {
		t.Errorf("entry[0].Provider = %q, want %q", e.Provider, "test-provider")
	}
	if e.ModelID != "test-model" {
		t.Errorf("entry[0].ModelID = %q, want %q", e.ModelID, "test-model")
	}
	if e.InputPerMillion != 1.50 {
		t.Errorf("entry[0].InputPerMillion = %f, want 1.50", e.InputPerMillion)
	}
	if e.OutputPerMillion != 9.00 {
		t.Errorf("entry[0].OutputPerMillion = %f, want 9.00", e.OutputPerMillion)
	}
	if !e.Enabled {
		t.Error("entry[0].Enabled should default to true")
	}

	// Verify tiered entry.
	e2 := entries[1]
	if e2.TierThresholdTokens != 200000 {
		t.Errorf("entry[1].TierThresholdTokens = %d, want 200000", e2.TierThresholdTokens)
	}
	if e2.InputPerMillionHigh != 2.50 {
		t.Errorf("entry[1].InputPerMillionHigh = %f, want 2.50", e2.InputPerMillionHigh)
	}
	if e2.OutputPerMillionHigh != 15.00 {
		t.Errorf("entry[1].OutputPerMillionHigh = %f, want 15.00", e2.OutputPerMillionHigh)
	}
}

func TestLoadCatalogFileYML(t *testing.T) {
	content := `models:
  - provider: google
    model: gemini-3.5-flash
    input_per_million: 1.50
    output_per_million: 9.00
`
	path := writeTestFile(t, "catalog.yml", content)
	entries, err := loadCatalogFile(path)
	if err != nil {
		t.Fatalf("loadCatalogFile(%q): %v", path, err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestLoadCatalogFileJSON(t *testing.T) {
	content := `{
  "models": [
    {
      "provider": "openai",
      "model": "gpt-4.1",
      "input_per_million": 2.00,
      "output_per_million": 8.00
    },
    {
      "provider": "anthropic",
      "model": "claude-sonnet-4",
      "input_per_million": 3.00,
      "output_per_million": 15.00,
      "enabled": false
    }
  ]
}`
	path := writeTestFile(t, "catalog.json", content)
	entries, err := loadCatalogFile(path)
	if err != nil {
		t.Fatalf("loadCatalogFile(%q): %v", path, err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// First entry should default to enabled=true.
	if !entries[0].Enabled {
		t.Error("entry[0] should default to enabled=true")
	}
	// Second entry explicitly set enabled=false.
	if entries[1].Enabled {
		t.Error("entry[1] should be enabled=false")
	}
}

func TestLoadCatalogFileExtraFields(t *testing.T) {
	content := `models:
  - provider: custom
    model: custom-model
    input_per_million: 1.00
    output_per_million: 5.00
    display_name: Custom Model
    category: premium
    context_window: 128000
    aliases:
      - custom-v1
      - custom-latest
    provider_model_id: custom-model-v1
    region: us-east1
    discount_percent: 0.15
`
	path := writeTestFile(t, "catalog-extra.yaml", content)
	entries, err := loadCatalogFile(path)
	if err != nil {
		t.Fatalf("loadCatalogFile(%q): %v", path, err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.DisplayName != "Custom Model" {
		t.Errorf("DisplayName = %q, want %q", e.DisplayName, "Custom Model")
	}
	if e.Category != "premium" {
		t.Errorf("Category = %q, want %q", e.Category, "premium")
	}
	if e.ContextWindow != 128000 {
		t.Errorf("ContextWindow = %d, want 128000", e.ContextWindow)
	}
	if len(e.Aliases) != 2 {
		t.Errorf("Aliases len = %d, want 2", len(e.Aliases))
	}
	if e.ProviderModelID != "custom-model-v1" {
		t.Errorf("ProviderModelID = %q, want %q", e.ProviderModelID, "custom-model-v1")
	}
	if e.Region != "us-east1" {
		t.Errorf("Region = %q, want %q", e.Region, "us-east1")
	}
	if e.DiscountPercent != 0.15 {
		t.Errorf("DiscountPercent = %f, want 0.15", e.DiscountPercent)
	}
}

func TestLoadCatalogFileMissingProvider(t *testing.T) {
	content := `models:
  - model: some-model
    input_per_million: 1.00
    output_per_million: 5.00
`
	path := writeTestFile(t, "no-provider.yaml", content)
	_, err := loadCatalogFile(path)
	if err == nil {
		t.Fatal("expected error for missing provider, got nil")
	}
}

func TestLoadCatalogFileMissingModel(t *testing.T) {
	content := `models:
  - provider: test
    input_per_million: 1.00
    output_per_million: 5.00
`
	path := writeTestFile(t, "no-model.yaml", content)
	_, err := loadCatalogFile(path)
	if err == nil {
		t.Fatal("expected error for missing model, got nil")
	}
}

func TestLoadCatalogFileZeroPrice(t *testing.T) {
	content := `models:
  - provider: test
    model: test-model
    input_per_million: 0
    output_per_million: 5.00
`
	path := writeTestFile(t, "zero-price.yaml", content)
	_, err := loadCatalogFile(path)
	if err == nil {
		t.Fatal("expected error for zero input price, got nil")
	}
}

func TestLoadCatalogFileEmptyModels(t *testing.T) {
	content := `models: []
`
	path := writeTestFile(t, "empty.yaml", content)
	_, err := loadCatalogFile(path)
	if err == nil {
		t.Fatal("expected error for empty models list, got nil")
	}
}

func TestLoadCatalogFileUnsupportedExtension(t *testing.T) {
	path := writeTestFile(t, "catalog.toml", "whatever")
	_, err := loadCatalogFile(path)
	if err == nil {
		t.Fatal("expected error for unsupported extension, got nil")
	}
}

func TestLoadCatalogFileNotFound(t *testing.T) {
	_, err := loadCatalogFile("/nonexistent/path/catalog.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestLoadCatalogFileInvalidYAML(t *testing.T) {
	content := `models:
  - this is: [not: valid: yaml
`
	path := writeTestFile(t, "invalid.yaml", content)
	_, err := loadCatalogFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadCatalogFileInvalidJSON(t *testing.T) {
	content := `{"models": [{"not valid json`
	path := writeTestFile(t, "invalid.json", content)
	_, err := loadCatalogFile(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoadCatalogFileMatchesPricingYAMLFormat(t *testing.T) {
	// Verify that the embedded pricing.yaml can be parsed by loadCatalogFile.
	// This validates format compatibility between the two systems.
	pricingPath := filepath.Join("..", "..", "pkg", "costcalc", "pricing.yaml")
	entries, err := loadCatalogFile(pricingPath)
	if err != nil {
		t.Fatalf("loadCatalogFile(pricing.yaml): %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected non-empty entries from pricing.yaml")
	}

	// Spot-check: all entries must have valid pricing.
	for _, e := range entries {
		if e.Provider == "" || e.ModelID == "" {
			t.Errorf("entry with empty provider/model: %+v", e)
		}
		if e.InputPerMillion <= 0 || e.OutputPerMillion <= 0 {
			t.Errorf("entry %s/%s has non-positive pricing", e.Provider, e.ModelID)
		}
		if !e.Enabled {
			t.Errorf("entry %s/%s should default to enabled=true", e.Provider, e.ModelID)
		}
	}
}

// writeTestFile writes content to a temp file with the given name and returns the path.
func writeTestFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
