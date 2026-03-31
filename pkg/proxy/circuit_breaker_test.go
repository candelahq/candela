package proxy

import (
	"testing"
	"time"
)

func TestCircuitBreaker_StartsClosedAllowsRequests(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed, got %s", cb.State())
	}
	if !cb.AllowRequest() {
		t.Error("expected allow on closed circuit")
	}
}

func TestCircuitBreaker_TripsAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    3,
		ResetTimeout: 1 * time.Hour,
		HalfOpenMax:  1,
	})

	// Record failures up to threshold.
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after 2 failures, got %s", cb.State())
	}

	cb.RecordFailure() // 3rd failure = trip
	if cb.State() != CircuitOpen {
		t.Errorf("expected open after 3 failures, got %s", cb.State())
	}
	if cb.AllowRequest() {
		t.Error("expected deny on open circuit")
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    3,
		ResetTimeout: 1 * time.Hour,
		HalfOpenMax:  1,
	})

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess() // Reset

	// Should need 3 more failures to trip.
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Errorf("expected closed, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenAfterResetTimeout(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    1,
		ResetTimeout: 10 * time.Millisecond,
		HalfOpenMax:  1,
	})

	cb.RecordFailure() // Trip immediately.
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}

	// Wait for reset timeout.
	time.Sleep(20 * time.Millisecond)

	// Should transition to half-open on AllowRequest.
	if !cb.AllowRequest() {
		t.Error("expected allow after reset timeout (half-open)")
	}
	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected half-open, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenClosesAfterSuccesses(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    1,
		ResetTimeout: 10 * time.Millisecond,
		HalfOpenMax:  2,
	})

	cb.RecordFailure() // Trip.
	time.Sleep(20 * time.Millisecond)
	cb.AllowRequest() // → half-open

	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected still half-open after 1 success, got %s", cb.State())
	}

	cb.RecordSuccess() // 2nd success → close.
	if cb.State() != CircuitClosed {
		t.Errorf("expected closed, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenReOpensOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Threshold:    1,
		ResetTimeout: 10 * time.Millisecond,
		HalfOpenMax:  2,
	})

	cb.RecordFailure() // Trip.
	time.Sleep(20 * time.Millisecond)
	cb.AllowRequest() // → half-open

	cb.RecordFailure() // Fail during half-open → re-open.
	if cb.State() != CircuitOpen {
		t.Errorf("expected re-open, got %s", cb.State())
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
