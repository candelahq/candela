package session

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// helper to build a messages JSON array.
func makeMessages(msgs ...struct{ Role, Content string }) json.RawMessage {
	b, _ := json.Marshal(msgs)
	return b
}

func msg(role, content string) struct{ Role, Content string } {
	return struct{ Role, Content string }{Role: role, Content: content}
}

func TestHeaderResolver_Present(t *testing.T) {
	r := NewHeaderResolver("")
	h := http.Header{}
	h.Set("X-Session-Id", "abc-123")

	got := r.Resolve(SessionInfo{Headers: h})
	if got != "abc-123" {
		t.Errorf("expected abc-123, got %q", got)
	}
}

func TestHeaderResolver_Missing(t *testing.T) {
	r := NewHeaderResolver("")
	got := r.Resolve(SessionInfo{Headers: http.Header{}})
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHeaderResolver_NilHeaders(t *testing.T) {
	r := NewHeaderResolver("")
	got := r.Resolve(SessionInfo{})
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHeaderResolver_CustomName(t *testing.T) {
	r := NewHeaderResolver("X-Custom-Session")
	h := http.Header{}
	h.Set("X-Custom-Session", "custom-id")

	got := r.Resolve(SessionInfo{Headers: h})
	if got != "custom-id" {
		t.Errorf("expected custom-id, got %q", got)
	}
}

func TestUserMsgResolver_SameConversationGrows(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)

	// Turn 1: system + user
	msgs1 := makeMessages(
		msg("system", "You are a helpful assistant."),
		msg("user", "Explain auth code"),
	)
	id1 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs1})

	// Turn 2: same conversation grows
	msgs2 := makeMessages(
		msg("system", "You are a helpful assistant."),
		msg("user", "Explain auth code"),
		msg("assistant", "The auth code..."),
		msg("user", "Refactor it"),
	)
	id2 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs2})

	if id1 != id2 {
		t.Errorf("expected same session, got %q and %q", id1, id2)
	}
}

func TestUserMsgResolver_ModelSwitchSameSession(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)

	msgs := makeMessages(
		msg("system", "You are a helpful assistant."),
		msg("user", "Explain auth code"),
	)

	id1 := r.Resolve(SessionInfo{UserID: "alice", Model: "gpt-4", Messages: msgs})
	id2 := r.Resolve(SessionInfo{UserID: "alice", Model: "claude-3", Messages: msgs})

	if id1 != id2 {
		t.Errorf("model switch should not create new session, got %q and %q", id1, id2)
	}
}

func TestUserMsgResolver_DifferentConversations(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)

	msgs1 := makeMessages(
		msg("system", "You are a helpful assistant."),
		msg("user", "Explain auth code"),
	)
	msgs2 := makeMessages(
		msg("system", "You are a helpful assistant."),
		msg("user", "Write a unit test"),
	)

	id1 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs1})
	id2 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs2})

	if id1 == id2 {
		t.Errorf("different conversations should have different sessions")
	}
}

func TestUserMsgResolver_DifferentUsers(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)

	msgs := makeMessages(
		msg("system", "You are a helpful assistant."),
		msg("user", "Explain auth code"),
	)

	id1 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs})
	id2 := r.Resolve(SessionInfo{UserID: "bob", Messages: msgs})

	if id1 == id2 {
		t.Errorf("different users should have different sessions")
	}
}

func TestUserMsgResolver_DynamicSystemPrompt(t *testing.T) {
	// Cline rebuilds messages[0] every turn — fingerprint should still match
	// because we only hash the first USER message, not the system prompt.
	r := NewUserMsgResolver(30 * time.Minute)

	msgs1 := makeMessages(
		msg("system", "You are Cline v1. CWD: /home/alice/project. OS: macOS."),
		msg("user", "Explain auth code"),
	)
	msgs2 := makeMessages(
		msg("system", "You are Cline v1. CWD: /home/alice/project. OS: macOS. Time: 12:00."),
		msg("user", "Explain auth code"),
		msg("assistant", "The auth code..."),
		msg("user", "Refactor it"),
	)

	id1 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs1})
	id2 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs2})

	if id1 != id2 {
		t.Errorf("dynamic system prompt should not break session, got %q and %q", id1, id2)
	}
}

