package costcalc

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestPricingExpiry_ClaudeSonnet5(t *testing.T) {
	c := New()

	// 1. Before launch pricing expiry (e.g., Aug 15, 2026)
	beforeExpiry := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return beforeExpiry })

	pricingBefore, ok := c.Resolve("anthropic", "claude-sonnet-5")
	if !ok {
		t.Fatalf("failed to resolve claude-sonnet-5 before expiry")
	}
	if pricingBefore.InputPerMillion != 2.00 || pricingBefore.OutputPerMillion != 10.00 {
		t.Errorf("before expiry expected $2/$10, got input=%f output=%f",
			pricingBefore.InputPerMillion, pricingBefore.OutputPerMillion)
	}

	costBefore := c.Calculate("anthropic", "claude-sonnet-5", 1_000_000, 1_000_000)
	expectedBefore := 12.00 // $2 + $10
	if costBefore != expectedBefore {
		t.Errorf("cost before expiry = %f, want %f", costBefore, expectedBefore)
	}

	// 2. Exactly at expiry (Aug 31, 2026 23:59:59 UTC) — still valid
	atExpiry := time.Date(2026, time.August, 31, 23, 59, 59, 0, time.UTC)
	c.SetClock(func() time.Time { return atExpiry })

	pricingAt, ok := c.Resolve("anthropic", "claude-sonnet-5")
	if !ok {
		t.Fatalf("failed to resolve claude-sonnet-5 at expiry")
	}
	if pricingAt.InputPerMillion != 2.00 || pricingAt.OutputPerMillion != 10.00 {
		t.Errorf("at expiry expected $2/$10, got input=%f output=%f",
			pricingAt.InputPerMillion, pricingAt.OutputPerMillion)
	}

	// 3. One second after expiry (Sep 1, 2026 00:00:00 UTC) — fallback active ($3/$15)
	afterExpiry := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return afterExpiry })

	pricingAfter, ok := c.Resolve("anthropic", "claude-sonnet-5")
	if !ok {
		t.Fatalf("failed to resolve claude-sonnet-5 after expiry")
	}
	if pricingAfter.InputPerMillion != 3.00 || pricingAfter.OutputPerMillion != 15.00 {
		t.Errorf("after expiry expected $3/$15, got input=%f output=%f",
			pricingAfter.InputPerMillion, pricingAfter.OutputPerMillion)
	}

	costAfter := c.Calculate("anthropic", "claude-sonnet-5", 1_000_000, 1_000_000)
	expectedAfter := 18.00 // $3 + $15
	if costAfter != expectedAfter {
		t.Errorf("cost after expiry = %f, want %f", costAfter, expectedAfter)
	}

	// 4. Test Models() reflects effective rates after expiry
	found := false
	for _, m := range c.Models() {
		if m.Model == "claude-sonnet-5" && m.Provider == "anthropic" {
			found = true
			if m.InputPerMillion != 3.00 || m.OutputPerMillion != 15.00 {
				t.Errorf("Models() after expiry expected $3/$15, got input=%f output=%f",
					m.InputPerMillion, m.OutputPerMillion)
			}
		}
	}
	if !found {
		t.Errorf("claude-sonnet-5 not found in Models()")
	}
}

func TestPricingExpiry_Alerts(t *testing.T) {
	c := New()

	// Set clock 5 days before expiry: Aug 26, 2026
	fiveDaysBefore := time.Date(2026, time.August, 26, 23, 59, 59, 0, time.UTC)
	c.SetClock(func() time.Time { return fiveDaysBefore })

	alerts := c.CheckExpirations(7 * 24 * time.Hour)
	var sonnetAlert *PricingAlert
	for i := range alerts {
		if alerts[i].Model == "claude-sonnet-5" {
			sonnetAlert = &alerts[i]
			break
		}
	}
	if sonnetAlert == nil {
		t.Fatalf("expected expiring_soon alert for claude-sonnet-5")
	}
	if sonnetAlert.Status != "expiring_soon" {
		t.Errorf("expected status 'expiring_soon', got %s", sonnetAlert.Status)
	}
	if sonnetAlert.ExpiresIn <= 0 {
		t.Errorf("expected positive ExpiresIn, got %v", sonnetAlert.ExpiresIn)
	}

	// Set clock after expiry: Sep 10, 2026
	tenDaysAfter := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return tenDaysAfter })

	alertsAfter := c.CheckExpirations(7 * 24 * time.Hour)
	var sonnetExpired *PricingAlert
	for i := range alertsAfter {
		if alertsAfter[i].Model == "claude-sonnet-5" {
			sonnetExpired = &alertsAfter[i]
			break
		}
	}
	if sonnetExpired == nil {
		t.Fatalf("expected expired alert for claude-sonnet-5")
	}
	if sonnetExpired.Status != "expired" {
		t.Errorf("expected status 'expired', got %s", sonnetExpired.Status)
	}
	if sonnetExpired.FallbackInputPerMillion != 3.00 || sonnetExpired.FallbackOutputPerMillion != 15.00 {
		t.Errorf("unexpected fallback rates in alert: %f / %f",
			sonnetExpired.FallbackInputPerMillion, sonnetExpired.FallbackOutputPerMillion)
	}
}

func TestFlexibleTime_YAMLFormats(t *testing.T) {
	yamlData := `
models:
  - provider: test
    model: test-date-only
    input_per_million: 1.0
    output_per_million: 2.0
    valid_until: "2026-10-31"
    fallback_input_per_million: 3.0
    fallback_output_per_million: 4.0

  - provider: test
    model: test-rfc3339
    input_per_million: 1.0
    output_per_million: 2.0
    valid_until: "2026-10-31T15:04:05Z"
    fallback_input_per_million: 3.0
    fallback_output_per_million: 4.0
`

	var cfg struct {
		Models []ModelPricing `yaml:"models"`
	}
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("failed to unmarshal YAML with flexible dates: %v", err)
	}

	if len(cfg.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(cfg.Models))
	}

	// First entry: date-only format sets to end-of-day UTC
	m1 := cfg.Models[0]
	if m1.ValidUntil == nil {
		t.Fatalf("m1 ValidUntil is nil")
	}
	if m1.ValidUntil.Day() != 31 || m1.ValidUntil.Month() != time.October || m1.ValidUntil.Hour() != 23 {
		t.Errorf("unexpected date-only parsed time: %v", m1.ValidUntil)
	}

	// Second entry: RFC3339
	m2 := cfg.Models[1]
	if m2.ValidUntil == nil {
		t.Fatalf("m2 ValidUntil is nil")
	}
	if m2.ValidUntil.Hour() != 15 || m2.ValidUntil.Minute() != 4 {
		t.Errorf("unexpected RFC3339 parsed time: %v", m2.ValidUntil)
	}
}
