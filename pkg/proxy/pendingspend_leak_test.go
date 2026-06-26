package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// TestPendingSpend_ReleasedOnCostCapEarlyReturn verifies that the pending-spend
// reservation is released when the request is rejected by the per-request cost
// cap check (early return AFTER Reserve).
func TestPendingSpend_ReleasedOnCostCapEarlyReturn(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when cost cap rejects request")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p, _ := New(Config{
		Providers:      []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID:      "test",
		MaxRequestCost: 0.0001, // Extremely low cap to force rejection
	}, submitter, calc)

	userID := "leak-test-user@example.com"
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      true,
			RemainingUSD: 100.0, // Budget passes, but cost cap will reject
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{ID: userID, Email: userID})
		mux.ServeHTTP(w, r.WithContext(ctx))
	}))
	defer srv.Close()

	// Pre-check: no pending spend.
	if got := p.pendingSpend.Get(userID); got != 0 {
		t.Fatalf("pre-request pending spend = %f, want 0", got)
	}

	// Send a request with a large body so estimated cost exceeds the tiny cap.
	bigBody := fmt.Sprintf(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"%s"}]}`,
		strings.Repeat("x", 50000))
	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body) // drain

	// Should be rejected by cost cap.
	if resp.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 (cost cap)", resp.StatusCode)
	}

	// CRITICAL: pending spend must be zero — the deferred Release should have cleaned it up.
	if got := p.pendingSpend.Get(userID); got != 0 {
		t.Errorf("pending spend after cost-cap rejection = %f, want 0 (reservation leaked!)", got)
	}
}

// TestPendingSpend_ReleasedOnDailyLimitEarlyReturn verifies that the pending-spend
// reservation is released when the request is rejected by the daily spend limit.
func TestPendingSpend_ReleasedOnDailyLimitEarlyReturn(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when daily limit rejects request")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
		DailyLimits: []SpendLimitConfig{
			{Model: "claude-sonnet-4", MaxDailyUSD: 0.0001}, // Tiny limit
		},
	}, submitter, calc)

	userID := "daily-limit-leak@example.com"
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      true,
			RemainingUSD: 100.0,
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{ID: userID, Email: userID})
		mux.ServeHTTP(w, r.WithContext(ctx))
	}))
	defer srv.Close()

	// Pre-spend to exhaust the daily limit.
	limitUser := userID
	p.spendTracker.Record(limitUser, "claude-sonnet-4-20250514", 1.0, p.config.DailyLimits)

	// Pre-check: no pending spend.
	if got := p.pendingSpend.Get(userID); got != 0 {
		t.Fatalf("pre-request pending spend = %f, want 0", got)
	}

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body) // drain

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 (daily limit)", resp.StatusCode)
	}

	// CRITICAL: pending spend must be zero.
	if got := p.pendingSpend.Get(userID); got != 0 {
		t.Errorf("pending spend after daily-limit rejection = %f, want 0 (reservation leaked!)", got)
	}
}

// TestPendingSpend_ReleasedOnUpstreamFailure verifies that the pending-spend
// reservation is released when the upstream request fails (connection refused).
// This is an early return AFTER the reservation at L1324.
func TestPendingSpend_ReleasedOnUpstreamFailure(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := costcalc.New()

	// Point at a closed server so the upstream request fails immediately.
	closedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	closedServer.Close() // close it so connections are refused

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: closedServer.URL}},
		ProjectID: "test",
	}, submitter, calc)

	userID := "upstream-fail-leak@example.com"
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      true,
			RemainingUSD: 100.0,
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{ID: userID, Email: userID})
		mux.ServeHTTP(w, r.WithContext(ctx))
	}))
	defer srv.Close()

	// Pre-check: no pending spend.
	if got := p.pendingSpend.Get(userID); got != 0 {
		t.Fatalf("pre-request pending spend = %f, want 0", got)
	}

	body := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	// Upstream failure returns 502.
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (upstream failure)", resp.StatusCode)
	}

	// CRITICAL: pending spend must be zero.
	if got := p.pendingSpend.Get(userID); got != 0 {
		t.Errorf("pending spend after upstream failure = %f, want 0 (reservation leaked!)", got)
	}
}

// TestPendingSpend_ReleasedOnSuccessfulRequest verifies that the defer does not
// double-release on the happy path (handler releases, then defer sees 0).
func TestPendingSpend_ReleasedOnSuccessfulRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	userID := "happy-path@example.com"
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      true,
			RemainingUSD: 100.0,
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{ID: userID, Email: userID})
		mux.ServeHTTP(w, r.WithContext(ctx))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// After successful request, pending spend must be zero (no double-release, no leak).
	if got := p.pendingSpend.Get(userID); got != 0 {
		t.Errorf("pending spend after successful request = %f, want 0", got)
	}
}
