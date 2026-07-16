package proxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Retry config defaults
// ---------------------------------------------------------------------------

func TestRetryDefaultsDisabled(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.Enabled {
		t.Error("default config should have Enabled=false")
	}
	if cfg.MaxAttempts != 2 {
		t.Errorf("expected MaxAttempts=2, got %d", cfg.MaxAttempts)
	}
	if len(cfg.RetryableStatusCodes) == 0 {
		t.Error("expected non-empty retryable status codes")
	}
}

// ---------------------------------------------------------------------------
// effectiveMaxAttempts clamping
// ---------------------------------------------------------------------------

func TestRetryEffectiveMaxAttempts(t *testing.T) {
	tests := []struct {
		name string
		max  int
		want int
	}{
		{"ZeroClampsToOne", 0, 1},
		{"NegativeClampsToOne", -3, 1},
		{"ThreePassthrough", 3, 3},
		{"FivePassthrough", 5, 5},
		{"TenClampsToFive", 10, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := RetryConfig{MaxAttempts: tt.max}
			if got := cfg.effectiveMaxAttempts(); got != tt.want {
				t.Errorf("effectiveMaxAttempts(%d) = %d, want %d", tt.max, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsRetryableStatus
// ---------------------------------------------------------------------------

func TestRetryIsRetryableStatus(t *testing.T) {
	cfg := DefaultRetryConfig()
	tests := []struct {
		code int
		want bool
	}{
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{529, true},
		{400, false},
		{200, false},
		{401, false},
		{404, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.code), func(t *testing.T) {
			if got := cfg.IsRetryableStatus(tt.code); got != tt.want {
				t.Errorf("IsRetryableStatus(%d) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ShouldRetry
// ---------------------------------------------------------------------------

func TestShouldRetry(t *testing.T) {
	enabled := RetryConfig{
		Enabled:              true,
		MaxAttempts:          3,
		RetryableStatusCodes: []int{429, 500, 502, 503},
		RetryOnTimeout:       true,
		Backoff:              BackoffConfig{InitialMs: 100, MaxMs: 1000, Multiplier: 2},
	}
	disabled := RetryConfig{Enabled: false, MaxAttempts: 3}

	connErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}

	makeResp := func(code int) *http.Response {
		return &http.Response{StatusCode: code}
	}

	tests := []struct {
		name          string
		cfg           RetryConfig
		attempt       int
		resp          *http.Response
		connErr       error
		streamStarted bool
		want          bool
	}{
		{"Disabled", disabled, 0, makeResp(500), nil, false, false},
		{"ExhaustedAttempts", enabled, 3, makeResp(500), nil, false, false},
		{"StreamStarted", enabled, 0, makeResp(500), nil, true, false},
		{"ConnErrorEnabled", enabled, 0, nil, connErr, false, true},
		{"ConnErrorDisabledTimeout", RetryConfig{
			Enabled: true, MaxAttempts: 3, RetryOnTimeout: false,
		}, 0, nil, connErr, false, false},
		{"Status500", enabled, 0, makeResp(500), nil, false, true},
		{"Status429", enabled, 0, makeResp(429), nil, false, true},
		{"Status400NotRetryable", enabled, 0, makeResp(400), nil, false, false},
		{"Status401NotRetryable", enabled, 0, makeResp(401), nil, false, false},
		{"NilResponse", enabled, 0, nil, nil, false, false},
		{"SecondAttemptOK", enabled, 1, makeResp(502), nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.ShouldRetry(tt.attempt, tt.resp, tt.connErr, tt.streamStarted)
			if got != tt.want {
				t.Errorf("ShouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BackoffDuration
// ---------------------------------------------------------------------------

func TestBackoffDuration_Exponential(t *testing.T) {
	cfg := RetryConfig{
		Backoff: BackoffConfig{InitialMs: 100, MaxMs: 10000, Multiplier: 2},
	}

	// Run multiple samples to verify statistical ordering.
	const samples = 200
	var sum0, sum1, sum2 float64
	for range samples {
		sum0 += float64(cfg.BackoffDuration(0))
		sum1 += float64(cfg.BackoffDuration(1))
		sum2 += float64(cfg.BackoffDuration(2))
	}
	avg0 := sum0 / float64(samples)
	avg1 := sum1 / float64(samples)
	avg2 := sum2 / float64(samples)

	if avg0 >= avg1 {
		t.Errorf("expected avg attempt 0 (%v) < avg attempt 1 (%v)", avg0, avg1)
	}
	if avg1 >= avg2 {
		t.Errorf("expected avg attempt 1 (%v) < avg attempt 2 (%v)", avg1, avg2)
	}
}

func TestBackoffDuration_CappedAtMax(t *testing.T) {
	cfg := RetryConfig{
		Backoff: BackoffConfig{InitialMs: 1000, MaxMs: 2000, Multiplier: 10},
	}

	for range 100 {
		d := cfg.BackoffDuration(5)
		if d > 2000*time.Millisecond {
			t.Fatalf("backoff %v exceeded max 2000ms", d)
		}
	}
}

func TestBackoffDuration_ZeroConfig(t *testing.T) {
	cfg := RetryConfig{}
	d := cfg.BackoffDuration(0)
	if d < 0 || d > 5*time.Second {
		t.Errorf("zero-config backoff = %v, expected [0, 5s]", d)
	}
}

// ---------------------------------------------------------------------------
// ParseRetryAfter
// ---------------------------------------------------------------------------

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name string
		hdr  string
		want time.Duration
	}{
		{"FiveSeconds", "5", 5 * time.Second},
		{"CappedAt60", "120", 60 * time.Second},
		{"Empty", "", 0},
		{"Garbage", "not-a-number", 0},
		{"Zero", "0", 0},
		{"NegativeSeconds", "-5", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.hdr != "" {
				resp.Header.Set("Retry-After", tt.hdr)
			}
			got := ParseRetryAfter(resp)
			if got != tt.want {
				t.Errorf("ParseRetryAfter(%q) = %v, want %v", tt.hdr, got, tt.want)
			}
		})
	}
}

func TestParseRetryAfter_NilResponse(t *testing.T) {
	if got := ParseRetryAfter(nil); got != 0 {
		t.Errorf("ParseRetryAfter(nil) = %v, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// IsPreConnectionError
// ---------------------------------------------------------------------------

func TestIsPreConnectionError_Nil(t *testing.T) {
	if IsPreConnectionError(nil) {
		t.Error("nil error should not be pre-connection")
	}
}

func TestIsPreConnectionError_DialError(t *testing.T) {
	err := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: errors.New("connection refused"),
	}
	if !IsPreConnectionError(err) {
		t.Error("dial OpError should be pre-connection")
	}
}

func TestIsPreConnectionError_ReadError(t *testing.T) {
	err := &net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: errors.New("connection reset by peer"),
	}
	if IsPreConnectionError(err) {
		t.Error("read OpError should NOT be pre-connection")
	}
}

func TestIsPreConnectionError_DNSError(t *testing.T) {
	err := &net.DNSError{
		Err:  "no such host",
		Name: "api.example.com",
	}
	if !IsPreConnectionError(err) {
		t.Error("DNS error should be pre-connection")
	}
}

func TestIsPreConnectionError_WrappedDialError(t *testing.T) {
	inner := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}
	wrapped := fmt.Errorf("upstream: %w", inner)
	if !IsPreConnectionError(wrapped) {
		t.Error("wrapped dial error should be pre-connection")
	}
}

func TestIsPreConnectionError_EOF(t *testing.T) {
	if IsPreConnectionError(io.EOF) {
		t.Error("io.EOF should NOT be pre-connection (connection was established)")
	}
}

func TestIsPreConnectionError_GenericError(t *testing.T) {
	if IsPreConnectionError(errors.New("something went wrong")) {
		t.Error("generic error should NOT be pre-connection")
	}
}

// Verify ShouldRetry uses IsPreConnectionError for connection errors.
func TestShouldRetry_ConnError_DialIsSafe(t *testing.T) {
	cfg := RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		RetryOnTimeout: true,
	}
	dialErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}
	if !cfg.ShouldRetry(1, nil, dialErr, false) {
		t.Error("should retry on dial error")
	}
}

func TestShouldRetry_ConnError_ReadIsUnsafe(t *testing.T) {
	cfg := RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		RetryOnTimeout: true,
	}
	readErr := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("reset")}
	if cfg.ShouldRetry(1, nil, readErr, false) {
		t.Error("should NOT retry on read error (ambiguous post-transmission)")
	}
}

func TestShouldRetry_ConnError_TimeoutIsUnsafe(t *testing.T) {
	cfg := RetryConfig{
		Enabled:        true,
		MaxAttempts:    3,
		RetryOnTimeout: true,
	}
	if cfg.ShouldRetry(1, nil, io.EOF, false) {
		t.Error("should NOT retry on EOF (ambiguous)")
	}
}
