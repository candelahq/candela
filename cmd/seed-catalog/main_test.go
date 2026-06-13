package main

import (
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

func TestDryRunDoesNotPanic(t *testing.T) {
	// Exercise the full ModelPricing → catalog.Entry conversion path
	// (mirrors the inline loop in main) and verify it doesn't panic.
	calc := costcalc.New()
	defaults := calc.Defaults()

	entries := make([]catalog.Entry, 0, len(defaults))
	for _, p := range defaults {
		entries = append(entries, catalog.Entry{
			ModelID:              p.Model,
			Provider:             p.Provider,
			InputPerMillion:      p.InputPerMillion,
			OutputPerMillion:     p.OutputPerMillion,
			InputPerMillionHigh:  p.InputPerMillionHigh,
			OutputPerMillionHigh: p.OutputPerMillionHigh,
			TierThresholdTokens:  p.TierThresholdTokens,
			DiscountPercent:      p.DiscountPercent,
			Enabled:              true,
		})
	}

	if len(entries) == 0 {
		t.Fatal("no entries generated")
	}
	if len(entries) != len(defaults) {
		t.Errorf("entry count %d != defaults count %d", len(entries), len(defaults))
	}
}
