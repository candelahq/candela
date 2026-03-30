package proxy

import (
	"crypto/rand"
	"encoding/hex"
)

// generateTraceID returns a random 32-char hex trace ID (16 bytes).
func generateTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// generateSpanID returns a random 16-char hex span ID (8 bytes).
func generateSpanID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
