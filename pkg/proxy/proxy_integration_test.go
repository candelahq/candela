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

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// TestProxy_EndToEnd_OpenAI tests a full OpenAI proxy round-trip with mock upstream.
func TestProxy_EndToEnd_OpenAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request was forwarded correctly.
		if r.Header.Get("Authorization") != "Bearer sk-test-key" {
			t.Error("auth header not forwarded to upstream")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("content-type not forwarded")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]string{"role": "assistant", "content": "Hello from OpenAI!"},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     15,
				"completion_tokens": 8,
				"total_tokens":      23,
			},
		})
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test-project",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result["model"] != "gpt-4o" {
		t.Errorf("model = %v, want gpt-4o", result["model"])
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
		t.Fatal("expected span to be submitted")
	}

	span := spans[0]
	if span.GenAI == nil {
		t.Fatal("expected GenAI attributes")
	}
	if span.GenAI.Provider != "openai" {
		t.Errorf("provider = %s, want openai", span.GenAI.Provider)
	}
	if span.GenAI.InputTokens != 15 {
		t.Errorf("input_tokens = %d, want 15", span.GenAI.InputTokens)
	}
	if span.GenAI.OutputTokens != 8 {
		t.Errorf("output_tokens = %d, want 8", span.GenAI.OutputTokens)
	}
	if span.Name != "openai.chat" {
		t.Errorf("span name = %q, want openai.chat", span.Name)
	}
}

// TestProxy_EndToEnd_CompatBudgetExhausted tests that budget enforcement works
// through the compat route (/v1/chat/completions) in addition to the main proxy.
func TestProxy_EndToEnd_CompatBudgetExhausted(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be reached when budget is exhausted")
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test-budget-compat",
	}, submitter, calc)

	// Wire up a user store with exhausted budget.
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      false,
			RemainingUSD: 0,
		},
	})

	mux := http.NewServeMux()
	p.RegisterCompatRoutes(mux, []CompatModel{{ID: "gpt-4o", Provider: "openai"}})

	// Wrap with auth context.
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

// TestProxy_EndToEnd_W3CTraceContext tests that W3C traceparent headers
// are correctly parsed and propagated to the upstream and to spans.
func TestProxy_EndToEnd_W3CTraceContext(t *testing.T) {
	const (
		incomingTraceID = "0af7651916cd43dd8448eb211c80319c"
		incomingSpanID  = "b7ad6b7169203331"
		incomingTP      = "00-" + incomingTraceID + "-" + incomingSpanID + "-01"
	)

	var upstreamTP string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamTP = r.Header.Get("Traceparent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()
	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test-trace",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"trace test"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Traceparent", incomingTP)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	// Verify upstream received an updated traceparent with the same trace ID
	// but a different span ID (the proxy's span).
	if upstreamTP == "" {
		t.Fatal("upstream did not receive Traceparent header")
	}
	if !strings.HasPrefix(upstreamTP, "00-"+incomingTraceID+"-") {
		t.Errorf("upstream traceparent should contain original trace ID, got: %s", upstreamTP)
	}
	if upstreamTP == incomingTP {
		t.Error("upstream traceparent should have proxy's span ID, not the original")
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
		t.Fatal("expected span")
	}

	span := spans[0]
	// Span should inherit the caller's trace ID.
	if span.TraceID != incomingTraceID {
		t.Errorf("span TraceID = %q, want %q", span.TraceID, incomingTraceID)
	}
	// Span's parent should be the caller's span ID.
	if span.ParentSpanID != incomingSpanID {
		t.Errorf("span ParentSpanID = %q, want %q", span.ParentSpanID, incomingSpanID)
	}
}
