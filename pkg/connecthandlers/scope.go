package connecthandlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
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
// In dev mode (auth.DevModeFromContext(ctx) is true), unauthenticated callers
// are permitted with empty string scoping (admin-like access for local development).
// In production mode, a nil caller identity fails closed by returning
// connect.CodeUnauthenticated (#627).
//
// The returned ID is the sanitized email (matching Firestore doc IDs and
// the user_id written to BQ spans by the proxy).
//
// If no UserStore is available (e.g. local dev without Firestore),
// returns empty string (admin-like access).
func scopeUserID(ctx context.Context, users storage.UserStore) (string, error) {
	caller := auth.FromContext(ctx)
	if caller == nil {
		if auth.DevModeFromContext(ctx) {
			return "", nil // unauthenticated allowed in explicit dev mode
		}
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated: missing caller identity"))
	}

	if users == nil {
		if auth.DevModeFromContext(ctx) {
			return "", nil // no user store in dev mode = unscoped access
		}
		return "", connect.NewError(connect.CodeFailedPrecondition, errors.New("user store unavailable"))
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
		// Unknown user — scope to sanitized email, falling back to user ID for safety.
		if caller.Email != "" {
			return strings.ToLower(caller.Email), nil
		}
		return caller.ID, nil
	}

	if record.Role == storage.RoleAdmin {
		return "", nil // admins see everything
	}

	return record.ID, nil // Firestore doc ID = sanitized email
}
