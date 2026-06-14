// Command loadtest-runner is a CLI tool for running load tests against a live
// Candela proxy instance.
//
// Usage:
//
//	go run ./test/loadtest/cmd -url http://localhost:8080/proxy/openai/v1/chat/completions -c 10 -d 30s
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/candelahq/candela/test/loadtest"
)

func main() {
	url := flag.String("url", "http://localhost:8080/proxy/openai/v1/chat/completions", "target URL")
	concurrency := flag.Int("c", 10, "number of concurrent workers")
	duration := flag.Duration("d", 30*time.Second, "test duration")
	rps := flag.Int("rps", 0, "requests per second limit (0 = unlimited)")
	provider := flag.String("provider", "openai", "LLM provider name")
	model := flag.String("model", "gpt-4o-mini", "model identifier")
	prompt := flag.String("prompt", "Say hello in one word.", "prompt text")
	token := flag.String("token", "", "Bearer auth token (env: LOADTEST_TOKEN)")
	flag.Parse()

	authToken := *token
	if authToken == "" {
		authToken = os.Getenv("LOADTEST_TOKEN")
	}

	cfg := loadtest.Config{
		TargetURL:      *url,
		Concurrency:    *concurrency,
		Duration:       *duration,
		RequestsPerSec: *rps,
		AuthToken:      authToken,
		Provider:       *provider,
		Model:          *model,
		Prompt:         *prompt,
	}

	fmt.Printf("🔥 Load test starting\n")
	fmt.Printf("   Target:      %s\n", cfg.TargetURL)
	fmt.Printf("   Workers:     %d\n", cfg.Concurrency)
	fmt.Printf("   Duration:    %v\n", cfg.Duration)
	if cfg.RequestsPerSec > 0 {
		fmt.Printf("   Rate limit:  %d RPS\n", cfg.RequestsPerSec)
	} else {
		fmt.Printf("   Rate limit:  unlimited\n")
	}
	fmt.Printf("   Provider:    %s\n", cfg.Provider)
	fmt.Printf("   Model:       %s\n", cfg.Model)
	fmt.Println()

	// Allow Ctrl-C to gracefully stop the test.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	result, err := loadtest.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📊 Results\n")
	fmt.Printf("   Total requests:  %d\n", result.TotalRequests)
	fmt.Printf("   Success:         %d\n", result.SuccessCount)
	fmt.Printf("   Errors:          %d\n", result.ErrorCount)
	fmt.Printf("   Duration:        %v\n", result.TotalDuration.Round(time.Millisecond))
	fmt.Printf("   RPS:             %.1f\n", result.RequestsPerSec)
	fmt.Println()
	fmt.Printf("   Latency:\n")
	fmt.Printf("     avg:  %v\n", result.AvgLatency.Round(time.Microsecond))
	fmt.Printf("     p50:  %v\n", result.P50Latency.Round(time.Microsecond))
	fmt.Printf("     p95:  %v\n", result.P95Latency.Round(time.Microsecond))
	fmt.Printf("     p99:  %v\n", result.P99Latency.Round(time.Microsecond))
	fmt.Printf("     max:  %v\n", result.MaxLatency.Round(time.Microsecond))

	if len(result.Errors) > 0 {
		fmt.Println()
		fmt.Printf("   Error breakdown:\n")
		for errType, count := range result.Errors {
			fmt.Printf("     %s: %d\n", errType, count)
		}
	}
}
