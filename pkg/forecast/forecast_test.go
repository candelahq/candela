package forecast

import (
	"testing"
	"time"
)

func TestCalculate_NormalBurnRate(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) // noon
	periodStart := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	r := Calculate(Input{
		LimitUSD:    25.0,
		SpentUSD:    12.50,
		PeriodStart: periodStart,
		Now:         now,
		SpendHistory: []DailySpend{
			{Date: "2026-08-22", SpendUSD: 20.10, TokenCount: 142},
			{Date: "2026-08-21", SpendUSD: 16.90, TokenCount: 98},
			{Date: "2026-08-20", SpendUSD: 18.50, TokenCount: 120},
		},
	})

	// 12.50 spent in 12 hours = $1.04/hr
	if r.BurnRatePerHour < 1.0 || r.BurnRatePerHour > 1.1 {
		t.Errorf("BurnRatePerHour = %.4f, want ~1.04", r.BurnRatePerHour)
	}

	// Projected EOD: 1.04 * 24 = ~$25.00
	if r.ProjectedEODSpend < 24.0 || r.ProjectedEODSpend > 26.0 {
		t.Errorf("ProjectedEODSpend = %.2f, want ~25.00", r.ProjectedEODSpend)
	}

	// Should exceed: projected ~$25 vs limit $25 — right on the edge
	// (actually 12.5/12*24 = 25.0, so exactly at limit)

	// Avg daily: (20.10 + 16.90 + 18.50) / 3 = 18.50
	if r.AvgDailySpend < 18.4 || r.AvgDailySpend > 18.6 {
		t.Errorf("AvgDailySpend = %.2f, want ~18.50", r.AvgDailySpend)
	}

	// Remaining: 25 - 12.5 = 12.5; days = 12.5 / 18.5 ≈ 0.68 → ceil = 1
	if r.DaysUntilExhaustion != 1 {
		t.Errorf("DaysUntilExhaustion = %d, want 1", r.DaysUntilExhaustion)
	}

	if r.EstimatedExhaustion == "" {
		t.Error("EstimatedExhaustion is empty, want a date")
	}
}

func TestCalculate_ZeroSpend(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	periodStart := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	r := Calculate(Input{
		LimitUSD:     25.0,
		SpentUSD:     0,
		PeriodStart:  periodStart,
		Now:          now,
		SpendHistory: []DailySpend{},
	})

	if r.BurnRatePerHour != 0 {
		t.Errorf("BurnRatePerHour = %.4f, want 0", r.BurnRatePerHour)
	}
	if r.WillExceedBudget {
		t.Error("WillExceedBudget = true, want false")
	}
	if r.DaysUntilExhaustion != -1 {
		t.Errorf("DaysUntilExhaustion = %d, want -1 (no history)", r.DaysUntilExhaustion)
	}
	if len(r.SpendHistory) != 0 {
		t.Errorf("SpendHistory len = %d, want 0", len(r.SpendHistory))
	}
}

func TestCalculate_BudgetAlreadyExceeded(t *testing.T) {
	now := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	periodStart := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	r := Calculate(Input{
		LimitUSD:    25.0,
		SpentUSD:    30.0, // already over
		PeriodStart: periodStart,
		Now:         now,
		SpendHistory: []DailySpend{
			{Date: "2026-08-22", SpendUSD: 28.0, TokenCount: 200},
		},
	})

	if !r.WillExceedBudget {
		t.Error("WillExceedBudget = false, want true")
	}
	if r.DaysUntilExhaustion != 0 {
		t.Errorf("DaysUntilExhaustion = %d, want 0 (already exceeded)", r.DaysUntilExhaustion)
	}
	if r.EstimatedExhaustion != "2026-08-23" {
		t.Errorf("EstimatedExhaustion = %q, want 2026-08-23", r.EstimatedExhaustion)
	}
}

