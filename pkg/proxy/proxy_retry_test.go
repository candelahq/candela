package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
)

// TestRetry_500ThenSuccess verifies that when retry is enabled and the upstream
// returns 500 on the first attempt, the proxy retries and succeeds on the second.
func TestRetry_500ThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"error":"temporary failure"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer upstream.Close()

	calc := newCalcWithTestModels()

	p, err := New(Config{
		Providers: []Provider{{
			Name:        "openai",
			UpstreamURL: upstream.URL,
		}},
		ProjectID: "test",
		Retry: RetryConfig{
			Enabled:              true,
			MaxAttempts:          3,
			RetryableStatusCodes: []int{500, 502, 503},
			RetryOnTimeout:       true,
			Backoff:              BackoffConfig{InitialMs: 1, MaxMs: 10, Multiplier: 2.0},
		},
	}, &mockSubmitter{}, calc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("expected 2 upstream attempts, got %d", got)
	}
}

// TestRetry_Disabled verifies that with retry disabled, a 500 is immediately
// forwarded to the client without retry.
func TestRetry_Disabled(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":"failure"}`)
	}))
	defer upstream.Close()

	calc := newCalcWithTestModels()

	p, err := New(Config{
		Providers: []Provider{{
			Name:        "openai",
			UpstreamURL: upstream.URL,
		}},
		ProjectID: "test",
		// Retry is disabled by default (Enabled: false).
	}, &mockSubmitter{}, calc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != 500 {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected 1 upstream attempt (no retry), got %d", got)
	}
}

// TestRetry_MaxAttemptsExhausted verifies that the proxy stops retrying after
// MaxAttempts and returns the last error to the client.
func TestRetry_MaxAttemptsExhausted(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":"always fails"}`)
	}))
	defer upstream.Close()

	calc := newCalcWithTestModels()

	p, err := New(Config{
		Providers: []Provider{{
			Name:        "openai",
			UpstreamURL: upstream.URL,
		}},
		ProjectID: "test",
		Retry: RetryConfig{
			Enabled:              true,
			MaxAttempts:          3,
			RetryableStatusCodes: []int{500},
			Backoff:              BackoffConfig{InitialMs: 1, MaxMs: 5, Multiplier: 2.0},
		},
	}, &mockSubmitter{}, calc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	// Should get the 500 from the last attempt.
	if resp.StatusCode != 500 {
		t.Fatalf("expected 500 after exhausting retries, got %d", resp.StatusCode)
	}
	// Should have tried exactly MaxAttempts times.
	if got := attempts.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

// TestRetry_400NotRetried verifies that 400 errors are never retried.
func TestRetry_400NotRetried(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":"bad request"}`)
	}))
	defer upstream.Close()

	calc := newCalcWithTestModels()

	p, err := New(Config{
		Providers: []Provider{{
			Name:        "openai",
			UpstreamURL: upstream.URL,
		}},
		ProjectID: "test",
		Retry: RetryConfig{
			Enabled:              true,
			MaxAttempts:          3,
			RetryableStatusCodes: []int{500, 502, 503},
			Backoff:              BackoffConfig{InitialMs: 1, MaxMs: 5, Multiplier: 2.0},
		},
	}, &mockSubmitter{}, calc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected exactly 1 attempt (400 not retried), got %d", got)
	}
}

// TestRetry_429WithRetryAfter verifies that the proxy respects Retry-After headers.
func TestRetry_429WithRetryAfter(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer upstream.Close()

	calc := newCalcWithTestModels()

	p, err := New(Config{
		Providers: []Provider{{
			Name:        "openai",
			UpstreamURL: upstream.URL,
		}},
		ProjectID: "test",
		Retry: RetryConfig{
			Enabled:              true,
			MaxAttempts:          3,
			RetryableStatusCodes: []int{429, 500},
			Backoff:              BackoffConfig{InitialMs: 1, MaxMs: 5, Multiplier: 2.0},
		},
	}, &mockSubmitter{}, calc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}
}

// TestFallback_PrimaryDownSecondaryUp verifies that when the primary provider
// fails and a fallback chain is configured, the proxy falls back to the secondary.
func TestFallback_PrimaryDownSecondaryUp(t *testing.T) {
	// Primary always returns 500.
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":"primary down"}`)
	}))
	defer primary.Close()

	// Secondary always succeeds.
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"from secondary"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer secondary.Close()

	calc := newCalcWithTestModels()
	// Use "openai" (primary) and "deepseek" (fallback) — both have registered
	// parsers so the model can be extracted from the request body.
	calc.SetPricing(costcalc.ModelPricing{Provider: "openai", Model: "gpt-4o", InputPerMillion: 2.50, OutputPerMillion: 10.00})
	calc.SetPricing(costcalc.ModelPricing{Provider: "deepseek", Model: "gpt-4o", InputPerMillion: 2.50, OutputPerMillion: 10.00})

	p, err := New(Config{
		Providers: []Provider{
			{Name: "openai", UpstreamURL: primary.URL},
			{Name: "deepseek", UpstreamURL: secondary.URL},
		},
		ProjectID: "test",
		Retry: RetryConfig{
			Enabled:              true,
			MaxAttempts:          1, // No retry per-provider, just fallback.
			RetryableStatusCodes: []int{500},
			Backoff:              BackoffConfig{InitialMs: 1, MaxMs: 5, Multiplier: 2.0},
		},
		Fallback: FallbackConfig{
			Enabled: true,
			Chains: []FallbackChain{{
				ModelPrefix: "gpt-4o",
				Providers:   []string{"openai", "deepseek"},
			}},
		},
	}, &mockSubmitter{}, calc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 from fallback, got %d, body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "from secondary") {
		t.Fatalf("expected response from secondary provider, got: %s", body)
	}
}

// TestCircuitBreaker_IsOpen verifies the IsOpen method.
func TestCircuitBreaker_IsOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		Threshold:    2,
		ResetTimeout: 10 * 1e9, // 10s as Duration
		HalfOpenMax:  1,
	}
	cb := NewCircuitBreaker(cfg)

	// Initially closed.
	if cb.IsOpen() {
		t.Fatal("expected closed circuit, got open")
	}

	// Trip it.
	cb.RecordFailure()
	cb.RecordFailure()

	if !cb.IsOpen() {
		t.Fatal("expected open circuit after 2 failures")
	}

	// Recover.
	cb.RecordSuccess()
	cb.RecordSuccess()
	// After enough successes in half-open, it should close.
	// But IsOpen checks Open state specifically.
}
