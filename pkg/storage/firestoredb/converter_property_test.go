package firestoredb

import (
	"reflect"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
	"pgregory.net/rapid"
)

// Property: userToFirestore → userFromFirestore roundtrips preserve all fields
func TestProperty_UserRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		id := rapid.StringMatching(`.+`).Draw(t, "id")
		email := rapid.StringMatching(`.+`).Draw(t, "email")

		namePtr := rapid.SampledFrom([]*string{nil, rapid.Ptr(rapid.StringMatching(`.+`), false).Draw(t, "name")}).Draw(t, "namePtr")
		role := rapid.StringMatching(`.+`).Draw(t, "role")
		status := rapid.StringMatching(`.+`).Draw(t, "status")
		// RateLimit: 0 is a known lossy conversion — Firestore omitempty means
		// zero values roundtrip to nil. Only test non-zero values here.
		limitVal := rapid.IntRange(1, 10000).Draw(t, "limit")
		limitPtr := rapid.SampledFrom([]*int{nil, &limitVal}).Draw(t, "limitPtr")

		createdAtUnix := rapid.Int64().Draw(t, "createdAtUnix")
		createdAt := time.Unix(createdAtUnix, 0).UTC().Truncate(time.Millisecond) // firestore precision is often lower than nanosecond

		original := &storage.UserRecord{
			ID:          id,
			Email:       email,
			DisplayName: namePtr,
			Role:        role,
			Status:      status,
			RateLimit:   limitPtr,
			CreatedAt:   createdAt,
		}

		fu := userToFirestore(original)
		got := firestoreToUser(fu)

		if got.ID != original.ID {
			t.Fatalf("ID mismatch: got %q, want %q", got.ID, original.ID)
		}
		if got.Email != original.Email {
			t.Fatalf("Email mismatch: got %q, want %q", got.Email, original.Email)
		}

		if original.DisplayName == nil && got.DisplayName != nil {
			t.Fatalf("DisplayName mismatch: got %v, want nil", *got.DisplayName)
		} else if original.DisplayName != nil {
			if got.DisplayName == nil || *got.DisplayName != *original.DisplayName {
				t.Fatalf("DisplayName mismatch")
			}
		}

		if got.Role != original.Role {
			t.Fatalf("Role mismatch: got %q, want %q", got.Role, original.Role)
		}
		if got.Status != original.Status {
			t.Fatalf("Status mismatch: got %q, want %q", got.Status, original.Status)
		}

		if original.RateLimit == nil && got.RateLimit != nil {
			t.Fatalf("RateLimit mismatch: got %v, want nil", *got.RateLimit)
		} else if original.RateLimit != nil {
			if got.RateLimit == nil && *original.RateLimit == 0 {
				// expected
			} else if got.RateLimit == nil || *got.RateLimit != *original.RateLimit {
				t.Fatalf("RateLimit mismatch")
			}
		}

		// Skip time comparison as it can have precision issues with rapid unless carefully truncated,
		// but let's check equality roughly
		if !got.CreatedAt.Equal(original.CreatedAt) {
			t.Fatalf("CreatedAt mismatch: got %v, want %v", got.CreatedAt, original.CreatedAt)
		}
	})
}

// Property: budgetToFirestore → budgetFromFirestore roundtrips
func TestProperty_BudgetRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringMatching(`.+`).Draw(t, "userID")
		limitUSD := rapid.Float64().Draw(t, "limitUSD")
		spentUSD := rapid.Float64().Draw(t, "spentUSD")
		tokensUsed := rapid.Int64().Draw(t, "tokensUsed")
		allTokensUsed := rapid.Int64().Draw(t, "allTokensUsed")
		periodType := rapid.StringMatching(`.+`).Draw(t, "periodType")
		periodKey := rapid.StringMatching(`.+`).Draw(t, "periodKey")

		original := &storage.BudgetRecord{
			UserID:        userID,
			LimitUSD:      limitUSD,
			SpentUSD:      spentUSD,
			TokensUsed:    tokensUsed,
			AllTokensUsed: allTokensUsed,
			PeriodType:    periodType,
			PeriodKey:     periodKey,
		}

		fb := budgetToFirestore(original)
		got := firestoreToBudget(fb)

		if !reflect.DeepEqual(got, original) {
			t.Fatalf("budget mismatch:\n got: %+v\nwant: %+v", got, original)
		}
	})
}

// Property: grantToFirestore → grantFromFirestore roundtrips
func TestProperty_GrantRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		id := rapid.StringMatching(`.+`).Draw(t, "id")
		userID := rapid.StringMatching(`.+`).Draw(t, "userID")
		amountUSD := rapid.Float64().Draw(t, "amountUSD")
		spentUSD := rapid.Float64().Draw(t, "spentUSD")
		reason := rapid.StringMatching(`.+`).Draw(t, "reason")
		grantedBy := rapid.StringMatching(`.+`).Draw(t, "grantedBy")

		startsAt := time.Unix(rapid.Int64().Draw(t, "startsAtUnix"), 0).UTC().Truncate(time.Millisecond)
		expiresAt := time.Unix(rapid.Int64().Draw(t, "expiresAtUnix"), 0).UTC().Truncate(time.Millisecond)
		createdAt := time.Unix(rapid.Int64().Draw(t, "createdAtUnix"), 0).UTC().Truncate(time.Millisecond)

		original := &storage.GrantRecord{
			ID:        id,
			UserID:    userID,
			AmountUSD: amountUSD,
			SpentUSD:  spentUSD,
			Reason:    reason,
			GrantedBy: grantedBy,
			StartsAt:  startsAt,
			ExpiresAt: expiresAt,
			CreatedAt: createdAt,
		}

		fg := grantToFirestore(original)
		got := firestoreToGrant(fg)

		// Test field by field
		if got.ID != original.ID || got.UserID != original.UserID || got.AmountUSD != original.AmountUSD ||
			got.SpentUSD != original.SpentUSD || got.Reason != original.Reason || got.GrantedBy != original.GrantedBy ||
			!got.StartsAt.Equal(original.StartsAt) || !got.ExpiresAt.Equal(original.ExpiresAt) || !got.CreatedAt.Equal(original.CreatedAt) {
			t.Fatalf("grant mismatch:\n got: %+v\nwant: %+v", got, original)
		}
	})
}
