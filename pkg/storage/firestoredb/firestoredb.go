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
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/candelahq/candela/pkg/storage"
)

const (
	usersCol       = "users"
	budgetsCol     = "budgets"
	grantsCol      = "grants"
	auditCol       = "audit"
	globalAuditCol = "global_audit"
	rateLimitCol   = "rate_limit"

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
	// Use lowercase and sanitized email as primary ID.
	user.ID = sanitizeID(user.Email)
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
	id = sanitizeID(id)
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
	return s.GetUser(ctx, email)
}

func (s *Store) GetUsers(ctx context.Context, ids []string) (map[string]*storage.UserRecord, error) {
	if len(ids) == 0 {
		return make(map[string]*storage.UserRecord), nil
	}

	refs := make([]*firestore.DocumentRef, len(ids))
	for i, id := range ids {
		refs[i] = s.client.Collection(usersCol).Doc(sanitizeID(id))
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
		q = q.Where("status", "==", statusFilter)
	}

	// Count total documents. Use aggregation but fall back to a full
	// fetch if the count comes back as an unexpected type.
	total := 0
	countQ := q.NewAggregationQuery().WithCount("total")
	results, err := countQ.Get(ctx)
	if err != nil {
		slog.Warn("firestoredb: aggregation count failed, falling back to full scan", "error", err)
	} else if countVal, ok := results["total"]; ok {
		switch v := countVal.(type) {
		case int64:
			total = int(v)
		default:
			// Handle SDK wrapper types (emulator returns *firestorepb.Value).
			if n, parseErr := fmt.Sscanf(fmt.Sprint(v), "%d", &total); n != 1 || parseErr != nil {
				slog.Warn("firestoredb: unexpected count type, falling back", "type", fmt.Sprintf("%T", v))
			}
		}
	}

	// Server-side pagination.
	pageQ := q.Offset(offset).Limit(limit)
	snaps, err := pageQ.Documents(ctx).GetAll()
	if err != nil {
		return nil, 0, fmt.Errorf("firestoredb: listing users: %w", err)
	}

	// If aggregation count returned 0 but we fetched users, use the
	// fetched length as a fallback (handles count API mismatches).
	if total == 0 && len(snaps) > 0 {
		total = len(snaps)
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
	id := sanitizeID(user.ID)
	ref := s.client.Collection(usersCol).Doc(id)
	// Use targeted updates for mutable fields only. This avoids the
	// "MergeAll can only be specified with map data" error that occurs
	// when passing a struct to Set with MergeAll.
	updates := []firestore.Update{
		{Path: "role", Value: user.Role},
		{Path: "status", Value: user.Status},
		{Path: "display_name", Value: user.DisplayName},
		{Path: "rate_limit", Value: user.RateLimit},
	}
	if !user.LastSeenAt.IsZero() {
		updates = append(updates, firestore.Update{Path: "last_seen_at", Value: user.LastSeenAt})
	}
	_, err := ref.Update(ctx, updates)
	if err != nil {
		return fmt.Errorf("firestoredb: updating user: %w", err)
	}
	return nil
}

func (s *Store) TouchLastSeen(ctx context.Context, id string) error {
	id = sanitizeID(id)
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

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	id = sanitizeID(id)
	userRef := s.client.Collection(usersCol).Doc(id)

	// Discover and delete all subcollections (Firestore doesn't cascade).
	// Use a single BulkWriter across all subcollections for efficiency.
	bw := s.client.BulkWriter(ctx)
	collIter := userRef.Collections(ctx)
	for {
		collRef, err := collIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			bw.End()
			return fmt.Errorf("firestoredb: listing subcollections: %w", err)
		}

		// Use DocumentRefs — we only need refs for deletion, not full snapshots.
		docIter := collRef.DocumentRefs(ctx)
		for {
			docRef, err := docIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				bw.End()
				return fmt.Errorf("firestoredb: iterating subcollection %s: %w", collRef.ID, err)
			}
			if _, err := bw.Delete(docRef); err != nil {
				slog.WarnContext(ctx, "bulk delete enqueue failed",
					"collection", collRef.ID, "doc", docRef.ID, "error", err)
			}
		}
	}
	bw.End()

	// Delete the user document.
	if _, err := userRef.Delete(ctx); err != nil {
		return fmt.Errorf("firestoredb: deleting user: %w", err)
	}
	return nil
}

// Budgets
// ──────────────────────────────────────────

// budgetConfigDocID is the Firestore document ID of the stable budget
// configuration document within the budgets subcollection.
// Period spend docs use YYYY-MM-DD keys; "config" never collides with those.
const budgetConfigDocID = "config"

