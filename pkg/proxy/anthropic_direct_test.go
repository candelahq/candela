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

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/costcalc"
)

// ====================================================================
// anthropic-direct: Native Anthropic Messages API passthrough
// ====================================================================

// TestAnthropicDirect_EndToEnd_Passthrough verifies the full request/response
// cycle through the anthropic-direct provider: no format translation, native
// Anthropic Messages API in → native out.
func TestAnthropicDirect_EndToEnd_Passthrough(t *testing.T) {
	var gotHeaders http.Header
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "msg_test123",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "Hello from Claude direct!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  25,
				"output_tokens": 12,
			},
		})
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic-direct", UpstreamURL: upstream.URL}},
		ProjectID: "test-project",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"Hello"}],"max_tokens":1024}`
	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic-direct/v1/messages",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "sk-ant-test-key-12345")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", "messages-2024-12-19")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, respBody)
	}

	// Verify response is native Anthropic format (NOT translated to OpenAI).
	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result["type"] != "message" {
		t.Errorf("response type = %v, want 'message' (native Anthropic format)", result["type"])
	}
	if _, hasChoices := result["choices"]; hasChoices {
		t.Error("response contains 'choices' — should NOT be translated to OpenAI format")
	}
	if _, hasObject := result["object"]; hasObject {
		t.Error("response contains 'object' — should NOT be translated to OpenAI format")
	}

	// Verify request was NOT translated (no anthropic_version injected, model still present).
	var upstreamReq map[string]any
	_ = json.Unmarshal(gotBody, &upstreamReq)
	if upstreamReq["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("upstream model = %v, want original model preserved", upstreamReq["model"])
	}
	if _, hasAnthropicVersion := upstreamReq["anthropic_version"]; hasAnthropicVersion {
		t.Error("upstream body contains anthropic_version — FormatTranslator should be nil")
	}

	// Verify headers were forwarded.
	if gotHeaders.Get("X-Api-Key") != "sk-ant-test-key-12345" {
		t.Errorf("upstream X-Api-Key = %q, want sk-ant-test-key-12345", gotHeaders.Get("X-Api-Key"))
	}
	if gotHeaders.Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("upstream Anthropic-Version = %q, want 2023-06-01", gotHeaders.Get("Anthropic-Version"))
	}
	if gotHeaders.Get("Anthropic-Beta") != "messages-2024-12-19" {
		t.Errorf("upstream Anthropic-Beta = %q, want messages-2024-12-19", gotHeaders.Get("Anthropic-Beta"))
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
	if span.GenAI.Provider != "anthropic-direct" {
		t.Errorf("provider = %s, want anthropic-direct", span.GenAI.Provider)
	}
	if span.GenAI.Model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %s, want claude-sonnet-4-20250514", span.GenAI.Model)
	}
	if span.GenAI.InputTokens != 25 {
		t.Errorf("input_tokens = %d, want 25", span.GenAI.InputTokens)
	}
	if span.GenAI.OutputTokens != 12 {
		t.Errorf("output_tokens = %d, want 12", span.GenAI.OutputTokens)
	}
	if span.ProjectID != "test-project" {
		t.Errorf("project_id = %s, want test-project", span.ProjectID)
	}
}

// TestAnthropicDirect_NoADCInjection verifies that anthropic-direct does NOT
// inject ADC tokens — the client's own API key must reach upstream unchanged.
func TestAnthropicDirect_NoADCInjection(t *testing.T) {
	var gotAuth string
	var gotApiKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotApiKey = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	// No TokenSource — that's the point.
	p := New(Config{
		Providers: []Provider{{Name: "anthropic-direct", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic-direct/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "sk-ant-my-real-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	// X-Api-Key must be forwarded as-is.
	if gotApiKey != "sk-ant-my-real-key" {
		t.Errorf("upstream X-Api-Key = %q, want 'sk-ant-my-real-key'", gotApiKey)
	}
	// Authorization should NOT be replaced with an ADC token.
	if strings.Contains(gotAuth, "gcp") || strings.Contains(gotAuth, "adc") {
		t.Errorf("upstream Authorization = %q, should not contain ADC token", gotAuth)
	}
}

// TestAnthropicDirect_StreamingPassthrough verifies native Anthropic SSE streaming
// passes through without translation.
func TestAnthropicDirect_StreamingPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher := w.(http.Flusher)

		events := []string{
			`data: {"type":"message_start","message":{"id":"msg_1","role":"assistant","content":[],"usage":{"input_tokens":10,"output_tokens":0}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
			`data: {"type":"message_stop"}`,
		}
		for _, e := range events {
			_, _ = fmt.Fprintf(w, "%s\n\n", e)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic-direct", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic-direct/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "sk-ant-test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	// Read SSE events — should be native Anthropic format, not OpenAI.
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

	// Verify Anthropic-native events are passed through (NOT translated to OpenAI chunks).
	foundMessageStart := false
	foundContentDelta := false
	foundMessageStop := false
	for _, event := range events {
		if strings.Contains(event, `"message_start"`) {
			foundMessageStart = true
		}
		if strings.Contains(event, `"content_block_delta"`) {
			foundContentDelta = true
		}
		if strings.Contains(event, `"message_stop"`) {
			foundMessageStop = true
		}
		// These would indicate unwanted translation to OpenAI format.
		if strings.Contains(event, `"chat.completion.chunk"`) {
			t.Error("stream contains OpenAI format chunks — should be native Anthropic")
		}
	}

	if !foundMessageStart {
		t.Error("missing message_start event")
	}
	if !foundContentDelta {
		t.Error("missing content_block_delta event")
	}
	if !foundMessageStop {
		t.Error("missing message_stop event")
	}

	// Wait for span.
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
	if spans[0].GenAI.Provider != "anthropic-direct" {
		t.Errorf("provider = %q, want anthropic-direct", spans[0].GenAI.Provider)
	}
}

// TestAnthropicDirect_UserAttribution verifies per-user cost attribution
// works for anthropic-direct (same as other providers).
func TestAnthropicDirect_UserAttribution(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic-direct", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := &auth.User{ID: "uid-pencil", Email: "dev@pencil.dev"}
		mux.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), user)))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic-direct/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "sk-ant-test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

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
	if spans[0].UserID != "dev@pencil.dev" {
		t.Errorf("UserID = %q, want 'dev@pencil.dev'", spans[0].UserID)
	}
}

