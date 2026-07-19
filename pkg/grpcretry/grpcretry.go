// Package grpcretry provides shared exponential backoff logic for gRPC
// reconnection loops. It is used by both hubbleaudit and tetragonaudit
// to avoid duplicating the backoff computation and configuration.
package grpcretry

import (
	"math"
	"math/rand/v2"
	"time"
)

// Config controls the reconnect backoff behavior.
type Config struct {
	// InitialDelay is the first backoff duration (default: 1s).
	InitialDelay time.Duration
	// MaxDelay is the backoff ceiling (default: 30s).
	MaxDelay time.Duration
	// Multiplier is the backoff factor (default: 2.0).
	Multiplier float64
}

// WithDefaults returns a copy of the config with zero-valued fields
// replaced by sensible defaults.
func (c Config) WithDefaults() Config {
	if c.InitialDelay <= 0 {
		c.InitialDelay = time.Second
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = 30 * time.Second
	}
	if c.Multiplier <= 0 {
		c.Multiplier = 2.0
	}
	return c
}

// Backoff computes the delay for a given attempt with ±25% jitter.
func Backoff(attempt int, c Config) time.Duration {
	// Guard against overflow: math.Pow(2, 31+) → +Inf.
	if attempt < 0 {
		attempt = 0
	} else if attempt > 30 {
		attempt = 30
	}
	d := float64(c.InitialDelay) * math.Pow(c.Multiplier, float64(attempt))
	if d > float64(c.MaxDelay) {
		d = float64(c.MaxDelay)
	}
	jitter := d * 0.25 * (2*rand.Float64() - 1) //nolint:gosec
	return time.Duration(d + jitter)
}
