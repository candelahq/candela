// Package firestoredb implements storage.UserStore backed by Google Cloud
// Firestore. It stores users, budgets, grants, audit entries, and rate-limit
// windows as Firestore documents. All transactional operations (budget
// deduction) use Firestore transactions for consistency.
//
// Document paths:
//
//	users/{id}                          - User document
//	users/{id}/budgets/{period_key}     - Budget document
//	users/{id}/grants/{id}              - Grant document
//	users/{id}/audit/{id}               - Audit entry
//	rate_limit/{user_id}:{window_key}   - Rate limit counter (TTL: 2 min)
package firestoredb

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/candelahq/candela/pkg/storage"
)

const (
	usersCol     = "users"
	budgetsCol   = "budgets"
	grantsCol    = "grants"
	auditCol     = "audit"
	rateLimitCol = "rate_limit"

	defaultRateLimit = 60 // requests/minute
)

// Store implements storage.UserStore using Firestore.
type Store struct {
	client *firestore.Client
}

// New creates a new Firestore-backed UserStore.
func New(ctx context.Context, projectID, databaseID string) (*Store, error) {
	client, err := firestore.NewClientWithDatabase(ctx, projectID, databaseID)
	if err != nil {
		return nil, fmt.Errorf("firestoredb: creating client: %w", err)
	}
	return &Store{client: client}, nil
}

// NewWithClient creates a Store with an existing Firestore client (useful for tests).
func NewWithClient(client *firestore.Client) *Store {
	return &Store{client: client}
}

// Close releases Firestore resources.
func (s *Store) Close() error {
	return s.client.Close()
}

// ──────────────────────────────────────────
// User CRUD
// ──────────────────────────────────────────

func (s *Store) CreateUser(ctx context.Context, user *storage.UserRecord) error {
	if user.Email == "" {
		return fmt.Errorf("firestoredb: email is required for user creation")
	}
	// Use lowercase email as the primary document ID for search efficiency and uniqueness.
	user.ID = strings.ToLower(user.Email)
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	if user.Status == "" {
		user.Status = "provisioned"
	}
	if user.Role == "" {
		user.Role = "developer"
	}

	ref := s.client.Collection(usersCol).Doc(user.ID)
	_, err := ref.Create(ctx, user)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return fmt.Errorf("firestoredb: user already exists: %s", user.ID)
		}
		return fmt.Errorf("firestoredb: creating user: %w", err)
	}
	return nil
}

func (s *Store) GetUser(ctx context.Context, id string) (*storage.UserRecord, error) {
	snap, err := s.client.Collection(usersCol).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, fmt.Errorf("firestoredb: user %s: %w", id, storage.ErrNotFound)
		}
		return nil, fmt.Errorf("firestoredb: getting user: %w", err)
	}
	return snapToUser(snap)
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*storage.UserRecord, error) {
	// With email-as-ID, this becomes a O(1) direct document lookup.
	id := strings.ToLower(email)
	return s.GetUser(ctx, id)
}

func (s *Store) GetUsers(ctx context.Context, ids []string) (map[string]*storage.UserRecord, error) {
	if len(ids) == 0 {
		return make(map[string]*storage.UserRecord), nil
	}

	refs := make([]*firestore.DocumentRef, len(ids))
	for i, id := range ids {
		refs[i] = s.client.Collection(usersCol).Doc(id)
	}

	snaps, err := s.client.GetAll(ctx, refs)
	if err != nil {
		return nil, fmt.Errorf("firestoredb: batch fetching users: %w", err)
	}

	users := make(map[string]*storage.UserRecord, len(snaps))
	for _, snap := range snaps {
		if !snap.Exists() {
			continue
		}
		u, err := snapToUser(snap)
		if err != nil {
			slog.Warn("skipping malformed user doc during batch fetch", "id", snap.Ref.ID, "error", err)
			continue
		}
		users[u.ID] = u
	}

	return users, nil
}

