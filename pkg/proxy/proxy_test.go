package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// mockSubmitter captures submitted spans for assertions.
type mockSubmitter struct {
	mu    sync.Mutex
	spans []storage.Span
}

func (m *mockSubmitter) SubmitBatch(spans []storage.Span) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spans = append(m.spans, spans...)
}

func (m *mockSubmitter) getSpans() []storage.Span {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]storage.Span, len(m.spans))
	copy(cp, m.spans)
	return cp
}

func TestExtractRequestInfoAnthropic(t *testing.T) {
	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [{"role": "user", "content": "Hi"}]
	}`
	model, content := extractRequestInfo("anthropic", []byte(body))
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %s, want claude-sonnet-4-20250514", model)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestExtractRequestInfoGoogle(t *testing.T) {
	body := `{
		"contents": [{"parts": [{"text": "Hello"}]}]
	}`
	_, content := extractRequestInfo("google", []byte(body))
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestExtractResponseInfoAnthropic(t *testing.T) {
	body := `{
		"content": [{"type": "text", "text": "Hi there!"}],
		"usage": {"input_tokens": 12, "output_tokens": 5}
	}`

	content, input, output := extractResponseInfo("anthropic", []byte(body))

	if content != "Hi there!" {
		t.Errorf("content = %q", content)
	}
	if input != 12 {
		t.Errorf("input_tokens = %d, want 12", input)
	}
	if output != 5 {
		t.Errorf("output_tokens = %d, want 5", output)
	}
}

func TestExtractResponseInfoGoogle(t *testing.T) {
	body := `{
		"candidates": [{"content": {"parts": [{"text": "Hello from Gemini"}]}}],
		"usageMetadata": {"promptTokenCount": 15, "candidatesTokenCount": 10}
	}`

	content, input, output := extractResponseInfo("google", []byte(body))

	if content != "Hello from Gemini" {
		t.Errorf("content = %q", content)
	}
	if input != 15 {
		t.Errorf("input_tokens = %d, want 15", input)
	}
	if output != 10 {
		t.Errorf("output_tokens = %d, want 10", output)
	}
}

func TestIsStreamingRequest(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		body     string
		want     bool
	}{
		{"anthropic stream true", "anthropic", `{"stream": true}`, true},
		{"anthropic stream false", "anthropic", `{"stream": false}`, false},
		{"anthropic no stream", "anthropic", `{"model": "claude-sonnet-4-20250514"}`, false},
		{"google always false", "google", `{"contents": []}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStreamingRequest(tt.provider, []byte(tt.body))
			if got != tt.want {
				t.Errorf("isStreaming = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractStreamingUsageAnthropic(t *testing.T) {
	data := `data: {"type":"message_start","message":{"usage":{"input_tokens":10}}}

data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}

data: {"type":"content_block_delta","delta":{"type":"text_delta","text":" world"}}

data: {"type":"message_delta","usage":{"output_tokens":4}}

`
	content, input, output := extractStreamingUsage("anthropic", []byte(data))
	if content != "Hello world" {
		t.Errorf("content = %q, want 'Hello world'", content)
	}
	if input != 10 {
		t.Errorf("input = %d, want 10", input)
	}
	if output != 4 {
		t.Errorf("output = %d, want 4", output)
	}
}

func TestProxyEndToEndAnthropic(t *testing.T) {
	// Create a fake upstream that mimics Anthropic/Vertex AI response.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header was forwarded (Bearer token for Vertex AI).
		if r.Header.Get("Authorization") != "Bearer test-adc-token" {
			t.Error("auth header not forwarded")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "I'm Claude via Vertex AI"},
			},
			"usage": map[string]interface{}{
				"input_tokens":  80,
				"output_tokens": 30,
			},
		})
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test-project",
	}, submitter, calc)

	// Create test server with proxy routes.
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Make a proxied request.
	body := `{"model": "claude-sonnet-4-20250514", "messages": [{"role": "user", "content": "test"}]}`
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/proxy/anthropic/v1/messages", srv.URL),
		strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-adc-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	contentArr, ok := result["content"].([]interface{})
	if !ok || len(contentArr) == 0 {
		t.Fatal("expected content in response")
	}

	// Wait for async span creation.
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
	if span.GenAI.Model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %s, want claude-sonnet-4-20250514", span.GenAI.Model)
	}
	if span.GenAI.Provider != "anthropic" {
		t.Errorf("provider = %s, want anthropic", span.GenAI.Provider)
	}
	if span.GenAI.InputTokens != 80 {
		t.Errorf("input_tokens = %d, want 80", span.GenAI.InputTokens)
	}
	if span.GenAI.OutputTokens != 30 {
		t.Errorf("output_tokens = %d, want 30", span.GenAI.OutputTokens)
	}
	if span.ProjectID != "test-project" {
		t.Errorf("project_id = %s, want test-project", span.ProjectID)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	long := strings.Repeat("a", 200)
	got := truncate(long, 50)
	if len(got) > 70 {
		t.Errorf("truncate long len = %d", len(got))
	}
	if !strings.Contains(got, "[truncated]") {
		t.Error("expected truncated marker")
	}
}

