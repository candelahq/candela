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

func TestExtractRequestInfoOpenAI(t *testing.T) {
	body := `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hello"}]
	}`
	model, content := extractRequestInfo("openai", []byte(body))
	if model != "gpt-4o" {
		t.Errorf("model = %s, want gpt-4o", model)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestExtractRequestInfoAnthropic(t *testing.T) {
	body := `{
		"model": "claude-3-5-sonnet-20241022",
		"messages": [{"role": "user", "content": "Hi"}]
	}`
	model, content := extractRequestInfo("anthropic", []byte(body))
	if model != "claude-3-5-sonnet-20241022" {
		t.Errorf("model = %s, want claude-3-5-sonnet-20241022", model)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestExtractResponseInfoOpenAI(t *testing.T) {
	body := `{
		"choices": [{"message": {"content": "Hello! How can I help?"}}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18}
	}`

	content, input, output := extractResponseInfo("openai", []byte(body))

	if content != "Hello! How can I help?" {
		t.Errorf("content = %q", content)
	}
	if input != 10 {
		t.Errorf("input_tokens = %d, want 10", input)
	}
	if output != 8 {
		t.Errorf("output_tokens = %d, want 8", output)
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
		{"openai stream true", "openai", `{"stream": true}`, true},
		{"openai stream false", "openai", `{"stream": false}`, false},
		{"openai no stream", "openai", `{"model": "gpt-4o"}`, false},
		{"anthropic stream", "anthropic", `{"stream": true}`, true},
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

func TestExtractStreamingUsageOpenAI(t *testing.T) {
	data := `data: {"choices":[{"delta":{"content":"Hello"}}]}

data: {"choices":[{"delta":{"content":" world"}}]}

data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2}}

data: [DONE]

`
	content, input, output := extractStreamingUsage("openai", []byte(data))
	if content != "Hello world" {
		t.Errorf("content = %q, want 'Hello world'", content)
	}
	if input != 5 {
		t.Errorf("input = %d, want 5", input)
	}
	if output != 2 {
		t.Errorf("output = %d, want 2", output)
	}
}

func TestProxyEndToEnd(t *testing.T) {
	// Create a fake upstream that mimics OpenAI.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header was forwarded.
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("auth header not forwarded")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "I'm a test response"}},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     100,
				"completion_tokens": 50,
				"total_tokens":      150,
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

	// Create test server with proxy routes.
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Make a proxied request.
	body := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "test"}]}`
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/proxy/openai/v1/chat/completions", srv.URL),
		strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify the response was proxied correctly.
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		t.Fatal("expected choices in response")
	}

	// Wait briefly for async span creation.
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
	if span.GenAI.Model != "gpt-4o" {
		t.Errorf("model = %s, want gpt-4o", span.GenAI.Model)
	}
	if span.GenAI.InputTokens != 100 {
		t.Errorf("input_tokens = %d, want 100", span.GenAI.InputTokens)
	}
	if span.GenAI.OutputTokens != 50 {
		t.Errorf("output_tokens = %d, want 50", span.GenAI.OutputTokens)
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
	if len(got) > 70 { // 50 + "[truncated]"
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
	if generateTraceID() == generateTraceID() {
		t.Error("trace IDs should be unique")
	}
}