func (s *Store) ListUsers(ctx context.Context, statusFilter string, limit, offset int) ([]*storage.UserRecord, int, error) {
	q := s.client.Collection(usersCol).OrderBy(firestore.DocumentID, firestore.Asc)
	if statusFilter != "" {
		q = q.Where("Status", "==", statusFilter)
	}

	// Efficient count via Firestore aggregation query.
	countQ := q.NewAggregationQuery().WithCount("total")
	results, err := countQ.Get(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("firestoredb: counting users: %w", err)
	}
	total := 0
	if countVal, ok := results["total"]; ok {
		// The SDK returns the count as an interface{} — handle both
		// int64 (production) and *firestorepb.Value (emulator) cases.
		switch v := countVal.(type) {
		case int64:
			total = int(v)
		default:
			// Attempt fmt.Sprint fallback for any other wrapper type.
			_, _ = fmt.Sscanf(fmt.Sprint(v), "%d", &total)
		}
	}

	// Use native Offset + Limit for efficient pagination.
	pageQ := q.Offset(offset).Limit(limit)
	snaps, err := pageQ.Documents(ctx).GetAll()
	if err != nil {
		return nil, 0, fmt.Errorf("firestoredb: listing users: %w", err)
	}

	users := make([]*storage.UserRecord, 0, len(snaps))
	for _, snap := range snaps {
		u, err := snapToUser(snap)
		if err != nil {
			slog.Warn("skipping malformed user doc", "id", snap.Ref.ID, "error", err)
			continue
		}
		users = append(users, u)
	}
	return users, total, nil
}

func (s *Store) UpdateUser(ctx context.Context, user *storage.UserRecord) error {
	ref := s.client.Collection(usersCol).Doc(user.ID)
	// Convert struct to map because Firestore MergeAll only supports map data.
	asMap := map[string]any{
		"ID":          user.ID,
		"Email":       user.Email,
		"DisplayName": user.DisplayName,
		"Role":        user.Role,
		"Status":      user.Status,
		"CreatedAt":   user.CreatedAt,
		"LastSeenAt":  user.LastSeenAt,
		"RateLimit":   user.RateLimit,
	}
	_, err := ref.Set(ctx, asMap, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("firestoredb: updating user: %w", err)
	}
	return nil
}

func (s *Store) TouchLastSeen(ctx context.Context, id string) error {
	ref := s.client.Collection(usersCol).Doc(id)
	_, err := ref.Update(ctx, []firestore.Update{
		{Path: "last_seen_at", Value: time.Now().UTC()},
		{Path: "status", Value: "active"},
	})
	if err != nil {
		return fmt.Errorf("firestoredb: touching last_seen: %w", err)
	}
	return nil
}

// ──────────────────────────────────────────
// Budgets
// ──────────────────────────────────────────

func (s *Store) SetBudget(ctx context.Context, budget *storage.BudgetRecord) error {
	if budget.PeriodKey == "" {
		budget.PeriodKey = currentPeriodKey(budget.PeriodType)
	}
	ref := s.client.Collection(usersCol).Doc(budget.UserID).
		Collection(budgetsCol).Doc(budget.PeriodKey)
	_, err := ref.Set(ctx, budget)
	if err != nil {
		return fmt.Errorf("firestoredb: setting budget: %w", err)
	}
	return nil
}

func (s *Store) GetBudget(ctx context.Context, userID string) (*storage.BudgetRecord, error) {
	periodKey := currentPeriodKey("monthly")
	ref := s.client.Collection(usersCol).Doc(userID).
		Collection(budgetsCol).Doc(periodKey)
	snap, err := ref.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil // No budget set — not an error.
		}
		return nil, fmt.Errorf("firestoredb: getting budget: %w", err)
	}
	return snapToBudget(snap)
}

func (s *Store) ResetSpend(ctx context.Context, userID string) error {
	periodKey := currentPeriodKey("monthly")
	ref := s.client.Collection(usersCol).Doc(userID).
		Collection(budgetsCol).Doc(periodKey)
	_, err := ref.Update(ctx, []firestore.Update{
		{Path: "spent_usd", Value: 0},
		{Path: "tokens_used", Value: 0},
	})
	if err != nil {
		return fmt.Errorf("firestoredb: resetting spend: %w", err)
	}
	return nil
}

// ──────────────────────────────────────────
// Grants
// ──────────────────────────────────────────

func (s *Store) CreateGrant(ctx context.Context, grant *storage.GrantRecord) error {
	if grant.ID == "" {
		grant.ID = uuid.New().String()
	}
	if grant.CreatedAt.IsZero() {
		grant.CreatedAt = time.Now().UTC()
	}
	ref := s.client.Collection(usersCol).Doc(grant.UserID).
		Collection(grantsCol).Doc(grant.ID)
	_, err := ref.Create(ctx, grant)
	if err != nil {
		return fmt.Errorf("firestoredb: creating grant: %w", err)
	}
	return nil
}

