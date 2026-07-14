package proxy

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryConfig controls automatic retry behaviour for failed upstream requests.
type RetryConfig struct {
	Enabled              bool          `yaml:"enabled"`
	MaxAttempts          int           `yaml:"max_attempts"`
	RetryableStatusCodes []int         `yaml:"retryable_status_codes"`
	RetryOnTimeout       bool          `yaml:"retry_on_timeout"`
	Backoff              BackoffConfig `yaml:"backoff"`
}

// BackoffConfig defines exponential backoff parameters with full jitter.
type BackoffConfig struct {
	InitialMs  int     `yaml:"initial_ms"`
	MaxMs      int     `yaml:"max_ms"`
	Multiplier float64 `yaml:"multiplier"`
}

// DefaultRetryConfig returns sensible defaults with retries disabled.
// Callers must explicitly set Enabled=true to activate retry logic.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		Enabled:              false,
		MaxAttempts:          2,
		RetryableStatusCodes: []int{429, 500, 502, 503, 529},
		RetryOnTimeout:       true,
		Backoff: BackoffConfig{
			InitialMs:  500,
			MaxMs:      5000,
			Multiplier: 2.0,
		},
	}
}

// effectiveMaxAttempts clamps MaxAttempts to the range [1, 5].
func (c *RetryConfig) effectiveMaxAttempts() int {
	if c.MaxAttempts < 1 {
		return 1
	}
	if c.MaxAttempts > 5 {
		return 5
	}
	return c.MaxAttempts
}

// IsRetryableStatus reports whether the given HTTP status code is retryable
// according to this configuration.
func (c *RetryConfig) IsRetryableStatus(code int) bool {
	for _, rc := range c.RetryableStatusCodes {
		if rc == code {
			return true
		}
	}
	return false
}

// ShouldRetry decides whether the request should be retried for the given
// attempt number, upstream response, connection error, and streaming state.
//
// SAFETY: once SSE chunks have started flowing to the client (streamStarted),
// we must never retry — the client has already consumed partial data.
func (c *RetryConfig) ShouldRetry(attempt int, resp *http.Response, connErr error, streamStarted bool) bool {
	if !c.Enabled {
		return false
	}
	if attempt >= c.effectiveMaxAttempts() {
		return false
	}
	// SAFETY: never retry once SSE chunks have started flowing.
	if streamStarted {
		return false
	}

	// Connection-level error (timeout, DNS, reset).
	if connErr != nil {
		return c.RetryOnTimeout
	}

	// No response to inspect — don't retry blindly.
	if resp == nil {
		return false
	}

	return c.IsRetryableStatus(resp.StatusCode)
}

// BackoffDuration returns the backoff duration for the given attempt using
// exponential backoff with full jitter. The result is capped at BackoffConfig.MaxMs.
//
// Formula: jitter in [0, min(MaxMs, InitialMs * Multiplier^attempt))
func (c *RetryConfig) BackoffDuration(attempt int) time.Duration {
	initialMs := c.Backoff.InitialMs
	if initialMs <= 0 {
		initialMs = 500
	}
	maxMs := c.Backoff.MaxMs
	if maxMs <= 0 {
		maxMs = 5000
	}
	multiplier := c.Backoff.Multiplier
	if multiplier <= 0 {
		multiplier = 2.0
	}

	uncapped := float64(initialMs) * math.Pow(multiplier, float64(attempt))
	cap := math.Min(uncapped, float64(maxMs))

	// Full jitter: uniform random in [0, cap).
	jittered := rand.Float64() * cap //nolint:gosec // jitter does not need crypto rand
	return time.Duration(jittered) * time.Millisecond
}

// maxRetryAfter caps the Retry-After value the proxy will honour.
const maxRetryAfter = 60 * time.Second

// ParseRetryAfter extracts a backoff duration from the Retry-After header.
// It supports both delta-seconds ("5") and HTTP-date formats.
// Returns 0 if the header is absent, empty, or unparseable.
// The result is capped at 60 s to prevent adversarial providers from stalling
// the proxy indefinitely.
func ParseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	val := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if val == "" {
		return 0
	}

	// Try delta-seconds first (most common from LLM APIs).
	if secs, err := strconv.Atoi(val); err == nil {
		d := time.Duration(secs) * time.Second
		if d > maxRetryAfter {
			d = maxRetryAfter
		}
		if d < 0 {
			return 0
		}
		return d
	}

	// Try HTTP-date (RFC 7231 §7.1.1.1).
	if t, err := http.ParseTime(val); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		if d > maxRetryAfter {
			d = maxRetryAfter
		}
		return d
	}

	return 0
}
