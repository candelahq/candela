package proxy

import (
	"fmt"
	"slices"
)

// checkAccessTags verifies the user has at least one tag matching the model's
// required access tags. Both lists are freeform strings — admin-defined, no
// predefined set required.
//
// Returns nil if access is granted, or a descriptive error for the 403 body.
//
// Rules:
//   - Empty requiredAccess on the model → open to all users.
//   - Empty userTags with non-empty requiredAccess → denied.
//   - User has ANY tag in requiredAccess → allowed.
func checkAccessTags(userTags, requiredAccess []string) error {
	if len(requiredAccess) == 0 {
		return nil // model is open to all
	}
	if len(userTags) == 0 {
		return fmt.Errorf("model requires access tags %v — contact your admin", requiredAccess)
	}
	for _, tag := range userTags {
		if slices.Contains(requiredAccess, tag) {
			return nil
		}
	}
	return fmt.Errorf(
		"model requires one of %v (you have %v) — contact your admin to upgrade",
		requiredAccess, userTags)
}

// checkTenantAccess verifies the request's tenant is in the model's allowed
// tenant list.
//
// Rules:
//   - Empty allowedTenants on the model → visible to all tenants.
//   - Empty tenantID with non-empty allowedTenants → denied.
//   - tenantID in allowedTenants → allowed.
func checkTenantAccess(tenantID string, allowedTenants []string) error {
	if len(allowedTenants) == 0 {
		return nil // model is visible to all tenants
	}
	if tenantID == "" {
		return fmt.Errorf("model restricted to specific tenants — provide X-Candela-Tenant-Id header")
	}
	if !slices.Contains(allowedTenants, tenantID) {
		return fmt.Errorf("model not available for tenant %q", tenantID)
	}
	return nil
}