func TestGenerateIDs(t *testing.T) {
	traceID := generateTraceID()
	if len(traceID) != 32 {
		t.Errorf("trace ID len = %d, want 32", len(traceID))
	}

	spanID := generateSpanID()
	if len(spanID) != 16 {
		t.Errorf("span ID len = %d, want 16", len(spanID))
	}

	// Should be unique.
	id1 := generateTraceID()
	id2 := generateTraceID()
	if id1 == id2 {
		t.Error("trace IDs should be unique")
	}
}

func TestRequestID_GeneratedWhenAbsent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify upstream receives a request ID.
		if r.Header.Get("X-Request-ID") == "" {
			t.Error("expected X-Request-ID forwarded to upstream")
		}
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

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// No X-Request-ID in request.
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

	// Response should contain a generated X-Request-ID.
	rid := resp.Header.Get("X-Request-ID")
	if rid == "" {
		t.Fatal("expected X-Request-ID in response")
	}
	if len(rid) != 32 {
		t.Errorf("expected 32-char generated request ID, got %d chars: %q", len(rid), rid)
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
	if spans[0].Attributes["http.request_id"] != rid {
		t.Errorf("span request_id = %q, want %q", spans[0].Attributes["http.request_id"], rid)
	}
}

func TestRequestID_AcceptedWhenProvided(t *testing.T) {
	const clientRequestID = "my-custom-request-id-12345678"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify client's request ID is forwarded upstream.
		if got := r.Header.Get("X-Request-ID"); got != clientRequestID {
			t.Errorf("upstream X-Request-ID = %q, want %q", got, clientRequestID)
		}
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

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Provide X-Request-ID.
	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("X-Request-ID", clientRequestID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	// Response should echo back the same ID.
	if got := resp.Header.Get("X-Request-ID"); got != clientRequestID {
		t.Errorf("response X-Request-ID = %q, want %q", got, clientRequestID)
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
	if spans[0].Attributes["http.request_id"] != clientRequestID {
		t.Errorf("span request_id = %q, want %q", spans[0].Attributes["http.request_id"], clientRequestID)
	}
}

func TestCircuitBreaker_SkipsSpanOnOpen(t *testing.T) {
	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":"fail"}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Blast 6 requests to trip the circuit (threshold=5).
	for i := 0; i < 6; i++ {
		req, _ := http.NewRequest("POST",
			srv.URL+"/proxy/anthropic/v1/messages",
			strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer tok")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}

	// All 6 requests should have been forwarded upstream (fail-open).
	if callCount != 6 {
		t.Errorf("expected 6 upstream calls (fail-open), got %d", callCount)
	}

	// Circuit should be open now.
	cb := p.breakers["anthropic"]
	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit open, got %s", cb.State())
	}

	// Wait a bit for any async span submissions.
	time.Sleep(100 * time.Millisecond)

	// After circuit opens (at request 5), subsequent requests skip spans.
	// Requests 1-5 had circuit closed (spans created), request 6 had circuit open (span skipped).
	// But the circuit breaker AllowRequest() is also called — so the 6th request's span is skipped.
	spans := submitter.getSpans()
	// We should have fewer spans than requests since the circuit opened.
	if len(spans) >= 6 {
		t.Errorf("expected fewer than 6 spans (circuit should skip some), got %d", len(spans))
	}
}
