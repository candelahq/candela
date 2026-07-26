package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestServerErrChPattern verifies that when a server fails to bind,
// the error flows through errCh instead of calling os.Exit.
// This mirrors the pattern used in candela-server, candela-sidecar,
// and candela's runForeground.
func TestServerErrChPattern(t *testing.T) {
	// Bind a port so the second server fails.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()

	// Mimic the production pattern: errCh + goroutine + select.
	errCh := make(chan error, 1)
	srv := &http.Server{Addr: addr}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case srvErr := <-errCh:
		// Expected: port already in use.
		if srvErr == nil {
			t.Fatal("expected non-nil error from errCh")
		}
		t.Logf("errCh received expected error: %v", srvErr)
	case <-ctx.Done():
		t.Fatal("timed out waiting for server error — errCh pattern may be broken")
		_ = srv.Shutdown(ctx)
	}
}

// TestServerGracefulShutdown verifies that a server that starts
// successfully can be shut down cleanly through the signal path.
func TestServerGracefulShutdown(t *testing.T) {
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: http.NewServeMux(),
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Give server a moment to start.
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Simulate graceful shutdown (what happens on SIGINT).
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// Verify no error was sent to errCh.
	select {
	case srvErr := <-errCh:
		t.Fatalf("unexpected error on errCh after clean shutdown: %v", srvErr)
	default:
		t.Log("clean shutdown — no error on errCh, defers would execute")
	}
}

// TestErrChDoesNotBlock verifies the errCh is buffered and doesn't
// block the goroutine when no one is reading yet.
func TestErrChDoesNotBlock(t *testing.T) {
	errCh := make(chan error, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		errCh <- errors.New("bind: address already in use")
	}()

	select {
	case <-done:
		// Goroutine completed without blocking — buffer works.
	case <-time.After(1 * time.Second):
		t.Fatal("goroutine blocked sending to errCh — channel must be buffered")
	}

	err := <-errCh
	if err == nil {
		t.Fatal("expected error from errCh")
	}
}
