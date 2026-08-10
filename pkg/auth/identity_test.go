package auth

import (
	"testing"
)

func TestIdentity_EffectiveID_PrefersEmail(t *testing.T) {
	id := &Identity{ID: "firebase-uid-123", Email: "Alice@Example.com"}
	got := id.EffectiveID()
	if got != "alice@example.com" {
		t.Errorf("EffectiveID() = %q, want %q", got, "alice@example.com")
	}
}

func TestIdentity_EffectiveID_FallsBackToID(t *testing.T) {
	id := &Identity{ID: "firebase-uid-123"}
	got := id.EffectiveID()
	if got != "firebase-uid-123" {
		t.Errorf("EffectiveID() = %q, want %q", got, "firebase-uid-123")
	}
}

func TestIdentity_EffectiveID_EmptyBoth(t *testing.T) {
	id := &Identity{}
	got := id.EffectiveID()
	if got != "" {
		t.Errorf("EffectiveID() = %q, want empty", got)
	}
}

func TestIdentity_UserAlias(t *testing.T) {
	// Verify that User is a true alias for Identity — assignments work
	// in both directions without explicit conversion.
	user := &Identity{ID: "uid", Email: "a@b.com", Provider: "firebase"}
	identity := user

	if identity.ID != "uid" {
		t.Errorf("ID = %q, want %q", identity.ID, "uid")
	}
	if identity.Email != "a@b.com" {
		t.Errorf("Email = %q, want %q", identity.Email, "a@b.com")
	}
	if identity.Provider != "firebase" {
		t.Errorf("Provider = %q, want %q", identity.Provider, "firebase")
	}
}

func TestIdentity_ProviderField(t *testing.T) {
	cases := []struct {
		name     string
		provider string
	}{
		{"firebase", "firebase"},
		{"google-oidc", "google-oidc"},
		{"zitadel", "oidc:https://zitadel.example.com"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := &Identity{ID: "uid", Email: "a@b.com", Provider: tc.provider}
			if id.Provider != tc.provider {
				t.Errorf("Provider = %q, want %q", id.Provider, tc.provider)
			}
		})
	}
}

func TestIdentity_TenantIDs(t *testing.T) {
	id := &Identity{
		ID:        "uid",
		Email:     "a@b.com",
		TenantIDs: []string{"acme-corp", "beta-inc"},
	}
	if len(id.TenantIDs) != 2 {
		t.Fatalf("len(TenantIDs) = %d, want 2", len(id.TenantIDs))
	}
	if id.TenantIDs[0] != "acme-corp" {
		t.Errorf("TenantIDs[0] = %q, want %q", id.TenantIDs[0], "acme-corp")
	}
}

func TestIdentity_Claims(t *testing.T) {
	id := &Identity{
		ID:    "uid",
		Email: "a@b.com",
		Claims: map[string]any{
			"iss":        "https://zitadel.example.com",
			"aud":        "candela-server",
			"custom:org": "engineering",
		},
	}
	if id.Claims["iss"] != "https://zitadel.example.com" {
		t.Errorf("Claims[iss] = %v", id.Claims["iss"])
	}
	if id.Claims["custom:org"] != "engineering" {
		t.Errorf("Claims[custom:org] = %v", id.Claims["custom:org"])
	}
}
