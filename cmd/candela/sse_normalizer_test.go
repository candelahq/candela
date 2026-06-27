package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
			name:     "normalize null content to empty string (Qwen finish)",
			input:    `data: {"choices":[{"delta":{"content":null,"role":null},"finish_reason":"stop","index":0}],"created":123,"id":"x","model":"qwen","object":"chat.completion.chunk"}` + "\n\n",
			wantNil:  false,
			wantJSON: `"content":""`,
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
