// Package proxy — cost_gate.go implements per-request cost caps (#277) and
// per-model daily spend limits (#278). These are in-memory pre-flight gates
// that reject requests before they reach upstream providers.
package proxy

import (
	"strings"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
)

// SpendLimitConfig defines a per-model daily spend ceiling.
// Model is matched by prefix: "claude-opus-4" matches "claude-opus-4",
// "claude-opus-4-20250514", "claude-opus-4.6", etc.
type SpendLimitConfig struct {
	Model       string  `yaml:"model" json:"model"`                 // Model prefix (e.g. "claude-opus-4")
	MaxDailyUSD float64 `yaml:"max_daily_usd" json:"max_daily_usd"` // Daily spend cap per user
}

// spendEntry tracks a single user+model_prefix daily spend total.
type spendEntry struct {
	spent float64
	date  string // "2026-06-07" — resets on new UTC day
}

// SpendTracker is a thread-safe in-memory tracker for per-user per-model
// daily spend. Counters auto-reset when the UTC date changes.
type SpendTracker struct {
	mu      sync.Mutex
	entries map[string]*spendEntry // key: "user\x00model_prefix"
}

// NewSpendTracker creates a new SpendTracker.
func NewSpendTracker() *SpendTracker {
	return &SpendTracker{
		entries: make(map[string]*spendEntry),
	}
}

// Record adds costUSD to the user's daily spend for the model.
// It finds the matching SpendLimitConfig by prefix and records under that prefix.
// If no limit matches, the spend is not tracked (no limit = no tracking needed).
func (t *SpendTracker) Record(userID, model string, costUSD float64, limits []SpendLimitConfig) {
	if costUSD <= 0 || len(limits) == 0 {
		return
	}

	lc := matchSpendLimit(model, limits)
	if lc == nil {
		return // no limit configured for this model
	}

	today := time.Now().UTC().Format("2006-01-02")
	key := userID + "\x00" + strings.ToLower(lc.Model)

	t.mu.Lock()
	defer t.mu.Unlock()

	e, ok := t.entries[key]
	if !ok || e.date != today {
		t.entries[key] = &spendEntry{spent: costUSD, date: today}
		return
	}
	e.spent += costUSD
}

// Check returns whether the user is allowed to make a request for the given
// model based on daily spend limits. Returns (allowed, spent, limit).
// If no limit matches the model, returns (true, 0, 0).
func (t *SpendTracker) Check(userID, model string, limits []SpendLimitConfig) (allowed bool, spent, limit float64) {
	lc := matchSpendLimit(model, limits)
	if lc == nil {
		return true, 0, 0 // no limit for this model
	}

	today := time.Now().UTC().Format("2006-01-02")
	key := userID + "\x00" + strings.ToLower(lc.Model)

	t.mu.Lock()
	defer t.mu.Unlock()

	e, ok := t.entries[key]
	if !ok || e.date != today {
		return true, 0, lc.MaxDailyUSD
	}
	return e.spent < lc.MaxDailyUSD, e.spent, lc.MaxDailyUSD
}

// matchSpendLimit finds the best-matching SpendLimitConfig for a model name.
// Uses prefix matching — the longest matching prefix wins, giving more specific
// limits precedence (e.g. "claude-opus-4.7" beats "claude-opus-4").
// Returns nil if no limit matches.
func matchSpendLimit(model string, limits []SpendLimitConfig) *SpendLimitConfig {
	model = strings.ToLower(model)
	var best *SpendLimitConfig
	bestLen := 0

	for i := range limits {
		prefix := strings.ToLower(limits[i].Model)
		if strings.HasPrefix(model, prefix) && len(prefix) > bestLen {
			best = &limits[i]
			bestLen = len(prefix)
		}
	}
	return best
}

// estimateRequestCost estimates the USD cost of a request based on the request
// body size and model pricing. This is a conservative estimate used for the
// per-request cost cap (#277).
//
// Methodology:
//   - Input tokens: body_bytes / 4 (conservative 4 bytes per token)
//   - Output tokens: estimated at 2× input tokens (capped at 32K)
//   - Cost: calculated using the cost calculator's pricing table
//
// The estimate is intentionally conservative (over-estimates) since it's used
// as a gate — false positives (blocking cheap requests) are better than false
// negatives (allowing expensive ones through).
func estimateRequestCost(calc *costcalc.Calculator, provider, model string, bodySize int) float64 {
	if calc == nil || bodySize == 0 || model == "" {
		return 0
	}

	// Approximate input tokens from body size.
	// JSON overhead means not all bytes are content, but messages contain
	// system prompts, conversation history, etc. 4 bytes/token is conservative.
	inputTokens := int64(bodySize) / 4

	// Estimate output tokens: 2× input is generous for most use cases.
	// Cap at 32K to avoid absurd estimates for large-context requests.
	outputTokens := inputTokens * 2
	const maxOutputEstimate = 32_000
	if outputTokens > maxOutputEstimate {
		outputTokens = maxOutputEstimate
	}

	return calc.Calculate(provider, model, inputTokens, outputTokens)
}
