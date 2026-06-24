package main

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/catalog"
	"github.com/candelahq/candela/pkg/costcalc"
)

func TestDefaultsToEntries(t *testing.T) {
	calc := costcalc.New()
	defaults := calc.Defaults()

	if len(defaults) == 0 {
		t.Fatal("expected non-empty defaults")
	}

	// Verify sorted order (provider first, then model).
	for i := 1; i < len(defaults); i++ {
		prev := defaults[i-1]
		curr := defaults[i]
		if prev.Provider > curr.Provider ||
			(prev.Provider == curr.Provider && prev.Model > curr.Model) {
			t.Errorf("defaults not sorted at index %d: %s/%s > %s/%s",
				i, prev.Provider, prev.Model, curr.Provider, curr.Model)
		}
	}
}

func TestPricingToEntryMapping(t *testing.T) {
	calc := costcalc.New()
	defaults := calc.Defaults()

	// Find a model with tiered pricing (e.g. gemini-2.5-pro).
	var found bool
	for _, p := range defaults {
		if p.TierThresholdTokens > 0 {
			// Verify tiered pricing fields are populated.
			if p.InputPerMillionHigh == 0 {
				t.Errorf("model %s/%s has TierThresholdTokens=%d but InputPerMillionHigh=0",
					p.Provider, p.Model, p.TierThresholdTokens)
			}
			if p.OutputPerMillionHigh == 0 {
				t.Errorf("model %s/%s has TierThresholdTokens=%d but OutputPerMillionHigh=0",
					p.Provider, p.Model, p.TierThresholdTokens)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("no model with tiered pricing found in defaults")
	}
}

func TestDefaultsAllHaveValidPricing(t *testing.T) {
	calc := costcalc.New()
	defaults := calc.Defaults()

	for _, p := range defaults {
		if p.Provider == "" {
			t.Errorf("model %s has empty provider", p.Model)
		}
		if p.Model == "" {
			t.Errorf("provider %s has empty model", p.Provider)
		}
		if p.InputPerMillion <= 0 {
			t.Errorf("%s/%s has non-positive InputPerMillion: %f",
				p.Provider, p.Model, p.InputPerMillion)
		}
		if p.OutputPerMillion <= 0 {
			t.Errorf("%s/%s has non-positive OutputPerMillion: %f",
				p.Provider, p.Model, p.OutputPerMillion)
		}
		if p.DiscountPercent < 0 || p.DiscountPercent > 1 {
			t.Errorf("%s/%s has out-of-range DiscountPercent: %f",
				p.Provider, p.Model, p.DiscountPercent)
		}
	}
}

func TestBuildEntriesFromDefaults(t *testing.T) {
	// Exercise the full ModelPricing → catalog.Entry conversion path
	// via the refactored buildEntriesFromDefaults helper.
	entries := buildEntriesFromDefaults()

	defaults := costcalc.New().Defaults()
	if len(entries) == 0 {
		t.Fatal("no entries generated")
	}
	if len(entries) != len(defaults) {
		t.Errorf("entry count %d != defaults count %d", len(entries), len(defaults))
	}
}

func TestDatabaseIDFlagDefault(t *testing.T) {
	// Verify the --database-id flag defaults to "(default)" so that
	// NewClientWithDatabase always receives a valid value.
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	databaseID := fs.String("database-id", "(default)", "Firestore database ID")

	// Parse with no args — should use default.
	if err := fs.Parse([]string{}); err != nil {
		t.Fatal(err)
	}
	if *databaseID != "(default)" {
		t.Errorf("expected default database-id to be '(default)', got %q", *databaseID)
	}
}

func TestDatabaseIDFlagOverride(t *testing.T) {
	// Verify --database-id can be overridden to a named database.
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	databaseID := fs.String("database-id", "(default)", "Firestore database ID")

	if err := fs.Parse([]string{"--database-id=candela"}); err != nil {
		t.Fatal(err)
	}
	if *databaseID != "candela" {
		t.Errorf("expected database-id to be 'candela', got %q", *databaseID)
	}
}

func TestOutputFormatIncludesDatabaseID(t *testing.T) {
	// Verify the output format string includes the database ID.
	projectID := "test-project"
	databaseID := "my-db"
	collection := "model_catalog"
	dryRun := true

	got := fmt.Sprintf("🕯️  seed-catalog (project=%s database=%s collection=%s dry-run=%v)",
		projectID, databaseID, collection, dryRun)

	if !strings.Contains(got, "database=my-db") {
		t.Errorf("output should include database ID: %s", got)
	}
	if !strings.Contains(got, "project=test-project") {
		t.Errorf("output should include project ID: %s", got)
	}
}

func TestVertexModelID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude-opus-4.7", "claude-opus-4-7"},
		{"claude-opus-4.8", "claude-opus-4-8"},
		{"claude-haiku-4.5", "claude-haiku-4-5"},
		{"claude-sonnet-4.6", "claude-sonnet-4-6"},
		{"claude-sonnet-4", "claude-sonnet-4"},                       // no dots — unchanged
		{"claude-3-5-sonnet-20241022", "claude-3-5-sonnet-20241022"}, // no dots
	}
	for _, tt := range tests {
		got := vertexModelID(tt.input)
		if got != tt.want {
			t.Errorf("vertexModelID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAnthropicEntriesHaveProviderModelID(t *testing.T) {
	calc := costcalc.New()
	defaults := calc.Defaults()

	for _, p := range defaults {
		if p.Provider != "anthropic" {
			continue
		}
		e := catalog.Entry{
			ModelID:  p.Model,
			Provider: p.Provider,
		}
		if vid := vertexModelID(p.Model); vid != p.Model {
			e.ProviderModelID = vid
		}
		e.Region = "global"

		// Models with dots must have a ProviderModelID set.
		if strings.Contains(p.Model, ".") {
			if e.ProviderModelID == "" {
				t.Errorf("anthropic model %q has dots but no ProviderModelID", p.Model)
			}
			if strings.Contains(e.ProviderModelID, ".") {
				t.Errorf("ProviderModelID %q should not contain dots", e.ProviderModelID)
			}
		}

		// All Anthropic models should have region = "global".
		if e.Region != "global" {
			t.Errorf("anthropic model %q should have region=global, got %q", p.Model, e.Region)
		}
	}
}
