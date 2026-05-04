package proxy

import (
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
	"github.com/candelahq/candela/pkg/storage"
)

// ──────────────────────────────────────────
// I-1: Budget gate blocks exhausted user via OpenAI compat route
// ──────────────────────────────────────────

func TestIntegration_BudgetGateBlocksViaCompatRoute(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should NOT be reached when budget is exhausted")
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test-budget-compat-v5",
	}, submitter, calc)

	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      false,
			RemainingUSD: 0,
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	p.RegisterCompatRoutes(mux, []CompatModel{{ID: "gpt-4o", Provider: "openai"}})
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPaymentRequired {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 402, body = %s", resp.StatusCode, respBody)
	}
}

// ──────────────────────────────────────────
// I-2: Streaming response forwarding
// ──────────────────────────────────────────

func TestIntegration_StreamingResponseForwarding(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":" world"}}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			_, _ = fmt.Fprintln(w, chunk)
			_, _ = fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test-stream",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	respStr := string(respBody)

	if !strings.Contains(respStr, "Hello") {
		t.Errorf("streaming response should contain 'Hello': %s", respStr)
	}
	if !strings.Contains(respStr, "[DONE]") {
		t.Errorf("streaming response should contain [DONE]: %s", respStr)
	}

	// Wait for async span.
	for i := 0; i < 50; i++ {
		if len(submitter.getSpans()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	spans := submitter.getSpans()
	if len(spans) == 0 {
		t.Fatal("expected streaming span")
	}
	if !strings.Contains(spans[0].Name, "stream") {
		t.Errorf("span name = %q, should contain 'stream'", spans[0].Name)
	}
}

// ──────────────────────────────────────────
// I-3: Circuit breaker trips on consecutive failures
// ──────────────────────────────────────────

func TestIntegration_CircuitBreakerTrips(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test-cb-v5",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for i := 0; i < 6; i++ {
		req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions",
			strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer tok")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}

	cb := p.breakers["openai"]
	if cb.State() != CircuitOpen {
		t.Errorf("expected CircuitOpen, got %s", cb.State())
	}
}

// ──────────────────────────────────────────
// I-4: CORS + compat models endpoint
// ──────────────────────────────────────────

func TestIntegration_CompatModelsEndpoint(t *testing.T) {
	calc := costcalc.New()
	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://localhost:1"}},
		ProjectID: "test-models",
	}, nil, calc)

	mux := http.NewServeMux()
	p.RegisterCompatRoutes(mux, []CompatModel{{ID: "gpt-4o", Provider: "openai"}})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var modelsResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&modelsResp)
	if modelsResp["object"] != "list" {
		t.Errorf("object = %v, want 'list'", modelsResp["object"])
	}
	data, ok := modelsResp["data"].([]interface{})
	if !ok || len(data) == 0 {
		t.Fatal("expected at least 1 model in data")
	}
	model := data[0].(map[string]interface{})
	if model["id"] != "gpt-4o" {
		t.Errorf("model id = %v, want gpt-4o", model["id"])
	}
}

// ──────────────────────────────────────────
// I-5: Service account auth skips budget check
// ──────────────────────────────────────────

func TestIntegration_ServiceAccountSkipsBudget(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test-sa-v5",
	}, submitter, calc)

	// Budget is exhausted — but SA should bypass.
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      false,
			RemainingUSD: 0,
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sa := &auth.User{
			ID:    "sa-123",
			Email: "my-sa@my-project.iam.gserviceaccount.com",
		}
		mux.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), sa)))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// SA bypasses budget → 200, not 402.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 200 (SA bypasses budget), body = %s", resp.StatusCode, body)
	}
}
