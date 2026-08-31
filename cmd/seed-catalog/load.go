package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/candelahq/candela/pkg/catalog"
	"gopkg.in/yaml.v3"
)

// catalogFile is the schema for external catalog files.
// It matches the structure of pkg/costcalc/pricing.yaml so users can
// export and re-import catalog data using the same format.
type catalogFile struct {
	Models []catalogModel `yaml:"models" json:"models"`
}

// catalogModel represents a single model entry in an external catalog file.
type catalogModel struct {
	Provider             string   `yaml:"provider" json:"provider"`
	Model                string   `yaml:"model" json:"model"`
	InputPerMillion      float64  `yaml:"input_per_million" json:"input_per_million"`
	OutputPerMillion     float64  `yaml:"output_per_million" json:"output_per_million"`
	InputPerMillionHigh  float64  `yaml:"input_per_million_high,omitempty" json:"input_per_million_high,omitempty"`
	OutputPerMillionHigh float64  `yaml:"output_per_million_high,omitempty" json:"output_per_million_high,omitempty"`
	TierThresholdTokens  int64    `yaml:"tier_threshold_tokens,omitempty" json:"tier_threshold_tokens,omitempty"`
	DiscountPercent      float64  `yaml:"discount_percent,omitempty" json:"discount_percent,omitempty"`
	Enabled              *bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"` // defaults to true if omitted
	DisplayName          string   `yaml:"display_name,omitempty" json:"display_name,omitempty"`
	Category             string   `yaml:"category,omitempty" json:"category,omitempty"`
	ContextWindow        int64    `yaml:"context_window,omitempty" json:"context_window,omitempty"`
	Aliases              []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	ProviderModelID      string   `yaml:"provider_model_id,omitempty" json:"provider_model_id,omitempty"`
	Region               string   `yaml:"region,omitempty" json:"region,omitempty"`
}

// loadCatalogFile reads model entries from an external YAML or JSON file.
// The file format is detected from the extension (.yaml, .yml, .json).
func loadCatalogFile(path string) ([]catalog.Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading catalog file: %w", err)
	}

	var cf catalogFile
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cf); err != nil {
			return nil, fmt.Errorf("parsing YAML catalog file: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &cf); err != nil {
			return nil, fmt.Errorf("parsing JSON catalog file: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported file extension %q (expected .yaml, .yml, or .json)", ext)
	}

	if len(cf.Models) == 0 {
		return nil, fmt.Errorf("catalog file %s contains no model entries", path)
	}

	seen := make(map[string]int, len(cf.Models))
	entries := make([]catalog.Entry, 0, len(cf.Models))
	for i, m := range cf.Models {
		if m.Provider == "" || m.Model == "" {
			return nil, fmt.Errorf("catalog file entry %d: provider and model are required", i)
		}
		if m.InputPerMillion <= 0 || m.OutputPerMillion < 0 {
			return nil, fmt.Errorf("catalog file entry %d (%s/%s): input price must be > 0 and output price must be >= 0", i, m.Provider, m.Model)
		}

		key := m.Provider + "/" + m.Model
		if prevIdx, ok := seen[key]; ok {
			return nil, fmt.Errorf("catalog file entry %d: duplicate model %s (first seen at entry %d)", i, key, prevIdx)
		}
		seen[key] = i

		enabled := true
		if m.Enabled != nil {
			enabled = *m.Enabled
		}

		e := catalog.Entry{
			ModelID:              m.Model,
			Provider:             m.Provider,
			InputPerMillion:      m.InputPerMillion,
			OutputPerMillion:     m.OutputPerMillion,
			InputPerMillionHigh:  m.InputPerMillionHigh,
			OutputPerMillionHigh: m.OutputPerMillionHigh,
			TierThresholdTokens:  m.TierThresholdTokens,
			DiscountPercent:      m.DiscountPercent,
			Enabled:              enabled,
			DisplayName:          m.DisplayName,
			Category:             m.Category,
			ContextWindow:        m.ContextWindow,
			Aliases:              m.Aliases,
			ProviderModelID:      m.ProviderModelID,
			Region:               m.Region,
		}
		entries = append(entries, e)
	}

	return entries, nil
}
