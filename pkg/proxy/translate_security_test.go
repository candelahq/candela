package proxy

import (
	"testing"
)

// TestTranslateRequest_GarbageInput verifies that garbage input returns a clean error.
func TestTranslateRequest_GarbageInput(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	_, _, err := translator.TranslateRequest([]byte(`{not json at all!!!`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// TestTranslateRequest_MissingRequiredFields verifies handling of empty messages array.
func TestTranslateRequest_MissingRequiredFields(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	// Valid JSON but empty messages — should still produce a translatable request.
	body := `{"model":"claude-sonnet-4-20250514","messages":[]}`
	translated, model, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %q, want claude-sonnet-4-20250514", model)
	}
	if len(translated) == 0 {
		t.Error("expected non-empty translated body")
	}
}

// TestTranslateStreamChunk_MalformedSSE verifies that corrupt SSE data
// is handled gracefully without panics.
func TestTranslateStreamChunk_MalformedSSE(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	testCases := []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"no data prefix", "event: message_start\ngarbage line\n"},
		{"invalid json", "data: {broken json!!!}\n"},
		{"unknown event type", "data: {\"type\":\"alien_event\"}\n"},
		{"mixed valid and invalid", "data: {\"type\":\"ping\"}\ndata: NOT JSON\ndata: {\"type\":\"content_block_stop\"}\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := translator.TranslateStreamChunk([]byte(tc.data), "claude-sonnet-4-20250514")
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.name, err)
			}
			// Should not panic and should return something (even if empty).
			_ = result
		})
	}
}
