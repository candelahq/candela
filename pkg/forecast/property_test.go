package forecast

import (
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Property: BurnRatePerHour is always >= 0 for any valid input.
func TestProperty_BurnRateNonNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := drawInput(t)
		result := Calculate(in)
		if result.BurnRatePerHour < 0 {
			t.Fatalf("negative burn rate: %f (spent=%f, hours=%f)",
				result.BurnRatePerHour, in.SpentUSD,
				in.Now.Sub(in.PeriodStart).Hours())
		}
	})
}

// Property: ProjectedEODSpend is always >= 0.
func TestProperty_ProjectedEODNonNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := drawInput(t)
		result := Calculate(in)
		if result.ProjectedEODSpend < 0 {
			t.Fatalf("negative projected EOD: %f", result.ProjectedEODSpend)
		}
	})
}

// Property: AvgDailySpend is always >= 0.
func TestProperty_AvgDailySpendNonNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := drawInput(t)
		result := Calculate(in)
		if result.AvgDailySpend < 0 {
			t.Fatalf("negative avg daily spend: %f", result.AvgDailySpend)
		}
	})
}

// Property: DaysUntilExhaustion is either -1 (N/A), 0, or positive.
func TestProperty_DaysUntilExhaustionValid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := drawInput(t)
		result := Calculate(in)
		if result.DaysUntilExhaustion < -1 {
			t.Fatalf("invalid DaysUntilExhaustion: %d", result.DaysUntilExhaustion)
		}
	})
}

// Property: WillExceedBudget is true iff ProjectedEODSpend > LimitUSD
// (when a budget is configured).
func TestProperty_WillExceedConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := drawInput(t)
		// Only check when budget is configured.
		if in.LimitUSD <= 0 {
			return
		}
		result := Calculate(in)
		expected := result.ProjectedEODSpend > in.LimitUSD
		if result.WillExceedBudget != expected {
			t.Fatalf("WillExceedBudget=%v but projected=%.2f, limit=%.2f",
				result.WillExceedBudget, result.ProjectedEODSpend, in.LimitUSD)
		}
	})
}

// Property: When spent >= limit, DaysUntilExhaustion == 0 (if we have history).
func TestProperty_AlreadyExhausted(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.Float64Range(1, 1000).Draw(t, "limit")
		overspend := rapid.Float64Range(0, 500).Draw(t, "overspend")
		spent := limit + overspend

		in := Input{
			LimitUSD:    limit,
			SpentUSD:    spent,
			PeriodStart: time.Now().UTC().Truncate(24 * time.Hour),
			Now:         time.Now().UTC(),
			SpendHistory: []DailySpend{
				{Date: "2026-08-22", SpendUSD: 10, TokenCount: 50},
			},
		}
		result := Calculate(in)
		if result.DaysUntilExhaustion != 0 {
			t.Fatalf("spent(%.2f) >= limit(%.2f) but DaysUntilExhaustion=%d, want 0",
				spent, limit, result.DaysUntilExhaustion)
		}
	})
}

// Property: Zero LimitUSD → DaysUntilExhaustion == -1 (no budget).
func TestProperty_NoBudgetReturnsNegativeOne(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		spent := rapid.Float64Range(0, 10000).Draw(t, "spent")
		numDays := rapid.IntRange(0, 7).Draw(t, "numDays")
		history := drawHistory(t, numDays)

		in := Input{
			LimitUSD:     0,
			SpentUSD:     spent,
			PeriodStart:  time.Now().UTC().Truncate(24 * time.Hour),
			Now:          time.Now().UTC(),
			SpendHistory: history,
		}
		result := Calculate(in)
		if result.DaysUntilExhaustion != -1 {
			t.Fatalf("no budget but DaysUntilExhaustion=%d, want -1", result.DaysUntilExhaustion)
		}
	})
}

// Property: SpendHistory output always matches input (or normalized empty).
func TestProperty_SpendHistoryPreserved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := drawInput(t)
		result := Calculate(in)
		if result.SpendHistory == nil {
			t.Fatal("SpendHistory is nil, should be normalized to empty slice")
		}
		// Length should match input, even if input is empty.
		if len(in.SpendHistory) > 0 && len(result.SpendHistory) != len(in.SpendHistory) {
			t.Fatalf("SpendHistory len %d != input len %d",
				len(result.SpendHistory), len(in.SpendHistory))
		}
	})
}

// Property: No NaN or Inf in any result field.
func TestProperty_NoNaNOrInf(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := drawInput(t)
		result := Calculate(in)

		checks := map[string]float64{
			"BurnRatePerHour":   result.BurnRatePerHour,
			"ProjectedEODSpend": result.ProjectedEODSpend,
			"AvgDailySpend":     result.AvgDailySpend,
		}
		for name, v := range checks {
			if math.IsNaN(v) {
				t.Fatalf("%s is NaN", name)
			}
			if math.IsInf(v, 0) {
				t.Fatalf("%s is Inf", name)
			}
		}
	})
}

// ── Generators ───────────────────────────────────────────────────────────

func drawInput(t *rapid.T) Input {
	limit := rapid.Float64Range(0, 10000).Draw(t, "limit")
	spent := rapid.Float64Range(0, 10000).Draw(t, "spent")
	numDays := rapid.IntRange(0, 7).Draw(t, "numDays")
	hoursIntoDay := rapid.Float64Range(0.1, 23.9).Draw(t, "hoursIntoDay")

	now := time.Now().UTC()
	periodStart := now.Add(-time.Duration(hoursIntoDay * float64(time.Hour)))

	return Input{
		LimitUSD:     limit,
		SpentUSD:     spent,
		PeriodStart:  periodStart,
		Now:          now,
		SpendHistory: drawHistory(t, numDays),
	}
}

func drawHistory(t *rapid.T, n int) []DailySpend {
	history := make([]DailySpend, n)
	for i := range history {
		history[i] = DailySpend{
			Date:       time.Now().AddDate(0, 0, -(i + 1)).Format("2006-01-02"),
			SpendUSD:   rapid.Float64Range(0, 500).Draw(t, "histSpend"),
			TokenCount: int64(rapid.IntRange(0, 10000).Draw(t, "histCalls")),
		}
	}
	return history
}
