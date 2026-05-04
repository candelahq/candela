package proxy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// generateTraceID returns a random 32-char hex trace ID (16 bytes).
// Falls back to a time-based ID if crypto/rand fails (should never happen
// in practice, but avoids crashing the entire production server).
func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: time-based ID is not cryptographically random but
		// keeps the server alive instead of panicking.
		now := time.Now().UnixNano()
		return fmt.Sprintf("%016x%016x", now, now^0xdeadbeef)
	}
	return hex.EncodeToString(b)
}

// generateSpanID returns a random 16-char hex span ID (8 bytes).
func generateSpanID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// traceContext holds extracted W3C Trace Context values.
type traceContext struct {
	traceID      string // 32-char lowercase hex
	parentSpanID string // 16-char lowercase hex
}

// parseTraceparent extracts trace context from a W3C traceparent header.
// Format: "00-{traceID 32hex}-{spanID 16hex}-{flags 2hex}"
// Returns nil if the header is empty or malformed.
//
// See: https://www.w3.org/TR/trace-context/#traceparent-header-field-values
// hexPattern validates that a string contains only lowercase hex characters.
var hexPattern = regexp.MustCompile(`^[0-9a-f]+$`)

func parseTraceparent(header string) *traceContext {
	// Fast reject: minimum valid traceparent is 55 chars
	// ("00-{32}-{16}-{2}" = 2+1+32+1+16+1+2 = 55).
	if len(header) < 55 {
		return nil
	}
	// Use Split (not SplitN) for forward compatibility — future versions
	// may include additional "-" separated fields.
	parts := strings.Split(header, "-")
	if len(parts) < 4 {
		return nil
	}
	// Version "ff" is explicitly invalid per W3C spec.
	version := strings.ToLower(parts[0])
	if len(version) != 2 || version == "ff" {
		return nil
	}
	// Normalize to lowercase — the W3C spec requires case-insensitive
	// parsing, and we must store canonical lowercase to prevent trace ID
	// fragmentation (e.g. "AA..." vs "aa..." creating separate traces).
	traceID := strings.ToLower(parts[1])
	spanID := strings.ToLower(parts[2])
	// Validate lengths: trace-id = 32 hex, parent-id = 16 hex, flags = 2 hex.
	if len(traceID) != 32 || len(spanID) != 16 || len(parts[3]) != 2 {
		return nil
	}
	// Reject all-zero trace-id or span-id (invalid per spec).
	if traceID == "00000000000000000000000000000000" || spanID == "0000000000000000" {
		return nil
	}
	// Validate hex characters — prevents injection of non-hex values into
	// trace IDs that get stored in databases and queried by ID.
	if !hexPattern.MatchString(traceID) || !hexPattern.MatchString(spanID) {
		return nil
	}
	return &traceContext{
		traceID:      traceID,
		parentSpanID: spanID,
	}
}
