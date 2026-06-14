package billing

import "context"

// Service defines the billing operations contract.
// Implementations handle budget checking, spend tracking, and grants.
//
// Any storage backend (Firestore, PostgreSQL, in-memory, etc.) can implement
// this interface to provide billing capabilities without coupling to a
// specific database technology.
type Service interface {
	// ── Budgets ──

	// SetBudget creates or updates a user's recurring budget.
	SetBudget(ctx context.Context, budget *BudgetRecord) error

	// GetBudget returns the current-period budget for a user.
	GetBudget(ctx context.Context, userID string) (*BudgetRecord, error)

	// ResetSpend zeroes a user's current-period spend (emergency override).
	ResetSpend(ctx context.Context, userID string) error

	// ── Grants ──

	// CreateGrant issues a one-time bonus budget.
	CreateGrant(ctx context.Context, grant *GrantRecord) error

	// ListGrants returns grants for a user, optionally only active ones.
	ListGrants(ctx context.Context, userID string, activeOnly bool) ([]*GrantRecord, error)

	// RevokeGrant cancels an active grant by setting its expiry to now.
	RevokeGrant(ctx context.Context, userID, grantID string) error

	// GetGrant returns a specific grant by ID.
	GetGrant(ctx context.Context, userID, grantID string) (*GrantRecord, error)

	// ── Budget Enforcement ──

	// CheckBudget evaluates whether an estimated cost is within the user's budget.
	// This is a read-only pre-flight check.
	CheckBudget(ctx context.Context, userID string, estimatedCostUSD float64) (*BudgetCheckResult, error)

	// DeductSpend subtracts actual cost from the user's budget using the
	// budget-first waterfall: recurring budget → active grants (earliest-expiring first).
	// Grants absorb overflow when the budget is exhausted.
	// This must be transactional.
	DeductSpend(ctx context.Context, userID string, costUSD float64, tokens int64) error
}

// Notifier sends budget alerts through a specific channel.
// Implementations: logging (v1), Slack, Microsoft Teams.
type Notifier interface {
	NotifyBudgetThreshold(ctx context.Context, alert BudgetAlert) error
}