func (s *Store) SetBudget(ctx context.Context, budget *storage.BudgetRecord) error {
	if budget.PeriodKey == "" {
		budget.PeriodKey = currentPeriodKey(budget.PeriodType)
	}
	userID := sanitizeID(budget.UserID)
	userRef := s.client.Collection(usersCol).Doc(userID)

	// Write 1: stable config doc — survives daily period rollover.
	// This is the only place limit_usd/period_type are persisted long-term.
	configRef := userRef.Collection(budgetsCol).Doc(budgetConfigDocID)
	_, err := configRef.Set(ctx, map[string]interface{}{
		"limit_usd":   budget.LimitUSD,
		"period_type": budget.PeriodType,
		"user_id":     userID,
	})
	if err != nil {
		return fmt.Errorf("firestoredb: setting budget config: %w", err)
	}

	// Write 2: today's spend doc — config fields only (Merge preserves counters).
	ref := userRef.Collection(budgetsCol).Doc(budget.PeriodKey)
	_, err = ref.Set(ctx, budget, firestore.Merge(
		firestore.FieldPath{"user_id"},
		firestore.FieldPath{"limit_usd"},
		firestore.FieldPath{"period_type"},
		firestore.FieldPath{"period_key"},
	))
	if err != nil {
		return fmt.Errorf("firestoredb: setting budget: %w", err)
	}
	return nil
}

func (s *Store) GetBudget(ctx context.Context, userID string) (*storage.BudgetRecord, error) {
	periodKey := currentPeriodKey("daily")
	userID = sanitizeID(userID)
	userRef := s.client.Collection(usersCol).Doc(userID)
	ref := userRef.Collection(budgetsCol).Doc(periodKey)
	snap, err := ref.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// #9: Period doc missing — check config doc for auto-rollover.
			// This happens every day after midnight UTC until the first call
			// triggers DeductSpend (which auto-creates the period doc).
			return s.budgetFromConfig(ctx, userRef, userID, periodKey)
		}
		return nil, fmt.Errorf("firestoredb: getting budget: %w", err)
	}
	return snapToBudget(snap)
}

// budgetFromConfig synthesizes a zero-spend BudgetRecord from the stable
// config doc. Returns nil (no error) if no config exists yet.
func (s *Store) budgetFromConfig(ctx context.Context, userRef *firestore.DocumentRef, userID, periodKey string) (*storage.BudgetRecord, error) {
	configRef := userRef.Collection(budgetsCol).Doc(budgetConfigDocID)
	configSnap, err := configRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil // no budget configured at all
		}
		return nil, fmt.Errorf("firestoredb: reading budget config: %w", err)
	}
	data := configSnap.Data()
	// #E: Firestore stores whole numbers as int64, not float64.
	// Use firestoreFloat to handle both numeric types.
	limitUSD := firestoreFloat(data["limit_usd"])
	periodType, _ := data["period_type"].(string)
	if periodType == "" {
		periodType = "daily"
	}
	return &storage.BudgetRecord{
		UserID:     userID,
		LimitUSD:   limitUSD,
		PeriodType: periodType,
		PeriodKey:  periodKey,
		// Spend counters start at 0 — no calls yet this period.
	}, nil
}

// firestoreFloat safely converts a Firestore numeric field to float64.
// Firestore stores whole-number values as int64 (not float64), so a plain
// .(float64) assertion silently returns 0.0 for values like 100 or 50.
func firestoreFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	}
	return 0
}

func (s *Store) ResetSpend(ctx context.Context, userID string) error {
	periodKey := currentPeriodKey("daily")
	userID = sanitizeID(userID)
	ref := s.client.Collection(usersCol).Doc(userID).
		Collection(budgetsCol).Doc(periodKey)
	// #H: Use Set+Merge instead of Update so this is idempotent on a missing
	// period doc (new day). Update fails with NOT_FOUND when the spend doc
	// doesn't exist yet (e.g. called by an admin just after midnight).
	_, err := ref.Set(ctx, map[string]any{
		"spent_usd":       0,
		"tokens_used":     0,
		"all_tokens_used": 0,
	}, firestore.Merge(
		firestore.FieldPath{"spent_usd"},
		firestore.FieldPath{"tokens_used"},
		firestore.FieldPath{"all_tokens_used"},
	))
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
	userID := sanitizeID(grant.UserID)
	ref := s.client.Collection(usersCol).Doc(userID).
		Collection(grantsCol).Doc(grant.ID)
	_, err := ref.Create(ctx, grant)
	if err != nil {
		return fmt.Errorf("firestoredb: creating grant: %w", err)
	}
	return nil
}

