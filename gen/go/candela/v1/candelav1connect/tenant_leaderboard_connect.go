// Hand-authored Connect RPC extension for the tenant cost leaderboard.
//
// This extends the generated DashboardService with GetTenantLeaderboard without
// modifying the BSR-managed proto. The new RPC will be upstreamed to the BSR
// proto definition in a follow-up PR; at that point this file can be removed
// and replaced with the regenerated output.

package candelav1connect

import (
	connect "connectrpc.com/connect"
	context "context"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	http "net/http"
)

const (
	// DashboardServiceGetTenantLeaderboardProcedure is the fully-qualified name of the
	// GetTenantLeaderboard RPC (admin-only, multitenant cost attribution).
	DashboardServiceGetTenantLeaderboardProcedure = "/candela.v1.DashboardService/GetTenantLeaderboard"
)

// DashboardServiceWithTenantHandler extends DashboardServiceHandler with tenant leaderboard support.
// Embed this interface instead of DashboardServiceHandler to opt into the new RPC.
type DashboardServiceWithTenantHandler interface {
	DashboardServiceHandler
	// GetTenantLeaderboard returns per-tenant LLM cost aggregations ranked by spend (admin only).
	GetTenantLeaderboard(context.Context, *connect.Request[v1.GetTenantLeaderboardRequest]) (*connect.Response[v1.GetTenantLeaderboardResponse], error)
}

// NewDashboardServiceHandlerWithTenant builds the full HTTP handler including the
// GetTenantLeaderboard RPC. Use this instead of NewDashboardServiceHandler when
// the service implementation embeds DashboardServiceWithTenantHandler.
func NewDashboardServiceHandlerWithTenant(svc DashboardServiceWithTenantHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	// Build the base handler (all existing RPCs).
	basePath, baseHandler := NewDashboardServiceHandler(svc, opts...)

	// Build the tenant leaderboard handler separately.
	tenantHandler := connect.NewUnaryHandler(
		DashboardServiceGetTenantLeaderboardProcedure,
		svc.GetTenantLeaderboard,
		connect.WithHandlerOptions(opts...),
	)

	// Wrap: route the new procedure, fall through to the base handler for all others.
	return basePath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == DashboardServiceGetTenantLeaderboardProcedure {
			tenantHandler.ServeHTTP(w, r)
			return
		}
		baseHandler.ServeHTTP(w, r)
	})
}
