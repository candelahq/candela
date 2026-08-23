// Package forecast provides a REST API handler for budget forecasting (#719).
//
// Endpoint:
//
//	GET /api/v1/users/{userID}/budget-forecast — get budget forecast
//
// Authorization: self-service (own data) or admin.
package forecast

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

// BudgetStore is the minimal interface for reading budget data.
type BudgetStore interface {
	GetBudget(ctx context.Context, userID string) (*storage.BudgetRecord, error)
	GetSpendHistory(ctx context.Context, userID string, days int) ([]storage.DailySpendRecord, error)
}

// UserLookup is the minimal interface for admin role checks.
type UserLookup interface {
	GetUser(ctx context.Context, id string) (*storage.UserRecord, error)
}

// cachedForecast holds a cached forecast result with a fetch timestamp.
type cachedForecast struct {
	result    Result
	fetchedAt time.Time
}

// forecastCache provides a 5-minute TTL cache for forecast results.
type forecastCache struct {
	mu      sync.RWMutex
	entries map[string]*cachedForecast
	ttl     time.Duration
}

func newForecastCache(ttl time.Duration) *forecastCache {
	return &forecastCache{
		entries: make(map[string]*cachedForecast),
		ttl:     ttl,
	}
}

func (c *forecastCache) get(userID string) (Result, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[userID]
	if !ok || time.Since(entry.fetchedAt) >= c.ttl {
		return Result{}, false
	}
	return entry.result, true
}

func (c *forecastCache) set(userID string, r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[userID] = &cachedForecast{result: r, fetchedAt: time.Now()}
}

// Handler returns an http.HandlerFunc for the budget forecast endpoint.
//
// Authorization: self-service (own data) or admin can view any user.
func Handler(store BudgetStore, users UserLookup) http.HandlerFunc {
	cache := newForecastCache(5 * time.Minute)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse userID from path: /api/v1/users/{userID}/budget-forecast
		targetUserID := parseUserID(r.URL.Path)
		if targetUserID == "" {
			http.Error(w, `{"error":"missing user_id in path"}`, http.StatusBadRequest)
			return
		}

		// Auth: must be authenticated.
		caller := auth.FromContext(r.Context())
		if caller == nil {
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
			return
		}

		// Auth: self-service or admin.
		if caller.ID != targetUserID {
			if !isAdmin(r.Context(), users, caller.ID) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
		}

		// Check cache.
		if cached, ok := cache.get(targetUserID); ok {
			writeJSON(w, cached)
			return
		}

		// Fetch budget.
		budget, err := store.GetBudget(r.Context(), targetUserID)
		if err != nil {
			slog.Warn("forecast: failed to get budget", "user_id", targetUserID, "error", err)
			http.Error(w, `{"error":"failed to load budget"}`, http.StatusInternalServerError)
			return
		}

		// Fetch spend history (last 7 days, excluding today).
		history, err := store.GetSpendHistory(r.Context(), targetUserID, 7)
		if err != nil {
			slog.Warn("forecast: failed to get spend history", "user_id", targetUserID, "error", err)
			// Non-fatal: calculate forecast without history.
			history = nil
		}

		// Convert to forecast input.
		var spendHistory []DailySpend
		for _, h := range history {
			spendHistory = append(spendHistory, DailySpend{
				Date:       h.Date,
				SpendUSD:   h.SpendUSD,
				TokenCount: h.TokenCount,
			})
		}

		// TODO(#719): Use the configured budgetLocation from the store instead
		// of UTC. Currently all budget periods are daily and UTC-aligned, but
		// if non-UTC timezones are configured, the intraday burn rate will use
		// the wrong period boundary. See CodeRabbit review on PR #797.
		now := time.Now().UTC()
		periodStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

		var limitUSD float64
		var spentUSD float64
		if budget != nil {
			limitUSD = budget.LimitUSD
			spentUSD = budget.SpentUSD
		}

		result := Calculate(Input{
			LimitUSD:     limitUSD,
			SpentUSD:     spentUSD,
			PeriodStart:  periodStart,
			Now:          now,
			SpendHistory: spendHistory,
		})

		cache.set(targetUserID, result)
		writeJSON(w, result)
	}
}

// parseUserID extracts the userID from /api/v1/users/{userID}/budget-forecast
func parseUserID(path string) string {
	// Expected: /api/v1/users/{userID}/budget-forecast
	segments := strings.Split(strings.Trim(path, "/"), "/")
	// segments: [api, v1, users, {userID}, budget-forecast]
	if len(segments) != 5 {
		return ""
	}
	if segments[0] != "api" || segments[1] != "v1" || segments[2] != "users" || segments[4] != "budget-forecast" {
		return ""
	}
	return segments[3]
}

func isAdmin(ctx context.Context, users UserLookup, callerID string) bool {
	u, err := users.GetUser(ctx, callerID)
	if err != nil || u == nil {
		return false
	}
	return u.Role == "admin" || u.Role == "owner"
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Warn("forecast: failed to encode response", "error", err)
	}
}
