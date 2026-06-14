package loadtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadTestBasic(t *testing.T) {
	// Create a mock server that simulates an LLM API.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some latency.
		time.Sleep(5 * time.Millisecond)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]string{
					"role":    "assistant",
					"content": "Hello!",
				},
			}},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 5,
			},
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := Run(ctx, Config{
		TargetURL:   server.URL,
		Concurrency: 5,
		Duration:    2 * time.Second,
		Provider:    "test",
		Model:       "test-model",
		Prompt:      "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.TotalRequests == 0 {
		t.Error("expected some requests")
	}
	// A small number of context-deadline errors is expected at the tail end
	// of the run when the duration timer fires and cancels in-flight requests.
	// Allow up to 1% error rate.
	maxAllowedErrors := result.TotalRequests / 100
	if maxAllowedErrors < 5 {
		maxAllowedErrors = 5
	}
	if result.ErrorCount > maxAllowedErrors {
		t.Errorf("too many errors (%d/%d): %v", result.ErrorCount, result.TotalRequests, result.Errors)
	}
	if result.AvgLatency == 0 {
		t.Error("expected non-zero avg latency")
	}
	if result.RequestsPerSec == 0 {
		t.Error("expected non-zero RPS")
	}
	t.Logf("Results: %d requests, %.1f RPS, avg=%v, p50=%v, p95=%v, p99=%v",
		result.TotalRequests, result.RequestsPerSec,
		result.AvgLatency, result.P50Latency, result.P95Latency, result.P99Latency)
}

func TestLoadTestWithErrors(t *testing.T) {
	var requestCount atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count%3 == 0 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{}})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := Run(ctx, Config{
		TargetURL:   server.URL,
		Concurrency: 3,
		Duration:    1 * time.Second,
		Provider:    "test",
		Model:       "test-model",
		Prompt:      "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ErrorCount == 0 {
		t.Error("expected some errors")
	}
	t.Logf("Results: %d total, %d success, %d errors",
		result.TotalRequests, result.SuccessCount, result.ErrorCount)
	t.Logf("Error breakdown: %v", result.Errors)
}

func TestLoadTestCancellation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result, err := Run(ctx, Config{
		TargetURL:   server.URL,
		Concurrency: 10,
		Duration:    30 * time.Second, // much longer than context
		Provider:    "test",
		Model:       "test-model",
		Prompt:      "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Should have stopped early due to context cancellation.
	if result.TotalDuration > 2*time.Second {
		t.Errorf("expected early termination, ran for %v", result.TotalDuration)
	}
	t.Logf("Terminated after %v with %d requests", result.TotalDuration, result.TotalRequests)
}

func TestLoadTestRateLimited(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{}})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := Run(ctx, Config{
		TargetURL:      server.URL,
		Concurrency:    5,
		Duration:       2 * time.Second,
		RequestsPerSec: 50, // cap at 50 RPS
		Provider:       "test",
		Model:          "test-model",
		Prompt:         "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	// With 50 RPS cap over 2 seconds, expect roughly 100 requests (±margin).
	targetRPS := 50
	duration := 2 * time.Second
	maxExpected := int64(150) // generous upper bound
	if result.TotalRequests > maxExpected {
		t.Errorf("rate limiter not effective: got %d requests, expected at most ~%d",
			result.TotalRequests, maxExpected)
	}
	minExpected := int64(float64(targetRPS) * duration.Seconds() * 0.5) // at least 50% of target
	if result.TotalRequests < minExpected {
		t.Errorf("expected at least %d requests at %d RPS, got %d", minExpected, targetRPS, result.TotalRequests)
	}
	t.Logf("Rate-limited results: %d requests, %.1f RPS (target: 50)",
		result.TotalRequests, result.RequestsPerSec)
}

func TestLoadTestUnlimitedRPS(t *testing.T) {
	// Verify that RPS=0 means unlimited (no ticker panic).
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := Run(ctx, Config{
		TargetURL:   server.URL,
		Concurrency: 2,
		Duration:    500 * time.Millisecond,
		Provider:    "test",
		Model:       "test",
		Prompt:      "Hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalRequests == 0 {
		t.Error("expected requests with unlimited RPS")
	}
	t.Logf("Unlimited RPS results: %d requests, %.1f RPS",
		result.TotalRequests, result.RequestsPerSec)
}
