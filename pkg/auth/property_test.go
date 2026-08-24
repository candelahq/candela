package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Property: EffectiveID always returns lowercase
func TestProperty_EffectiveIDLowercase(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		email := rapid.String().Draw(t, "email")
		id := rapid.String().Draw(t, "id")

		identity := &Identity{
			Email: email,
			ID:    id,
		}

		eff := identity.EffectiveID()
		if email != "" {
			if eff != strings.ToLower(email) {
				t.Fatalf("expected lowercase email %q, got %q", strings.ToLower(email), eff)
			}
		}
	})
}

// Property: EffectiveID prefers email over ID
func TestProperty_EffectiveIDPrefersEmail(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		email := rapid.StringMatching(`.+`).Draw(t, "email") // Non-empty string
		id := rapid.String().Draw(t, "id")

		identity := &Identity{
			Email: email,
			ID:    id,
		}

		eff := identity.EffectiveID()
		if eff != strings.ToLower(email) {
			t.Fatalf("expected email %q, got %q", strings.ToLower(email), eff)
		}
	})
}

type propMockResolver struct {
	id  *Identity
	err error
}

func (m *propMockResolver) Name() string { return "mock" }
func (m *propMockResolver) Resolve(ctx context.Context, token string) (*Identity, error) {
	return m.id, m.err
}

// Property: ResolverChain returns first match (resolver ordering)
func TestProperty_ResolverChainReturnsFirstMatch(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		matchIdx := rapid.IntRange(0, 10).Draw(t, "matchIdx")

		var resolvers []IdentityResolver
		for i := 0; i < 15; i++ {
			if i == matchIdx {
				resolvers = append(resolvers, &propMockResolver{id: &Identity{ID: "match"}})
			} else {
				resolvers = append(resolvers, &propMockResolver{})
			}
		}

		chain := NewResolverChain(nil, resolvers...)
		id, err := chain.Resolve(context.Background(), "token")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if id == nil || id.ID != "match" {
			t.Fatalf("expected match, got %v", id)
		}
	})
}

// Property: ResolverChain rejects when any resolver returns error
func TestProperty_ResolverChainRejectsOnError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		errIdx := rapid.IntRange(0, 5).Draw(t, "errIdx")

		var resolvers []IdentityResolver
		for i := 0; i < 10; i++ {
			if i == errIdx {
				resolvers = append(resolvers, &propMockResolver{err: errors.New("boom")})
			} else if i > errIdx {
				// Should not be reached, but just in case, make it return success
				resolvers = append(resolvers, &propMockResolver{id: &Identity{ID: "late_match"}})
			} else {
				resolvers = append(resolvers, &propMockResolver{})
			}
		}

		chain := NewResolverChain(nil, resolvers...)
		id, err := chain.Resolve(context.Background(), "token")
		if err == nil {
			t.Fatalf("expected error, got success: %v", id)
		}
		if err.Error() != "boom" {
			t.Fatalf("expected 'boom', got %v", err)
		}
	})
}
