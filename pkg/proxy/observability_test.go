package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/proxy/spendoutbox"
	"github.com/candelahq/candela/pkg/storage"
)

// failDeductUserStore embeds budgetUserStore so it inherits all stubs,
// but overrides DeductSpend to always fail.
type failDeductUserStore struct {
	budgetUserStore
}

func (f *failDeductUserStore) DeductSpend(_ context.Context, _ string, _ float64, _ int64) error {
	return fmt.Errorf("firestore unavailable")
}

// ── Test 1: HIGH-2 — SA spend is tracked via atomic counter ──

func TestSASpend_TracksAtomicCounter(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "hello from SA"},
			},
			"usage": map[string]interface{}{
				"input_tokens":  50,
				"output_tokens": 20,
			},
		})
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	// Wire in a user store so deductBudget runs (SA path).
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      true,
			RemainingUSD: 100,
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	// Inject a service account identity (same pattern as TestBudgetGate_SkippedForServiceAccount).
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sa := &auth.User{ID: "sa-uid", Email: "candela-proxy@my-project.iam.gserviceaccount.com"}
		mux.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), sa)))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Wait for async span creation to finish so the deductBudget SA
	// path (which runs synchronously before the async span) has completed.
	for i := 0; i < 50; i++ {
		if p.SASpendMicroUSD() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := p.SASpendMicroUSD(); got <= 0 {
		t.Errorf("SASpendMicroUSD() = %d, want > 0", got)
	}
}

// ── Test 2: HIGH-5 — droppedSpans incremented when semaphore is full ──

func TestDroppedSpans_IncrementedWhenSemaphoreFull(t *testing.T) {
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

	// Fill the spanSem channel completely (200 slots) so the next span is dropped.
	for i := 0; i < cap(p.spanSem); i++ {
		p.spanSem <- struct{}{}
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Poll DroppedSpans with a short timeout: the server-side
	// droppedSpans.Add(1) runs after the response body is written,
	// so the client may finish reading before the increment lands.
	var got int64
	for i := 0; i < 50; i++ {
		if got = p.DroppedSpans(); got > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got <= 0 {
		t.Errorf("DroppedSpans() = %d, want > 0 (span should have been dropped)", got)
	}

	// Drain the semaphore to avoid leaking blocked goroutines.
	for i := 0; i < cap(p.spanSem); i++ {
		<-p.spanSem
	}
}

// ── Test 3: CRIT-3 — failed DeductSpend enqueues to outbox ──

func TestDeductBudget_EnqueuesToOutboxOnFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "hello"},
			},
			"usage": map[string]interface{}{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		})
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	// Set up failing user store and in-memory outbox.
	p.SetUserStore(&failDeductUserStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{
				Allowed:      true,
				RemainingUSD: 50,
			},
		},
	})

	outbox, err := spendoutbox.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create outbox: %v", err)
	}
	defer func() { _ = outbox.Close() }()
	p.SetSpendOutbox(outbox)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	// Inject a regular user (not SA) so deductBudget actually fires.
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// deductBudget runs synchronously after the response is written,
	// so by the time Do() returns the outbox should already be populated.
	// Give a small grace period for retries with backoff.
	ctx := context.Background()
	var pending int64
	for i := 0; i < 100; i++ {
		pending, err = outbox.Pending(ctx)
		if err != nil {
			t.Fatalf("outbox.Pending failed: %v", err)
		}
		if pending > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if pending <= 0 {
		t.Errorf("outbox.Pending() = %d, want > 0 (failed deduction should be queued)", pending)
	}
}

// ── Test 4: Proxy metric accessors ──

func TestProxyMetrics_Accessors(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: "http://localhost"}},
		ProjectID: "test",
	}, submitter, calc)

	// Initial values should be zero.
	if got := p.DroppedSpans(); got != 0 {
		t.Errorf("DroppedSpans() = %d, want 0", got)
	}
	if got := p.SASpendMicroUSD(); got != 0 {
		t.Errorf("SASpendMicroUSD() = %d, want 0", got)
	}

	// After adding, values should reflect.
	p.droppedSpans.Add(5)
	p.saSpendMicroUSD.Add(42000)
	if got := p.DroppedSpans(); got != 5 {
		t.Errorf("DroppedSpans() = %d, want 5", got)
	}
	if got := p.SASpendMicroUSD(); got != 42000 {
		t.Errorf("SASpendMicroUSD() = %d, want 42000", got)
	}
}
