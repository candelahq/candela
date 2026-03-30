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
		json.NewEncoder(w).Encode(map[string]interface{}{
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
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

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