// TestAnthropicDirect_InDefaultProviders verifies the provider is registered.
func TestAnthropicDirect_InDefaultProviders(t *testing.T) {
	providers := DefaultProviders()
	found := false
	for _, p := range providers {
		if p.Name == "anthropic-direct" {
			found = true
			if p.UpstreamURL != "https://api.anthropic.com" {
				t.Errorf("UpstreamURL = %q, want https://api.anthropic.com", p.UpstreamURL)
			}
			if p.FormatTranslator != nil {
				t.Error("FormatTranslator should be nil for passthrough")
			}
			if p.PathRewriter != nil {
				t.Error("PathRewriter should be nil for passthrough")
			}
			if p.TokenSource != nil {
				t.Error("TokenSource should be nil (no ADC)")
			}
			break
		}
	}
	if !found {
		t.Error("anthropic-direct not found in DefaultProviders()")
	}
}

// TestAnthropicDirect_ParserRegistered verifies the parser is wired up.
func TestAnthropicDirect_ParserRegistered(t *testing.T) {
	parser := getParser("anthropic-direct")
	if parser == nil {
		t.Fatal("no parser for anthropic-direct")
	}

	// Should parse Anthropic-format requests.
	model, content := parser.ParseRequest([]byte(`{"model":"claude-opus-4-20250514","messages":[{"role":"user","content":"test"}]}`))
	if model != "claude-opus-4-20250514" {
		t.Errorf("model = %q, want claude-opus-4-20250514", model)
	}
	if content == "" {
		t.Error("expected non-empty content from ParseRequest")
	}

	// Should parse Anthropic-format responses.
	_, input, output := parser.ParseResponse([]byte(`{"content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":5,"output_tokens":3}}`))
	if input != 5 {
		t.Errorf("input = %d, want 5", input)
	}
	if output != 3 {
		t.Errorf("output = %d, want 3", output)
	}
}

