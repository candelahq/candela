package connecthandlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/storage"
)

// scopeUserID returns the authenticated user's ID for non-admin users,
// or empty string for admins (meaning "all users").
//
// This implements the access control rule:
//   - Admins see all traces across the organization
//   - Developers see only their own traces
//
// The returned ID is the sanitized email (matching Firestore doc IDs and
// the user_id written to BQ spans by the proxy).
//
// If no UserStore is available (e.g. local dev without Firestore),
// returns empty string (admin-like access).
func scopeUserID(ctx context.Context, users storage.UserStore) string {
	if users == nil {
		return "" // no user store = no scoping
	}

	caller := auth.FromContext(ctx)
	if caller == nil {
		return "" // unauthenticated (dev mode)
	}

	// Look up the caller's role in the user store.
	record, err := users.GetUserByEmail(ctx, caller.Email)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			slog.Debug("user not found in store, scoping to own ID", "email", caller.Email)
		} else {
			slog.Warn("failed to look up user role, scoping to own ID",
				"email", caller.Email, "error", err)
		}
		// Unknown user — scope to sanitized email (matching proxy span attribution).
		return strings.ToLower(caller.Email)
	}

	if record.Role == storage.RoleAdmin {
		return "" // admins see everything
	}

	return record.ID // Firestore doc ID = sanitized email
}