func (s *Store) ListGrants(ctx context.Context, userID string, activeOnly bool) ([]*storage.GrantRecord, error) {
	userID = sanitizeID(userID)
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
	userID = sanitizeID(userID)
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

// CheckBudget returns whether the user has sufficient funds (grants + budget)
// for an estimated call cost.
//
// # TOCTOU note (#G)
//
// CheckBudget and DeductSpend are intentionally separate operations. There is
// a small window between a passing CheckBudget and the subsequent DeductSpend
// where another concurrent request could exhaust the remaining balance. This
// means a user near their limit may occasionally make one extra call.
//
// The risk is bounded and acceptable: DeductSpend still records actual spend
// post-fact, and the pre-flight floor ($0.001) prevents truly zero-balance
// calls from succeeding. A future improvement would collapse the two into a
// single Firestore transaction, but that would require reading grants inside
// the DeductSpend transaction (significant latency increase on the hot path).
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

		// Sanitize into a local so we don't mutate the outer parameter.
		// Firestore transactions auto-retry on contention — mutating the outer
		// userID would cause it to be double-sanitized on each retry.
		localUserID := sanitizeID(userID)
		// Read 1: Load active grants (earliest-expiring first).
		grantsRef := s.client.Collection(usersCol).Doc(localUserID).
			Collection(grantsCol)
		grantsSnaps, err := tx.Documents(grantsRef.
			Where("expires_at", ">", time.Now().UTC()).
			OrderBy("expires_at", firestore.Asc)).GetAll()
		if err != nil {
			return fmt.Errorf("firestoredb: loading grants in tx: %w", err)
		}

		// Read 2: Load daily budget (period spend doc).
		periodKey := currentPeriodKey("daily")
		userRef := s.client.Collection(usersCol).Doc(localUserID)
		budgetRef := userRef.Collection(budgetsCol).Doc(periodKey)
		budgetSnap, err := tx.Get(budgetRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("firestoredb: getting budget in tx: %w", err)
		}

		// Read 3: Config doc for auto-rollover (#9).
		// When the period doc is NotFound (new day), read the stable config
		// to get limit_usd and auto-create the spend doc in this transaction.
		configRef := userRef.Collection(budgetsCol).Doc(budgetConfigDocID)
		configSnap, configErr := tx.Get(configRef)
		if configErr != nil && status.Code(configErr) != codes.NotFound {
			return fmt.Errorf("firestoredb: getting budget config in tx: %w", configErr)
		}

		// ── ALL WRITES AFTER READS ──

		remaining := costUSD

		// Write 1: Deduct from daily budget first.
		// Budget is the developer's primary daily pool. Grants act as overflow
		// when the budget is exhausted (budget-first waterfall).
		var b *storage.BudgetRecord
		var isNewPeriod bool
		if budgetSnap != nil && budgetSnap.Exists() {
			b, err = snapToBudget(budgetSnap)
			if err != nil {
				return err
			}
		} else if configSnap != nil && configSnap.Exists() {
			// #9: Auto-rollover — create today's spend doc from config.
			data := configSnap.Data()
			// #E: firestoreFloat handles int64 (Firestore whole numbers) and float64.
			limitUSD := firestoreFloat(data["limit_usd"])
			periodType, _ := data["period_type"].(string)
			if periodType == "" {
				periodType = "daily"
			}
			b = &storage.BudgetRecord{
				UserID:     localUserID,
				LimitUSD:   limitUSD,
				PeriodType: periodType,
				PeriodKey:  periodKey,
			}
			isNewPeriod = true
		}

		if b != nil {
			budgetAvailable := b.LimitUSD - b.SpentUSD
			if budgetAvailable < 0 {
				budgetAvailable = 0
			}
			budgetDeduct := min(remaining, budgetAvailable)
			if budgetDeduct > 0 || tokens > 0 {
				// Attribute tokens proportional to the budget fraction absorbed.
				// If budget absorbs 100% of cost → 100% of tokens.
				// If budget absorbs 30% → 30% of tokens (remainder lands on grants).
				budgetTokens := tokens
				if costUSD > 0 && budgetDeduct < costUSD {
					budgetTokens = int64(math.Round(float64(tokens) * (budgetDeduct / costUSD)))
				}
				// all_tokens_used is always the full token count regardless of
				// whether grants absorb any cost — used by GetMyBudget fast path.
				if isNewPeriod {
					// Auto-rollover: create the period doc atomically with initial spend.
					// tx.Update would fail on a nonexistent document.
					newDoc := &storage.BudgetRecord{
						UserID:        localUserID,
						LimitUSD:      b.LimitUSD,
						PeriodType:    b.PeriodType,
						PeriodKey:     periodKey,
						SpentUSD:      budgetDeduct,
						TokensUsed:    budgetTokens,
						AllTokensUsed: tokens,
					}
					if err := tx.Set(budgetRef, newDoc); err != nil {
						return fmt.Errorf("firestoredb: creating new period budget: %w", err)
					}
				} else {
					updates := []firestore.Update{
						{Path: "all_tokens_used", Value: b.AllTokensUsed + tokens},
						{Path: "tokens_used", Value: b.TokensUsed + budgetTokens},
					}
					if budgetDeduct > 0 {
						updates = append(updates, firestore.Update{
							Path:  "spent_usd",
							Value: b.SpentUSD + budgetDeduct,
						})
					}
					if err := tx.Update(budgetRef, updates); err != nil {
						return fmt.Errorf("firestoredb: deducting from budget: %w", err)
					}
				}
			}
			remaining -= budgetDeduct
		}

		// Write 2: Overflow into grants (earliest-expiry-first).
		// Draining the soonest-to-expire grant first minimises waste from
		// grants expiring with unused balance.
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
	userID = sanitizeID(userID)
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
	userID := sanitizeID(entry.UserID)
	ref := s.client.Collection(usersCol).Doc(userID).
		Collection(auditCol).Doc(entry.ID)
	_, err := ref.Create(ctx, entry)
	if err != nil {
		return fmt.Errorf("firestoredb: logging action: %w", err)
	}
	return nil
}

