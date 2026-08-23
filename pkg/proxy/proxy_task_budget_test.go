package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// taskBudgetStore extends budgetUserStore with configurable task budget behavior.
type taskBudgetStore struct {
	budgetUserStore
	taskCheckResult   *storage.TaskBudgetCheckResult
	taskCheckErr      error
	taskDeductErr     error
	taskDeductCount   atomic.Int64
	taskDeductLastJob string
}

func (t *taskBudgetStore) CheckTaskBudget(_ context.Context, _ string, _ float64) (*storage.TaskBudgetCheckResult, error) {
	if t.taskCheckErr != nil {
		return nil, t.taskCheckErr
	}
	return t.taskCheckResult, nil
}

func (t *taskBudgetStore) DeductTaskSpend(_ context.Context, taskID string, _ float64) error {
	t.taskDeductCount.Add(1)
	t.taskDeductLastJob = taskID
	return t.taskDeductErr
}

// withTestSAAuth injects a service account identity into the request context.
func withTestSAAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.NewContext(r.Context(), &auth.User{ID: "sa@project.iam.gserviceaccount.com", Email: "sa@project.iam.gserviceaccount.com"})
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func makeAnthropicRequest(srvURL, jobID string) (*http.Response, error) {
	req, _ := http.NewRequest("POST",
		srvURL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")
	if jobID != "" {
		req.Header.Set("X-Candela-Job-Id", jobID)
	}
	return http.DefaultClient.Do(req)
}

func parseErrorType(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	var errResp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v (body=%s)", err, body)
	}
	return errResp.Error.Type
}

// ── Test: missing task budget → 402 task_budget_missing ──

func TestTaskBudget_MissingBudgetBlocks(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when task budget is missing")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
		taskCheckErr: storage.ErrNotFound,
	}

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, costcalc.New())
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	resp, err := makeAnthropicRequest(srv.URL, "task-no-budget")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 402; body = %s", resp.StatusCode, body)
	}
	if got := parseErrorType(t, resp); got != "task_budget_missing" {
		t.Errorf("error type = %q, want 'task_budget_missing'", got)
	}
}

// ── Test: exhausted task budget → 402 task_budget_exhausted ──

func TestTaskBudget_ExhaustedBudgetBlocks(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when task budget is exhausted")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
		taskCheckResult: &storage.TaskBudgetCheckResult{
			Allowed:      false,
			RemainingUSD: 0,
			LimitUSD:     5.0,
		},
	}

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, costcalc.New())
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	resp, err := makeAnthropicRequest(srv.URL, "task-exhausted")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 402; body = %s", resp.StatusCode, body)
	}
	if got := parseErrorType(t, resp); got != "task_budget_exhausted" {
		t.Errorf("error type = %q, want 'task_budget_exhausted'", got)
	}
}

// ── Test: expired task budget → 402 task_budget_expired ──

func TestTaskBudget_ExpiredBudgetBlocks(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when task budget is expired")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
		taskCheckResult: &storage.TaskBudgetCheckResult{
			Allowed: false,
			Reason:  "task_budget_expired",
		},
	}

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, costcalc.New())
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	resp, err := makeAnthropicRequest(srv.URL, "task-expired")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 402; body = %s", resp.StatusCode, body)
	}
	if got := parseErrorType(t, resp); got != "task_budget_expired" {
		t.Errorf("error type = %q, want 'task_budget_expired'", got)
	}
}

// ── Test: valid task budget → request proceeds ──

func TestTaskBudget_ValidBudgetAllows(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()

	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
		taskCheckResult: &storage.TaskBudgetCheckResult{
			Allowed:      true,
			RemainingUSD: 5.0,
			LimitUSD:     10.0,
		},
	}

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, costcalc.New())
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	resp, err := makeAnthropicRequest(srv.URL, "task-ok")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
}

// ── Test: no jobID → task budget check skipped ──

func TestTaskBudget_NoJobIDSkipsCheck(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()

	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
		// If task check is called, it would fail — proving the skip works.
		taskCheckErr: fmt.Errorf("should not be called"),
	}

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, costcalc.New())
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	// No X-Candela-Job-Id header.
	resp, err := makeAnthropicRequest(srv.URL, "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
}

// ── Test: SA with jobID → task budget enforced ──

func TestTaskBudget_SAWithJobIDEnforced(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when task budget is missing for SA")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
		taskCheckErr: storage.ErrNotFound,
	}

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, costcalc.New())
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestSAAuth(mux))
	defer srv.Close()

	resp, err := makeAnthropicRequest(srv.URL, "sa-task")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 402; body = %s", resp.StatusCode, body)
	}
	if got := parseErrorType(t, resp); got != "task_budget_missing" {
		t.Errorf("error type = %q, want 'task_budget_missing'", got)
	}
}

// ── Test: deductBudget calls DeductTaskSpend with jobID ──

func TestDeductBudget_CallsDeductTaskSpend(t *testing.T) {
	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
		taskCheckResult: &storage.TaskBudgetCheckResult{Allowed: true, RemainingUSD: 5.0},
	}
	calc := costcalc.New()
	p, _ := New(Config{ProjectID: "test"}, &mockSubmitter{}, calc)
	p.SetUserStore(store)

	// deductBudget with a jobID and positive tokens → should call DeductTaskSpend.
	p.deductBudget(context.Background(), Provider{Name: "anthropic"}, "claude-sonnet-4-20250514",
		"user@test.com", "task-deduct-1", 100, 50, 0)

	if got := store.taskDeductCount.Load(); got != 1 {
		t.Errorf("DeductTaskSpend called %d times, want 1", got)
	}
	if store.taskDeductLastJob != "task-deduct-1" {
		t.Errorf("DeductTaskSpend jobID = %q, want 'task-deduct-1'", store.taskDeductLastJob)
	}
}

// ── Test: deductBudget with empty jobID skips DeductTaskSpend ──

func TestDeductBudget_NoJobIDSkipsTaskDeduct(t *testing.T) {
	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
	}
	calc := costcalc.New()
	p, _ := New(Config{ProjectID: "test"}, &mockSubmitter{}, calc)
	p.SetUserStore(store)

	p.deductBudget(context.Background(), Provider{Name: "anthropic"}, "claude-sonnet-4-20250514",
		"user@test.com", "", 100, 50, 0)

	if got := store.taskDeductCount.Load(); got != 0 {
		t.Errorf("DeductTaskSpend called %d times, want 0", got)
	}
}

// ── Test: SA deductBudget still calls DeductTaskSpend ──

func TestDeductBudget_SACallsDeductTaskSpend(t *testing.T) {
	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
	}
	calc := costcalc.New()
	p, _ := New(Config{ProjectID: "test"}, &mockSubmitter{}, calc)
	p.SetUserStore(store)

	// SA + jobID → should call DeductTaskSpend even though user DeductSpend is skipped.
	p.deductBudget(context.Background(), Provider{Name: "anthropic"}, "claude-sonnet-4-20250514",
		"sa@project.iam.gserviceaccount.com", "sa-task-1", 100, 50, 0)

	if got := store.taskDeductCount.Load(); got != 1 {
		t.Errorf("DeductTaskSpend called %d times for SA, want 1", got)
	}
}

// ── Test: task budget check unavailable → 503 ──

func TestTaskBudget_CheckUnavailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should NOT be called when task budget check fails")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	store := &taskBudgetStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
		taskCheckErr: fmt.Errorf("firestore unavailable"),
	}

	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, &mockSubmitter{}, costcalc.New())
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	resp, err := makeAnthropicRequest(srv.URL, "task-err")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 503; body = %s", resp.StatusCode, body)
	}
}