func TestUserMsgResolver_Timeout(t *testing.T) {
	now := time.Now()
	r := NewUserMsgResolver(30 * time.Minute)
	r.nowFunc = func() time.Time { return now }

	msgs := makeMessages(
		msg("system", "sys"),
		msg("user", "hello"),
	)
	id1 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs})

	// Advance past TTL.
	r.nowFunc = func() time.Time { return now.Add(31 * time.Minute) }
	id2 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs})

	if id1 == id2 {
		t.Errorf("expired session should get new ID")
	}
}

func TestUserMsgResolver_Compaction(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)

	// Long conversation: 6 messages
	msgs1 := makeMessages(
		msg("system", "sys"),
		msg("user", "hello"),
		msg("assistant", "hi"),
		msg("user", "how are you"),
		msg("assistant", "good"),
		msg("user", "great"),
	)
	id1 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs1})

	// After compaction: messages shrunk but same first user message
	msgs2 := makeMessages(
		msg("system", "sys"),
		msg("user", "hello"),
		msg("assistant", "good"),
		msg("user", "great"),
	)
	id2 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs2})

	// Compaction creates a new session (message count decreased).
	if id1 == id2 {
		t.Errorf("compaction (message count decrease) should create new session")
	}
}

func TestUserMsgResolver_EmptyMessages(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)
	got := r.Resolve(SessionInfo{UserID: "alice", Messages: nil})
	if got != "" {
		t.Errorf("nil messages should return empty, got %q", got)
	}
}

func TestUserMsgResolver_NoUserMessage(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)
	msgs := makeMessages(msg("system", "You are a helpful assistant."))
	got := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs})
	if got != "" {
		t.Errorf("no user message should return empty, got %q", got)
	}
}

func TestChainResolver_HeaderWins(t *testing.T) {
	chain := NewChainResolver(
		NewHeaderResolver(""),
		NewUserMsgResolver(30*time.Minute),
	)

	h := http.Header{}
	h.Set("X-Session-Id", "explicit-id")
	msgs := makeMessages(
		msg("system", "sys"),
		msg("user", "hello"),
	)

	got := chain.Resolve(SessionInfo{
		UserID:   "alice",
		Messages: msgs,
		Headers:  h,
	})

	if got != "explicit-id" {
		t.Errorf("header should win, got %q", got)
	}
}

func TestChainResolver_FallsThrough(t *testing.T) {
	chain := NewChainResolver(
		NewHeaderResolver(""), // no header → empty
		NewUserMsgResolver(30*time.Minute),
	)

	msgs := makeMessages(
		msg("system", "sys"),
		msg("user", "hello"),
	)

	got := chain.Resolve(SessionInfo{
		UserID:   "alice",
		Messages: msgs,
		Headers:  http.Header{},
	})

	if got == "" {
		t.Error("chain should fall through to UserMsgResolver")
	}
}

func TestChainResolver_FallbackUUID(t *testing.T) {
	// No resolvers match → generates a UUID.
	chain := NewChainResolver(
		NewHeaderResolver(""),
	)

	got := chain.Resolve(SessionInfo{Headers: http.Header{}})
	if got == "" {
		t.Error("chain should generate fallback UUID")
	}
}

func TestCacheSize(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)
	if r.CacheSize() != 0 {
		t.Errorf("initial cache size should be 0")
	}

	msgs := makeMessages(msg("system", "sys"), msg("user", "hello"))
	r.Resolve(SessionInfo{UserID: "alice", Messages: msgs})

	if r.CacheSize() != 1 {
		t.Errorf("cache size should be 1 after one session, got %d", r.CacheSize())
	}
}

