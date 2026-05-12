package connecthandlers

import (
	"strings"

	connect "connectrpc.com/connect"
)

// attribution holds Tenant and Job IDs extracted from request metadata.
type attribution struct {
	TenantID string
	JobID    string
}

// getAttribution extracts Tenant and Job IDs from request headers, prioritizing
// explicit X-Candela-* headers over W3C Baggage.
func getAttribution[T any](req *connect.Request[T]) attribution {
	h := req.Header()

	tenantID := h.Get("X-Candela-Tenant-Id")
	jobID := h.Get("X-Candela-Job-Id")

	// Fallback to W3C Baggage if either is missing.
	if tenantID == "" || jobID == "" {
		baggage := h.Get("Baggage")
		if baggage != "" {
			if tenantID == "" {
				tenantID = extractBaggage(baggage, "candela.tenant_id")
			}
			if jobID == "" {
				jobID = extractBaggage(baggage, "candela.job_id")
			}
		}
	}

	return attribution{
		TenantID: tenantID,
		JobID:    jobID,
	}
}

// extractBaggage parses a W3C Baggage string and returns the value for a key.
// Format: key1=val1,key2=val2;prop1=v1
func extractBaggage(baggage, key string) string {
	for _, item := range strings.Split(baggage, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		// Split by semicolon to remove optional properties.
		kvPart := strings.Split(item, ";")[0]
		kv := strings.SplitN(kvPart, "=", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) == key {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}
