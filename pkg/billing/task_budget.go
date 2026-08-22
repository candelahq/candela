package billing

import (
	"fmt"
	"time"
)

// Reason constants for task budget denials.
const (
	// ReasonTaskBudgetExhausted indicates the task's ephemeral budget is spent.
	ReasonTaskBudgetExhausted = "task_budget_exhausted"
	// ReasonTaskBudgetExpired indicates the task budget has passed its ExpiresAt.
	ReasonTaskBudgetExpired = "task_budget_expired"
)

// TaskBudget is an ephemeral, per-job budget that isolates spend for a single
// agent task (identified by the X-Candela-Job-Id header). Unlike the recurring
// user budget, a task budget is one-shot: it lives for the task's lifetime and
// is deleted when the task completes.
//
// Stored in Firestore at tasks/{taskID}/budget.
type TaskBudget struct {
	TaskID    string    `json:"task_id" firestore:"task_id"`
	UserID    string    `json:"user_id" firestore:"user_id"`
	LimitUSD  float64   `json:"limit_usd" firestore:"limit_usd"`
	SpentUSD  float64   `json:"spent_usd" firestore:"spent_usd"`
	CreatedAt time.Time `json:"created_at" firestore:"created_at"`
	ExpiresAt time.Time `json:"expires_at" firestore:"expires_at"`
}

// Remaining returns the unspent portion of the task budget.
// Returns 0 if the budget is overspent.
func (b *TaskBudget) Remaining() float64 {
	r := b.LimitUSD - b.SpentUSD
	if r < 0 {
		return 0
	}
	return r
}

// IsExpired reports whether the task budget has passed its expiry time.
func (b *TaskBudget) IsExpired() bool {
	if b.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().UTC().After(b.ExpiresAt)
}

// Validate checks that the task budget has valid field values.
func (b *TaskBudget) Validate() error {
	if b.TaskID == "" {
		return fmt.Errorf("billing: task budget: task_id is required")
	}
	if b.LimitUSD <= 0 {
		return fmt.Errorf("billing: task budget: limit_usd must be positive, got %.6f", b.LimitUSD)
	}
	if b.SpentUSD < 0 {
		return fmt.Errorf("billing: task budget: spent_usd must be non-negative, got %.6f", b.SpentUSD)
	}
	return nil
}

// TaskBudgetCheckResult is the outcome of a pre-flight task budget check.
type TaskBudgetCheckResult struct {
	Allowed      bool    `json:"allowed"`
	Reason       string  `json:"reason,omitempty"`
	RemainingUSD float64 `json:"remaining_usd"`
	LimitUSD     float64 `json:"limit_usd"`
	SpentUSD     float64 `json:"spent_usd"`
}
