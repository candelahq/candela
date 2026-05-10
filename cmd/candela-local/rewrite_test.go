package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// U1: rewriteModelInBody must NOT corrupt user message content that contains
// the same "model":"<name>" byte sequence as the top-level field.
func TestRewriteModelInBody_MessageContentPreserved(t *testing.T) {
	// Craft a request where the user's message contains the same key-value pair
	// that rewriteModelInBody is trying to replace. Only the top-level "model"
	// field should be rewritten, not the one inside the message content.
	body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"Explain \"model\":\"gpt-4\" to me"}]}`)
	result := rewriteModelInBody(body, "gpt-4", "gpt-4-turbo")

	var parsed struct {
		Model    string `json:"model"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v\nresult: %s", err, result)
	}
	if parsed.Model != "gpt-4-turbo" {
		t.Errorf("top-level model field not updated: got %q, want %q", parsed.Model, "gpt-4-turbo")
	}
	if len(parsed.Messages) == 0 {
		t.Fatal("messages array was lost")
	}
	// The user content should be unchanged.
	want := `Explain "model":"gpt-4" to me`
	if parsed.Messages[0].Content != want {
		t.Errorf("user content was corrupted:\n  got  %q\n  want %q", parsed.Messages[0].Content, want)
	}
}

// U2: rewriteModelInBody only replaces the first occurrence (n=1).
func TestRewriteModelInBody_FirstOccurrenceOnly(t *testing.T) {
	// Artificially construct a body with two `"model":"old"` occurrences.
	// Only the first (top-level) should be replaced.
	body := []byte(`{"model":"old","extra":{"model":"old"}}`)
	result := rewriteModelInBody(body, "old", "new")

	// Count remaining occurrences of "old".
	count := bytes.Count(result, []byte(`"model":"old"`))
	if count != 1 {
		t.Errorf("expected exactly 1 remaining occurrence of the old model, got %d\nresult: %s", count, result)
	}
	// Verify first field was replaced.
	if !bytes.HasPrefix(result, []byte(`{"model":"new"`)) {
		t.Errorf("first occurrence not replaced; result: %s", result)
	}
}

// U3: rewriteModelInBody handles compact JSON (no spaces around colon).
func TestRewriteModelInBody_CompactJSONRoundTrip(t *testing.T) {
	body := []byte(`{"model":"claude-3-opus","stream":true}`)
	result := rewriteModelInBody(body, "claude-3-opus", "claude-3-5-sonnet-20241022")

	var parsed struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("model not replaced: got %q", parsed.Model)
	}
}
