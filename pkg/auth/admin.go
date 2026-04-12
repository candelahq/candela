package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	connect "connectrpc.com/connect"
	"github.com/candelahq/candela/pkg/storage"
)

// adminOnlyProcedures lists the ConnectRPC procedures that require admin role.
// Self-service RPCs (GetCurrentUser, GetMyBudget) are NOT in this list.
var adminOnlyProcedures = map[string]bool{
	"/candela.v1.UserService/CreateUser":     true,
	"/candela.v1.UserService/ListUsers":      true,
	"/candela.v1.UserService/GetUser":        true,
	"/candela.v1.UserService/UpdateUser":     true,
	"/candela.v1.UserService/DeactivateUser": true,
	"/candela.v1.UserService/ReactivateUser": true,
	"/candela.v1.UserService/SetBudget":      true,
	"/candela.v1.UserService/GetBudget":      true,
	"/candela.v1.UserService/ResetSpend":     true,
	"/candela.v1.UserService/CreateGrant":    true,
	"/candela.v1.UserService/ListGrants":     true,
	"/candela.v1.UserService/RevokeGrant":    true,
	"/candela.v1.UserService/ListAuditLog":   true,
}

// AdminInterceptor returns a ConnectRPC interceptor that enforces admin-only
// access for UserService admin RPCs. It looks up the caller's role from the
// UserStore. If the user is not found (first login), they are not an admin.
//
// Self-service RPCs (GetCurrentUser, GetMyBudget) bypass this check.
func AdminInterceptor(userStore storage.UserStore) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure

			// Only enforce on admin-only procedures.
			if !adminOnlyProcedures[procedure] {
				return next(ctx, req)
			}

			// Must be authenticated.
			authUser := FromContext(ctx)
			if authUser == nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
			}

			// Look up the user's role from the store.
			user, err := userStore.GetUserByEmail(ctx, authUser.Email)
			if err != nil {
				if errors.Is(err, storage.ErrNotFound) {
					slog.WarnContext(ctx, "admin check: user not found",
						"email", authUser.Email, "procedure", procedure)
					return nil, connect.NewError(connect.CodePermissionDenied,
						fmt.Errorf("admin access required"))
				}
				return nil, connect.NewError(connect.CodeInternal,
					fmt.Errorf("failed to look up user role: %w", err))
			}

			if user.Role != storage.RoleAdmin {
				slog.WarnContext(ctx, "admin check: insufficient role",
					"email", authUser.Email, "role", user.Role, "procedure", procedure)
				return nil, connect.NewError(connect.CodePermissionDenied,
					fmt.Errorf("admin access required"))
			}

			return next(ctx, req)
		}
	}
}
