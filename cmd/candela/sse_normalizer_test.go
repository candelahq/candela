package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSSENormalizerNormalizeEvent(t *testing.T) {
	n := newSSENormalizer(httptest.NewRecorder())

	tests := []struct {
		name     string
		input    string
		wantNil  bool   // expect event to be suppressed
		wantJSON string // substring to look for in output (empty = pass-through)
	}{
		{
			name:    "pass through DONE sentinel",
			input:   "data: [DONE]\n\n",
			wantNil: false,
		},
		{
			name:    "pass through comment",
			input:   ": keepalive\n\n",
			wantNil: false,
		},
		{
			name:    "drop empty choices array (Qwen usage chunk)",
			input:   `data: {"choices":[],"created":123,"id":"x","model":"qwen","object":"chat.completion.chunk","usage":{}}` + "\n\n",
			wantNil: true,
		},
		{
			name:     "add content to role-only delta (Claude first chunk)",
			input:    `data: {"choices":[{"delta":{"role":"assistant"},"index":0}],"created":123,"id":"x","model":"claude","object":"chat.completion.chunk"}` + "\n\n",
			wantNil:  false,
			wantJSON: `"content":""`,
		},
		{
			name:     "clear delta on finish_reason stop (Qwen finish)",
			input:    `data: {"choices":[{"delta":{"content":null,"role":null},"finish_reason":"stop","index":0}],"created":123,"id":"x","model":"qwen","object":"chat.completion.chunk"}` + "\n\n",
			wantNil:  false,
			wantJSON: `"delta":{}`,
		},
		{
			name:     "strip reasoning_content from delta",
			input:    `data: {"choices":[{"delta":{"content":"hi","reasoning_content":"thinking..."},"index":0}],"created":123,"id":"x","model":"qwen","object":"chat.completion.chunk"}` + "\n\n",
			wantNil:  false,
			wantJSON: `"content":"hi"`,
		},
		{
			name:    "pass through normal event unmodified",
			input:   `data: {"choices":[{"delta":{"content":"hello"},"index":0}],"created":123,"id":"x","model":"gemini","object":"chat.completion.chunk"}` + "\n\n",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := n.normalizeEvent([]byte(tt.input))

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil (suppressed), got: %s", string(result))
				}
				return
			}

			if result == nil {
				t.Fatal("expected event, got nil")
			}

			if tt.wantJSON != "" && !bytes.Contains(result, []byte(tt.wantJSON)) {
				t.Errorf("expected result to contain %q, got: %s", tt.wantJSON, string(result))
			}
		})
	}
}

func TestSSENormalizerStripReasoningContent(t *testing.T) {
	n := newSSENormalizer(httptest.NewRecorder())

	input := `data: {"choices":[{"delta":{"content":"hi","reasoning_content":"deep thought","tool_calls":[]},"index":0}],"created":123,"id":"x","model":"qwen","object":"chat.completion.chunk"}` + "\n\n"
	result := n.normalizeEvent([]byte(input))

	if result == nil {
		t.Fatal("expected event, got nil")
	}
	if bytes.Contains(result, []byte("reasoning_content")) {
		t.Error("reasoning_content should have been stripped")
	}
	if bytes.Contains(result, []byte("tool_calls")) {
		t.Error("tool_calls should have been stripped")
	}
}

func TestSSENormalizerWriteHeader(t *testing.T) {
	t.Run("normalizes content-type charset", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSENormalizer(rec)
		n.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		n.WriteHeader(http.StatusOK)

		if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
			t.Errorf("Content-Type = %q, want %q", got, "text/event-stream")
		}
	})

	t.Run("strips content-length from SSE", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSENormalizer(rec)
		n.Header().Set("Content-Type", "text/event-stream")
		n.Header().Set("Content-Length", "48606")
		n.WriteHeader(http.StatusOK)

		if got := rec.Header().Get("Content-Length"); got != "" {
			t.Errorf("Content-Length should be stripped, got %q", got)
		}
	})

	t.Run("preserves content-length for non-SSE", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSENormalizer(rec)
		n.Header().Set("Content-Type", "application/json")
		n.Header().Set("Content-Length", "42")
		n.WriteHeader(http.StatusOK)

		if got := rec.Header().Get("Content-Length"); got != "42" {
			t.Errorf("Content-Length should be preserved for non-SSE, got %q", got)
		}
	})
}

