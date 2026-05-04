package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/candelahq/candela/pkg/costcalc"
)

// ====================================================================
// Mock TokenSource for ADC testing
// ====================================================================

type mockTokenSource struct {
	token string
	err   error
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &oauth2.Token{AccessToken: m.token}, nil
}

// ====================================================================
// ADC Token Injection
// ====================================================================

func TestADCTokenInjection(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{
			Name:        "anthropic",
			UpstreamURL: upstream.URL,
			TokenSource: &mockTokenSource{token: "gcp-adc-token-xyz"},
		}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer user-placeholder-key") // should be replaced

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	// The upstream should have received the ADC token, NOT the user's placeholder.
	if gotAuth != "Bearer gcp-adc-token-xyz" {
		t.Errorf("upstream auth = %q, want 'Bearer gcp-adc-token-xyz'", gotAuth)
	}
}

func TestADCTokenFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should not be called when ADC fails")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{
			Name:        "anthropic",
			UpstreamURL: upstream.URL,
			TokenSource: &mockTokenSource{err: fmt.Errorf("ADC: no credentials found")},
		}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "GCP credentials") {
		t.Errorf("error body = %q, should mention GCP credentials", body)
	}
}

// ====================================================================
// Error Path Tests: Upstream Errors Surfaced to Client
// ====================================================================

func TestUpstream429_SurfacedToClient(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":{"type":"rate_limit_error","message":"You have exceeded your rate limit."}}`)
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

	// Upstream 429 should be forwarded to the client as-is.
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", resp.StatusCode)
	}

	// Retry-After header should be forwarded.
	if resp.Header.Get("Retry-After") != "30" {
		t.Errorf("Retry-After = %q, want '30'", resp.Header.Get("Retry-After"))
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "rate_limit_error") {
		t.Errorf("body = %q, should contain upstream error", body)
	}
}

func TestUpstream500_SurfacedToClient(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":{"type":"server_error","message":"Internal server error"}}`)
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

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "server_error") {
		t.Errorf("body = %q, should contain upstream error", body)
	}

	// Wait for async span creation.
	time.Sleep(100 * time.Millisecond)

	// Error response should still create a span (for observability).
	spans := submitter.getSpans()
	if len(spans) == 0 {
		t.Fatal("expected span even for error responses")
	}
}

func TestUpstreamUnreachable_502(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := costcalc.New()

	// Point to a guaranteed-closed port.
	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: "http://127.0.0.1:1"}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
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

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "upstream provider unavailable") {
		t.Errorf("body = %q, should mention upstream provider unavailable", body)
	}
}

func TestUpstream500_TracksErrorInSpan(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":{"message":"overloaded"}}`)
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

	// Wait for async span.
	for i := 0; i < 50; i++ {
		if len(submitter.getSpans()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	spans := submitter.getSpans()
	if len(spans) == 0 {
		t.Fatal("expected error span")
	}

	span := spans[0]
	if span.Status != 2 { // SpanStatusError
		t.Errorf("span status = %d, want SpanStatusError (2)", span.Status)
	}
	if span.GenAI == nil {
		t.Fatal("expected GenAI attributes even on error")
	}
	if span.GenAI.Provider != "anthropic" {
		t.Errorf("provider = %q, want 'anthropic'", span.GenAI.Provider)
	}
}

// ====================================================================
// Full SSE Streaming Pipeline (end-to-end with translation)
// ====================================================================

func TestStreamingSSE_EndToEnd_WithTranslation(t *testing.T) {
	// Simulate Anthropic Vertex AI SSE stream.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request was translated to Anthropic format.
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("failed to parse translated request: %v", err)
		}
		if req["anthropic_version"] == nil {
			t.Error("expected anthropic_version in translated request")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		flusher := w.(http.Flusher)

		// Send Anthropic-format SSE chunks.
		chunks := []string{
			`data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-20250514","usage":{"input_tokens":10,"output_tokens":0}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			`data: {"type":"message_stop"}`,
		}

		for _, chunk := range chunks {
			_, _ = fmt.Fprintf(w, "%s\n\n", chunk)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{
			Name:             "anthropic",
			UpstreamURL:      upstream.URL,
			FormatTranslator: &AnthropicFormatTranslator{},
		}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Send OpenAI-format request (will be translated to Anthropic format).
	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/chat/completions",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	// Read all SSE events from the response (should be OpenAI format now).
	var events []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, line)
		}
	}

	if len(events) == 0 {
		t.Fatal("expected SSE events in response")
	}

	// Check that content deltas were translated to OpenAI format.
	foundContent := false
	foundDone := false
	for _, event := range events {
		if strings.Contains(event, `"content":"Hello"`) || strings.Contains(event, `"content":" world"`) {
			foundContent = true
		}
		if strings.Contains(event, "[DONE]") {
			foundDone = true
		}
	}

	if !foundContent {
		t.Errorf("expected translated content deltas in SSE stream, got events: %v", events)
	}
	if !foundDone {
		t.Errorf("expected [DONE] terminator in SSE stream, got events: %v", events)
	}
}

func TestStreamingSSE_Passthrough_NoTranslation(t *testing.T) {
	// OpenAI provider — no translation, just passthrough.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		flusher := w.(http.Flusher)

		chunks := []string{
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}

		for _, chunk := range chunks {
			_, _ = fmt.Fprintf(w, "%s\n\n", chunk)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	// Read all SSE events — should be forwarded unchanged.
	var events []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, line)
		}
	}

	// Should contain the original OpenAI format chunks.
	if len(events) != 3 {
		t.Errorf("expected 3 SSE events, got %d: %v", len(events), events)
	}

	foundDone := false
	for _, event := range events {
		if strings.Contains(event, "[DONE]") {
			foundDone = true
		}
	}
	if !foundDone {
		t.Error("expected [DONE] in passthrough stream")
	}
}

// ====================================================================
// Translation Error Handling
// ====================================================================

func TestTranslateRequest_MalformedJSON(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	_, _, err := translator.TranslateRequest([]byte(`{not valid json`))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestTranslateResponse_MalformedJSON(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	_, err := translator.TranslateResponse([]byte(`{broken`), "claude-sonnet-4")
	if err == nil {
		t.Error("expected error for malformed response JSON")
	}
}

func TestTranslateRequest_MissingModel(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	body := `{"messages":[{"role":"user","content":"hi"}]}`
	translated, model, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still translate, just with empty model.
	if model != "" {
		t.Errorf("model = %q, want empty", model)
	}
	if len(translated) == 0 {
		t.Error("expected non-empty translated body")
	}
}