func (s *Store) ListGrants(ctx context.Context, userID string, activeOnly bool) ([]*storage.GrantRecord, error) {
	q := s.client.Collection(usersCol).Doc(userID).
		Collection(grantsCol).OrderBy("expires_at", firestore.Asc)

	if activeOnly {
		q = q.Where("expires_at", ">", time.Now().UTC())
	}

	snaps, err := q.Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("firestoredb: listing grants: %w", err)
	}

	grants := make([]*storage.GrantRecord, 0, len(snaps))
	for _, snap := range snaps {
		g, err := snapToGrant(snap)
		if err != nil {
			slog.Warn("skipping malformed grant", "id", snap.Ref.ID, "error", err)
			continue
		}
		// For active only, also filter out fully-spent grants.
		if activeOnly && g.Remaining() <= 0 {
			continue
		}
		grants = append(grants, g)
	}
	return grants, nil
}

func (s *Store) RevokeGrant(ctx context.Context, userID, grantID string) error {
	ref := s.client.Collection(usersCol).Doc(userID).
		Collection(grantsCol).Doc(grantID)
	_, err := ref.Update(ctx, []firestore.Update{
		{Path: "expires_at", Value: time.Now().UTC()},
	})
	if err != nil {
		return fmt.Errorf("firestoredb: revoking grant: %w", err)
	}
	return nil
}

// ──────────────────────────────────────────
// Budget Enforcement
// ──────────────────────────────────────────

func (s *Store) CheckBudget(ctx context.Context, userID string, estimatedCostUSD float64) (*storage.BudgetCheckResult, error) {
	// Sum remaining grants.
	grants, err := s.ListGrants(ctx, userID, true)
	if err != nil {
		return nil, err
	}
	var grantsRemaining float64
	for _, g := range grants {
		grantsRemaining += g.Remaining()
	}

	// Get recurring budget remaining.
	var budgetRemaining float64
	budget, err := s.GetBudget(ctx, userID)
	if err != nil {
		return nil, err
	}
	if budget != nil {
		budgetRemaining = budget.LimitUSD - budget.SpentUSD
		if budgetRemaining < 0 {
			budgetRemaining = 0
		}
	}

	totalRemaining := grantsRemaining + budgetRemaining
	return &storage.BudgetCheckResult{
		Allowed:       totalRemaining >= estimatedCostUSD,
		RemainingUSD:  totalRemaining,
		GrantsUSD:     grantsRemaining,
		BudgetUSD:     budgetRemaining,
		EstimatedCost: estimatedCostUSD,
	}, nil
}

func (s *Store) DeductSpend(ctx context.Context, userID string, costUSD float64, tokens int64) error {
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// ── ALL READS FIRST (Firestore requirement) ──

		// Read 1: Load active grants (earliest-expiring first).
		grantsRef := s.client.Collection(usersCol).Doc(userID).
			Collection(grantsCol)
		grantsSnaps, err := tx.Documents(grantsRef.
			Where("expires_at", ">", time.Now().UTC()).
			OrderBy("expires_at", firestore.Asc)).GetAll()
		if err != nil {
			return fmt.Errorf("firestoredb: loading grants in tx: %w", err)
		}

		// Read 2: Load monthly budget.
		periodKey := currentPeriodKey("monthly")
		budgetRef := s.client.Collection(usersCol).Doc(userID).
			Collection(budgetsCol).Doc(periodKey)
		budgetSnap, err := tx.Get(budgetRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("firestoredb: getting budget in tx: %w", err)
		}

		// ── ALL WRITES AFTER READS ──

		remaining := costUSD

		// Write 1: Deduct from grants first (waterfall).
		for _, snap := range grantsSnaps {
			if remaining <= 0 {
				break
			}
			g, err := snapToGrant(snap)
			if err != nil {
				continue
			}
			available := g.Remaining()
			if available <= 0 {
				continue
			}
			deduct := min(remaining, available)
			if err := tx.Update(snap.Ref, []firestore.Update{
				{Path: "spent_usd", Value: g.SpentUSD + deduct},
			}); err != nil {
				return fmt.Errorf("firestoredb: deducting from grant %s: %w", g.ID, err)
			}
			remaining -= deduct
		}

		// Write 2: Deduct remainder from monthly budget.
		if remaining > 0 && budgetSnap != nil && budgetSnap.Exists() {
			b, err := snapToBudget(budgetSnap)
			if err != nil {
				return err
			}
			if err := tx.Update(budgetRef, []firestore.Update{
				{Path: "spent_usd", Value: b.SpentUSD + remaining},
				{Path: "tokens_used", Value: b.TokensUsed + tokens},
			}); err != nil {
				return fmt.Errorf("firestoredb: deducting from budget: %w", err)
			}
		}
		return nil
	})
}

