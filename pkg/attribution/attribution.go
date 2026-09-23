// Package attribution provides shared utilities for extracting multitenant
// cost-attribution metadata from HTTP requests.
//
// Both the cloud proxy (pkg/proxy) and the local proxy (cmd/candela-local)
// need to extract tenant_id and job_id from W3C Baggage headers and fallback
// X-Candela-* headers. This package consolidates that logic.
package attribution

import (
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
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
// Invalid values are logged and discarded. To prevent log flooding under sustained attack,
// invalid attribute warnings are rate-limited per source and downgraded to Debug.
func FromRequest(r *http.Request) Attribution {
	source := requestSource(r)
	tenantID, jobID := parseBaggageWithSource(strings.Join(r.Header.Values("Baggage"), ","), source)
	if tenantID == "" {
		hdr := r.Header.Get("X-Candela-Tenant-Id")
		if IDPattern.MatchString(hdr) {
			tenantID = hdr
		} else if hdr != "" {
			logInvalid(source, "X-Candela-Tenant-Id", "discarding invalid X-Candela-Tenant-Id header", hdr)
		}
	}
	if jobID == "" {
		hdr := r.Header.Get("X-Candela-Job-Id")
		if IDPattern.MatchString(hdr) {
			jobID = hdr
		} else if hdr != "" {
			logInvalid(source, "X-Candela-Job-Id", "discarding invalid X-Candela-Job-Id header", hdr)
		}
	}
	return Attribution{TenantID: tenantID, JobID: jobID}
}

// ParseBaggageHeaders joins multiple Baggage header values (W3C allows multiple
// Baggage: header instances in a single HTTP request) and delegates to
// ParseBaggage for extraction.
func ParseBaggageHeaders(values []string) (tenantID, jobID string) {
	return parseBaggageWithSource(strings.Join(values, ","), "baggage")
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
	return parseBaggageWithSource(header, "baggage")
}

func parseBaggageWithSource(header, source string) (tenantID, jobID string) {
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
				logInvalid(source, "candela.tenant_id", "skipping invalid candela.tenant_id in Baggage header", val)
			}
		} else if strings.EqualFold(key, "candela.job_id") {
			if IDPattern.MatchString(val) {
				jobID = val
			} else {
				logInvalid(source, "candela.job_id", "skipping invalid candela.job_id in Baggage header", val)
			}
		}
	}
	return tenantID, jobID
}

const (
	defaultCooldownWindow = 1 * time.Minute
	maxTrackedSources     = 1024
)

var defaultLimiter = newWarnRateLimiter(defaultCooldownWindow)

type warnRateLimiter struct {
	mu       sync.Mutex
	window   time.Duration
	lastWarn map[string]time.Time
}

func newWarnRateLimiter(window time.Duration) *warnRateLimiter {
	return &warnRateLimiter{
		window:   window,
		lastWarn: make(map[string]time.Time),
	}
}

func (rl *warnRateLimiter) shouldWarn(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	last, exists := rl.lastWarn[key]
	if !exists || now.Sub(last) >= rl.window {
		if len(rl.lastWarn) >= maxTrackedSources {
			for k, t := range rl.lastWarn {
				if now.Sub(t) >= rl.window {
					delete(rl.lastWarn, k)
				}
			}
			if len(rl.lastWarn) >= maxTrackedSources {
				rl.lastWarn = make(map[string]time.Time, maxTrackedSources/2)
			}
		}
		rl.lastWarn[key] = now
		return true
	}
	return false
}

func logInvalid(source, headerName, msg, value string) {
	key := source + ":" + headerName
	if defaultLimiter.shouldWarn(key, time.Now()) {
		slog.Warn(msg, "value", value, "source", source)
	} else {
		slog.Debug(msg+" (rate-limited)", "value", value, "source", source)
	}
}

func requestSource(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" {
		return "unknown"
	}
	return host
}

// ResetRateLimiterForTesting resets the rate limiter state.
func ResetRateLimiterForTesting() {
	defaultLimiter.mu.Lock()
	defer defaultLimiter.mu.Unlock()
	defaultLimiter.lastWarn = make(map[string]time.Time)
}

// SetCooldownForTesting updates the cooldown window for testing.
func SetCooldownForTesting(d time.Duration) {
	defaultLimiter.mu.Lock()
	defer defaultLimiter.mu.Unlock()
	defaultLimiter.window = d
}
