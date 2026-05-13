// Package candela provides lightweight HTTP middleware for propagating
// Candela enrichment metadata (tenant_id, job_id) via W3C Baggage headers.
//
// Usage with net/http:
//
//	client := &http.Client{
//	    Transport: candela.NewTransport(nil,
//	        candela.WithTenantID("acme-corp"),
//	        candela.WithJobID("training-run-42"),
//	    ),
//	}
//	resp, err := client.Do(req) // headers are injected automatically
//
// The SDK injects both W3C Baggage entries and explicit X-Candela-* fallback
// headers for maximum compatibility with Candela proxies.
package candela

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// IDPattern enforces the allowed character set for tenant and job IDs:
// alphanumeric, hyphens, dots, and underscores, 1–128 chars.
var IDPattern = regexp.MustCompile(`^[a-zA-Z0-9\-._]{1,128}$`)

// Option configures the enrichment transport.
type Option func(*config)

type config struct {
	tenantID string
	jobID    string
}

// WithTenantID sets the tenant ID to propagate on all outgoing requests.
func WithTenantID(id string) Option {
	return func(c *config) { c.tenantID = id }
}

// WithJobID sets the job/experiment ID to propagate on all outgoing requests.
func WithJobID(id string) Option {
	return func(c *config) { c.jobID = id }
}

// Transport wraps an http.RoundTripper and injects Candela enrichment headers.
type Transport struct {
	base http.RoundTripper
	cfg  config
}

// NewTransport creates an enrichment transport that wraps the given base
// transport. If base is nil, http.DefaultTransport is used.
func NewTransport(base http.RoundTripper, opts ...Option) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	t := &Transport{base: base}
	for _, o := range opts {
		o(&t.cfg)
	}
	return t
}

// RoundTrip injects Candela baggage and headers, then delegates to the
// underlying transport.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid mutating the caller's headers.
	r := req.Clone(req.Context())

	var parts []string

	if t.cfg.tenantID != "" {
		if !IDPattern.MatchString(t.cfg.tenantID) {
			return nil, fmt.Errorf("candela: invalid tenant_id %q", t.cfg.tenantID)
		}
		parts = append(parts, "candela.tenant_id="+t.cfg.tenantID)
		r.Header.Set("X-Candela-Tenant-Id", t.cfg.tenantID)
	}

	if t.cfg.jobID != "" {
		if !IDPattern.MatchString(t.cfg.jobID) {
			return nil, fmt.Errorf("candela: invalid job_id %q", t.cfg.jobID)
		}
		parts = append(parts, "candela.job_id="+t.cfg.jobID)
		r.Header.Set("X-Candela-Job-Id", t.cfg.jobID)
	}

	// Append to existing Baggage header (don't clobber).
	if len(parts) > 0 {
		existing := strings.Join(r.Header.Values("Baggage"), ",")
		baggage := strings.Join(parts, ",")
		if existing != "" {
			baggage = existing + "," + baggage
		}
		r.Header.Set("Baggage", baggage)
	}

	return t.base.RoundTrip(r)
}

// InjectHeaders is a lower-level helper that adds Candela enrichment headers
// to an existing request without wrapping the transport. Useful for one-off
// injection or custom HTTP clients.
func InjectHeaders(req *http.Request, opts ...Option) error {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	var parts []string

	if cfg.tenantID != "" {
		if !IDPattern.MatchString(cfg.tenantID) {
			return fmt.Errorf("candela: invalid tenant_id %q", cfg.tenantID)
		}
		parts = append(parts, "candela.tenant_id="+cfg.tenantID)
		req.Header.Set("X-Candela-Tenant-Id", cfg.tenantID)
	}

	if cfg.jobID != "" {
		if !IDPattern.MatchString(cfg.jobID) {
			return fmt.Errorf("candela: invalid job_id %q", cfg.jobID)
		}
		parts = append(parts, "candela.job_id="+cfg.jobID)
		req.Header.Set("X-Candela-Job-Id", cfg.jobID)
	}

	if len(parts) > 0 {
		existing := strings.Join(req.Header.Values("Baggage"), ",")
		baggage := strings.Join(parts, ",")
		if existing != "" {
			baggage = existing + "," + baggage
		}
		req.Header.Set("Baggage", baggage)
	}

	return nil
}