// ──────────────────────────────────────────
// Rate Limiting
// ──────────────────────────────────────────

func (s *Store) CheckRateLimit(ctx context.Context, userID string) (bool, int, int, error) {
	// Get user's rate limit.
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return false, 0, 0, err
	}
	limit := user.RateLimit
	if limit <= 0 {
		limit = defaultRateLimit
	}

	// Window key: minute-level granularity.
	windowKey := fmt.Sprintf("%s:%s", userID, time.Now().UTC().Format("2006-01-02T15:04"))
	ref := s.client.Collection(rateLimitCol).Doc(windowKey)

	var count int
	err = s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}

		if snap != nil && snap.Exists() {
			c, _ := snap.DataAt("request_count")
			if v, ok := c.(int64); ok {
				count = int(v)
			}
		}

		count++
		return tx.Set(ref, map[string]any{
			"user_id":       userID,
			"request_count": count,
			"limit":         limit,
			"window_key":    windowKey,
			"expire_at":     time.Now().UTC().Add(2 * time.Minute),
		})
	})
	if err != nil {
		return false, 0, limit, fmt.Errorf("firestoredb: rate limit: %w", err)
	}

	return count <= limit, count, limit, nil
}

// ──────────────────────────────────────────
// Audit
// ──────────────────────────────────────────

func (s *Store) LogAction(ctx context.Context, entry *storage.AuditRecord) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	ref := s.client.Collection(usersCol).Doc(entry.UserID).
		Collection(auditCol).Doc(entry.ID)
	_, err := ref.Create(ctx, entry)
	if err != nil {
		return fmt.Errorf("firestoredb: logging action: %w", err)
	}
	return nil
}

func (s *Store) ListAuditLog(ctx context.Context, userID string, limit int) ([]*storage.AuditRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	q := s.client.Collection(usersCol).Doc(userID).
		Collection(auditCol).
		OrderBy("timestamp", firestore.Desc).
		Limit(limit)

	snaps, err := q.Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("firestoredb: listing audit: %w", err)
	}

	entries := make([]*storage.AuditRecord, 0, len(snaps))
	for _, snap := range snaps {
		e, err := snapToAudit(snap)
		if err != nil {
			slog.Warn("skipping malformed audit entry", "id", snap.Ref.ID, "error", err)
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ──────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────

func snapToUser(snap *firestore.DocumentSnapshot) (*storage.UserRecord, error) {
	var u storage.UserRecord
	if err := snap.DataTo(&u); err != nil {
		return nil, fmt.Errorf("decoding user: %w", err)
	}
	u.ID = snap.Ref.ID
	return &u, nil
}

func snapToBudget(snap *firestore.DocumentSnapshot) (*storage.BudgetRecord, error) {
	var b storage.BudgetRecord
	if err := snap.DataTo(&b); err != nil {
		return nil, fmt.Errorf("decoding budget: %w", err)
	}
	return &b, nil
}

func snapToGrant(snap *firestore.DocumentSnapshot) (*storage.GrantRecord, error) {
	var g storage.GrantRecord
	if err := snap.DataTo(&g); err != nil {
		return nil, fmt.Errorf("decoding grant: %w", err)
	}
	g.ID = snap.Ref.ID
	return &g, nil
}

func snapToAudit(snap *firestore.DocumentSnapshot) (*storage.AuditRecord, error) {
	var a storage.AuditRecord
	if err := snap.DataTo(&a); err != nil {
		return nil, fmt.Errorf("decoding audit: %w", err)
	}
	a.ID = snap.Ref.ID
	return &a, nil
}

// currentPeriodKey returns the period key for the current time.
func currentPeriodKey(periodType string) string {
	now := time.Now().UTC()
	switch periodType {
	case "weekly":
		y, w := now.ISOWeek()
		return fmt.Sprintf("%d-W%02d", y, w)
	case "quarterly":
		q := (int(now.Month())-1)/3 + 1
		return fmt.Sprintf("%d-Q%d", now.Year(), q)
	case "daily":
		return now.Format("2006-01-02")
	case "monthly":
		fallthrough
	default:
		return now.Format("2006-01")
	}
}

// Ensure Store implements UserStore at compile time.
var _ storage.UserStore = (*Store)(nil)
