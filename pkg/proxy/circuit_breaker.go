package proxy

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Failing — skip observability, forward directly
	CircuitHalfOpen                     // Testing recovery
)

// CircuitBreaker implements a per-provider circuit breaker.
// When tripped, the proxy continues forwarding requests to upstream (fail-open)
// but skips span creation to avoid blocking on a failing storage pipeline.
//
// This is NOT about blocking upstream calls — LLM requests always go through.
// It's about protecting the observability pipeline from cascading failures.
type CircuitBreaker struct {
	mu              sync.Mutex
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time

	// Config
	threshold    int           // consecutive failures to trip
	resetTimeout time.Duration // how long to wait before half-open
	halfOpenMax  int           // successes needed to close
}

// CircuitBreakerConfig holds circuit breaker settings.
type CircuitBreakerConfig struct {
	Threshold    int           // default: 5
	ResetTimeout time.Duration // default: 30s
	HalfOpenMax  int           // default: 2
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Threshold:    5,
		ResetTimeout: 30 * time.Second,
		HalfOpenMax:  2,
	}
}

// NewCircuitBreaker creates a circuit breaker with the given config.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.Threshold == 0 {
		cfg.Threshold = 5
	}
	if cfg.ResetTimeout == 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMax == 0 {
		cfg.HalfOpenMax = 2
	}

	return &CircuitBreaker{
		state:        CircuitClosed,
		threshold:    cfg.Threshold,
		resetTimeout: cfg.ResetTimeout,
		halfOpenMax:  cfg.HalfOpenMax,
	}
}

// AllowRequest returns true if the request should include observability.
// When the circuit is open, requests are still forwarded but observability is skipped.
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if reset timeout has elapsed.
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.successes = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}
	return true
}

// RecordSuccess records a successful upstream call.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0

	if cb.state == CircuitHalfOpen {
		cb.successes++
		if cb.successes >= cb.halfOpenMax {
			cb.state = CircuitClosed
		}
	}
}

// RecordFailure records a failed upstream call (5xx).
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.threshold {
		cb.state = CircuitOpen
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// String returns a human-readable state name.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	}
	return "unknown"
}
