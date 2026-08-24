// Package billing defines storage-agnostic billing types and the Service
// interface for budget checking, spend tracking, and grant management.
//
// These types were extracted from pkg/storage so that billing logic can
// be implemented against any backend (Firestore, PostgreSQL, in-memory, etc.)
// without importing the full storage package.
package billing

import "time"

// BudgetRecord is the Go representation of a user's recurring budget.
type BudgetRecord struct {
	UserID     string  `json:"user_id"`
	LimitUSD   float64 `json:"limit_usd,omitempty"`
	SpentUSD   float64 `json:"spent_usd,omitempty"`
	TokensUsed int64   `json:"tokens_used,omitempty"`
	// AllTokensUsed is incremented for every LLM call before the grants waterfall —
	// regardless of whether the cost was absorbed by a grant or the budget.
	// This gives GetMyBudget an accurate "tokens used today" count from Firestore
	// without needing BigQuery. Contrast with TokensUsed, which is budget-portion only.
	AllTokensUsed int64     `json:"all_tokens_used,omitempty"`
	PeriodType    string    `json:"period_type,omitempty"` // "daily"
	PeriodKey     string    `json:"period_key,omitempty"`  // "2026-04", "2026-W15"
	PeriodStart   time.Time `json:"period_start,omitempty"`
	PeriodEnd     time.Time `json:"period_end,omitempty"`
}

// GrantRecord is the Go representation of a one-time budget grant.
type GrantRecord struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	AmountUSD float64   `json:"amount_usd,omitempty"`
	SpentUSD  float64   `json:"spent_usd,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	GrantedBy string    `json:"granted_by,omitempty"`
	StartsAt  time.Time `json:"starts_at,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Remaining returns how much of this grant is still available.
// Clamped to 0 to prevent floating-point overdraft from reducing
// the apparent total budget in CheckBudget (BILL-3).
func (g *GrantRecord) Remaining() float64 {
	r := g.AmountUSD - g.SpentUSD
	if r < 0 {
		return 0
	}
	return r
}

// BudgetCheckResult is returned by CheckBudget.
type BudgetCheckResult struct {
	Allowed       bool    // Whether the estimated cost is within budget
	Reason        string  // Why the request was allowed/denied
	RemainingUSD  float64 // Total remaining across grants + budget
	GrantsUSD     float64 // Remaining in active grants
	BudgetUSD     float64 // Remaining in recurring budget
	EstimatedCost float64 // The estimated cost that was checked
}

// Reason constants for BudgetCheckResult.
const (
	ReasonAllowed         = "allowed"
	ReasonBudgetExhausted = "budget_exhausted"
	ReasonSoftBlocked     = "soft_blocked"
	ReasonNoBudget        = "no_budget"
)

// BudgetAlert represents a threshold notification event.
type BudgetAlert struct {
	UserID    string    `json:"user_id" firestore:"user_id"`
	Email     string    `json:"email" firestore:"email"`
	TaskID    string    `json:"task_id,omitempty" firestore:"task_id,omitempty"` // optional: from X-Candela-Job-Id
	Threshold float64   `json:"threshold" firestore:"threshold"`                 // 0.8, 0.9, 1.0
	SpentUSD  float64   `json:"spent_usd" firestore:"spent_usd"`
	LimitUSD  float64   `json:"limit_usd" firestore:"limit_usd"`
	PeriodKey string    `json:"period_key" firestore:"period_key"`
	SentAt    time.Time `json:"sent_at" firestore:"sent_at"`
}

// ModelLimitRecord is a per-user per-model daily spend ceiling.
// Stored in Firestore at users/{uid}/model_limits/{model_prefix}.
// When present, overrides any global daily_limits YAML config for that model prefix.
type ModelLimitRecord struct {
	UserID      string    `json:"user_id" firestore:"user_id"`
	ModelPrefix string    `json:"model_prefix" firestore:"model_prefix"` // e.g. "claude-opus-4", "gpt-4o"
	MaxDailyUSD float64   `json:"max_daily_usd" firestore:"max_daily_usd"`
	CreatedAt   time.Time `json:"created_at" firestore:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" firestore:"updated_at"`
}

// DailySpendRecord represents a single day's spend total for a user (#719).
// Used for budget forecasting and trend analysis.
type DailySpendRecord struct {
	Date       string  `json:"date"` // "2026-08-23"
	SpendUSD   float64 `json:"spend_usd"`
	TokenCount int64   `json:"token_count"` // total tokens used (not request count)
}