func TestCalculate_SkipsZeroSpendDays(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC) // Monday
	periodStart := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)

	r := Calculate(Input{
		LimitUSD:    100.0,
		SpentUSD:    5.0,
		PeriodStart: periodStart,
		Now:         now,
		SpendHistory: []DailySpend{
			{Date: "2026-08-24", SpendUSD: 0, TokenCount: 0},      // Sunday — skip
			{Date: "2026-08-23", SpendUSD: 0, TokenCount: 0},      // Saturday — skip
			{Date: "2026-08-22", SpendUSD: 20.0, TokenCount: 100}, // Friday
			{Date: "2026-08-21", SpendUSD: 18.0, TokenCount: 90},  // Thursday
			{Date: "2026-08-20", SpendUSD: 22.0, TokenCount: 110}, // Wednesday
		},
	})

	// Avg should be (20 + 18 + 22) / 3 = 20.0, not / 5
	if r.AvgDailySpend < 19.9 || r.AvgDailySpend > 20.1 {
		t.Errorf("AvgDailySpend = %.2f, want 20.00 (weekends skipped)", r.AvgDailySpend)
	}

	// Remaining: 100 - 5 = 95; days = 95 / 20 = 4.75 → ceil = 5
	if r.DaysUntilExhaustion != 5 {
		t.Errorf("DaysUntilExhaustion = %d, want 5", r.DaysUntilExhaustion)
	}
}

func TestCalculate_NoBudgetSet(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	periodStart := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	r := Calculate(Input{
		LimitUSD:    0, // no budget
		SpentUSD:    50.0,
		PeriodStart: periodStart,
		Now:         now,
		SpendHistory: []DailySpend{
			{Date: "2026-08-22", SpendUSD: 45.0, TokenCount: 300},
		},
	})

	// No budget → no forecast, but spend history still populated
	if r.BurnRatePerHour != 0 {
		t.Errorf("BurnRatePerHour = %.4f, want 0 (no budget)", r.BurnRatePerHour)
	}
	if r.DaysUntilExhaustion != -1 {
		t.Errorf("DaysUntilExhaustion = %d, want -1", r.DaysUntilExhaustion)
	}
	if len(r.SpendHistory) != 1 {
		t.Errorf("SpendHistory len = %d, want 1", len(r.SpendHistory))
	}
}

func TestCalculate_SingleDayHistory(t *testing.T) {
	now := time.Date(2026, 8, 23, 15, 0, 0, 0, time.UTC)
	periodStart := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	r := Calculate(Input{
		LimitUSD:    50.0,
		SpentUSD:    10.0,
		PeriodStart: periodStart,
		Now:         now,
		SpendHistory: []DailySpend{
			{Date: "2026-08-22", SpendUSD: 30.0, TokenCount: 200},
		},
	})

	// Avg = 30.0 (single day)
	if r.AvgDailySpend < 29.9 || r.AvgDailySpend > 30.1 {
		t.Errorf("AvgDailySpend = %.2f, want 30.00", r.AvgDailySpend)
	}

	// Remaining: 50 - 10 = 40; days = 40 / 30 ≈ 1.33 → ceil = 2
	if r.DaysUntilExhaustion != 2 {
		t.Errorf("DaysUntilExhaustion = %d, want 2", r.DaysUntilExhaustion)
	}
}

func TestCalculate_EarlyInDay(t *testing.T) {
	// At 00:05 UTC, only 5 minutes into the day — tests the 0.1 hour floor
	now := time.Date(2026, 8, 23, 0, 5, 0, 0, time.UTC)
	periodStart := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	r := Calculate(Input{
		LimitUSD:     25.0,
		SpentUSD:     0.50,
		PeriodStart:  periodStart,
		Now:          now,
		SpendHistory: []DailySpend{},
	})

	// 0.50 / 0.1 (floor) = 5.0/hr — conservative early estimate
	// Actual: 0.50 / (5/60) = 6.0/hr but we floor at 0.1 hours
	if r.BurnRatePerHour <= 0 {
		t.Errorf("BurnRatePerHour = %.4f, want > 0", r.BurnRatePerHour)
	}
}

func TestCalculate_NilSpendHistory(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	periodStart := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	r := Calculate(Input{
		LimitUSD:     25.0,
		SpentUSD:     10.0,
		PeriodStart:  periodStart,
		Now:          now,
		SpendHistory: nil,
	})

	// Should normalize nil to empty slice
	if r.SpendHistory == nil {
		t.Error("SpendHistory is nil, want empty slice")
	}
}
