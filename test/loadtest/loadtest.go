// Package loadtest provides a reusable load testing framework for the Candela
// proxy. It drives concurrent HTTP requests against a target URL, collects
// latency percentiles (p50/p95/p99), and reports per-status-code error
// breakdowns.
//
// Usage:
//
//	result, err := loadtest.Run(ctx, loadtest.Config{
//	    TargetURL:   "http://localhost:8080/proxy/openai/v1/chat/completions",
//	    Concurrency: 10,
//	    Duration:    30 * time.Second,
//	})
package loadtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config configures a load test run.
type Config struct {
	// TargetURL is the endpoint to send requests to.
	TargetURL string

	// Concurrency is the number of concurrent worker goroutines.
	Concurrency int

	// Duration is how long the load test runs before stopping.
	Duration time.Duration

	// RequestsPerSec limits the aggregate request rate across all workers.
	// 0 means unlimited.
	RequestsPerSec int

	// AuthToken is the Bearer token for Authorization header. Empty = no auth.
	AuthToken string

	// Provider is the LLM provider name (used in request body).
	Provider string

	// Model is the model identifier (used in request body).
	Model string

	// Prompt is the user message content.
	Prompt string
}

// Result holds the aggregated results of a load test.
type Result struct {
	TotalRequests  int64
	SuccessCount   int64
	ErrorCount     int64
	TotalDuration  time.Duration
	AvgLatency     time.Duration
	P50Latency     time.Duration
	P95Latency     time.Duration
	P99Latency     time.Duration
	MaxLatency     time.Duration
	RequestsPerSec float64
	Errors         map[string]int // error description → count
}

// requestResult captures the outcome of a single HTTP request.
type requestResult struct {
	latency    time.Duration
	statusCode int
	err        error
}

// Run executes a load test with the given configuration.
// It spawns cfg.Concurrency worker goroutines that continuously send requests
// until cfg.Duration elapses or ctx is cancelled.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.Duration <= 0 {
		cfg.Duration = 10 * time.Second
	}
	if cfg.Prompt == "" {
		cfg.Prompt = "Hello"
	}

	// Build a context that expires after Duration or when the parent ctx is done.
	runCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()

	// Channel for collecting per-request results.
	results := make(chan requestResult, cfg.Concurrency*100)

	// Rate limiter: simple token-bucket using a ticker.
	var ticker *time.Ticker
	var tokenCh <-chan time.Time
	if cfg.RequestsPerSec > 0 {
		interval := time.Second / time.Duration(cfg.RequestsPerSec)
		ticker = time.NewTicker(interval)
		tokenCh = ticker.C
		defer ticker.Stop()
	}

	// Shared HTTP client with keep-alive.
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.Concurrency * 2,
			MaxIdleConnsPerHost: cfg.Concurrency * 2,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	body := buildRequestBody(cfg)

	var wg sync.WaitGroup
	var totalSent atomic.Int64

	startTime := time.Now()

	// Launch workers.
	for i := range cfg.Concurrency {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				// Check context before sending.
				select {
				case <-runCtx.Done():
					return
				default:
				}

				// Rate limiting: wait for a token if configured.
				if tokenCh != nil {
					select {
					case <-tokenCh:
					case <-runCtx.Done():
						return
					}
				}

				rr := sendRequest(runCtx, client, cfg, body)
				totalSent.Add(1)

				select {
				case results <- rr:
				case <-runCtx.Done():
					return
				}
			}
		}(i)
	}

	// Close results channel when all workers are done.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results.
	var latencies []time.Duration
	errors := make(map[string]int)
	var successCount, errorCount int64

	for rr := range results {
		latencies = append(latencies, rr.latency)
		if rr.err != nil {
			errorCount++
			errors[rr.err.Error()]++
		} else if rr.statusCode >= 400 {
			errorCount++
			key := fmt.Sprintf("HTTP %d", rr.statusCode)
			errors[key]++
		} else {
			successCount++
		}
	}

	totalDuration := time.Since(startTime)

	result := &Result{
		TotalRequests:  int64(len(latencies)),
		SuccessCount:   successCount,
		ErrorCount:     errorCount,
		TotalDuration:  totalDuration,
		Errors:         errors,
		RequestsPerSec: float64(len(latencies)) / totalDuration.Seconds(),
	}

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		result.AvgLatency = total / time.Duration(len(latencies))
		result.P50Latency = percentile(latencies, 0.50)
		result.P95Latency = percentile(latencies, 0.95)
		result.P99Latency = percentile(latencies, 0.99)
		result.MaxLatency = latencies[len(latencies)-1]
	}

	return result, nil
}

// sendRequest sends a single HTTP POST to the target and measures latency.
func sendRequest(ctx context.Context, client *http.Client, cfg Config, body string) requestResult {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TargetURL, strings.NewReader(body))
	if err != nil {
		return requestResult{latency: time.Since(start), err: fmt.Errorf("create request: %w", err)}
	}

	req.Header.Set("Content-Type", "application/json")
	if cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return requestResult{latency: time.Since(start), err: fmt.Errorf("do request: %w", err)}
	}
	// Drain and close body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	return requestResult{
		latency:    time.Since(start),
		statusCode: resp.StatusCode,
	}
}

// buildRequestBody constructs an OpenAI-compatible chat completion request.
func buildRequestBody(cfg Config) string {
	reqBody := map[string]interface{}{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "user", "content": cfg.Prompt},
		},
		"max_tokens": 10,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		// Fallback — should never happen with simple string maps.
		return fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"%s"}],"max_tokens":10}`, cfg.Model, cfg.Prompt)
	}
	return string(b)
}

// percentile returns the value at the given percentile (0.0–1.0) from a
// pre-sorted slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
