package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockResolver struct {
	name        string
	resolveFunc func(ctx context.Context, token string) (*Identity, error)
}

func (m *mockResolver) Name() string { return m.name }
func (m *mockResolver) Resolve(ctx context.Context, token string) (*Identity, error) {
	return m.resolveFunc(ctx, token)
}

func TestResolverChain_CacheHit(t *testing.T) {
	cache := NewIdentityCache(10, time.Minute)
	id := &Identity{ID: "cached-id"}
	cache.Put("test-token", id)

	called := false
	resolver := &mockResolver{
		name: "mock",
		resolveFunc: func(ctx context.Context, token string) (*Identity, error) {
			called = true
			return nil, nil
		},
	}
	chain := NewResolverChain(cache, resolver)
	res, err := chain.Resolve(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != id {
		t.Fatalf("expected %v, got %v", id, res)
	}
	if called {
		t.Fatal("resolver should not have been called on cache hit")
	}
}

func TestResolverChain_Fallback(t *testing.T) {
	r1 := &mockResolver{
		name: "r1",
		resolveFunc: func(ctx context.Context, token string) (*Identity, error) {
			return nil, nil
		},
	}
	r2 := &mockResolver{
		name: "r2",
		resolveFunc: func(ctx context.Context, token string) (*Identity, error) {
			return &Identity{ID: "r2-id"}, nil
		},
	}
	chain := NewResolverChain(nil, r1, r2)
	res, err := chain.Resolve(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ID != "r2-id" {
		t.Fatalf("expected r2-id, got %s", res.ID)
	}
}

func TestResolverChain_Error(t *testing.T) {
	r1 := &mockResolver{
		name: "r1",
		resolveFunc: func(ctx context.Context, token string) (*Identity, error) {
			return nil, errors.New("r1 error")
		},
	}
	chain := NewResolverChain(nil, r1)
	_, err := chain.Resolve(context.Background(), "test-token")
	if err == nil || err.Error() != "r1 error" {
		t.Fatalf("expected 'r1 error', got %v", err)
	}
}
