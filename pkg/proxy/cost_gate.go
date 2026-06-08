// Package proxy — cost_gate.go implements per-request cost caps (#277) and
// per-model daily spend limits (#278). These are in-memory pre-flight gates
// that reject requests before they reach upstream providers.
package proxy

import (
	"bytes"
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
	MaxDailyUSD float64 `yaml:"max_daily_usd" json:"max_daily_usd"` // Daily spend cap per user (must be > 0)
}

// spendEntry tracks a single user+model_prefix daily spend total.
type spendEntry struct {
	spent float64
	date  string // "2026-06-07" — resets on new UTC day
}

// SpendTracker is a thread-safe in-memory tracker for per-user per-model
// daily spend. Counters auto-reset when the UTC date changes. Stale entries
// (from previous days) are evicted lazily on the first Record() of a new day.
type SpendTracker struct {
	mu            sync.Mutex
	entries       map[string]*spendEntry // key: "user\x00model_prefix"
	lastEvictDate string                 // prevents eviction on every call
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

	// Evict stale entries when the date rolls over (once per day).
	t.evictStaleLocked(today)

	e, ok := t.entries[key]
	if !ok || e.date != today {
		t.entries[key] = &spendEntry{spent: costUSD, date: today}
		return
	}
	e.spent += costUSD
}

// Check returns whether the user is allowed to make a request for the given
// model based on daily spend limits. estimatedCost is added to current spend
// for a forward-looking check, preventing a request that would breach the limit
// from being sent upstream. Returns (allowed, spent, limit).
// If no limit matches the model, returns (true, 0, 0).
func (t *SpendTracker) Check(userID, model string, limits []SpendLimitConfig, estimatedCost float64) (allowed bool, spent, limit float64) {
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
		// No spend today — check if just the estimated cost exceeds the limit.
		return estimatedCost <= lc.MaxDailyUSD, 0, lc.MaxDailyUSD
	}
	return e.spent+estimatedCost <= lc.MaxDailyUSD, e.spent, lc.MaxDailyUSD
}

// Len returns the number of tracked entries (for testing/metrics).
func (t *SpendTracker) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

// evictStaleLocked removes entries from previous days.
// Must be called with t.mu held. Only sweeps once per calendar day.
func (t *SpendTracker) evictStaleLocked(today string) {
	if t.lastEvictDate == today {
		return
	}
	t.lastEvictDate = today
	for k, e := range t.entries {
		if e.date != today {
			delete(t.entries, k)
		}
	}
}

// matchSpendLimit finds the best-matching SpendLimitConfig for a model name.
// Uses prefix matching — the longest matching prefix wins, giving more specific
// limits precedence (e.g. "claude-opus-4.7" beats "claude-opus-4").
// Skips entries with MaxDailyUSD <= 0 (invalid configuration).
// Returns nil if no limit matches.
func matchSpendLimit(model string, limits []SpendLimitConfig) *SpendLimitConfig {
	model = strings.ToLower(model)
	var best *SpendLimitConfig
	bestLen := 0

	for i := range limits {
		if limits[i].MaxDailyUSD <= 0 {
			continue // skip invalid limits
		}
		prefix := strings.ToLower(limits[i].Model)
		if strings.HasPrefix(model, prefix) && len(prefix) > bestLen {
			best = &limits[i]
			bestLen = len(prefix)
		}
	}
	return best
}

// estimateRequestCost estimates the USD cost of a request based on the request
// body content and model pricing. This is a conservative estimate used for the
// per-request cost cap (#277).
//
// Methodology:
//   - Input tokens: text_bytes / 4 (conservative 4 bytes per token)
//   - For multimodal requests (base64 images), the base64 data is discounted
//     and each detected image adds ~1000 tokens (typical vision model cost)
//   - Output tokens: estimated at 2× input tokens (capped at 32K)
//   - Cost: calculated using the cost calculator's pricing table
//
// The estimate is intentionally conservative (over-estimates) since it's used
// as a gate — false positives (blocking cheap requests) are better than false
// negatives (allowing expensive ones through).
func estimateRequestCost(calc *costcalc.Calculator, provider, model string, reqBody []byte) float64 {
	if calc == nil || len(reqBody) == 0 || model == "" {
		return 0
	}

	bodySize := len(reqBody)
	inputTokens := int64(bodySize) / 4

	// Detect multimodal requests containing base64-encoded images.
	// Base64 images inflate body size massively (a 1MB image = 1.3MB base64)
	// but only cost ~1000 tokens per image in vision models.
	// Without this discount, a single 1MB image would be estimated as
	// 250K input tokens ($0.50 on GPT-4o) when the actual cost is ~$0.003.
	imageCount := countBase64Images(reqBody)
	if imageCount > 0 {
		// Heuristic: each base64 image averages ~100KB of JSON body.
		// Subtract that from the body size and add ~1000 tokens per image.
		estimatedImageBytes := int64(imageCount) * 100_000
		textBytes := int64(bodySize) - estimatedImageBytes
		if textBytes < 0 {
			textBytes = 0
		}
		inputTokens = textBytes/4 + int64(imageCount)*1000
	}

	// Estimate output tokens: 2× input is generous for most use cases.
	// Cap at 32K to avoid absurd estimates for large-context requests.
	outputTokens := inputTokens * 2
	const maxOutputEstimate = 32_000
	if outputTokens > maxOutputEstimate {
		outputTokens = maxOutputEstimate
	}

	return calc.Calculate(provider, model, inputTokens, outputTokens)
}

// countBase64Images counts approximate number of base64-encoded images in a
// request body by looking for common patterns across OpenAI, Anthropic, and
// Google vision API formats.
func countBase64Images(body []byte) int {
	count := 0
	// OpenAI/Gemini: "data:image/..."
	count += bytes.Count(body, []byte("data:image/"))
	// Anthropic: "type": "image" + "source": {"type": "base64", ...}
	count += bytes.Count(body, []byte(`"type":"base64"`))
	count += bytes.Count(body, []byte(`"type": "base64"`))
	return count
}
