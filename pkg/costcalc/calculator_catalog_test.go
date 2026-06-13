package costcalc_test

import (
	"testing"

	"github.com/candelahq/candela/pkg/catalog"
	"github.com/candelahq/candela/pkg/costcalc"
)

func TestLoadFromCatalog(t *testing.T) {
	c := costcalc.New()

	// Verify a known default exists.
	cost := c.Calculate("google", "gemini-2.5-pro", 1_000_000, 0)
	if cost == 0 {
		t.Fatal("expected non-zero cost for default model")
	}

	// Load catalog with a single custom entry.
	entries := []catalog.Entry{{
		ModelID:          "test-model",
		Provider:         "test-provider",
		InputPerMillion:  5.0,
		OutputPerMillion: 10.0,
		Enabled:          true,
	}}
	c.LoadFromCatalog(entries)

	// Custom entry should now be priced.
	cost = c.Calculate("test-provider", "test-model", 1_000_000, 0)
	if cost != 5.0 {
		t.Errorf("expected 5.0, got %f", cost)
	}

	// Old defaults should be gone (replaced by catalog).
	cost = c.Calculate("google", "gemini-2.5-pro", 1_000_000, 0)
	if cost != 0 {
		t.Errorf("expected 0 (cleared defaults), got %f", cost)
	}
}

func TestLoadFromCatalog_SkipsDisabledEntries(t *testing.T) {
	c := costcalc.New()

	entries := []catalog.Entry{
		{
			ModelID:          "enabled-model",
			Provider:         "test",
			InputPerMillion:  5.0,
			OutputPerMillion: 10.0,
			Enabled:          true,
		},
		{
			ModelID:          "disabled-model",
			Provider:         "test",
			InputPerMillion:  3.0,
			OutputPerMillion: 6.0,
			Enabled:          false,
		},
	}
	c.LoadFromCatalog(entries)

	// Enabled model should be priced.
	cost := c.Calculate("test", "enabled-model", 1_000_000, 0)
	if cost != 5.0 {
		t.Errorf("enabled model: expected 5.0, got %f", cost)
	}

	// Disabled model should NOT be priced.
	cost = c.Calculate("test", "disabled-model", 1_000_000, 0)
	if cost != 0 {
		t.Errorf("disabled model: expected 0, got %f", cost)
	}
}

func TestLoadFromCatalog_PreservesConfigOverrides(t *testing.T) {
	c := costcalc.New()

	// Apply a config override first.
	c.LoadFromConfig(costcalc.PricingConfig{
		Models: []costcalc.ModelPricing{{
			Provider:         "test-provider",
			Model:            "override-model",
			InputPerMillion:  99.0,
			OutputPerMillion: 199.0,
		}},
	})

	// Load catalog — this replaces defaults but NOT overrides.
	entries := []catalog.Entry{{
		ModelID:          "catalog-model",
		Provider:         "test-provider",
		InputPerMillion:  5.0,
		OutputPerMillion: 10.0,
		Enabled:          true,
	}}
	c.LoadFromCatalog(entries)

	// Config override should still be in effect.
	cost := c.Calculate("test-provider", "override-model", 1_000_000, 0)
	if cost != 99.0 {
		t.Errorf("config override: expected 99.0, got %f", cost)
	}

	// Catalog model should also be priced.
	cost = c.Calculate("test-provider", "catalog-model", 1_000_000, 0)
	if cost != 5.0 {
		t.Errorf("catalog model: expected 5.0, got %f", cost)
	}
}

func TestLoadFromCatalog_ClampsDiscount(t *testing.T) {
	c := costcalc.New()

	entries := []catalog.Entry{{
		ModelID:          "test-model",
		Provider:         "test",
		InputPerMillion:  10.0,
		OutputPerMillion: 20.0,
		DiscountPercent:  1.5, // invalid: > 1.0
		Enabled:          true,
	}}
	c.LoadFromCatalog(entries)

	// With clamped discount of 1.0, cost should be 0.
	cost := c.Calculate("test", "test-model", 1_000_000, 0)
	if cost != 0 {
		t.Errorf("expected 0 with 100%% discount, got %f", cost)
	}
}
