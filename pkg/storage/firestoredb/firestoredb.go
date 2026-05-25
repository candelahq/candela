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
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
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

	// rateLimitCacheTTL controls how long a user's configured RateLimit value
	// is cached in-process. RateLimit only changes on explicit admin updates,
	// so 5 minutes is a safe propagation window.
	rateLimitCacheTTL = 5 * time.Minute
)

// rateLimitValueCache is an in-process cache of userID → configured rate limit.
// CheckRateLimit previously called GetUser (one Firestore RPC) on every
// proxied LLM request just to fetch this value. Caching it eliminates that
// hot-path read for the common case where no admin has recently changed the limit.
var rateLimitValueCache struct {
	mu      sync.RWMutex
	entries map[string]rlCacheEntry
}

func init() {
	rateLimitValueCache.entries = make(map[string]rlCacheEntry)
	// CRIT-10: sweep expired entries on a background goroutine to prevent
	// unbounded map growth in environments with high user-ID churn.
	go func() {
		ticker := time.NewTicker(rateLimitCacheTTL)
		defer ticker.Stop()
		for range ticker.C {
			sweepRateLimitCache()
		}
	}()
}

type rlCacheEntry struct {
	limit     int
	expiresAt time.Time
}

func getCachedRateLimit(userID string) (int, bool) {
	rateLimitValueCache.mu.RLock()
	defer rateLimitValueCache.mu.RUnlock()
	if e, ok := rateLimitValueCache.entries[userID]; ok && time.Now().Before(e.expiresAt) {
		return e.limit, true
	}
	return 0, false
}

func setCachedRateLimit(userID string, limit int) {
	rateLimitValueCache.mu.Lock()
	rateLimitValueCache.entries[userID] = rlCacheEntry{
		limit:     limit,
		expiresAt: time.Now().Add(rateLimitCacheTTL),
	}
	rateLimitValueCache.mu.Unlock()
}

func evictRateLimitCache(userID string) {
	rateLimitValueCache.mu.Lock()
	delete(rateLimitValueCache.entries, userID)
	rateLimitValueCache.mu.Unlock()
}

func sweepRateLimitCache() {
	now := time.Now().UTC()
	rateLimitValueCache.mu.Lock()
	for k, e := range rateLimitValueCache.entries {
		if now.After(e.expiresAt) {
			delete(rateLimitValueCache.entries, k)
		}
	}
	rateLimitValueCache.mu.Unlock()
}

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

// firestoreUser is the Firestore-internal representation of a user document.
// It owns the firestore:" struct tags so storage.UserRecord remains
// database-agnostic (no Firestore-specific annotations on the shared type).
type firestoreUser struct {
	ID           string    `firestore:"id"`
	Email        string    `firestore:"email"`
	DisplayName  string    `firestore:"display_name,omitempty"`
	Role         string    `firestore:"role,omitempty"`
	Status       string    `firestore:"status,omitempty"`
	CreatedAt    time.Time `firestore:"created_at,omitempty"`
	LastSeenAt   time.Time `firestore:"last_seen_at,omitempty"`
	LastActiveAt time.Time `firestore:"last_active_at,omitempty"`
	RateLimit    int       `firestore:"rate_limit,omitempty"`
}

func userToFirestore(u *storage.UserRecord) *firestoreUser {
	fu := &firestoreUser{
		ID:           u.ID,
		Email:        u.Email,
		Role:         u.Role,
		Status:       u.Status,
		CreatedAt:    u.CreatedAt,
		LastSeenAt:   u.LastSeenAt,
		LastActiveAt: u.LastActiveAt,
	}
	if u.DisplayName != nil {
		fu.DisplayName = *u.DisplayName
	}
	if u.RateLimit != nil {
		fu.RateLimit = *u.RateLimit
	}
	return fu
}

func firestoreToUser(fu *firestoreUser) *storage.UserRecord {
	u := &storage.UserRecord{
		ID:           fu.ID,
		Email:        fu.Email,
		Role:         fu.Role,
		Status:       fu.Status,
		CreatedAt:    fu.CreatedAt,
		LastSeenAt:   fu.LastSeenAt,
		LastActiveAt: fu.LastActiveAt,
	}
	if fu.DisplayName != "" {
		u.DisplayName = &fu.DisplayName
	}
	if fu.RateLimit != 0 {
		u.RateLimit = &fu.RateLimit
	}
	return u
}

