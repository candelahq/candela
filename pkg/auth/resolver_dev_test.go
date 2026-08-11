package auth

import (
	"context"
	"testing"
)

func TestDevResolver(t *testing.T) {
	resolver := NewDevResolver()
	id, err := resolver.Resolve(context.Background(), "any")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.ID != "dev-admin" {
		t.Errorf("expected dev-admin, got %s", id.ID)
	}
	if id.Provider != "dev" {
		t.Errorf("expected dev provider, got %s", id.Provider)
	}
}