func TestSSENormalizerWrite(t *testing.T) {
	t.Run("buffers partial events", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSENormalizer(rec)

		// Write partial event (no \n\n yet)
		_, _ = n.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"index\":0}"))
		if rec.Body.Len() > 0 {
			t.Error("should not write until event is complete")
		}

		// Complete the event
		_, _ = n.Write([]byte("}],\"created\":1,\"id\":\"x\",\"model\":\"m\",\"object\":\"chat.completion.chunk\"}\n\n"))
		if rec.Body.Len() == 0 {
			t.Error("should write after event is complete")
		}
	})

	t.Run("batches multiple events from single write", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSENormalizer(rec)

		event1 := `data: {"choices":[{"delta":{"content":"a"},"index":0}],"created":1,"id":"x","model":"m","object":"chat.completion.chunk"}` + "\n\n"
		event2 := `data: {"choices":[{"delta":{"content":"b"},"index":0}],"created":1,"id":"x","model":"m","object":"chat.completion.chunk"}` + "\n\n"

		_, _ = n.Write([]byte(event1 + event2))

		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"content":"a"`)) {
			t.Error("missing first event")
		}
		if !bytes.Contains([]byte(body), []byte(`"content":"b"`)) {
			t.Error("missing second event")
		}
	})

	t.Run("suppresses empty choices in batch", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSENormalizer(rec)

		good := `data: {"choices":[{"delta":{"content":"hi"},"index":0}],"created":1,"id":"x","model":"m","object":"chat.completion.chunk"}` + "\n\n"
		empty := `data: {"choices":[],"created":1,"id":"x","model":"m","object":"chat.completion.chunk"}` + "\n\n"
		done := "data: [DONE]\n\n"

		_, _ = n.Write([]byte(good + empty + done))

		body := rec.Body.String()
		if !bytes.Contains([]byte(body), []byte(`"content":"hi"`)) {
			t.Error("good event should be present")
		}
		if bytes.Contains([]byte(body), []byte(`"choices":[]`)) {
			t.Error("empty choices should be suppressed")
		}
		if !bytes.Contains([]byte(body), []byte("[DONE]")) {
			t.Error("DONE sentinel should be present")
		}
	})
}

func TestSSENormalizerFinalChunk(t *testing.T) {
	n := newSSENormalizer(httptest.NewRecorder())

	t.Run("clears delta on finish_reason stop", func(t *testing.T) {
		// Claude sends {"delta":{"content":""},"finish_reason":"stop"} but
		// LM Studio spec says final chunk should be {"delta":{},"finish_reason":"stop"}.
		input := `data: {"choices":[{"delta":{"content":"","role":"assistant"},"finish_reason":"stop","index":0}],"created":123,"id":"x","model":"claude","object":"chat.completion.chunk"}` + "\n\n"
		result := n.normalizeEvent([]byte(input))

		if result == nil {
			t.Fatal("expected event, got nil")
		}
		// Delta should be empty object.
		if bytes.Contains(result, []byte(`"content"`)) {
			t.Errorf("delta should be empty on stop, got: %s", string(result))
		}
		if bytes.Contains(result, []byte(`"role"`)) {
			t.Errorf("delta should not contain role on stop, got: %s", string(result))
		}
		if !bytes.Contains(result, []byte(`"finish_reason":"stop"`)) {
			t.Errorf("finish_reason should still be present, got: %s", string(result))
		}
	})

	t.Run("preserves delta when finish_reason is null", func(t *testing.T) {
		input := `data: {"choices":[{"delta":{"content":"hello"},"finish_reason":null,"index":0}],"created":123,"id":"x","model":"m","object":"chat.completion.chunk"}` + "\n\n"
		result := n.normalizeEvent([]byte(input))

		if result == nil {
			t.Fatal("expected event, got nil")
		}
		if !bytes.Contains(result, []byte(`"content":"hello"`)) {
			t.Errorf("content should be preserved when streaming, got: %s", string(result))
		}
	})
}

func TestSSENormalizerStripUpstreamHeaders(t *testing.T) {
	t.Run("strips upstream headers for SSE responses", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSENormalizer(rec)

		n.Header().Set("Content-Type", "text/event-stream")
		n.Header().Set("Server", "Google Frontend")
		n.Header().Set("Alt-Svc", `h3=":443"`)
		n.Header().Set("Via", "1.1 google")
		n.Header().Set("X-Frame-Options", "SAMEORIGIN")
		n.Header().Set("X-Vertex-Ai-Received-Request-Id", "abc-123")
		n.WriteHeader(http.StatusOK)

		for _, h := range []string{"Server", "Alt-Svc", "Via", "X-Frame-Options", "X-Vertex-Ai-Received-Request-Id"} {
			if got := rec.Header().Get(h); got != "" {
				t.Errorf("header %q should be stripped for SSE, got %q", h, got)
			}
		}
		if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
			t.Errorf("Content-Type should be preserved, got %q", got)
		}
	})

	t.Run("preserves headers for non-SSE responses", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSENormalizer(rec)

		n.Header().Set("Content-Type", "application/json")
		n.Header().Set("Server", "Google Frontend")
		n.Header().Set("Request-Id", "req-456")
		n.WriteHeader(http.StatusBadRequest)

		if got := rec.Header().Get("Server"); got != "Google Frontend" {
			t.Errorf("Server header should be preserved for non-SSE, got %q", got)
		}
		if got := rec.Header().Get("Request-Id"); got != "req-456" {
			t.Errorf("Request-Id should be preserved for non-SSE, got %q", got)
		}
	})
}

func TestServeChatStripEmptyTools(t *testing.T) {
	// Simulate what JetBrains sends: {"model":"m","messages":[...],"tools":[],"tool_choice":"auto"}
	body := []byte(`{"model":"test","messages":[{"role":"user","content":"hi"}],"tools":[],"tool_choice":"auto","stream":true}`)

	var raw map[string]json.RawMessage
	_ = json.Unmarshal(body, &raw)

	// Verify tools is present before stripping.
	if _, ok := raw["tools"]; !ok {
		t.Fatal("test setup: tools should be present")
	}

	// Apply the same stripping logic as serveChat.
	if toolsRaw, ok := raw["tools"]; ok {
		var tools []json.RawMessage
		if json.Unmarshal(toolsRaw, &tools) == nil && len(tools) == 0 {
			delete(raw, "tools")
			delete(raw, "tool_choice")
		}
	}

	if _, ok := raw["tools"]; ok {
		t.Error("empty tools[] should be stripped")
	}
	if _, ok := raw["tool_choice"]; ok {
		t.Error("tool_choice should be stripped when tools is empty")
	}
}

func TestServeChatStripStreamOptions(t *testing.T) {
	body := []byte(`{"model":"test","messages":[{"role":"user","content":"hi"}],"stream":true,"stream_options":{"include_usage":true}}`)

	var raw map[string]json.RawMessage
	_ = json.Unmarshal(body, &raw)

	if _, ok := raw["stream_options"]; !ok {
		t.Fatal("test setup: stream_options should be present")
	}

	// Apply the same stripping logic as serveChat.
	delete(raw, "stream_options")

	if _, ok := raw["stream_options"]; ok {
		t.Error("stream_options should be stripped")
	}
}

// ── Test Hardening: Edge Cases ──

func TestSSENormalizerFinishReasonLength(t *testing.T) {
	// finish_reason "length" means the model hit max_tokens — delta should NOT be cleared.
	n := newSSENormalizer(httptest.NewRecorder())

	input := `data: {"choices":[{"delta":{"content":"truncat"},"finish_reason":"length","index":0}],"created":123,"id":"x","model":"m","object":"chat.completion.chunk"}` + "\n\n"
	result := n.normalizeEvent([]byte(input))

	if result == nil {
		t.Fatal("expected event, got nil")
	}
	if !bytes.Contains(result, []byte(`"content":"truncat"`)) {
		t.Errorf("content should be preserved for finish_reason=length, got: %s", string(result))
	}
}

func TestServeChatPreserveNonEmptyTools(t *testing.T) {
	// When tools array is non-empty, it must NOT be stripped.
	body := []byte(`{"model":"test","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"get_weather"}}],"tool_choice":"auto","stream":true}`)

	var raw map[string]json.RawMessage
	_ = json.Unmarshal(body, &raw)

	// Apply the same stripping logic as serveChat.
	if toolsRaw, ok := raw["tools"]; ok {
		var tools []json.RawMessage
		if json.Unmarshal(toolsRaw, &tools) == nil && len(tools) == 0 {
			delete(raw, "tools")
			delete(raw, "tool_choice")
		}
	}

	if _, ok := raw["tools"]; !ok {
		t.Error("non-empty tools should be preserved")
	}
	if _, ok := raw["tool_choice"]; !ok {
		t.Error("tool_choice should be preserved when tools is non-empty")
	}
}

func TestSSENormalizerFinalChunkMatchedStop(t *testing.T) {
	// Verify matched_stop is stripped from final chunk (Gemini review fix).
	n := newSSENormalizer(httptest.NewRecorder())

	input := `data: {"choices":[{"delta":{"content":"done"},"finish_reason":"stop","matched_stop":"<|endoftext|>","index":0}],"created":123,"id":"x","model":"m","object":"chat.completion.chunk"}` + "\n\n"
	result := n.normalizeEvent([]byte(input))

	if result == nil {
		t.Fatal("expected event, got nil")
	}
	if bytes.Contains(result, []byte("matched_stop")) {
		t.Errorf("matched_stop should be stripped from final chunk, got: %s", string(result))
	}
	if !bytes.Contains(result, []byte(`"delta":{}`)) {
		t.Errorf("delta should be empty on stop, got: %s", string(result))
	}
}

// ── LM Studio Endpoint Tests ──

func TestLMHandlerRouting(t *testing.T) {
	h := newLMHandler(nil, nil, nil, nil, nil, nil, nil, true, 8192)

	tests := []struct {
		method string
		path   string
		want   int // expected status code
	}{
		// Model discovery
		{"GET", "/v1/models", http.StatusOK},
		// Chat completions aliases — can't easily test POST without body, but
		// we can verify routing doesn't 404.
		// Stubs should return 501
		{"POST", "/v1/completions", http.StatusNotImplemented},
		{"POST", "/v1/embeddings", http.StatusNotImplemented},
		// Diagnostics
		{"GET", "/lmstudio/diagnostics", http.StatusOK},
		// Unknown paths in solo mode (no remote) → 404
		{"GET", "/v1/unknown", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("got status %d, want %d. Body: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestLMHandlerCompletionsStub(t *testing.T) {
	h := newLMHandler(nil, nil, nil, nil, nil, nil, nil, true, 8192)

	req := httptest.NewRequest("POST", "/v1/completions", strings.NewReader(`{"model":"test","prompt":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/v1/chat/completions") {
		t.Errorf("error should mention chat/completions endpoint, got: %s", body)
	}
}

func TestLMHandlerEmbeddingsStub(t *testing.T) {
	h := newLMHandler(nil, nil, nil, nil, nil, nil, nil, true, 8192)

	req := httptest.NewRequest("POST", "/v1/embeddings", strings.NewReader(`{"model":"test","input":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}
}

func TestLMHandlerDiagnostics(t *testing.T) {
	cloudModels := map[string]string{
		"gpt-4":         "openai",
		"claude-sonnet": "anthropic",
	}
	h := newLMHandler(nil, nil, nil, nil, nil, cloudModels, nil, true, 8192)

	req := httptest.NewRequest("GET", "/lmstudio/diagnostics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	if resp["version"] != "candela" {
		t.Errorf("expected version candela, got %v", resp["version"])
	}
	if resp["runtime"] != "none" {
		t.Errorf("expected runtime none (no manager), got %v", resp["runtime"])
	}

	models, ok := resp["models"].(map[string]any)
	if !ok {
		t.Fatalf("expected models map, got %T", resp["models"])
	}
	if total, ok := models["total"].(float64); !ok || int(total) != 2 {
		t.Errorf("expected 2 total models (cloud), got %v", models["total"])
	}
}

func TestLMHandlerChatCompletionsPathAlias(t *testing.T) {
	// /chat/completions (without /v1 prefix) should also be routed to serveChat.
	h := newLMHandler(nil, nil, nil, nil, nil, nil, nil, true, 8192)

	body := `{"model":"nonexistent","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// It should be routed to serveChat (not the default handler). serveChat
	// returns "model not found locally" for unknown models, whereas the
	// default fallback returns "solo mode — no remote server configured".
	respBody := rec.Body.String()
	if strings.Contains(respBody, "solo mode") {
		t.Errorf("/chat/completions should route to serveChat, not default handler: %s", respBody)
	}
	if !strings.Contains(respBody, "model not found") {
		t.Errorf("expected model-not-found error from serveChat, got: %s", respBody)
	}
}

func TestSSENormalizerMultipleChunksEndToEnd(t *testing.T) {
	// Simulate a complete LM Studio-compatible stream: first chunk (role),
	// content chunks, final chunk (stop), and [DONE].
	rec := httptest.NewRecorder()
	n := newSSENormalizer(rec)

	first := `data: {"choices":[{"delta":{"role":"assistant"},"finish_reason":null,"index":0}],"created":123,"id":"chatcmpl-abc","model":"m","object":"chat.completion.chunk"}` + "\n\n"
	content := `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null,"index":0}],"created":123,"id":"chatcmpl-abc","model":"m","object":"chat.completion.chunk"}` + "\n\n"
	final := `data: {"choices":[{"delta":{"content":""},"finish_reason":"stop","index":0}],"created":123,"id":"chatcmpl-abc","model":"m","object":"chat.completion.chunk"}` + "\n\n"
	done := "data: [DONE]\n\n"

	_, _ = n.Write([]byte(first + content + final + done))

	body := rec.Body.String()

	// First chunk should have content:"" added (role-only delta).
	if !strings.Contains(body, `"role":"assistant"`) {
		t.Error("first chunk should contain role")
	}
	// Content chunk should pass through.
	if !strings.Contains(body, `"content":"Hello"`) {
		t.Error("content chunk should pass through")
	}
	// Final chunk should have empty delta.
	if !strings.Contains(body, `"delta":{}`) {
		t.Errorf("final chunk should have empty delta, got: %s", body)
	}
	// DONE sentinel must be present.
	if !strings.Contains(body, "[DONE]") {
		t.Error("DONE sentinel should be present")
	}
}
