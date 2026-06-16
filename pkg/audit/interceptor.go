package audit

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"github.com/candelahq/candela/pkg/auth"
)

// Interceptor returns a ConnectRPC unary interceptor that logs mutation RPCs
// to the provided Logger. Only procedures listed in the procedures map are logged.
//
// This interceptor should be wired AFTER auth and validation interceptors so it
// only fires for authorized, validated requests.
func Interceptor(logger Logger, procedures map[string]bool) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure
			if !procedures[procedure] {
				return next(ctx, req)
			}

			// Extract caller identity.
			var actorEmail, actorID string
			if user := auth.FromContext(ctx); user != nil {
				actorEmail = user.Email
				actorID = user.ID
			}

			// Call the actual handler.
			resp, err := next(ctx, req)

			// Build the audit event.
			service, method := ParseProcedure(procedure)
			event := Event{
				Timestamp:  time.Now(),
				ActorEmail: actorEmail,
				ActorID:    actorID,
				Service:    service,
				Method:     method,
				Procedure:  procedure,
				StatusCode: "ok",
			}
			if err != nil {
				event.StatusCode = connect.CodeOf(err).String()
				event.Error = err.Error()
			}

			logger.Log(ctx, event)
			return resp, err
		}
	}
}
