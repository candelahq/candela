package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAnthropicTranslatedStreamUsage_EndToEnd drives a streaming request
// through the proxy with the Anthropic translator. The upstream flushes the
// SSE body in small pieces so lines are split across reads.
func TestAnthropicTranslatedStreamUsage_EndToEnd(t *testing.T) {
	sse := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_e2e","role":"assistant","content":[],"usage":{"input_tokens":15,"output_tokens":0,"cache_read_input_tokens":12,"cache_creation_input_tokens":3}}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		const piece = 7
		for i := 0; i < len(sse); i += piece {
			_, _ = io.WriteString(w, sse[i:min(i+piece, len(sse))])
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	p, err := New(Config{
		Providers: []Provider{{
			Name:             "anthropic",
			UpstreamURL:      upstream.URL,
			FormatTranslator: &AnthropicFormatTranslator{},
		}},
		ProjectID: "test-project",
	}, &mockSubmitter{}, newCalcWithTestModels())
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"claude-sonnet-4-20250514","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(srv.URL+"/proxy/anthropic/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body: %s", resp.StatusCode, out)
	}

	var content string
	var usage *openAIUsage
	for _, line := range strings.Split(string(out), "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || payload == "[DONE]" {
			continue
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			t.Fatalf("invalid chunk %q: %v", payload, err)
		}
		for _, c := range chunk.Choices {
			content += c.Delta.Content
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	if content != "Hello" {
		t.Errorf("content = %q, want Hello", content)
	}
	assertUsage(t, usage, 30, 8, 38, int64Ptr(12))
}
