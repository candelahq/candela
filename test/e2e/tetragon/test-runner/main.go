// Command tetragon-test-runner validates Tetragon eBPF event generation
// against a real Tetragon agent. It connects to the Tetragon gRPC export
// API, generates known syscalls, and verifies that matching events arrive
// with the correct schema.
//
// Environment variables:
//
//	TETRAGON_ADDR   — Tetragon gRPC address (default: localhost:54321)
//	TEST_TIMEOUT    — Overall test timeout (default: 60s)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/tetragonaudit"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	addr := envOrDefault("TETRAGON_ADDR", "localhost:54321")
	timeoutStr := envOrDefault("TEST_TIMEOUT", "60s")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		fatal("invalid TEST_TIMEOUT %q: %v", timeoutStr, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	slog.Info("🧪 tetragon integration test starting",
		"addr", addr, "timeout", timeout)

	if err := runTests(ctx, addr); err != nil {
		fatal("❌ tests failed: %v", err)
	}

	slog.Info("✅ all tetragon integration tests passed")
}

func runTests(ctx context.Context, addr string) error {
	// ── Test 1: Event Schema Fidelity ────────────────────────────────────
	slog.Info("━━━ Test 1: Event schema fidelity (tcp_connect to :443)")

	sink := &tetragonaudit.CollectorSink{}
	pipeline := tetragonaudit.NewPipeline(tetragonaudit.PipelineConfig{
		Sink: sink,
	})

	src, err := tetragonaudit.NewGRPCSource(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to tetragon: %w", err)
	}
	defer func() { _ = src.Close() }()

	// Start streaming in background.
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := src.StreamEventsWithRetry(streamCtx, pipeline, tetragonaudit.RetryConfig{
			InitialDelay: 500 * time.Millisecond,
			MaxDelay:     2 * time.Second,
		})
		if err != nil && err != context.Canceled {
			slog.Warn("stream ended with error", "error", err)
		}
	}()

	// Wait for the pipeline to connect.
	if err := waitForConnection(ctx, pipeline, 15*time.Second); err != nil {
		return fmt.Errorf("pipeline did not connect: %w", err)
	}
	slog.Info("✓ pipeline connected to tetragon gRPC")

	// Give the stream a moment to stabilize before generating test traffic.
	time.Sleep(2 * time.Second)

	// Snapshot current record count to isolate our test events.
	baselineCount := len(sink.GetRecords())

	// ── Generate known syscalls ──────────────────────────────────────────
	// Attempt tcp_connect to well-known port 443 targets.
	// These connections will fail (no server), but the kernel tcp_connect
	// kprobe fires before the connection attempt completes, so Tetragon
	// will still generate an event.
	targets := []string{
		"1.1.1.1:443",   // Cloudflare — will RST
		"8.8.8.8:443",   // Google DNS — will RST
		"192.0.2.1:443", // TEST-NET — guaranteed unreachable
	}

	slog.Info("generating tcp_connect syscalls", "targets", targets)
	for _, target := range targets {
		conn, err := net.DialTimeout("tcp", target, 2*time.Second)
		if err != nil {
			slog.Debug("dial failed (expected)", "target", target, "error", err)
		} else {
			_ = conn.Close()
			slog.Debug("dial succeeded", "target", target)
		}
	}

	// ── Wait for events to arrive ────────────────────────────────────────
	slog.Info("waiting for tetragon events to arrive...")
	deadline := time.After(15 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var newRecords []tetragonaudit.AuditRecord
	for {
		select {
		case <-deadline:
			return fmt.Errorf("timed out waiting for events (got %d total, %d new)",
				len(sink.GetRecords()), len(newRecords))
		case <-ticker.C:
			all := sink.GetRecords()
			if len(all) > baselineCount {
				newRecords = all[baselineCount:]
				// Wait specifically for a port-443 event from our test
				// traffic, not just any event (avoids false positives from
				// background processes).
				for _, r := range newRecords {
					if r.DstPort == 443 {
						slog.Info("received matching events", "new", len(newRecords), "total", len(all))
						goto validate
					}
				}
			}
		}
	}

validate:
	// ── Validate event schema ────────────────────────────────────────────
	slog.Info("validating event schema", "records", len(newRecords))

	// Find events matching our test targets (port 443 connections).
	var matched []tetragonaudit.AuditRecord
	for _, r := range newRecords {
		if r.DstPort == 443 {
			matched = append(matched, r)
		}
	}

	if len(matched) == 0 {
		// If no port 443 events, check if we got ANY kprobe events at all.
		// Tetragon may filter differently depending on kernel version.
		slog.Warn("no port-443 events found, checking all kprobe events")
		for _, r := range newRecords {
			slog.Info("record", "binary", r.Binary, "function", r.FunctionName,
				"dst_addr", r.DstAddr, "dst_port", r.DstPort,
				"action", r.Action, "policy", r.PolicyName)
		}
		return fmt.Errorf("expected at least 1 tcp_connect event to port 443, got 0 out of %d total events",
			len(newRecords))
	}

	slog.Info("found matching events", "count", len(matched))

	// Validate schema fields on the first matched event.
	ev := matched[0]
	var errs []string

	if ev.FunctionName != "tcp_connect" {
		errs = append(errs, fmt.Sprintf("FunctionName=%q, want tcp_connect", ev.FunctionName))
	}
	if ev.DstPort != 443 {
		errs = append(errs, fmt.Sprintf("DstPort=%d, want 443", ev.DstPort))
	}
	if ev.Action == "" {
		errs = append(errs, "Action is empty")
	}
	if ev.Binary == "" {
		errs = append(errs, "Binary is empty")
	}
	if ev.Timestamp.IsZero() {
		errs = append(errs, "Timestamp is zero")
	}

	if len(errs) > 0 {
		return fmt.Errorf("schema validation failed:\n  %s", strings.Join(errs, "\n  "))
	}

	slog.Info("✓ event schema validated",
		"function", ev.FunctionName,
		"dst_addr", ev.DstAddr,
		"dst_port", ev.DstPort,
		"action", ev.Action,
		"binary", ev.Binary,
		"policy", ev.PolicyName,
		"severity", ev.Severity,
		"node", ev.NodeName,
	)

	// ── Test 2: Pipeline Health ──────────────────────────────────────────
	slog.Info("━━━ Test 2: Pipeline health check")

	health := pipeline.Health()
	slog.Info("pipeline health",
		"healthy", health.Healthy,
		"connected", health.Connected,
		"processed", health.Processed,
		"dropped", health.Dropped,
		"errors", health.Errors,
	)

	if !health.Connected {
		return fmt.Errorf("pipeline not connected after event processing")
	}
	if health.Processed == 0 {
		return fmt.Errorf("pipeline processed 0 events")
	}

	slog.Info("✓ pipeline health OK")

	// ── Test 3: Pipeline Stats ───────────────────────────────────────────
	slog.Info("━━━ Test 3: Pipeline stats")

	processed, dropped, errCount := pipeline.Stats().Snapshot()
	slog.Info("pipeline stats",
		"processed", processed,
		"dropped", dropped,
		"errors", errCount,
	)

	if processed == 0 {
		return fmt.Errorf("stats show 0 processed events")
	}
	// Errors should be 0 for well-formed Tetragon events.
	if errCount > 0 {
		slog.Warn("non-zero error count in pipeline stats", "errors", errCount)
	}

	slog.Info("✓ pipeline stats OK")

	// ── Cleanup ──────────────────────────────────────────────────────────
	streamCancel()
	wg.Wait()

	if err := pipeline.Drain(ctx); err != nil {
		slog.Warn("drain failed", "error", err)
	}

	return nil
}

// waitForConnection polls the pipeline health until it shows Connected.
func waitForConnection(ctx context.Context, pipeline *tetragonaudit.Pipeline, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out after %v", timeout)
		case <-ticker.C:
			if pipeline.Health().Connected {
				return nil
			}
		}
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fatal(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}
