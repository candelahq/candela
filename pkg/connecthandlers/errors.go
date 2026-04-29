package connecthandlers

import (
	"fmt"
	"log/slog"

	connect "connectrpc.com/connect"
)

// internalError logs the real error server-side and returns a generic
// "internal error" to the client. This prevents leaking storage-layer
// details (SQL errors, file paths, Firestore doc references) over the API.
func internalError(msg string, err error) *connect.Error {
	slog.Error(msg, "error", err)
	return connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
}
