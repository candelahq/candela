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
	"github.com/candelahq/candela/pkg/billing"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// saBudgetStore extends budgetUserStore to track SetBudget calls for SA tests.
type saBudgetStore struct {
	budgetUserStore
	setBudgetCalls    []storage.BudgetRecord
	setBudgetErr      error
	provisioned       bool
	provisionedResult *storage.BudgetCheckResult
}

func (s *saBudgetStore) SetBudget(_ context.Context, b *storage.BudgetRecord) error {
	s.setBudgetCalls = append(s.setBudgetCalls, *b)
	if s.setBudgetErr != nil {
		return s.setBudgetErr
	}
	s.provisioned = true
	return nil
}

func (s *saBudgetStore) CheckBudget(_ context.Context, _ string, _ float64) (*storage.BudgetCheckResult, error) {
	if s.provisioned && s.provisionedResult != nil {
		return s.provisionedResult, nil
	}
	return s.checkResult, s.checkErr
}

// withSABudgetTestAuth injects a service account identity into the request context.
func withSABudgetTestAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{
			ID:    "candela-ci@my-project.iam.gserviceaccount.com",
			Email: "candela-ci@my-project.iam.gserviceaccount.com",
		})
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ====================================================================
// SA Auto-Provisioning Tests (#608)
// ====================================================================

func TestSABudget_AutoProvisionOnNoBudget(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer upstream.Close()

	store := &saBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{
				Allowed: false,
				Reason:  billing.ReasonNoBudget,
			},
		},
		provisionedResult: &storage.BudgetCheckResult{
			Allowed:      true,
			Reason:       billing.ReasonAllowed,
			RemainingUSD: 10.0,
			BudgetUSD:    10.0,
		},
	}

	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, calc)
	p.SetUserStore(store)
	p.SetSADefaultBudget(10.0)

	handler := withSABudgetTestAuth(p)
	req := httptest.NewRequest("POST", "/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Result().Body)
		t.Fatalf("expected 200, got %d: %s", rec.Code, body)
	}

	if len(store.setBudgetCalls) != 1 {
		t.Fatalf("expected 1 SetBudget call, got %d", len(store.setBudgetCalls))
	}
	budget := store.setBudgetCalls[0]
	if budget.LimitUSD != 10.0 {
		t.Errorf("expected limit 10.0, got %f", budget.LimitUSD)
	}
	if budget.PeriodType != "daily" {
		t.Errorf("expected period_type daily, got %s", budget.PeriodType)
	}
	if !strings.HasSuffix(budget.UserID, ".iam.gserviceaccount.com") {
		t.Errorf("expected SA ID, got %s", budget.UserID)
	}
}

func TestSABudget_SkipsAutoProvisionForHumanUsers(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when budget is exhausted for human user")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	store := &saBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{
				Allowed: false,
				Reason:  billing.ReasonNoBudget,
			},
		},
	}

	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, calc)
	p.SetUserStore(store)
	p.SetSADefaultBudget(10.0)

	handler := withTestAuth(p) // human user
	req := httptest.NewRequest("POST", "/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", rec.Code)
	}
	if len(store.setBudgetCalls) != 0 {
		t.Fatalf("expected 0 SetBudget calls for human user, got %d", len(store.setBudgetCalls))
	}
}

func TestSABudget_FailOpenOnProvisionError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer upstream.Close()

	store := &saBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{
				Allowed: false,
				Reason:  billing.ReasonNoBudget,
			},
		},
		setBudgetErr: fmt.Errorf("firestore unavailable"),
	}

	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, calc)
	p.SetUserStore(store)
	p.SetSADefaultBudget(10.0)

	handler := withSABudgetTestAuth(p)
	req := httptest.NewRequest("POST", "/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Result().Body)
		t.Fatalf("expected 200 (fail-open), got %d: %s", rec.Code, body)
	}
}

func TestSABudget_BlocksExhaustedSA(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when SA budget is exhausted")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	store := &saBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{
				Allowed:      false,
				Reason:       billing.ReasonBudgetExhausted,
				RemainingUSD: 0,
			},
		},
	}

	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, calc)
	p.SetUserStore(store)
	p.SetSADefaultBudget(10.0)

	handler := withSABudgetTestAuth(p)
	req := httptest.NewRequest("POST", "/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		body, _ := io.ReadAll(rec.Result().Body)
		t.Fatalf("expected 402 for exhausted SA budget, got %d: %s", rec.Code, body)
	}
	if len(store.setBudgetCalls) != 0 {
		t.Fatalf("expected 0 SetBudget calls for exhausted SA, got %d", len(store.setBudgetCalls))
	}
}

func TestSABudget_NoAutoProvisionWhenDisabled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	store := &saBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{
				Allowed: false,
				Reason:  billing.ReasonNoBudget,
			},
		},
	}

	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, calc)
	p.SetUserStore(store)
	// saDefaultBudgetUSD is 0 (default) — auto-provision disabled

	handler := withSABudgetTestAuth(p)
	req := httptest.NewRequest("POST", "/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 when auto-provision disabled, got %d", rec.Code)
	}
	if len(store.setBudgetCalls) != 0 {
		t.Fatalf("expected 0 SetBudget calls when disabled, got %d", len(store.setBudgetCalls))
	}
}

func TestSABudget_OnlyProvisionedOnce(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer upstream.Close()

	store := &saBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{
				Allowed: false,
				Reason:  billing.ReasonNoBudget,
			},
		},
		provisionedResult: &storage.BudgetCheckResult{
			Allowed:      true,
			Reason:       billing.ReasonAllowed,
			RemainingUSD: 10.0,
			BudgetUSD:    10.0,
		},
	}

	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, calc)
	p.SetUserStore(store)
	p.SetSADefaultBudget(10.0)

	handler := withSABudgetTestAuth(p)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/proxy/openai/v1/chat/completions",
			strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			body, _ := io.ReadAll(rec.Result().Body)
			t.Fatalf("request %d: expected 200, got %d: %s", i, rec.Code, body)
		}
	}

	if len(store.setBudgetCalls) != 1 {
		t.Fatalf("expected 1 SetBudget call (cached), got %d", len(store.setBudgetCalls))
	}
}

func TestSABudget_ResponseContainsBudgetExhaustedError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	store := &saBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{
				Allowed:      false,
				Reason:       billing.ReasonBudgetExhausted,
				RemainingUSD: 0,
			},
		},
	}

	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, calc)
	p.SetUserStore(store)
	p.SetSADefaultBudget(10.0)

	handler := withSABudgetTestAuth(p)
	req := httptest.NewRequest("POST", "/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", rec.Code)
	}

	var errResp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	body, _ := io.ReadAll(rec.Result().Body)
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errResp.Error.Type != "insufficient_budget" {
		t.Errorf("expected error type 'insufficient_budget', got %q", errResp.Error.Type)
	}
}
