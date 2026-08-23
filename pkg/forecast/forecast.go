// Package forecast provides budget forecasting and usage prediction
// calculations (#719). It is a pure-logic package with no I/O dependencies,
// making it easy to test and reuse across REST handlers, gRPC handlers,
// and CLI tools.
//
// The forecasting algorithm operates at two levels:
//
//  1. Intraday burn rate: current spend ÷ hours elapsed → projected EOD spend
//  2. Multi-day trend: 7-day rolling average daily spend → exhaustion date
//
// Zero-spend days (weekends, holidays) are excluded from the average to give
// a "working day" burn rate that better reflects actual usage patterns.
package forecast

import (
	"math"
	"time"
)

// DailySpend represents one day's total spend for a user.
type DailySpend struct {
	Date       string  `json:"date"` // "2026-08-23"
	SpendUSD   float64 `json:"spend_usd"`
	TokenCount int64   `json:"token_count"`
}

// Input provides all data needed to compute a forecast.
type Input struct {
	LimitUSD     float64      // User's budget limit for the current period
	SpentUSD     float64      // Amount already spent in the current period
	PeriodStart  time.Time    // Start of current period (midnight UTC)
	Now          time.Time    // Current time
	SpendHistory []DailySpend // Historical daily spend (last 7 days, excluding today)
}

// Result holds the computed forecast values.
type Result struct {
	BurnRatePerHour     float64      `json:"burn_rate_usd_per_hour"`
	ProjectedEODSpend   float64      `json:"projected_eod_spend_usd"`
	WillExceedBudget    bool         `json:"will_exceed_budget"`
	AvgDailySpend       float64      `json:"avg_daily_spend_usd"`
	EstimatedExhaustion string       `json:"estimated_exhaustion_date"` // "2026-08-28" or ""
	DaysUntilExhaustion int          `json:"days_until_exhaustion"`     // -1 if N/A
	SpendHistory        []DailySpend `json:"spend_history"`
}

// Calculate computes budget forecasting from the given inputs.
//
// If LimitUSD is 0 (no budget configured), returns an empty result with
// DaysUntilExhaustion = -1 and only the spend history populated.
func Calculate(in Input) Result {
	r := Result{
		DaysUntilExhaustion: -1,
		SpendHistory:        in.SpendHistory,
	}

	if len(r.SpendHistory) == 0 {
		r.SpendHistory = []DailySpend{} // normalize nil to empty slice for JSON
	}

	// No budget configured — return spend history only.
	if in.LimitUSD <= 0 {
		return r
	}

	// ── Level 1: Intraday burn rate ──
	hoursElapsed := in.Now.Sub(in.PeriodStart).Hours()
	if hoursElapsed < 0.1 {
		hoursElapsed = 0.1 // avoid division by near-zero early in the day
	}

	r.BurnRatePerHour = in.SpentUSD / hoursElapsed
	r.ProjectedEODSpend = r.BurnRatePerHour * 24
	r.WillExceedBudget = r.ProjectedEODSpend > in.LimitUSD

	// ── Level 2: Multi-day trend ──
	// Calculate average daily spend, skipping zero-spend days.
	var totalSpend float64
	var activeDays int
	for _, d := range in.SpendHistory {
		if d.SpendUSD > 0 {
			totalSpend += d.SpendUSD
			activeDays++
		}
	}

	if activeDays > 0 {
		r.AvgDailySpend = totalSpend / float64(activeDays)

		remaining := in.LimitUSD - in.SpentUSD
		if remaining <= 0 {
			// Budget already exceeded.
			r.DaysUntilExhaustion = 0
			r.EstimatedExhaustion = in.Now.Format("2006-01-02")
		} else if r.AvgDailySpend > 0 {
			daysLeft := remaining / r.AvgDailySpend
			r.DaysUntilExhaustion = int(math.Ceil(daysLeft))
			exhaustionDate := in.Now.AddDate(0, 0, r.DaysUntilExhaustion)
			r.EstimatedExhaustion = exhaustionDate.Format("2006-01-02")
		}
	}

	return r
}
