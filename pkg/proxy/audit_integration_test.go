package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
)

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: Integration tests for proxy hot path and error handling
// ──────────────────────────────────────────────────────────────────────────────

// TestAudit_HealthEndpoints verifies healthz/readyz return correct JSON.
// NOTE: Health endpoints are registered by the server binary, not the proxy
// package. We register them here manually to test the pattern.
func TestAudit_HealthEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	mux := http.NewServeMux()
	// Register health endpoints as the server binary does.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://localhost:1"}},
		ProjectID: "test",
	}, submitter, calc)
	p.RegisterRoutes(mux)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Run("healthz", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("healthz request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			t.Errorf("healthz status = %d, want 200", resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("healthz Content-Type = %q, want application/json", ct)
		}
		body, _ := io.ReadAll(resp.Body)
		var result map[string]string
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("healthz response not valid JSON: %v", err)
		}
		if result["status"] != "ok" {
			t.Errorf("healthz status = %q, want %q", result["status"], "ok")
		}
	})

	t.Run("readyz", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/readyz")
		if err != nil {
			t.Fatalf("readyz request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			t.Errorf("readyz status = %d, want 200", resp.StatusCode)
		}
	})
}

// TestAudit_UnknownProviderReturnsError verifies unknown proxy provider → error.
// The proxy handler returns 400 for unknown providers, but Go's ServeMux
// may return 404 if the path pattern doesn't match the registered route.
func TestAudit_UnknownProviderReturnsError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://localhost:1"}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"test","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/nonexistent/v1/chat/completions",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Unknown provider should return an error (400 or 404 depending on route matching).
	if resp.StatusCode == 200 {
		t.Errorf("unknown provider returned 200 OK, expected error status")
	}
}

// TestAudit_ProxyForwardAuthHeader verifies auth headers are forwarded.
func TestAudit_ProxyForwardAuthHeader(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chat-1", "object": "chat.completion", "created": 1700000000,
			"model": "gpt-4o",
			"choices": []map[string]interface{}{
				{"index": 0, "message": map[string]string{"role": "assistant", "content": "hi"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
		})
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"test"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-audit-test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	if gotAuth != "Bearer sk-audit-test" {
		t.Errorf("auth header = %q, want %q", gotAuth, "Bearer sk-audit-test")
	}
}

// TestAudit_SpanCaptured verifies span is captured for successful proxy calls.
func TestAudit_SpanCaptured(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chat-2", "object": "chat.completion", "created": 1700000000,
			"model": "gpt-4o",
			"choices": []map[string]interface{}{
				{"index": 0, "message": map[string]string{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"audit"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Wait for async span submission.
	for i := 0; i < 100; i++ {
		if len(submitter.getSpans()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	spans := submitter.getSpans()
	if len(spans) == 0 {
		t.Fatal("no span submitted after proxy call")
	}
	if spans[0].GenAI == nil {
		t.Fatal("span missing GenAI attributes")
	}
	if spans[0].GenAI.Provider != "openai" {
		t.Errorf("span provider = %q, want openai", spans[0].GenAI.Provider)
	}
}

// TestAudit_CompatModelsEndpoint verifies /v1/models returns proper JSON.
func TestAudit_CompatModelsEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://localhost:1"}},
		ProjectID: "test",
	}, submitter, calc)

	models := []CompatModel{
		{ID: "gpt-4o", Provider: "openai"},
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	p.RegisterCompatRoutes(mux, models)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("/v1/models status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Object string          `json:"object"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if result.Object != "list" {
		t.Errorf("object = %q, want list", result.Object)
	}
	// Verify data is an array, not null.
	if string(result.Data) == "null" {
		t.Error("/v1/models data is null, want array")
	}
}

// TestAudit_CostCalculatorIntegration verifies end-to-end cost calculation.
func TestAudit_CostCalculatorIntegration(t *testing.T) {
	calc := costcalc.New()

	// Known model should have pricing.
	cost := calc.Calculate("google", "gemini-2.5-pro", 1_000_000, 100_000)
	if cost <= 0 {
		t.Errorf("Gemini 2.5 Pro cost = %v, want > 0", cost)
	}

	// Unknown model should return 0.
	cost = calc.Calculate("unknown", "nonexistent", 1000, 500)
	if cost != 0 {
		t.Errorf("unknown model cost = %v, want 0", cost)
	}
}

// TestAudit_UpstreamError verifies proxy handles 500 from upstream.
func TestAudit_UpstreamError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"fail"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Proxy should forward the upstream error.
	if resp.StatusCode != 500 {
		t.Errorf("upstream error status = %d, want 500", resp.StatusCode)
	}
}

// TestAudit_SpanHasCost verifies that spans include cost data.
func TestAudit_SpanHasCost(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chat-cost", "object": "chat.completion", "created": 1700000000,
			"model": "gpt-4o",
			"choices": []map[string]interface{}{
				{"index": 0, "message": map[string]string{"role": "assistant", "content": "cost check"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150},
		})
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"cost test"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	for i := 0; i < 100; i++ {
		if len(submitter.getSpans()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	spans := submitter.getSpans()
	if len(spans) == 0 {
		t.Fatal("no span captured")
	}

	span := spans[0]
	if span.GenAI == nil {
		t.Fatal("span missing GenAI")
	}
	if span.GenAI.InputTokens != 100 {
		t.Errorf("input tokens = %d, want 100", span.GenAI.InputTokens)
	}
	if span.GenAI.OutputTokens != 50 {
		t.Errorf("output tokens = %d, want 50", span.GenAI.OutputTokens)
	}
	if span.GenAI.CostUSD <= 0 {
		t.Errorf("cost = %v, want > 0", span.GenAI.CostUSD)
	}
}
