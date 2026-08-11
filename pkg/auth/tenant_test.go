package auth

import (
	"testing"
)

func TestTenantValidator(t *testing.T) {
	id := &Identity{TenantIDs: []string{"t1", "t2"}}

	openValidator := NewTenantValidator(TenantModeOpen)
	if err := openValidator.Validate(id, "t3"); err != nil {
		t.Errorf("open mode should accept any tenant, got: %v", err)
	}

	verifiedValidator := NewTenantValidator(TenantModeVerified)
	if err := verifiedValidator.Validate(id, "t1"); err != nil {
		t.Errorf("expected success for valid tenant, got: %v", err)
	}
	if err := verifiedValidator.Validate(id, "t3"); err == nil {
		t.Error("expected error for invalid tenant")
	}
	if err := verifiedValidator.Validate(id, ""); err == nil {
		t.Error("expected error for empty tenant")
	}
}