// TestAnthropicDirect_RequestInfo verifies extractRequestInfo works.
func TestAnthropicDirect_RequestInfo(t *testing.T) {
	body := `{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"test"}]}`
	model, content := extractRequestInfo("anthropic-direct", []byte(body))
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %q", model)
	}
	if content == "" {
		t.Error("expected content")
	}
}

// TestAnthropicDirect_IsStreaming verifies streaming detection.
func TestAnthropicDirect_IsStreaming(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"stream true", `{"stream":true,"model":"claude-sonnet-4-20250514"}`, true},
		{"stream false", `{"stream":false,"model":"claude-sonnet-4-20250514"}`, false},
		{"no stream field", `{"model":"claude-sonnet-4-20250514"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStreamingRequest("anthropic-direct", []byte(tt.body))
			if got != tt.want {
				t.Errorf("isStreaming = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestForwardHeaders_AnthropicDirect verifies all required Claude Code headers
// are forwarded to upstream.
func TestForwardHeaders_AnthropicDirect(t *testing.T) {
	src, _ := http.NewRequest("POST", "http://localhost/proxy/anthropic-direct/v1/messages", nil)
	src.Header.Set("Authorization", "Bearer sk-ant-test")
	src.Header.Set("X-Api-Key", "sk-ant-key-123")
	src.Header.Set("Anthropic-Version", "2023-06-01")
	src.Header.Set("Anthropic-Beta", "messages-2024-12-19")
	src.Header.Set("Content-Type", "application/json")

	dst, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	forwardHeaders(src, dst, "anthropic-direct")

	checks := map[string]string{
		"Authorization":     "Bearer sk-ant-test",
		"X-Api-Key":         "sk-ant-key-123",
		"Anthropic-Version": "2023-06-01",
		"Anthropic-Beta":    "messages-2024-12-19",
		"Content-Type":      "application/json",
	}
	for header, want := range checks {
		if got := dst.Header.Get(header); got != want {
			t.Errorf("dst %s = %q, want %q", header, got, want)
		}
	}
}

// TestForwardHeaders_Anthropic_IncludesBeta verifies the fix: the existing
// anthropic provider now also forwards Anthropic-Beta (was missing before).
func TestForwardHeaders_Anthropic_IncludesBeta(t *testing.T) {
	src, _ := http.NewRequest("POST", "http://localhost/proxy/anthropic/v1/messages", nil)
	src.Header.Set("Anthropic-Beta", "messages-2024-12-19")

	dst, _ := http.NewRequest("POST", "https://vertex.ai/v1/messages", nil)
	forwardHeaders(src, dst, "anthropic")

	if got := dst.Header.Get("Anthropic-Beta"); got != "messages-2024-12-19" {
		t.Errorf("anthropic provider: Anthropic-Beta = %q, want 'messages-2024-12-19'", got)
	}
}

// TestAnthropicDirect_CountTokens_NoSpan verifies that count_tokens requests
// are proxied transparently but do NOT create an observability span (CRIT-17).
func TestAnthropicDirect_CountTokens_NoSpan(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"input_tokens":42}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic-direct", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic-direct/v1/messages/count_tokens",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hello world"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "sk-ant-test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !upstreamCalled {
		t.Fatal("upstream was not called — proxy should forward count_tokens")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}

	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["input_tokens"] != float64(42) {
		t.Errorf("input_tokens = %v, want 42", result["input_tokens"])
	}

	time.Sleep(100 * time.Millisecond)

	spans := submitter.getSpans()
	if len(spans) > 0 {
		t.Errorf("expected no span for count_tokens, got %d", len(spans))
	}
}
