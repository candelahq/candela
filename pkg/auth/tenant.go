package auth

import (
	"errors"
)

type TenantMode string

const (
	TenantModeOpen     TenantMode = "open"
	TenantModeVerified TenantMode = "verified"
)

type TenantValidator struct {
	Mode TenantMode
}

func NewTenantValidator(mode TenantMode) *TenantValidator {
	return &TenantValidator{Mode: mode}
}

func (v *TenantValidator) Validate(identity *Identity, requestedTenant string) error {
	if v.Mode == TenantModeOpen {
		return nil
	}

	if requestedTenant == "" {
		return errors.New("tenant ID required")
	}

	for _, t := range identity.TenantIDs {
		if t == requestedTenant {
			return nil
		}
	}

	return errors.New("unauthorized for tenant")
}
