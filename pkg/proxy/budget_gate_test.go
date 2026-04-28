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

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// withTestAuth wraps a handler to inject a test user into the request context.
func withTestAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{ID: "test-user", Email: "test@example.com"})
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// budgetUserStore is a minimal UserStore that only implements CheckBudget
// for testing the pre-flight budget gate. All other methods panic.
type budgetUserStore struct {
	checkResult *storage.BudgetCheckResult
	checkErr    error
}

func (b *budgetUserStore) CheckBudget(_ context.Context, _ string, _ float64) (*storage.BudgetCheckResult, error) {
	return b.checkResult, b.checkErr
}

// Stub remaining interface methods — the budget gate runs before any of these are called.
func (b *budgetUserStore) CreateUser(context.Context, *storage.UserRecord) error { return nil }
func (b *budgetUserStore) GetUser(context.Context, string) (*storage.UserRecord, error) {
	return &storage.UserRecord{}, nil
}
func (b *budgetUserStore) GetUserByEmail(context.Context, string) (*storage.UserRecord, error) {
	return &storage.UserRecord{}, nil
}
func (b *budgetUserStore) GetUsers(context.Context, []string) (map[string]*storage.UserRecord, error) {
	return nil, nil
}
func (b *budgetUserStore) ListUsers(context.Context, string, int, int) ([]*storage.UserRecord, int, error) {
	return nil, 0, nil
}
func (b *budgetUserStore) UpdateUser(context.Context, *storage.UserRecord) error  { return nil }
func (b *budgetUserStore) TouchLastSeen(context.Context, string) error            { return nil }
func (b *budgetUserStore) DeleteUser(context.Context, string) error               { return nil }
func (b *budgetUserStore) SetBudget(context.Context, *storage.BudgetRecord) error { return nil }
func (b *budgetUserStore) GetBudget(context.Context, string) (*storage.BudgetRecord, error) {
	return nil, nil
}
func (b *budgetUserStore) ResetSpend(context.Context, string) error                { return nil }
func (b *budgetUserStore) CreateGrant(context.Context, *storage.GrantRecord) error { return nil }
func (b *budgetUserStore) ListGrants(context.Context, string, bool) ([]*storage.GrantRecord, error) {
	return nil, nil
}
func (b *budgetUserStore) RevokeGrant(context.Context, string, string) error { return nil }
func (b *budgetUserStore) DeductSpend(context.Context, string, float64, int64) error {
	return nil
}
func (b *budgetUserStore) CheckRateLimit(context.Context, string) (bool, int, int, error) {
	return true, 0, 60, nil
}
func (b *budgetUserStore) LogAction(context.Context, *storage.AuditRecord) error       { return nil }
func (b *budgetUserStore) LogGlobalAction(context.Context, *storage.AuditRecord) error { return nil }
func (b *budgetUserStore) ListAuditLog(context.Context, string, int) ([]*storage.AuditRecord, error) {
	return nil, nil
}
func (b *budgetUserStore) Close() error { return nil }

// ====================================================================
// Pre-flight Budget Gate
// ====================================================================

func TestBudgetGate_BlocksExhaustedBudget(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when budget is exhausted")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	// Wire in a user store that reports budget exhausted.
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      false,
			RemainingUSD: 0,
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
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

	// Should return 402 Payment Required.
	if resp.StatusCode != http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 402; body = %s", resp.StatusCode, body)
	}

	// Verify response body contains structured error.
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v (body=%s)", err, body)
	}
	if errResp.Error.Type != "insufficient_budget" {
		t.Errorf("error type = %q, want 'insufficient_budget'", errResp.Error.Type)
	}
}

func TestBudgetGate_AllowsWhenBudgetRemains(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	// Wire in a user store that reports budget available.
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      true,
			RemainingUSD: 5.00,
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
}

func TestBudgetGate_CheckErrorAllowsRequest(t *testing.T) {
	// When CheckBudget fails, the request should be allowed (fail-open).
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	p.SetUserStore(&budgetUserStore{
		checkErr: fmt.Errorf("firestore unavailable"),
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
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

	// Fail-open: should still get 200, not 5xx.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 200 (fail-open); body = %s", resp.StatusCode, body)
	}
}
