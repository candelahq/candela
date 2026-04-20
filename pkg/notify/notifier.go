// Package notify provides budget threshold notification logic.
// It checks post-deduction budget state and fires alerts through
// pluggable Notifier implementations (logging, Slack, Teams).
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// Default thresholds at which budget warnings fire.
var defaultThresholds = []float64{0.80, 0.90, 1.00}

// LogNotifier implements storage.Notifier by emitting structured log events.
// These can be captured by Cloud Logging alert policies.
type LogNotifier struct{}

// NotifyBudgetThreshold logs a structured warning for the alert.
func (n *LogNotifier) NotifyBudgetThreshold(_ context.Context, alert storage.BudgetAlert) error {
	pct := int(alert.Threshold * 100)
	slog.Warn(fmt.Sprintf("🔔 budget alert: %d%% threshold reached", pct),
		"user_id", alert.UserID,
		"email", alert.Email,
		"threshold", fmt.Sprintf("%d%%", pct),
		"spent_usd", fmt.Sprintf("%.2f", alert.SpentUSD),
		"limit_usd", fmt.Sprintf("%.2f", alert.LimitUSD),
		"period", alert.PeriodKey,
	)
	return nil
}

// DeductResult contains post-deduction budget state for threshold checks.
type DeductResult struct {
	SpentUSD float64
	LimitUSD float64
}

// Ratio returns the spend-to-limit ratio (0.0–1.0+).
func (d DeductResult) Ratio() float64 {
	if d.LimitUSD <= 0 {
		return 0
	}
	return d.SpentUSD / d.LimitUSD
}

// BudgetChecker checks post-deduction thresholds and fires alerts.
// It tracks which thresholds have already fired per period to avoid
// duplicate notifications.
type BudgetChecker struct {
	channels   []storage.Notifier
	thresholds []float64
	// sent tracks notified thresholds per user per period.
	// Key: "{userID}:{periodKey}:{threshold}"
	sent map[string]bool
	mu   sync.RWMutex
}

// NewBudgetChecker creates a checker with the given notification channels.
func NewBudgetChecker(channels ...storage.Notifier) *BudgetChecker {
	return &BudgetChecker{
		channels:   channels,
		thresholds: defaultThresholds,
		sent:       make(map[string]bool),
	}
}

// CheckAndNotify evaluates whether any budget threshold was crossed
// and sends at most one notification per threshold per period.
func (c *BudgetChecker) CheckAndNotify(ctx context.Context, userID, email, periodKey string, result DeductResult) {
	ratio := result.Ratio()
	if ratio <= 0 {
		return
	}

	for _, threshold := range c.thresholds {
		if ratio < threshold {
			continue
		}

		key := fmt.Sprintf("%s:%s:%.2f", userID, periodKey, threshold)
		c.mu.RLock()
		alreadySent := c.sent[key]
		c.mu.RUnlock()

		if alreadySent {
			continue
		}

		alert := storage.BudgetAlert{
			UserID:    userID,
			Email:     email,
			Threshold: threshold,
			SpentUSD:  result.SpentUSD,
			LimitUSD:  result.LimitUSD,
			PeriodKey: periodKey,
			SentAt:    time.Now().UTC(),
		}

		for _, ch := range c.channels {
			if err := ch.NotifyBudgetThreshold(ctx, alert); err != nil {
				slog.Error("failed to send budget notification",
					"channel", fmt.Sprintf("%T", ch),
					"user_id", userID,
					"threshold", threshold,
					"error", err,
				)
			}
		}

		c.mu.Lock()
		c.sent[key] = true
		c.mu.Unlock()
	}
}