// ── CRIT-1/2: Internal Firestore types for BudgetRecord, GrantRecord, AuditRecord ──
// These types carry firestore:" tags so the shared storage types remain
// backend-agnostic. Without these, the Firestore SDK falls back to lowercased
// Go field names (e.g. "limitusd" instead of "limit_usd"), corrupting reads.

type firestoreBudget struct {
	UserID        string    `firestore:"user_id"`
	LimitUSD      float64   `firestore:"limit_usd"`
	SpentUSD      float64   `firestore:"spent_usd"`
	TokensUsed    int64     `firestore:"tokens_used"`
	AllTokensUsed int64     `firestore:"all_tokens_used"`
	PeriodType    string    `firestore:"period_type,omitempty"`
	PeriodKey     string    `firestore:"period_key,omitempty"`
	PeriodStart   time.Time `firestore:"period_start,omitempty"`
	PeriodEnd     time.Time `firestore:"period_end,omitempty"`
}

func budgetToFirestore(b *storage.BudgetRecord) *firestoreBudget {
	return &firestoreBudget{
		UserID:        b.UserID,
		LimitUSD:      b.LimitUSD,
		SpentUSD:      b.SpentUSD,
		TokensUsed:    b.TokensUsed,
		AllTokensUsed: b.AllTokensUsed,
		PeriodType:    b.PeriodType,
		PeriodKey:     b.PeriodKey,
		PeriodStart:   b.PeriodStart,
		PeriodEnd:     b.PeriodEnd,
	}
}

func firestoreToBudget(fb *firestoreBudget) *storage.BudgetRecord {
	return &storage.BudgetRecord{
		UserID:        fb.UserID,
		LimitUSD:      fb.LimitUSD,
		SpentUSD:      fb.SpentUSD,
		TokensUsed:    fb.TokensUsed,
		AllTokensUsed: fb.AllTokensUsed,
		PeriodType:    fb.PeriodType,
		PeriodKey:     fb.PeriodKey,
		PeriodStart:   fb.PeriodStart,
		PeriodEnd:     fb.PeriodEnd,
	}
}

type firestoreGrant struct {
	ID        string    `firestore:"id"`
	UserID    string    `firestore:"user_id"`
	AmountUSD float64   `firestore:"amount_usd"`
	SpentUSD  float64   `firestore:"spent_usd"`
	Reason    string    `firestore:"reason,omitempty"`
	GrantedBy string    `firestore:"granted_by,omitempty"`
	StartsAt  time.Time `firestore:"starts_at,omitempty"`
	ExpiresAt time.Time `firestore:"expires_at,omitempty"`
	CreatedAt time.Time `firestore:"created_at,omitempty"`
}

func grantToFirestore(g *storage.GrantRecord) *firestoreGrant {
	return &firestoreGrant{
		ID:        g.ID,
		UserID:    g.UserID,
		AmountUSD: g.AmountUSD,
		SpentUSD:  g.SpentUSD,
		Reason:    g.Reason,
		GrantedBy: g.GrantedBy,
		StartsAt:  g.StartsAt,
		ExpiresAt: g.ExpiresAt,
		CreatedAt: g.CreatedAt,
	}
}

func firestoreToGrant(fg *firestoreGrant) *storage.GrantRecord {
	return &storage.GrantRecord{
		ID:        fg.ID,
		UserID:    fg.UserID,
		AmountUSD: fg.AmountUSD,
		SpentUSD:  fg.SpentUSD,
		Reason:    fg.Reason,
		GrantedBy: fg.GrantedBy,
		StartsAt:  fg.StartsAt,
		ExpiresAt: fg.ExpiresAt,
		CreatedAt: fg.CreatedAt,
	}
}

type firestoreAudit struct {
	ID         string    `firestore:"id"`
	UserID     string    `firestore:"user_id"`
	ActorEmail string    `firestore:"actor_email"`
	Action     string    `firestore:"action"`
	Details    string    `firestore:"details,omitempty"`
	Timestamp  time.Time `firestore:"timestamp"`
}

func auditToFirestore(a *storage.AuditRecord) *firestoreAudit {
	return &firestoreAudit{
		ID:         a.ID,
		UserID:     a.UserID,
		ActorEmail: a.ActorEmail,
		Action:     a.Action,
		Details:    a.Details,
		Timestamp:  a.Timestamp,
	}
}

