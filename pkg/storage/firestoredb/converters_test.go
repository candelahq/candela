package firestoredb

import (
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// ─── userToFirestore / firestoreToUser ───────────────────────────────────────

func TestUserToFirestore_NilPointers(t *testing.T) {
	u := &storage.UserRecord{
		ID:    "alice@example.com",
		Email: "alice@example.com",
		Role:  "developer",
	}
	fu := userToFirestore(u)
	if fu.DisplayName != "" {
		t.Errorf("DisplayName = %q, want empty", fu.DisplayName)
	}
	if fu.RateLimit != 0 {
		t.Errorf("RateLimit = %d, want 0", fu.RateLimit)
	}
}

func TestUserToFirestore_SetPointers(t *testing.T) {
	name := "Alice"
	limit := 100
	u := &storage.UserRecord{
		ID:          "alice@example.com",
		Email:       "alice@example.com",
		DisplayName: &name,
		RateLimit:   &limit,
	}
	fu := userToFirestore(u)
	if fu.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want Alice", fu.DisplayName)
	}
	if fu.RateLimit != 100 {
		t.Errorf("RateLimit = %d, want 100", fu.RateLimit)
	}
}

func TestUserToFirestore_ZeroRateLimit(t *testing.T) {
	// &0 means "explicitly set to zero" — should round-trip
	zero := 0
	u := &storage.UserRecord{Email: "x@example.com", RateLimit: &zero}
	fu := userToFirestore(u)
	// Note: omitempty on firestore tag means 0 won't be stored, but the
	// converter itself should still copy the value.
	if fu.RateLimit != 0 {
		t.Errorf("RateLimit = %d, want 0", fu.RateLimit)
	}
}

func TestFirestoreToUser_EmptyFields(t *testing.T) {
	fu := &firestoreUser{ID: "id1", Email: "x@example.com"}
	u := firestoreToUser(fu)
	if u.DisplayName != nil {
		t.Errorf("DisplayName should be nil, got %v", *u.DisplayName)
	}
	if u.RateLimit != nil {
		t.Errorf("RateLimit should be nil, got %v", *u.RateLimit)
	}
}

func TestFirestoreToUser_PopulatedFields(t *testing.T) {
	fu := &firestoreUser{
		ID:          "id1",
		Email:       "x@example.com",
		DisplayName: "Alice",
		RateLimit:   50,
	}
	u := firestoreToUser(fu)
	if u.DisplayName == nil || *u.DisplayName != "Alice" {
		t.Errorf("DisplayName = %v, want &Alice", u.DisplayName)
	}
	if u.RateLimit == nil || *u.RateLimit != 50 {
		t.Errorf("RateLimit = %v, want &50", u.RateLimit)
	}
}

func TestUserRoundTrip(t *testing.T) {
	name := "Bob"
	limit := 200
	original := &storage.UserRecord{
		ID:          "bob@example.com",
		Email:       "bob@example.com",
		DisplayName: &name,
		Role:        "admin",
		Status:      "active",
		RateLimit:   &limit,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	got := firestoreToUser(userToFirestore(original))
	if got.Email != original.Email {
		t.Errorf("Email = %q, want %q", got.Email, original.Email)
	}
	if got.DisplayName == nil || *got.DisplayName != *original.DisplayName {
		t.Errorf("DisplayName mismatch")
	}
	if got.RateLimit == nil || *got.RateLimit != *original.RateLimit {
		t.Errorf("RateLimit mismatch")
	}
}

// ─── budgetToFirestore / firestoreToBudget ───────────────────────────────────

func TestBudgetRoundTrip(t *testing.T) {
	b := &storage.BudgetRecord{
		UserID:        "alice@example.com",
		LimitUSD:      100.0,
		SpentUSD:      42.5,
		TokensUsed:    1000,
		AllTokensUsed: 1500,
		PeriodType:    "daily",
		PeriodKey:     "2026-05-10",
	}
	got := firestoreToBudget(budgetToFirestore(b))
	if got.UserID != b.UserID {
		t.Errorf("UserID = %q, want %q", got.UserID, b.UserID)
	}
	if got.LimitUSD != b.LimitUSD {
		t.Errorf("LimitUSD = %f, want %f", got.LimitUSD, b.LimitUSD)
	}
	if got.SpentUSD != b.SpentUSD {
		t.Errorf("SpentUSD = %f, want %f", got.SpentUSD, b.SpentUSD)
	}
	if got.TokensUsed != b.TokensUsed {
		t.Errorf("TokensUsed = %d, want %d", got.TokensUsed, b.TokensUsed)
	}
	if got.AllTokensUsed != b.AllTokensUsed {
		t.Errorf("AllTokensUsed = %d, want %d", got.AllTokensUsed, b.AllTokensUsed)
	}
	if got.PeriodKey != b.PeriodKey {
		t.Errorf("PeriodKey = %q, want %q", got.PeriodKey, b.PeriodKey)
	}
}

// ─── grantToFirestore / firestoreToGrant ─────────────────────────────────────

func TestGrantRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	g := &storage.GrantRecord{
		ID:        "grant-1",
		UserID:    "alice@example.com",
		AmountUSD: 50.0,
		SpentUSD:  10.0,
		Reason:    "hackathon",
		GrantedBy: "admin@example.com",
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		CreatedAt: now,
	}
	got := firestoreToGrant(grantToFirestore(g))
	if got.ID != g.ID {
		t.Errorf("ID = %q, want %q", got.ID, g.ID)
	}
	if got.AmountUSD != g.AmountUSD {
		t.Errorf("AmountUSD = %f, want %f", got.AmountUSD, g.AmountUSD)
	}
	if got.SpentUSD != g.SpentUSD {
		t.Errorf("SpentUSD = %f, want %f", got.SpentUSD, g.SpentUSD)
	}
	if got.Reason != g.Reason {
		t.Errorf("Reason = %q, want %q", got.Reason, g.Reason)
	}
	if !got.ExpiresAt.Equal(g.ExpiresAt) {
		t.Errorf("ExpiresAt mismatch")
	}
}

func TestGrantRemaining(t *testing.T) {
	g := &storage.GrantRecord{AmountUSD: 100.0, SpentUSD: 30.0}
	if got := g.Remaining(); got != 70.0 {
		t.Errorf("Remaining() = %f, want 70.0", got)
	}
}

// ─── auditToFirestore / firestoreToAudit ─────────────────────────────────────

func TestAuditRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	a := &storage.AuditRecord{
		ID:         "audit-1",
		UserID:     "alice@example.com",
		ActorEmail: "admin@example.com",
		Action:     "deactivate_user",
		Details:    `{"reason":"policy violation"}`,
		Timestamp:  now,
	}
	got := firestoreToAudit(auditToFirestore(a))
	if got.ID != a.ID {
		t.Errorf("ID = %q, want %q", got.ID, a.ID)
	}
	if got.ActorEmail != a.ActorEmail {
		t.Errorf("ActorEmail = %q, want %q", got.ActorEmail, a.ActorEmail)
	}
	if got.Action != a.Action {
		t.Errorf("Action = %q, want %q", got.Action, a.Action)
	}
	if got.Details != a.Details {
		t.Errorf("Details = %q, want %q", got.Details, a.Details)
	}
	if !got.Timestamp.Equal(a.Timestamp) {
		t.Errorf("Timestamp mismatch: got %v want %v", got.Timestamp, a.Timestamp)
	}
}
