package proxy

import (
	"github.com/candelahq/candela/pkg/auth"
	"pgregory.net/rapid"
	"strings"
	"testing"
)

// Property: isServiceAccountID(".iam.gserviceaccount.com" suffix) = true
func TestProperty_IsServiceAccountIDTrue(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		prefix := rapid.String().Draw(t, "prefix")

		id := prefix + ".iam.gserviceaccount.com"
		if !isServiceAccountID(id) {
			t.Fatalf("expected true for %q", id)
		}
	})
}

// Property: isServiceAccountID(no SA suffix) = false
func TestProperty_IsServiceAccountIDFalse(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		id := rapid.String().Filter(func(s string) bool {
			return !strings.HasSuffix(s, ".iam.gserviceaccount.com")
		}).Draw(t, "id")

		if isServiceAccountID(id) {
			t.Fatalf("expected false for %q", id)
		}
	})
}

// Property: Any request with valid auth gets a non-empty effectiveUserID
func TestProperty_ValidAuthHasEffectiveUserID(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		email := rapid.StringMatching(`[a-z0-9]+@[a-z0-9]+\.[a-z]+`).Draw(t, "email")

		// This property verifies the interaction between Identity and EffectiveID,
		// ensuring that a validly formed Identity (which represents valid auth)
		// will never return an empty effective UserID.
		identity := &auth.Identity{
			Email: email,
		}

		if identity.EffectiveID() == "" {
			t.Fatalf("valid auth yielded empty effective user ID")
		}
	})
}
