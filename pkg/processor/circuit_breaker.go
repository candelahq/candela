// Package processor — circuit breaker for sink writes.
//
// This is independent from pkg/proxy/circuit_breaker.go:
//   - Proxy CB: protects observability from upstream LLM failures (fail-open)
//   - Processor CB: protects fan-out writes from sink failures (fail-silent)
//
// The processor CB uses a Clock interface for deterministic testing.
package processor

import (
	"sync"
	"time"
)

// Clock abstracts time for testability. Production uses realClock;
// tests inject a mock to control time progression without sleeps.
type Clock interface {
	Now() time.Time
}

// realClock delegates to the standard library.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// BreakerState represents the state of a processor circuit breaker.
type BreakerState int

const (
	BreakerClosed   BreakerState = iota // Normal — writes go through
	BreakerOpen                         // Failing — writes are skipped (fail-silent)
	BreakerHalfOpen                     // Recovery — single probe write allowed
)

// String returns a human-readable state name.
func (s BreakerState) String() string {
	switch s {
	case BreakerClosed:
		return "closed"
	case BreakerOpen:
		return "open"
	case BreakerHalfOpen:
		return "half-open"
	}
	return "unknown"
}

// BreakerConfig holds circuit breaker settings for a processor sink.
type BreakerConfig struct {
	Threshold    int           // Consecutive failures to trip (default: 5)
	ResetTimeout time.Duration // Wait before half-open probe (default: 30s)
	HalfOpenMax  int           // Successes needed to re-close (default: 2)
}

// DefaultBreakerConfig returns sensible defaults for processor sinks.
func DefaultBreakerConfig() BreakerConfig {
	return BreakerConfig{
		Threshold:    5,
		ResetTimeout: 30 * time.Second,
		HalfOpenMax:  2,
	}
}

// Breaker implements a per-sink circuit breaker for the processor pipeline.
// When tripped, writes to the sink are silently skipped — data loss for
// secondary sinks is acceptable since the primary always receives writes.
type Breaker struct {
	mu              sync.Mutex
	state           BreakerState
	failures        int
	successes       int
	lastFailureTime time.Time
	clock           Clock

	// Config (immutable after construction).
	threshold    int
	resetTimeout time.Duration
	halfOpenMax  int
}

// NewBreaker creates a processor circuit breaker with the given config and clock.
func NewBreaker(cfg BreakerConfig, clock Clock) *Breaker {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 5
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMax <= 0 {
		cfg.HalfOpenMax = 2
	}
	if clock == nil {
		clock = realClock{}
	}

	return &Breaker{
		state:        BreakerClosed,
		threshold:    cfg.Threshold,
		resetTimeout: cfg.ResetTimeout,
		halfOpenMax:  cfg.HalfOpenMax,
		clock:        clock,
	}
}

// Allow returns true if the write should proceed.
// When the circuit is open, writes are silently skipped.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case BreakerClosed:
		return true
	case BreakerOpen:
		// Check if reset timeout has elapsed → transition to half-open.
		if b.clock.Now().Sub(b.lastFailureTime) > b.resetTimeout {
			b.state = BreakerHalfOpen
			b.successes = 0
			return true
		}
		return false
	case BreakerHalfOpen:
		return true
	}
	return true
}

// RecordSuccess records a successful sink write.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failures = 0

	if b.state == BreakerHalfOpen {
		b.successes++
		if b.successes >= b.halfOpenMax {
			b.state = BreakerClosed
		}
	}
}

// RecordFailure records a failed sink write.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Cap at threshold+1 to prevent unbounded growth.
	if b.failures <= b.threshold {
		b.failures++
	}
	b.lastFailureTime = b.clock.Now()

	switch b.state {
	case BreakerClosed:
		if b.failures >= b.threshold {
			b.state = BreakerOpen
		}
	case BreakerHalfOpen:
		// Probe failed — re-open.
		b.state = BreakerOpen
	}
}

// State returns the current breaker state.
func (b *Breaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// ConsecutiveFailures returns the current consecutive failure count.
func (b *Breaker) ConsecutiveFailures() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failures
}
