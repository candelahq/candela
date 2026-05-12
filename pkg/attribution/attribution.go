// Package attribution provides shared utilities for extracting multitenant
// cost-attribution metadata from HTTP requests.
//
// Both the cloud proxy (pkg/proxy) and the local proxy (cmd/candela-local)
// need to extract tenant_id and job_id from W3C Baggage headers and fallback
// X-Candela-* headers. This package consolidates that logic.
package attribution

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

// IDPattern enforces the allowed character set and length limits for
// tenant and job IDs: alphanumeric, hyphens, dots, and underscores, 1–128 chars.
// Exported so callers can use it for validation without re-defining.
var IDPattern = regexp.MustCompile(`^[a-zA-Z0-9\-._]{1,128}$`)

// Attribution holds the tenant and job IDs extracted from request metadata.
type Attribution struct {
	TenantID string
	JobID    string
}

// FromRequest extracts attribution metadata from an HTTP request using the
// standard precedence: W3C Baggage > explicit headers.
//
// Baggage keys checked: candela.tenant_id, candela.job_id (case-insensitive per RFC 8941).
// Header fallbacks: X-Candela-Tenant-Id, X-Candela-Job-Id.
// Invalid values are logged and discarded.
func FromRequest(r *http.Request) Attribution {
	tenantID, jobID := ParseBaggageHeaders(r.Header.Values("Baggage"))
	if tenantID == "" {
		hdr := r.Header.Get("X-Candela-Tenant-Id")
		if IDPattern.MatchString(hdr) {
			tenantID = hdr
		} else if hdr != "" {
			slog.Warn("discarding invalid X-Candela-Tenant-Id header", "value", hdr)
		}
	}
	if jobID == "" {
		hdr := r.Header.Get("X-Candela-Job-Id")
		if IDPattern.MatchString(hdr) {
			jobID = hdr
		} else if hdr != "" {
			slog.Warn("discarding invalid X-Candela-Job-Id header", "value", hdr)
		}
	}
	return Attribution{TenantID: tenantID, JobID: jobID}
}

// ParseBaggageHeaders joins multiple Baggage header values (W3C allows multiple
// Baggage: header instances in a single HTTP request) and delegates to
// ParseBaggage for extraction.
func ParseBaggageHeaders(values []string) (tenantID, jobID string) {
	return ParseBaggage(strings.Join(values, ","))
}

// ParseBaggage extracts candela.tenant_id and candela.job_id from a W3C Baggage
// header value string.
//
// W3C Baggage format: "key1=val1;prop, key2=val2" (RFC 8941 list).
// Baggage keys are case-insensitive per RFC 8941 — EqualFold is used.
//
// If multiple entries for the same key are present (RFC 8941 allows duplicates),
// the right-most valid one wins (per W3C spec). Invalid values are warned and skipped.
func ParseBaggage(header string) (tenantID, jobID string) {
	if header == "" {
		return "", ""
	}
	for _, member := range strings.Split(header, ",") {
		// Each member may have properties after a semicolon: "key=value;prop".
		kv := strings.SplitN(strings.TrimSpace(member), ";", 2)[0]
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// RFC 8941: Baggage keys are case-insensitive.
		if strings.EqualFold(key, "candela.tenant_id") {
			if IDPattern.MatchString(val) {
				// W3C spec: the right-most occurrence of a key wins.
				tenantID = val
			} else {
				slog.Warn("skipping invalid candela.tenant_id in Baggage header",
					"value", val)
			}
		} else if strings.EqualFold(key, "candela.job_id") {
			if IDPattern.MatchString(val) {
				jobID = val
			} else {
				slog.Warn("skipping invalid candela.job_id in Baggage header",
					"value", val)
			}
		}
	}
	return tenantID, jobID
}
