package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── Test 1: stripRemoteOnlyFields ──────────────────────────────────────────
//
// Verifies the JSON-level stripping of LM Studio / Ollama-specific fields
// that strict cloud backends (Mistral, vLLM) reject with HTTP 422.

func TestStripRemoteOnlyFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, result []byte)
	}{
		{
			name:  "strips format, keep_alive, and options fields",
			input: `{"model":"llama3","messages":[{"role":"user","content":"hi"}],"format":"json","keep_alive":"5m","options":{"temperature":0.7}}`,
			check: func(t *testing.T, result []byte) {
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(result, &raw); err != nil {
					t.Fatalf("result is not valid JSON: %v", err)
				}
				for _, field := range []string{"format", "keep_alive", "options"} {
					if _, ok := raw[field]; ok {
						t.Errorf("field %q should have been stripped", field)
					}
				}
			},
		},
		{
			name:  "preserves model, messages, stream, and max_tokens",
			input: `{"model":"llama3","messages":[{"role":"user","content":"hi"}],"stream":true,"max_tokens":1024,"format":"json"}`,
			check: func(t *testing.T, result []byte) {
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(result, &raw); err != nil {
					t.Fatalf("result is not valid JSON: %v", err)
				}
				for _, field := range []string{"model", "messages", "stream", "max_tokens"} {
					if _, ok := raw[field]; !ok {
						t.Errorf("field %q should have been preserved", field)
					}
				}
				// format should still be stripped
				if _, ok := raw["format"]; ok {
					t.Error("format should have been stripped")
				}
			},
		},
		{
			name:  "strips empty per-message tool_calls but keeps non-empty",
			input: `{"model":"m","messages":[{"role":"system","content":"you are helpful","tool_calls":[]},{"role":"assistant","content":"ok","tool_calls":[{"id":"call_1","function":{"name":"get_weather"}}]}]}`,
			check: func(t *testing.T, result []byte) {
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(result, &raw); err != nil {
					t.Fatalf("result is not valid JSON: %v", err)
				}
				var msgs []map[string]json.RawMessage
				if err := json.Unmarshal(raw["messages"], &msgs); err != nil {
					t.Fatalf("failed to parse messages: %v", err)
				}
				// System message: empty tool_calls should be stripped.
				if _, ok := msgs[0]["tool_calls"]; ok {
					t.Error("empty tool_calls on system message should have been stripped")
				}
				// Assistant message: non-empty tool_calls should be preserved.
				if _, ok := msgs[1]["tool_calls"]; !ok {
					t.Error("non-empty tool_calls on assistant message should have been preserved")
				}
			},
		},
		{
			name:  "no-op on clean input returns identical JSON",
			input: `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"stream":true}`,
			check: func(t *testing.T, result []byte) {
				// Re-parse both to compare semantically (key order may differ).
				var orig, got map[string]json.RawMessage
				if err := json.Unmarshal([]byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"stream":true}`), &orig); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(result, &got); err != nil {
					t.Fatal(err)
				}
				origBytes, _ := json.Marshal(orig)
				gotBytes, _ := json.Marshal(got)
				if !bytes.Equal(origBytes, gotBytes) {
					t.Errorf("clean input should be semantically identical\n  orig: %s\n  got:  %s", origBytes, gotBytes)
				}
			},
		},
		{
			name:  "no-op on invalid JSON returns input unchanged",
			input: `{not valid json!!!`,
			check: func(t *testing.T, result []byte) {
				if string(result) != `{not valid json!!!` {
					t.Errorf("invalid JSON should be returned unchanged, got: %s", string(result))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripRemoteOnlyFields([]byte(tt.input))
			tt.check(t, result)
		})
	}
}

// ─── Test 2: Ktor SSE Hijack ────────────────────────────────────────────────
//
// Verifies the TCP hijack behavior that prevents Go's chunked transfer
// terminator ("0\r\n\r\n") from reaching ktor clients. The test handler
// mimics the remote proxy SSE path: it writes SSE headers, streams data
// chunks, then conditionally hijacks the connection for ktor User-Agents.
//
// We use raw TCP (net.Dial) for the ktor test to read raw bytes on the wire
// and verify no chunked terminator is present after [DONE].

func TestKtorSSEHijack(t *testing.T) {
	// SSE events to send.
	sseEvents := []string{
		`data: {"choices":[{"delta":{"role":"assistant"},"index":0}],"id":"x","model":"m","object":"chat.completion.chunk"}`,
		`data: {"choices":[{"delta":{"content":"Hello"},"index":0}],"id":"x","model":"m","object":"chat.completion.chunk"}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop","index":0}],"id":"x","model":"m","object":"chat.completion.chunk"}`,
		`data: [DONE]`,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		for _, event := range sseEvents {
			_, _ = fmt.Fprintf(w, "%s\n\n", event)
			flusher.Flush()
		}

		// Ktor workaround: hijack to prevent chunked terminator.
		isKtor := strings.Contains(r.UserAgent(), "ktor")
		if isKtor {
			if hj, ok := w.(http.Hijacker); ok {
				conn, buf, err := hj.Hijack()
				if err == nil {
					if buf != nil {
						_ = buf.Flush()
					}
					// Short timeout for test — don't wait 30s.
					_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
					tmp := make([]byte, 1)
					_, _ = conn.Read(tmp)
					_ = conn.Close()
				}
			}
		}
		// For non-ktor: handler returns normally, Go writes chunked terminator.
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	t.Run("ktor client sees no chunked terminator after DONE", func(t *testing.T) {
		// Use raw TCP to see every byte on the wire.
		addr := ts.Listener.Addr().String()
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer func() { _ = conn.Close() }()

		// Send a raw HTTP/1.1 request with ktor user agent.
		reqLine := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: ktor-client/2.3.7\r\nAccept: text/event-stream\r\n\r\n", addr)
		if _, err := conn.Write([]byte(reqLine)); err != nil {
			t.Fatalf("failed to write request: %v", err)
		}

		// Read entire response with a deadline.
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		raw, err := io.ReadAll(conn)
		if err != nil {
			t.Fatalf("failed to read response: %v", err)
		}

		body := string(raw)

		// Verify all SSE events arrived.
		if !strings.Contains(body, "data: [DONE]") {
			t.Errorf("expected data: [DONE] in response, got:\n%s", body)
		}
		for _, event := range sseEvents[:3] {
			if !strings.Contains(body, event) {
				t.Errorf("missing SSE event: %s", event)
			}
		}

		// The critical assertion: after [DONE], there should be no
		// chunked terminator. Find the last occurrence of [DONE] and
		// check what follows.
		doneIdx := strings.LastIndex(body, "data: [DONE]")
		if doneIdx == -1 {
			t.Fatal("data: [DONE] not found")
		}

		// After "data: [DONE]\n\n", everything remaining should be
		// whitespace or nothing — NOT a chunked terminator "0\r\n\r\n".
		afterDone := body[doneIdx+len("data: [DONE]"):]
		trimmed := strings.TrimRight(afterDone, "\r\n ")
		if strings.Contains(trimmed, "0\r\n") {
			t.Errorf("chunked terminator found after [DONE] — ktor will see unexpected EOF.\nAfter [DONE]: %q", afterDone)
		}
		// Also check there's no "0" chunk marker at all.
		if strings.TrimSpace(trimmed) == "0" {
			t.Errorf("chunked terminator '0' found after [DONE].\nAfter [DONE]: %q", afterDone)
		}
	})

	t.Run("normal client completes normally", func(t *testing.T) {
		client := ts.Client()
		req, err := http.NewRequest("GET", ts.URL+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("User-Agent", "curl/8.0")
		req.Header.Set("Accept", "text/event-stream")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		// Read all events via standard SSE line scanning.
		var events []string
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				events = append(events, line)
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner error: %v", err)
		}

		// Should have all 4 data lines.
		if len(events) != 4 {
			t.Errorf("expected 4 SSE data events, got %d: %v", len(events), events)
		}

		// Verify [DONE] is the last event.
		if len(events) > 0 && events[len(events)-1] != "data: [DONE]" {
			t.Errorf("last event should be data: [DONE], got %q", events[len(events)-1])
		}
	})
}

// ─── Test 3: sseLiteNormalizer delta.content injection ──────────────────────
//
// Verifies that the lite normalizer injects delta.content:"" when missing
// (ktor crashes on role-only chunks). This is the key JetBrains compat fix
// for remote proxy traffic.

func TestSSELiteNormalizerDeltaContent(t *testing.T) {
	t.Run("injects content into role-only delta", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSELiteNormalizer(rec)

		n.Header().Set("Content-Type", "text/event-stream")
		n.WriteHeader(http.StatusOK)

		// Claude's first chunk: delta has role but no content.
		roleOnly := `data: {"choices":[{"delta":{"role":"assistant"}}]}` + "\n\n"
		_, err := n.Write([]byte(roleOnly))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		body := rec.Body.String()

		// Must contain injected content:"".
		if !strings.Contains(body, `"content":""`) {
			t.Errorf("expected delta.content:\"\" to be injected, got: %s", body)
		}
		// Must preserve role.
		if !strings.Contains(body, `"role":"assistant"`) {
			t.Errorf("expected role to be preserved, got: %s", body)
		}
		// Must contain system_fingerprint.
		if !strings.Contains(body, `"system_fingerprint"`) {
			t.Errorf("expected system_fingerprint to be injected, got: %s", body)
		}

		// Verify the output is valid SSE (starts with "data: " and ends with "\n\n").
		if !strings.HasPrefix(body, "data: ") {
			t.Errorf("output should start with 'data: ', got: %q", body)
		}
		if !strings.HasSuffix(body, "\n\n") {
			t.Errorf("output should end with '\\n\\n', got: %q", body)
		}

		// Verify the JSON payload is valid.
		payload := strings.TrimPrefix(strings.TrimSuffix(body, "\n\n"), "data: ")
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			t.Errorf("output JSON is invalid: %v\npayload: %s", err, payload)
		}
	})

	// Regression: JetBrains' ChatCompletionResponse DTO requires
	// system_fingerprint. Without it, kotlinx.serialization throws
	// "Field 'system_fingerprint' is required" → cascades to "unexpected EOF".
	t.Run("injects system_fingerprint when missing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSELiteNormalizer(rec)
		n.Header().Set("Content-Type", "text/event-stream")
		n.WriteHeader(http.StatusOK)

		// A normal content chunk without system_fingerprint.
		chunk := `data: {"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"claude-sonnet-4.6","choices":[{"index":0,"delta":{"content":"hello"}}]}` + "\n\n"
		_, err := n.Write([]byte(chunk))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		body := rec.Body.String()

		// Must inject system_fingerprint.
		if !strings.Contains(body, `"system_fingerprint"`) {
			t.Fatalf("system_fingerprint should be injected, got: %s", body)
		}

		// Verify the output is valid JSON.
		payload := strings.TrimPrefix(strings.TrimSuffix(body, "\n\n"), "data: ")
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			t.Fatalf("output JSON is invalid: %v\npayload: %s", err, payload)
		}

		// system_fingerprint should be a string.
		var fp string
		if err := json.Unmarshal(parsed["system_fingerprint"], &fp); err != nil {
			t.Fatalf("system_fingerprint is not a string: %v", err)
		}
		if fp == "" {
			t.Error("system_fingerprint should not be empty")
		}

		// Original content should be preserved.
		if !strings.Contains(body, `"content":"hello"`) {
			t.Errorf("content should be preserved, got: %s", body)
		}
	})

	t.Run("preserves existing system_fingerprint", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSELiteNormalizer(rec)
		n.Header().Set("Content-Type", "text/event-stream")
		n.WriteHeader(http.StatusOK)

		// A chunk that already has system_fingerprint (from OpenAI).
		chunk := `data: {"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","system_fingerprint":"fp_abc123","choices":[{"index":0,"delta":{"content":"hi"}}]}` + "\n\n"
		_, err := n.Write([]byte(chunk))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		body := rec.Body.String()

		// Must preserve the original fingerprint, not overwrite it.
		if !strings.Contains(body, `"fp_abc123"`) {
			t.Errorf("original system_fingerprint should be preserved, got: %s", body)
		}
	})

	t.Run("data: [DONE] passes through unchanged", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSELiteNormalizer(rec)
		n.Header().Set("Content-Type", "text/event-stream")
		n.WriteHeader(http.StatusOK)

		done := "data: [DONE]\n\n"
		_, err := n.Write([]byte(done))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		if rec.Body.String() != done {
			t.Errorf("DONE should pass through unchanged, got: %q", rec.Body.String())
		}
	})

	// Regression: FlushRemaining must drain non-SSE data (e.g., JSON error
	// bodies that don't contain \n\n event boundaries).
	t.Run("FlushRemaining drains non-SSE data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSELiteNormalizer(rec)
		n.WriteHeader(http.StatusBadRequest)

		// A JSON error body has no \n\n, so Write() buffers it forever.
		errBody := `{"error":{"type":"invalid_request_error","message":"unknown model"}}`
		_, err := n.Write([]byte(errBody))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Before FlushRemaining, nothing should be written.
		if rec.Body.Len() != 0 {
			t.Errorf("expected empty body before FlushRemaining, got: %q", rec.Body.String())
		}

		n.FlushRemaining()

		// After FlushRemaining, the full error body should appear.
		if rec.Body.String() != errBody {
			t.Errorf("expected error body after FlushRemaining, got: %q", rec.Body.String())
		}
	})

	// Regression: the hijack guard must check writer.Header() (the
	// normalizer) to determine if the upstream returned SSE. This test
	// verifies that Content-Type set on the normalizer is visible via
	// writer.Header().Get("Content-Type") — which is what serveChat checks.
	t.Run("writer.Header reflects Content-Type for hijack guard", func(t *testing.T) {
		rec := httptest.NewRecorder()
		n := newSSELiteNormalizer(rec)

		// Simulate what httputil.ReverseProxy does: set Content-Type
		// on the writer's Header() map before calling WriteHeader.
		n.Header().Set("Content-Type", "text/event-stream")

		// The hijack guard calls: writer.Header().Get("Content-Type")
		// This must return the value, not empty string.
		ct := n.Header().Get("Content-Type")
		if ct != "text/event-stream" {
			t.Errorf("writer.Header() should return Content-Type for hijack guard, got: %q", ct)
		}

		// For a JSON error, Content-Type should be application/json
		rec2 := httptest.NewRecorder()
		n2 := newSSELiteNormalizer(rec2)
		n2.Header().Set("Content-Type", "application/json")

		ct2 := n2.Header().Get("Content-Type")
		if strings.HasPrefix(ct2, "text/event-stream") {
			t.Error("JSON error response should NOT trigger SSE hijack guard")
		}
	})
}
