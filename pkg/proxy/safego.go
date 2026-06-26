package proxy

import (
	"log/slog"
	"runtime/debug"
)

// safeGo runs fn in a new goroutine with panic recovery.
// If fn panics, the panic is logged with a full stack trace
// instead of crashing the process.
func safeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in async goroutine",
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