func firestoreToAudit(fa *firestoreAudit) *storage.AuditRecord {
	return &storage.AuditRecord{
		ID:         fa.ID,
		UserID:     fa.UserID,
		ActorEmail: fa.ActorEmail,
		Action:     fa.Action,
		Details:    fa.Details,
		Timestamp:  fa.Timestamp,
	}
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
	_, err := ref.Create(ctx, userToFirestore(user))
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
	// Build a targeted update list from pointer fields:
	// - nil pointer means "leave unchanged"
	// - non-nil pointer (including &""/&0) means "write this value"
	// Plain string fields (Role, Status) are included when non-empty, as they
	// are always set by the handlers that call UpdateUser for those paths.
	var updates []firestore.Update
	if user.DisplayName != nil {
		updates = append(updates, firestore.Update{Path: "display_name", Value: *user.DisplayName})
	}
	if user.Role != "" {
		updates = append(updates, firestore.Update{Path: "role", Value: user.Role})
	}
	if user.Status != "" {
		updates = append(updates, firestore.Update{Path: "status", Value: user.Status})
	}
	if user.RateLimit != nil {
		updates = append(updates, firestore.Update{Path: "rate_limit", Value: *user.RateLimit})
		// Evict the rate-limit cache so the new value takes effect immediately.
		evictRateLimitCache(id)
	}
	if !user.LastSeenAt.IsZero() {
		updates = append(updates, firestore.Update{Path: "last_seen_at", Value: user.LastSeenAt})
	}
	if len(updates) == 0 {
		return nil // nothing to update
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

func (s *Store) TouchLastActive(ctx context.Context, id string) error {
	id = sanitizeID(id)
	ref := s.client.Collection(usersCol).Doc(id)
	_, err := ref.Update(ctx, []firestore.Update{
		{Path: "last_active_at", Value: time.Now().UTC()},
	})
	if err != nil {
		return fmt.Errorf("firestoredb: touching last_active: %w", err)
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	id = sanitizeID(id)
	userRef := s.client.Collection(usersCol).Doc(id)

	// Discover and delete all subcollections (Firestore doesn't cascade).
	// Use a single BulkWriter across all subcollections for efficiency.
	// CRIT-7: collect individual write result futures so errors are not
	// silently swallowed when bw.End() flushes the queue.
	bw := s.client.BulkWriter(ctx)
	type deleteJob struct {
		collID string
		docID  string
		job    *firestore.BulkWriterJob
	}
	var jobs []deleteJob

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
			job, err := bw.Delete(docRef)
			if err != nil {
				// Enqueue error (pre-flight validation) — doc ref is invalid.
				slog.WarnContext(ctx, "bulk delete enqueue failed",
					"collection", collRef.ID, "doc", docRef.ID, "error", err)
				continue
			}
			jobs = append(jobs, deleteJob{collID: collRef.ID, docID: docRef.ID, job: job})
		}
	}
	// Flush all enqueued deletes and wait for results.
	bw.End()

	// Check each write result for errors — previously these were silently ignored.
	var errs []error
	for _, j := range jobs {
		if _, err := j.job.Results(); err != nil {
			slog.ErrorContext(ctx, "bulk delete write failed",
				"collection", j.collID, "doc", j.docID, "error", err)
			errs = append(errs, fmt.Errorf("%s/%s: %w", j.collID, j.docID, err))
		}
	}
	if len(errs) > 0 {
		// Return all errors via errors.Join so the caller can inspect the full
		// set of failures (Go 1.20+). Orphaned docs are preferable to silently
		// succeeding with partial deletion — caller can retry DeleteUser.
		return fmt.Errorf("firestoredb: %d subcollection doc(s) failed to delete: %w", len(errs), errors.Join(errs...))
	}

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
	_, err = ref.Set(ctx, budgetToFirestore(budget), firestore.Merge(
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

	// CRIT-12 (Option 3): Clear the soft_blocked flag set by DeductSpend
	// when an overdraft was detected. Admin calling ResetSpend is explicitly
	// acknowledging the overdraft and unblocking the user.
	configRef := s.client.Collection(usersCol).Doc(userID).
		Collection(budgetsCol).Doc(budgetConfigDocID)
	_, err = configRef.Set(ctx, map[string]any{
		"soft_blocked":    false,
		"soft_blocked_at": nil,
	}, firestore.Merge(
		firestore.FieldPath{"soft_blocked"},
		firestore.FieldPath{"soft_blocked_at"},
	))
	if err != nil {
		// Non-fatal: log but don't fail ResetSpend — spend is already cleared.
		slog.Warn("ResetSpend: failed to clear soft_blocked flag",
			"user_id", userID, "error", err)
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
	_, err := ref.Create(ctx, grantToFirestore(grant))
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
	// CRIT-15: block inactive users regardless of grant balance.
	// An admin who deactivates a user intends to block all LLM access, even if
	// grants remain. Without this check, a deactivated user with an active grant
	// would still pass the budget gate and be billed.
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		// Non-fatal: if we can't read user status, let budget/grant checks proceed
		// rather than blocking all traffic on a transient Firestore read error.
		slog.Warn("CheckBudget: could not read user status, proceeding",
			"user_id", userID, "error", err)
	} else if user != nil && user.Status == storage.StatusInactive {
		return &storage.BudgetCheckResult{
			Allowed:       false,
			RemainingUSD:  0,
			EstimatedCost: estimatedCostUSD,
		}, nil
	}

	// CRIT-12 (Option 3): Check soft_blocked flag on the config doc.
	// DeductSpend sets this atomically when an overdraft is detected (i.e. a
	// concurrent request drained the pool between CheckBudget and DeductSpend).
	// This flag fires on the NEXT request after an overdraft, not the one that
	// caused it — that's the inherent limit of Option 3 vs a full atomic merge.
	// An admin calling ResetSpend clears the flag.
	sanitizedUID := sanitizeID(userID)
	configRef := s.client.Collection(usersCol).Doc(sanitizedUID).
		Collection(budgetsCol).Doc(budgetConfigDocID)
	if configSnap, err := configRef.Get(ctx); err == nil && configSnap.Exists() {
		if blocked, _ := configSnap.Data()["soft_blocked"].(bool); blocked {
			slog.Warn("CheckBudget: user soft-blocked due to overdraft — admin must call ResetSpend",
				"user_id", sanitizedUID)
			return &storage.BudgetCheckResult{
				Allowed:       false,
				RemainingUSD:  0,
				EstimatedCost: estimatedCostUSD,
			}, nil
		}
	}

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
	// CRIT-5: capture time.Now() once before the transaction so retries see
	// a consistent timestamp (avoids crossing a minute boundary on retry).
	now := time.Now().UTC()
	// CRIT-6: bound the transaction with a timeout so a slow or contended
	// Firestore call can't block the proxy goroutine indefinitely.
	txCtx, txCancel := context.WithTimeout(ctx, 10*time.Second)
	defer txCancel()
	return s.client.RunTransaction(txCtx, func(ctx context.Context, tx *firestore.Transaction) error {
		// ── ALL READS FIRST (Firestore requirement) ──

		// Sanitize into a local so we don't mutate the outer parameter.
		// Firestore transactions auto-retry on contention — mutating the outer
		// userID would cause it to be double-sanitized on each retry.
		localUserID := sanitizeID(userID)
		userRef := s.client.Collection(usersCol).Doc(localUserID)

		// Read 1: Load active grants (earliest-expiring first).
		grantsRef := userRef.Collection(grantsCol)
		grantsSnaps, err := tx.Documents(grantsRef.
			Where("expires_at", ">", now).
			OrderBy("expires_at", firestore.Asc)).GetAll()
		if err != nil {
			return fmt.Errorf("firestoredb: loading grants in tx: %w", err)
		}

		// Read 2 (CRIT-14): Config doc — read BEFORE the period spend doc so we
		// can use the configured period_type to compute the correct period key.
		// Previously this was read after the period doc (hardcoded "daily"), which
		// meant monthly/weekly budgets always resolved to a daily spend doc and
		// never exhausted across the full period.
		configRef := userRef.Collection(budgetsCol).Doc(budgetConfigDocID)
		configSnap, configErr := tx.Get(configRef)
		if configErr != nil && status.Code(configErr) != codes.NotFound {
			return fmt.Errorf("firestoredb: getting budget config in tx: %w", configErr)
		}

		// Derive the period type and key from config (falls back to "daily").
		periodType := "daily"
		if configSnap != nil && configSnap.Exists() {
			if pt, _ := configSnap.Data()["period_type"].(string); pt != "" {
				periodType = pt
			}
		}
		periodKey := currentPeriodKey(periodType)

		// Read 3: Load budget period spend doc using the correct period key.
		budgetRef := userRef.Collection(budgetsCol).Doc(periodKey)
		budgetSnap, err := tx.Get(budgetRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return fmt.Errorf("firestoredb: getting budget in tx: %w", err)
		}

		// ── ALL WRITES AFTER READS ──

		remaining := costUSD

		// Write 1: Deduct from budget first.
		// Budget is the developer's primary pool. Grants act as overflow
		// when the budget is exhausted (budget-first waterfall).
		var b *storage.BudgetRecord
		var isNewPeriod bool
		if budgetSnap != nil && budgetSnap.Exists() {
			b, err = snapToBudget(budgetSnap)
			if err != nil {
				return err
			}
		} else if configSnap != nil && configSnap.Exists() {
			// #9: Auto-rollover — create the period spend doc from config.
			data := configSnap.Data()
			// #E: firestoreFloat handles int64 (Firestore whole numbers) and float64.
			limitUSD := firestoreFloat(data["limit_usd"])
			b = &storage.BudgetRecord{
				UserID:     localUserID,
				LimitUSD:   limitUSD,
				PeriodType: periodType, // already derived above from config
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

			// Pre-compute how much the active grants can absorb from the overflow.
			// Any overflow NOT covered by grants is still charged to the budget
			// (it happened, the user owes it — CheckBudget is the gate, not here).
			grantCoverage := float64(0)
			for _, snap := range grantsSnaps {
				if g, err2 := snapToGrant(snap); err2 == nil {
					grantCoverage += g.Remaining()
				}
			}
			overflow := math.Max(0, costUSD-budgetDeduct)
			unabsorbed := math.Max(0, overflow-grantCoverage)
			budgetCharge := budgetDeduct + unabsorbed

			if budgetCharge > 0 || tokens > 0 {
				// Attribute tokens proportional to the budget fraction absorbed
				// (including unabsorbed overflow charged to the budget).
				// budgetCharge = budgetDeduct + unabsorbed overflow, so the ratio
				// correctly reflects all cost attributed to the recurring budget.
				budgetTokens := tokens
				if costUSD > 0 && budgetCharge < costUSD {
					budgetTokens = int64(math.Round(float64(tokens) * (budgetCharge / costUSD)))
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
						SpentUSD:      budgetCharge,
						TokensUsed:    budgetTokens,
						AllTokensUsed: tokens,
					}
					// CRIT-11: use budgetToFirestore so the Firestore SDK writes
					// snake_case field names (limit_usd, spent_usd …), not the
					// lowercase Go field names (limitusd, spentusd …).
					if err := tx.Set(budgetRef, budgetToFirestore(newDoc)); err != nil {
						return fmt.Errorf("firestoredb: creating new period budget: %w", err)
					}
				} else {
					updates := []firestore.Update{
						{Path: "all_tokens_used", Value: b.AllTokensUsed + tokens},
						{Path: "tokens_used", Value: b.TokensUsed + budgetTokens},
						{Path: "spent_usd", Value: b.SpentUSD + budgetCharge},
					}
					if err := tx.Update(budgetRef, updates); err != nil {
						return fmt.Errorf("firestoredb: deducting from budget: %w", err)
					}
				}
			}
			// Decrement by budgetCharge so the grants waterfall only covers
			// cost not already absorbed (directly or as unabsorbed overflow)
			// by the recurring budget. Using budgetDeduct here would cause
			// the same overflow to be double-charged to a grant.
			remaining -= budgetCharge
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

		// CRIT-12 (Option 3): If cost couldn't be fully absorbed, it means the
		// budget/grants were exhausted between CheckBudget passing and this
		// DeductSpend transaction committing (classic TOCTOU race).
		// We can't undo the LLM call, but we:
		//   1. Log a structured warning for ops reconciliation.
		//   2. Atomically set soft_blocked=true on the config doc so the NEXT
		//      request from this user is denied immediately by CheckBudget.
		//      This is within the same transaction — no extra round-trip.
		// An admin calling ResetSpend will clear the flag.
		if remaining > 0.000001 { // floating-point epsilon for near-zero residuals
			slog.Warn("deduct_spend: unabsorbed cost — balance exhausted by concurrent request; user soft-blocked",
				"user_id", localUserID,
				"total_cost_usd", costUSD,
				"unabsorbed_usd", remaining,
			)
			// Write soft_blocked flag atomically into the config doc.
			// MergeAll preserves all other config fields (limit_usd, period_type…).
			if err := tx.Set(configRef, map[string]any{
				"soft_blocked":    true,
				"soft_blocked_at": now,
			}, firestore.MergeAll); err != nil {
				// Non-fatal: if this write fails the deduction already landed.
				// Log at ERROR so ops know the block didn't persist.
				slog.Error("deduct_spend: failed to set soft_blocked flag — user NOT blocked",
					"user_id", localUserID, "error", err)
			}
		}

		return nil
	})
}

// ──────────────────────────────────────────
// Rate Limiting
// ──────────────────────────────────────────

func (s *Store) CheckRateLimit(ctx context.Context, userID string) (bool, int, int, error) {
	// Resolve the user's configured rate limit.
	// Use a short in-process cache so we don't issue a Firestore GetUser read
	// on every proxied LLM request (hot path). RateLimit only changes on
	// explicit admin updates, so a 5-minute TTL is safe.
	sanitized := sanitizeID(userID)
	limit, cached := getCachedRateLimit(sanitized)
	if !cached {
		user, err := s.GetUser(ctx, userID)
		if err != nil {
			return false, 0, 0, err
		}
		if user.RateLimit != nil {
			limit = *user.RateLimit
		}
		if limit <= 0 {
			limit = defaultRateLimit
		}
		setCachedRateLimit(sanitized, limit)
	}

	// Window key: minute-level granularity.
	// CRIT-4: use `sanitized` consistently — `userID` was already sanitized
	// above. The previous re-sanitization on the outer param was redundant
	// dead code (sanitizeID is idempotent but misleading to re-call).
	// CRIT-5 (rate limit): capture now and windowKey before the transaction so
	// retries see the same minute bucket even if a retry crosses a minute boundary.
	now := time.Now().UTC()
	windowKey := fmt.Sprintf("%s:%s", sanitized, now.Format("2006-01-02T15:04"))
	ref := s.client.Collection(rateLimitCol).Doc(windowKey)

	var count int
	// CRIT-6: bound the rate-limit transaction with a timeout — this runs on
	// the hot proxy path; a stuck Firestore call would block every request.
	rlCtx, rlCancel := context.WithTimeout(ctx, 5*time.Second)
	defer rlCancel()
	err := s.client.RunTransaction(rlCtx, func(ctx context.Context, tx *firestore.Transaction) error {
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
			"user_id":       sanitized,
			"request_count": count,
			"limit":         limit,
			"window_key":    windowKey,
			"expire_at":     now.Add(2 * time.Minute),
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
	_, err := ref.Create(ctx, auditToFirestore(entry))
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
	_, err := ref.Set(ctx, auditToFirestore(entry))
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
	var fu firestoreUser
	if err := snap.DataTo(&fu); err != nil {
		return nil, fmt.Errorf("decoding user: %w", err)
	}
	u := firestoreToUser(&fu)
	u.ID = snap.Ref.ID
	return u, nil
}

func snapToBudget(snap *firestore.DocumentSnapshot) (*storage.BudgetRecord, error) {
	var fb firestoreBudget
	if err := snap.DataTo(&fb); err != nil {
		return nil, fmt.Errorf("decoding budget: %w", err)
	}
	return firestoreToBudget(&fb), nil
}

func snapToGrant(snap *firestore.DocumentSnapshot) (*storage.GrantRecord, error) {
	var fg firestoreGrant
	if err := snap.DataTo(&fg); err != nil {
		return nil, fmt.Errorf("decoding grant: %w", err)
	}
	fg.ID = snap.Ref.ID
	return firestoreToGrant(&fg), nil
}

func snapToAudit(snap *firestore.DocumentSnapshot) (*storage.AuditRecord, error) {
	var fa firestoreAudit
	if err := snap.DataTo(&fa); err != nil {
		return nil, fmt.Errorf("decoding audit: %w", err)
	}
	fa.ID = snap.Ref.ID
	return firestoreToAudit(&fa), nil
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