func (s *Store) LogGlobalAction(ctx context.Context, entry *storage.AuditRecord) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	ref := s.client.Collection(globalAuditCol).Doc(entry.ID)
	_, err := ref.Set(ctx, entry)
	if err != nil {
		return fmt.Errorf("firestoredb: logging global action: %w", err)
	}
	return nil
}

func (s *Store) ListAuditLog(ctx context.Context, userID string, limit int) ([]*storage.AuditRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	userID = sanitizeID(userID)
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

// currentPeriodKey returns the period key for the given period type.
// Supported period types: "daily" (YYYY-MM-DD), "monthly" (YYYY-MM),
// "weekly" (YYYY-WNN). Unknown types fall back to "daily".
// #F: was `func currentPeriodKey(_ string)` — the argument was always
// ignored, so monthly and weekly budgets always rolled over daily.
func currentPeriodKey(periodType string) string {
	now := time.Now().UTC()
	switch periodType {
	case "monthly":
		return now.Format("2006-01")
	case "weekly":
		// Use the year returned by ISOWeek() — in the first days of January
		// the ISO week may belong to the previous year (e.g. 2024-W52).
		year, week := now.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	default: // "daily" and anything unrecognised
		return now.Format("2006-01-02")
	}
}

// sanitizeID makes a generic string (like an email) safe for use as a
// Firestore document ID by lowercasing, replacing path-separator and
// consecutive-dot characters, and enforcing Firestore's 1500-byte limit.
//
// Rules applied:
//   - Lowercase (emails are case-insensitive)
//   - `/` → `_` (Firestore path separator)
//   - `..` → `._` (consecutive dots are disallowed in Firestore IDs)
//   - Truncate to 1500 bytes (Firestore document ID limit) — #I
//   - Prefix `u_` if the result matches the reserved `__.*__` pattern
func sanitizeID(id string) string {
	s := strings.ToLower(id)
	s = strings.ReplaceAll(s, "/", "_")
	// Firestore disallows consecutive dots; replace with a visually-distinct
	// separator that keeps the ID human-readable.
	s = strings.ReplaceAll(s, "..", "._")
	// #I: Firestore document IDs must be ≤ 1500 bytes.
	// Walk back from byte 1500 to find a valid UTF-8 boundary to avoid
	// splitting a multi-byte character (which would produce an invalid ID).
	if len(s) > 1500 {
		end := 1500
		// Walk back from byte 1500 until we land on a UTF-8 leading byte.
		// utf8.RuneStart reports true for any byte that starts a rune (ASCII
		// or a multi-byte lead byte), so this is O(1) for well-formed input
		// — at most 3 steps back for a 4-byte sequence.
		for end > 0 && !utf8.RuneStart(s[end]) {
			end--
		}
		s = s[:end]
	}
	// Firestore reserves document IDs matching __.*__ (e.g. __user__@example.com).
	if len(s) >= 4 && strings.HasPrefix(s, "__") && strings.HasSuffix(s, "__") {
		s = "u_" + s
	}
	return s
}

// Ensure Store implements UserStore at compile time.
var _ storage.UserStore = (*Store)(nil)
