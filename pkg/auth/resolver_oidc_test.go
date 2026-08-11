package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestExtractClaimString(t *testing.T) {
	claims := map[string]interface{}{
		"sub": "user123",
		"num": 123,
	}

	if got := extractClaimString(claims, "sub"); got != "user123" {
		t.Errorf("expected 'user123', got %q", got)
	}
	if got := extractClaimString(claims, "num"); got != "" {
		t.Errorf("expected '', got %q", got)
	}
	if got := extractClaimString(claims, "missing"); got != "" {
		t.Errorf("expected '', got %q", got)
	}
	if got := extractClaimString(claims, ""); got != "" {
		t.Errorf("expected '', got %q", got)
	}
}

func TestExtractClaimStringSlice(t *testing.T) {
	claims := map[string]interface{}{
		"roles": []string{"admin", "user"},
		"perms": []interface{}{"read", "write", 123}, // ignore 123
		"str":   "not-a-slice",
	}

	gotRoles := extractClaimStringSlice(claims, "roles")
	if len(gotRoles) != 2 || gotRoles[0] != "admin" || gotRoles[1] != "user" {
		t.Errorf("expected [admin user], got %v", gotRoles)
	}

	gotPerms := extractClaimStringSlice(claims, "perms")
	if len(gotPerms) != 2 || gotPerms[0] != "read" || gotPerms[1] != "write" {
		t.Errorf("expected [read write], got %v", gotPerms)
	}

	if got := extractClaimStringSlice(claims, "str"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	if got := extractClaimStringSlice(claims, "missing"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestOIDCResolver(t *testing.T) {
	// 1. Generate RSA Key Pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}

	// Create JSON Web Key
	jwk := jose.JSONWebKey{
		Key:       privateKey,
		KeyID:     "test-key",
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}
	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{jwk.Public()},
	}

	// 2. Setup mock OIDC server
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	issuer := server.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		config := map[string]interface{}{
			"issuer":                                issuer,
			"jwks_uri":                              issuer + "/.well-known/jwks.json",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	})

	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	})

	// 3. Create OIDCResolver
	ctx := context.Background()
	resolver, err := NewOIDCResolver(ctx, issuer, "test-client", DefaultOIDCClaimMapping())
	if err != nil {
		t.Fatalf("failed to create resolver: %v", err)
	}

	if resolver.Name() != "oidc:"+issuer {
		t.Errorf("expected name 'oidc:%s', got %q", issuer, resolver.Name())
	}

	// 4. Helper to sign a token
	signToken := func(claims map[string]interface{}, overrideIssuer string, overrideAudience string) string {
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: privateKey}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-key"))
		if err != nil {
			t.Fatalf("failed to create signer: %v", err)
		}

		iss := issuer
		if overrideIssuer != "" {
			iss = overrideIssuer
		}
		aud := "test-client"
		if overrideAudience != "" {
			aud = overrideAudience
		}

		cl := jwt.Claims{
			Issuer:   iss,
			Audience: jwt.Audience{aud},
			Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt: jwt.NewNumericDate(time.Now()),
		}

		builder := jwt.Signed(signer).Claims(cl).Claims(claims)
		rawToken, err := builder.Serialize()
		if err != nil {
			t.Fatalf("failed to serialize token: %v", err)
		}
		return rawToken
	}

	t.Run("Valid Token", func(t *testing.T) {
		token := signToken(map[string]interface{}{
			"sub":             "user123",
			"email":           "Test@Example.com",
			"candela.tenants": []string{"tenant-1", "tenant-2"},
		}, "", "")

		identity, err := resolver.Resolve(ctx, token)
		if err != nil {
			t.Fatalf("resolve failed: %v", err)
		}
		if identity == nil {
			t.Fatalf("expected identity, got nil")
		}

		if identity.ID != "user123" {
			t.Errorf("expected ID 'user123', got %q", identity.ID)
		}
		if identity.Email != "test@example.com" { // testing normalization
			t.Errorf("expected email 'test@example.com', got %q", identity.Email)
		}
		if identity.Provider != "oidc:"+issuer {
			t.Errorf("expected provider 'oidc:%s', got %q", issuer, identity.Provider)
		}
		if len(identity.TenantIDs) != 2 || identity.TenantIDs[0] != "tenant-1" || identity.TenantIDs[1] != "tenant-2" {
			t.Errorf("expected tenants [tenant-1 tenant-2], got %v", identity.TenantIDs)
		}
	})

	t.Run("Missing Email", func(t *testing.T) {
		token := signToken(map[string]interface{}{
			"sub": "user123",
		}, "", "")

		_, err := resolver.Resolve(ctx, token)
		if err == nil || err.Error() != resolver.Name()+": token missing email claim" {
			t.Errorf("expected missing email error, got %v", err)
		}
	})

	t.Run("Wrong Issuer", func(t *testing.T) {
		token := signToken(map[string]interface{}{
			"sub":   "user123",
			"email": "test@example.com",
		}, "https://wrong-issuer.com", "")

		identity, err := resolver.Resolve(ctx, token)
		if err != nil {
			t.Errorf("expected no error for wrong issuer, got %v", err)
		}
		if identity != nil {
			t.Errorf("expected nil identity for wrong issuer, got %v", identity)
		}
	})

	t.Run("Wrong Audience", func(t *testing.T) {
		token := signToken(map[string]interface{}{
			"sub":   "user123",
			"email": "test@example.com",
		}, "", "wrong-audience")

		identity, err := resolver.Resolve(ctx, token)
		if err != nil {
			t.Errorf("expected no error for wrong audience, got %v", err)
		}
		if identity != nil {
			t.Errorf("expected nil identity for wrong audience, got %v", identity)
		}
	})
}
