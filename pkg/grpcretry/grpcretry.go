// Package grpcretry provides shared exponential backoff logic for gRPC
// reconnection loops. It is used by both hubbleaudit and tetragonaudit
// to avoid duplicating the backoff computation and configuration.
package grpcretry

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config controls the reconnect backoff behavior.
type Config struct {
	// InitialDelay is the first backoff duration (default: 1s).
	InitialDelay time.Duration
	// MaxDelay is the backoff ceiling (default: 30s).
	MaxDelay time.Duration
	// Multiplier is the backoff factor (default: 2.0).
	Multiplier float64
	// MaxAttempts is the maximum number of times to retry (0 means no retries).
	MaxAttempts int
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

// Do executes the operation with retries according to the Config.
func Do(ctx context.Context, c Config, operation func(context.Context) error) error {
	c = c.WithDefaults()

	var err error
	for attempt := 0; attempt <= c.MaxAttempts; attempt++ {
		err = operation(ctx)
		if err == nil {
			return nil
		}

		code := status.Code(err)
		switch code {
		case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
			// Transient errors, retry
		default:
			// Permanent error, do not retry
			return err
		}

		if attempt == c.MaxAttempts {
			break
		}

		delay := Backoff(attempt, c)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}
