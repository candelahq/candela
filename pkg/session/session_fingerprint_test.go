package session

import (
	"encoding/json"
	"testing"
	"time"
)

// TestFingerprint_DifferentModels verifies that the same messages sent to
// different models produce different session IDs.
func TestFingerprint_DifferentModels(t *testing.T) {
	resolver := NewUserMsgResolver(30 * time.Minute)

	msgs, _ := json.Marshal([]map[string]string{
		{"role": "user", "content": "What is the meaning of life?"},
	})

	info1 := SessionInfo{
		UserID:   "user1",
		Model:    "gpt-4o",
		Messages: msgs,
	}
	info2 := SessionInfo{
		UserID:   "user1",
		Model:    "claude-sonnet-4-20250514",
		Messages: msgs,
	}

	session1 := resolver.Resolve(info1)
	session2 := resolver.Resolve(info2)

	if session1 == session2 {
		t.Errorf("same messages to different models should produce different sessions: %s == %s",
			session1, session2)
	}
}

// TestFingerprint_MultiModalContent verifies that multi-modal content
// (array of objects) produces a stable fingerprint.
func TestFingerprint_MultiModalContent(t *testing.T) {
	resolver := NewUserMsgResolver(30 * time.Minute)

	// Multi-modal content per OpenAI API spec.
	msgs := json.RawMessage(`[
		{
			"role": "user",
			"content": [
				{"type": "text", "text": "What is in this image?"},
				{"type": "image_url", "image_url": {"url": "https://example.com/photo.jpg"}}
			]
		}
	]`)

	info := SessionInfo{
		UserID:   "user-multimodal",
		Model:    "gpt-4o",
		Messages: msgs,
	}

	// Call twice — should return the same session.
	session1 := resolver.Resolve(info)
	session2 := resolver.Resolve(info)

	if session1 != session2 {
		t.Errorf("same multi-modal request should return same session: %s != %s",
			session1, session2)
	}

	// Different content should produce a different session.
	msgs2 := json.RawMessage(`[
		{
			"role": "user",
			"content": [
				{"type": "text", "text": "Different question about the image"},
				{"type": "image_url", "image_url": {"url": "https://example.com/photo.jpg"}}
			]
		}
	]`)
	info2 := SessionInfo{
		UserID:   "user-multimodal",
		Model:    "gpt-4o",
		Messages: msgs2,
	}

	session3 := resolver.Resolve(info2)
	if session3 == session1 {
		t.Error("different multi-modal content should produce different session")
	}
}