func TestUserMsgResolver_MultiModalContent(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)

	// Multi-modal message: content is an array of objects (e.g., vision API).
	msgs := json.RawMessage(`[
		{"role": "system", "content": "You are helpful."},
		{"role": "user", "content": [
			{"type": "text", "text": "What is in this image?"},
			{"type": "image_url", "image_url": {"url": "https://example.com/img.png"}}
		]}
	]`)

	id1 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs})
	if id1 == "" {
		t.Error("multi-modal content should still produce a session ID")
	}

	// Same multi-modal message should match same session.
	id2 := r.Resolve(SessionInfo{UserID: "alice", Messages: msgs})
	if id1 != id2 {
		t.Errorf("same multi-modal content should match same session, got %q and %q", id1, id2)
	}
}

func TestUserMsgResolver_MultiModalVsString(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)

	// String content.
	stringMsgs := makeMessages(
		msg("system", "sys"),
		msg("user", "What is in this image?"),
	)

	// Multi-modal content with same text.
	multiMsgs := json.RawMessage(`[
		{"role": "system", "content": "sys"},
		{"role": "user", "content": [
			{"type": "text", "text": "What is in this image?"}
		]}
	]`)

	id1 := r.Resolve(SessionInfo{UserID: "alice", Messages: stringMsgs})
	id2 := r.Resolve(SessionInfo{UserID: "alice", Messages: multiMsgs})

	// String and array content should produce different sessions
	// because the raw bytes differ.
	if id1 == id2 {
		t.Error("string vs multi-modal content should produce different sessions")
	}
}

func TestUserMsgResolver_CacheEviction(t *testing.T) {
	r := NewUserMsgResolver(30 * time.Minute)
	r.maxEntries = 5 // small cap for testing

	// Fill cache with 5 unique sessions.
	for i := range 5 {
		msgs := makeMessages(
			msg("system", "sys"),
			msg("user", "msg-"+string(rune('a'+i))),
		)
		r.Resolve(SessionInfo{UserID: "alice", Messages: msgs})
	}

	if r.CacheSize() != 5 {
		t.Errorf("cache should have 5 entries, got %d", r.CacheSize())
	}

	// Add one more — should evict oldest to stay at 5.
	newMsgs := makeMessages(
		msg("system", "sys"),
		msg("user", "msg-new"),
	)
	r.Resolve(SessionInfo{UserID: "alice", Messages: newMsgs})

	if r.CacheSize() != 5 {
		t.Errorf("cache should stay at 5 after eviction, got %d", r.CacheSize())
	}
}

func TestUserMsgResolver_CacheEvictionPrefersTTL(t *testing.T) {
	now := time.Now()
	r := NewUserMsgResolver(30 * time.Minute)
	r.maxEntries = 5
	r.nowFunc = func() time.Time { return now }

	// Add 5 sessions.
	for i := range 5 {
		msgs := makeMessages(
			msg("system", "sys"),
			msg("user", "msg-"+string(rune('a'+i))),
		)
		r.Resolve(SessionInfo{UserID: "alice", Messages: msgs})
	}

	// Age 3 of them past TTL.
	r.nowFunc = func() time.Time { return now.Add(31 * time.Minute) }

	// Adding a new session should first evict the 3 expired ones.
	newMsgs := makeMessages(
		msg("system", "sys"),
		msg("user", "msg-new"),
	)
	r.Resolve(SessionInfo{UserID: "alice", Messages: newMsgs})

	// The 3 expired + 2 remaining could be evicted down to maxEntries.
	// Since all 5 were created at the same time, all expired → cache should be just 1 (the new one).
	if r.CacheSize() != 1 {
		t.Errorf("expired entries should be evicted, cache should be 1, got %d", r.CacheSize())
	}
}
