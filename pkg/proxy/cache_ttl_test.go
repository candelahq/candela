package proxy

import "testing"

// ── isAnthropicProvider ──────────────────────────────────────────────────────

func TestIsAnthropicProvider(t *testing.T) {
	tests := []struct {
		provider string
		want     bool
	}{
		{"anthropic", true},
		{"anthropic-direct", true},
		{"anthropic-vertex", true},
		{"anthropic-bedrock", true}, // future-proof: any anthropic- prefix
		{"openai", false},
		{"google", false},
		{"gemini-oai", false},
		{"local", false},
		{"", false},
		{"Anthropic", false}, // case-sensitive — proxy uses lowercase names
	}
	for _, tt := range tests {
		got := isAnthropicProvider(tt.provider)
		if got != tt.want {
			t.Errorf("isAnthropicProvider(%q) = %v, want %v", tt.provider, got, tt.want)
		}
	}
}

// ── extractAnthropicCacheTTL ─────────────────────────────────────────────────

func TestExtractAnthropicCacheTTL_SystemPrompt_1h(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": [{"type": "text", "text": "You are helpful.", "cache_control": {"type": "ephemeral", "ttl": "1h"}}],
		"messages": [{"role": "user", "content": "Hello"}]
	}`)
	if !extractAnthropicCacheTTL(body) {
		t.Error("should detect 1h TTL in system prompt")
	}
}

func TestExtractAnthropicCacheTTL_MessageContent_1h(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": "You are helpful.",
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "Analyze this:", "cache_control": {"type": "ephemeral", "ttl": "1h"}}
			]}
		]
	}`)
	if !extractAnthropicCacheTTL(body) {
		t.Error("should detect 1h TTL in message content block")
	}
}

func TestExtractAnthropicCacheTTL_5mDefault_NoTTLField(t *testing.T) {
	// Default 5-minute TTL omits the "ttl" field entirely for backward compat.
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": [{"type": "text", "text": "You are helpful.", "cache_control": {"type": "ephemeral"}}],
		"messages": [{"role": "user", "content": "Hello"}]
	}`)
	if extractAnthropicCacheTTL(body) {
		t.Error("should NOT detect extended TTL when ttl field is absent (5m default)")
	}
}

func TestExtractAnthropicCacheTTL_NoCacheControl(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": "You are helpful.",
		"messages": [{"role": "user", "content": "Hello"}]
	}`)
	if extractAnthropicCacheTTL(body) {
		t.Error("should NOT detect extended TTL when no cache_control present")
	}
}

func TestExtractAnthropicCacheTTL_InvalidJSON(t *testing.T) {
	body := []byte(`{invalid json`)
	if extractAnthropicCacheTTL(body) {
		t.Error("should return false for invalid JSON")
	}
}

func TestExtractAnthropicCacheTTL_EmptyBody(t *testing.T) {
	if extractAnthropicCacheTTL([]byte{}) {
		t.Error("should return false for empty body")
	}
}

func TestExtractAnthropicCacheTTL_StringSystemPrompt(t *testing.T) {
	// System prompt as a plain string (no cache_control possible).
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": "Just a string",
		"messages": [{"role": "user", "content": "Hello"}]
	}`)
	if extractAnthropicCacheTTL(body) {
		t.Error("should NOT detect TTL in string system prompt")
	}
}

func TestExtractAnthropicCacheTTL_MultipleBlocks_OnlyOne1h(t *testing.T) {
	// Multiple cache_control blocks, only one has 1h TTL.
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": [
			{"type": "text", "text": "Part 1", "cache_control": {"type": "ephemeral"}},
			{"type": "text", "text": "Part 2", "cache_control": {"type": "ephemeral", "ttl": "1h"}}
		],
		"messages": [{"role": "user", "content": "Hello"}]
	}`)
	if !extractAnthropicCacheTTL(body) {
		t.Error("should detect 1h TTL even when mixed with non-TTL blocks")
	}
}

func TestExtractAnthropicCacheTTL_FastPath_NoTTLString(t *testing.T) {
	// Body does NOT contain the string "ttl" at all — fast-path should skip unmarshal.
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": [{"type": "text", "text": "You are helpful.", "cache_control": {"type": "ephemeral"}}],
		"messages": [{"role": "user", "content": "Hello world"}]
	}`)
	if extractAnthropicCacheTTL(body) {
		t.Error("should return false via fast-path")
	}
}

func TestExtractAnthropicCacheTTL_TTLInUserContent_NotCacheControl(t *testing.T) {
	// The string "ttl" appears in user content but NOT in a cache_control block.
	// This tests that the fast-path doesn't cause false negatives — it triggers
	// the full parse, which correctly returns false.
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"system": "You are helpful.",
		"messages": [{"role": "user", "content": "What is the ttl for DNS?"}]
	}`)
	if extractAnthropicCacheTTL(body) {
		t.Error("should NOT detect TTL from user content text")
	}
}

// ── hasCacheTTL1h ────────────────────────────────────────────────────────────

func TestHasCacheTTL1h_Valid(t *testing.T) {
	block := map[string]interface{}{
		"type":          "text",
		"text":          "Hello",
		"cache_control": map[string]interface{}{"type": "ephemeral", "ttl": "1h"},
	}
	if !hasCacheTTL1h(block) {
		t.Error("should detect 1h TTL")
	}
}

func TestHasCacheTTL1h_5m(t *testing.T) {
	block := map[string]interface{}{
		"type":          "text",
		"text":          "Hello",
		"cache_control": map[string]interface{}{"type": "ephemeral", "ttl": "5m"},
	}
	if hasCacheTTL1h(block) {
		t.Error("should NOT detect 5m as 1h TTL")
	}
}

func TestHasCacheTTL1h_NoTTLField(t *testing.T) {
	block := map[string]interface{}{
		"type":          "text",
		"text":          "Hello",
		"cache_control": map[string]interface{}{"type": "ephemeral"},
	}
	if hasCacheTTL1h(block) {
		t.Error("should NOT detect TTL when field is absent")
	}
}

func TestHasCacheTTL1h_NoCacheControl(t *testing.T) {
	block := map[string]interface{}{
		"type": "text",
		"text": "Hello",
	}
	if hasCacheTTL1h(block) {
		t.Error("should NOT detect TTL when no cache_control")
	}
}

func TestHasCacheTTL1h_NotAMap(t *testing.T) {
	if hasCacheTTL1h("not a map") {
		t.Error("should return false for non-map block")
	}
}

func TestHasCacheTTL1h_NilBlock(t *testing.T) {
	if hasCacheTTL1h(nil) {
		t.Error("should return false for nil block")
	}
}
