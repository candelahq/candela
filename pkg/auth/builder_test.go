package auth

import (
	"context"
	"testing"

	fbauth "firebase.google.com/go/v4/auth"
)

func TestBuildResolverChain_Empty(t *testing.T) {
	_, err := BuildResolverChain(context.Background(), nil, BuildOptions{})
	if err == nil {
		t.Fatal("expected error for empty resolvers with dev_mode off")
	}
}

func TestBuildResolverChain_DevModeOnly(t *testing.T) {
	chain, err := BuildResolverChain(context.Background(), nil, BuildOptions{DevMode: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	id, err := chain.Resolve(context.Background(), "anything")
	if err != nil {
		t.Fatalf("dev resolver should accept any token: %v", err)
	}
	if id.Provider != "dev" {
		t.Errorf("expected provider=dev, got %q", id.Provider)
	}
}

func TestBuildResolverChain_UnknownType(t *testing.T) {
	entries := []ResolverEntry{{Type: "magic"}}
	_, err := BuildResolverChain(context.Background(), entries, BuildOptions{})
	if err == nil {
		t.Fatal("expected error for unknown resolver type")
	}
}

func TestBuildResolverChain_OIDCMissingIssuer(t *testing.T) {
	entries := []ResolverEntry{{Type: "oidc", Audience: "my-app"}}
	_, err := BuildResolverChain(context.Background(), entries, BuildOptions{})
	if err == nil {
		t.Fatal("expected error for OIDC without issuer")
	}
}

func TestBuildResolverChain_OIDCMissingAudience(t *testing.T) {
	entries := []ResolverEntry{{Type: "oidc", Issuer: "https://example.com"}}
	_, err := BuildResolverChain(context.Background(), entries, BuildOptions{})
	if err == nil {
		t.Fatal("expected error for OIDC without audience")
	}
}

func TestBuildResolverChain_FirebaseWithoutVerifier(t *testing.T) {
	entries := []ResolverEntry{{Type: "firebase"}}
	_, err := BuildResolverChain(context.Background(), entries, BuildOptions{})
	if err == nil {
		t.Fatal("expected error for firebase without verifier")
	}
}

func TestBuildResolverChain_FirebaseDevModeNoVerifier(t *testing.T) {
	// In dev mode, DevResolver handles all tokens — firebase resolver
	// can be configured without a verifier for config portability.
	entries := []ResolverEntry{{Type: "firebase"}}
	chain, err := BuildResolverChain(context.Background(), entries, BuildOptions{DevMode: true})
	if err != nil {
		t.Fatalf("dev mode should allow firebase without verifier: %v", err)
	}
	// DevResolver is prepended, so any token resolves via dev
	id, err := chain.Resolve(context.Background(), "any-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Provider != "dev" {
		t.Errorf("expected dev provider in dev mode, got %q", id.Provider)
	}
}

func TestBuildResolverChain_FirebaseWithVerifier(t *testing.T) {
	entries := []ResolverEntry{{Type: "firebase"}}
	chain, err := BuildResolverChain(context.Background(), entries, BuildOptions{
		FirebaseVerifier: &mockTokenVerifier{
			verifyFunc: func(ctx context.Context, idToken string) (*fbauth.Token, error) {
				return &fbauth.Token{UID: "uid", Claims: map[string]interface{}{"email": "test@example.com"}}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

func TestBuildResolverChain_GoogleOAuth(t *testing.T) {
	entries := []ResolverEntry{{Type: "google_oauth"}}
	chain, err := BuildResolverChain(context.Background(), entries, BuildOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

func TestBuildResolverChain_GoogleOIDC(t *testing.T) {
	entries := []ResolverEntry{{Type: "google_oidc"}}
	chain, err := BuildResolverChain(context.Background(), entries, BuildOptions{
		CloudRunAudience: "https://candela.run.app",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}

func TestBuildResolverChain_MultipleResolvers(t *testing.T) {
	entries := []ResolverEntry{
		{Type: "google_oidc"},
		{Type: "google_oauth"},
	}
	chain, err := BuildResolverChain(context.Background(), entries, BuildOptions{
		DevMode: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have 3 resolvers: dev + google_oidc + google_oauth
	if chain == nil {
		t.Fatal("expected non-nil chain")
	}
}
