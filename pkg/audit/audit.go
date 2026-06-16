package audit

import (
	"context"
	"strings"
	"time"
)

// Event represents a single auditable admin action.
type Event struct {
	Timestamp  time.Time
	ActorEmail string // from auth.FromContext
	ActorID    string
	Service    string // e.g. "UserService"
	Method     string // e.g. "CreateUser"
	Procedure  string // full RPC path, e.g. "/candela.v1.UserService/CreateUser"
	StatusCode string // "ok", "permission_denied", etc.
	Error      string // empty on success
}

// Logger writes audit events to a backend.
type Logger interface {
	Log(ctx context.Context, event Event)
}

// Multi fans out audit events to multiple loggers.
type Multi []Logger

func (m Multi) Log(ctx context.Context, event Event) {
	for _, l := range m {
		l.Log(ctx, event)
	}
}

// ParseProcedure extracts service and method names from a ConnectRPC procedure string.
// e.g. "/candela.v1.UserService/CreateUser" → ("UserService", "CreateUser")
func ParseProcedure(procedure string) (service, method string) {
	// Strip leading slash: "candela.v1.UserService/CreateUser"
	p := strings.TrimPrefix(procedure, "/")
	parts := strings.SplitN(p, "/", 2)
	if len(parts) != 2 {
		return procedure, ""
	}
	method = parts[1]
	// Extract service name from fully-qualified name: "candela.v1.UserService" → "UserService"
	svcParts := strings.Split(parts[0], ".")
	service = svcParts[len(svcParts)-1]
	return service, method
}

// DefaultMutationProcedures is the set of RPC procedures that should be audit-logged.
var DefaultMutationProcedures = map[string]bool{
	// UserService mutations
	"/candela.v1.UserService/CreateUser":     true,
	"/candela.v1.UserService/UpdateUser":     true,
	"/candela.v1.UserService/DeactivateUser": true,
	"/candela.v1.UserService/ReactivateUser": true,
	"/candela.v1.UserService/DeleteUser":     true,
	"/candela.v1.UserService/SetBudget":      true,
	"/candela.v1.UserService/ResetSpend":     true,
	"/candela.v1.UserService/CreateGrant":    true,
	"/candela.v1.UserService/RevokeGrant":    true,

	// CatalogService mutations
	"/candela.v1.ModelCatalogService/UpdateModelCatalogEntry": true,
	"/candela.v1.ModelCatalogService/DeleteModelCatalogEntry": true,

	// ProjectService mutations
	"/candela.v1.ProjectService/CreateProject": true,
	"/candela.v1.ProjectService/DeleteProject": true,
	"/candela.v1.ProjectService/CreateAPIKey":  true,
	"/candela.v1.ProjectService/RevokeAPIKey":  true,
}
