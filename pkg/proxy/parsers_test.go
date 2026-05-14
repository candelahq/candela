package proxy

import (
	"testing"
)

// ── Anthropic cache token tests ──────────────────────────────────────────────

func TestAnthropicParser_ParseResponse_CacheTokens(t *testing.T) {
	// Anthropic returns cache_read_input_tokens and cache_creation_input_tokens
	// alongside input_tokens. The real total input is the sum of all three.
	body := []byte(`{
		"id": "msg_01",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "hello"}],
		"usage": {
			"input_tokens": 21,
			"cache_read_input_tokens": 188086,
			"cache_creation_input_tokens": 500,
			"output_tokens": 393
		}
	}`)

	parser := &anthropicParser{}
	content, inputTokens, outputTokens := parser.ParseResponse(body)

	if content != "hello" {
		t.Errorf("content = %q, want %q", content, "hello")
	}
	// Cost-equivalent input = 21 + round(188086 * 0.1) + round(500 * 1.25)
	//                       = 21 + 18809 + 625 = 19455
	if inputTokens != 19455 {
		t.Errorf("inputTokens = %d, want 19455 (21 + 18809 cache_read@0.1x + 625 cache_creation@1.25x)", inputTokens)
	}
	if outputTokens != 393 {
		t.Errorf("outputTokens = %d, want 393", outputTokens)
	}
}

func TestAnthropicParser_ParseResponse_NoCacheTokens(t *testing.T) {
	// When no caching is used, the cache fields are absent — should still work.
	body := []byte(`{
		"content": [{"type": "text", "text": "hi"}],
		"usage": {"input_tokens": 100, "output_tokens": 50}
	}`)

	parser := &anthropicParser{}
	_, inputTokens, outputTokens := parser.ParseResponse(body)

	if inputTokens != 100 {
		t.Errorf("inputTokens = %d, want 100", inputTokens)
	}
	if outputTokens != 50 {
		t.Errorf("outputTokens = %d, want 50", outputTokens)
	}
}

func TestAnthropicParser_ParseStreamingResponse_CacheTokens(t *testing.T) {
	// In streaming, input tokens (including cache) come in message_start.message.usage,
	// and output tokens come in message_delta.usage.
	stream := `data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":50000,"cache_creation_input_tokens":200}}}
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}
data: {"type":"message_delta","usage":{"output_tokens":42}}
data: [DONE]
`

	parser := &anthropicParser{}
	content, inputTokens, outputTokens := parser.ParseStreamingResponse([]byte(stream))

	if content != "hello" {
		t.Errorf("content = %q, want %q", content, "hello")
	}
	// Cost-equivalent input = 10 + round(50000 * 0.1) + round(200 * 1.25)
	//                       = 10 + 5000 + 250 = 5260
	if inputTokens != 5260 {
		t.Errorf("inputTokens = %d, want 5260 (10 + 5000 cache_read@0.1x + 250 cache_creation@1.25x)", inputTokens)
	}
	if outputTokens != 42 {
		t.Errorf("outputTokens = %d, want 42", outputTokens)
	}
}

func TestAnthropicParser_ParseStreamingResponse_NoCacheTokens(t *testing.T) {
	stream := `data: {"type":"message_start","message":{"usage":{"input_tokens":100}}}
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}
data: {"type":"message_delta","usage":{"output_tokens":25}}
data: [DONE]
`
	parser := &anthropicParser{}
	_, inputTokens, outputTokens := parser.ParseStreamingResponse([]byte(stream))

	if inputTokens != 100 {
		t.Errorf("inputTokens = %d, want 100", inputTokens)
	}
	if outputTokens != 25 {
		t.Errorf("outputTokens = %d, want 25", outputTokens)
	}
}

// ── Google thinking token tests ──────────────────────────────────────────────

func TestGoogleParser_ParseResponse_ThinkingTokens(t *testing.T) {
	// Gemini 2.5 models include thoughtsTokenCount in usageMetadata.
	// These are billed as output tokens but NOT included in candidatesTokenCount.
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "result"}]}}],
		"usageMetadata": {
			"promptTokenCount": 100,
			"candidatesTokenCount": 500,
			"totalTokenCount": 3500,
			"thoughtsTokenCount": 2900
		}
	}`)

	parser := &googleParser{}
	content, inputTokens, outputTokens := parser.ParseResponse(body)

	if content != "result" {
		t.Errorf("content = %q, want %q", content, "result")
	}
	if inputTokens != 100 {
		t.Errorf("inputTokens = %d, want 100", inputTokens)
	}
	// Output = candidatesTokenCount + thoughtsTokenCount = 500 + 2900 = 3400
	if outputTokens != 3400 {
		t.Errorf("outputTokens = %d, want 3400 (500 candidates + 2900 thinking)", outputTokens)
	}
}

func TestGoogleParser_ParseResponse_NoThinkingTokens(t *testing.T) {
	// Non-thinking models (Gemini 2.0 Flash, etc.) don't have thoughtsTokenCount.
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "hi"}]}}],
		"usageMetadata": {
			"promptTokenCount": 50,
			"candidatesTokenCount": 30,
			"totalTokenCount": 80
		}
	}`)

	parser := &googleParser{}
	_, inputTokens, outputTokens := parser.ParseResponse(body)

	if inputTokens != 50 {
		t.Errorf("inputTokens = %d, want 50", inputTokens)
	}
	if outputTokens != 30 {
		t.Errorf("outputTokens = %d, want 30", outputTokens)
	}
}

// ── Google streaming parser tests ────────────────────────────────────────────

func TestGoogleParser_ParseStreamingResponse_ChunkedJSON(t *testing.T) {
	// Google streaming returns newline-delimited JSON chunks wrapped in an array.
	// The usageMetadata appears in the last chunk.
	data := []byte(`[{
  "candidates": [{"content": {"parts": [{"text": "chunk1"}]}}]
},
{
  "candidates": [{"content": {"parts": [{"text": "chunk2"}]}}],
  "usageMetadata": {
    "promptTokenCount": 200,
    "candidatesTokenCount": 100,
    "thoughtsTokenCount": 800,
    "totalTokenCount": 1100
  }
}]`)

	parser := &googleParser{}
	_, inputTokens, outputTokens := parser.ParseStreamingResponse(data)

	if inputTokens != 200 {
		t.Errorf("inputTokens = %d, want 200", inputTokens)
	}
	// Output = 100 + 800 = 900
	if outputTokens != 900 {
		t.Errorf("outputTokens = %d, want 900 (100 candidates + 800 thinking)", outputTokens)
	}
}

func TestGoogleParser_ParseStreamingResponse_SingleChunk(t *testing.T) {
	// When only a single chunk is buffered, the standard ParseResponse path handles it.
	data := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "one shot"}]}}],
		"usageMetadata": {
			"promptTokenCount": 50,
			"candidatesTokenCount": 25,
			"totalTokenCount": 75
		}
	}`)

	parser := &googleParser{}
	content, inputTokens, outputTokens := parser.ParseStreamingResponse(data)

	if content != "one shot" {
		t.Errorf("content = %q, want %q", content, "one shot")
	}
	if inputTokens != 50 {
		t.Errorf("inputTokens = %d, want 50", inputTokens)
	}
	if outputTokens != 25 {
		t.Errorf("outputTokens = %d, want 25", outputTokens)
	}
}

// ── Provider registry tests ─────────────────────────────────────────────────

func TestGetParser_AllProviders(t *testing.T) {
	// Verify all registered providers return the correct parser type.
	providers := map[string]string{
		"openai":           "*proxy.openaiParser",
		"gemini-oai":       "*proxy.openaiParser",
		"anthropic":        "*proxy.anthropicParser",
		"anthropic-direct": "*proxy.anthropicParser",
		"anthropic-vertex": "*proxy.anthropicParser",
		"google":           "*proxy.googleParser",
	}

	for name, wantType := range providers {
		p := getParser(name)
		if p == nil {
			t.Errorf("getParser(%q) returned nil", name)
			continue
		}
		// Fallback parser means the provider isn't registered.
		if _, isFallback := p.(*fallbackParser); isFallback {
			t.Errorf("getParser(%q) returned fallback parser, want %s", name, wantType)
		}
	}
}

func TestGetParser_UnknownProvider_ReturnsFallback(t *testing.T) {
	p := getParser("some-unknown-provider")
	if _, ok := p.(*fallbackParser); !ok {
		t.Error("expected fallback parser for unknown provider")
	}
}

// ── Cache token extraction tests ────────────────────────────────────────────

func TestExtractCacheTokens_Anthropic(t *testing.T) {
	body := []byte(`{
		"usage": {
			"input_tokens": 21,
			"cache_read_input_tokens": 188086,
			"cache_creation_input_tokens": 500,
			"output_tokens": 393
		}
	}`)

	ct := extractCacheTokens("anthropic", body)
	if ct.CacheReadTokens != 188086 {
		t.Errorf("CacheReadTokens = %d, want 188086", ct.CacheReadTokens)
	}
	if ct.CacheCreationTokens != 500 {
		t.Errorf("CacheCreationTokens = %d, want 500", ct.CacheCreationTokens)
	}

	// Also works for anthropic-direct and anthropic-vertex.
	for _, provider := range []string{"anthropic-direct", "anthropic-vertex"} {
		ct2 := extractCacheTokens(provider, body)
		if ct2.CacheReadTokens != 188086 {
			t.Errorf("%s: CacheReadTokens = %d, want 188086", provider, ct2.CacheReadTokens)
		}
	}
}

func TestExtractCacheTokens_OpenAI(t *testing.T) {
	body := []byte(`{
		"usage": {
			"prompt_tokens": 5000,
			"completion_tokens": 200,
			"prompt_tokens_details": {
				"cached_tokens": 4096
			}
		}
	}`)

	ct := extractCacheTokens("openai", body)
	if ct.CacheReadTokens != 4096 {
		t.Errorf("CacheReadTokens = %d, want 4096", ct.CacheReadTokens)
	}
	if ct.CacheCreationTokens != 0 {
		t.Errorf("CacheCreationTokens = %d, want 0 (OpenAI has no creation concept)", ct.CacheCreationTokens)
	}

	// gemini-oai uses the same OpenAI format.
	ct2 := extractCacheTokens("gemini-oai", body)
	if ct2.CacheReadTokens != 4096 {
		t.Errorf("gemini-oai: CacheReadTokens = %d, want 4096", ct2.CacheReadTokens)
	}
}

func TestExtractCacheTokens_Google(t *testing.T) {
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "hi"}]}}],
		"usageMetadata": {
			"promptTokenCount": 100,
			"candidatesTokenCount": 50,
			"cachedContentTokenCount": 80,
			"totalTokenCount": 150
		}
	}`)

	ct := extractCacheTokens("google", body)
	if ct.CacheReadTokens != 80 {
		t.Errorf("CacheReadTokens = %d, want 80", ct.CacheReadTokens)
	}
	if ct.CacheCreationTokens != 0 {
		t.Errorf("CacheCreationTokens = %d, want 0 (Google has no creation concept)", ct.CacheCreationTokens)
	}
}

func TestExtractCacheTokens_UnknownProvider(t *testing.T) {
	body := []byte(`{"usage": {"cached_tokens": 999}}`)
	ct := extractCacheTokens("unknown-provider", body)
	if ct.CacheReadTokens != 0 || ct.CacheCreationTokens != 0 {
		t.Errorf("unknown provider should return zeros, got read=%d creation=%d",
			ct.CacheReadTokens, ct.CacheCreationTokens)
	}
}

func TestExtractStreamingCacheTokens_Anthropic(t *testing.T) {
	stream := []byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":50000,"cache_creation_input_tokens":200}}}
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}
data: {"type":"message_delta","usage":{"output_tokens":42}}
data: [DONE]
`)

	ct := extractStreamingCacheTokens("anthropic", stream)
	if ct.CacheReadTokens != 50000 {
		t.Errorf("CacheReadTokens = %d, want 50000", ct.CacheReadTokens)
	}
	if ct.CacheCreationTokens != 200 {
		t.Errorf("CacheCreationTokens = %d, want 200", ct.CacheCreationTokens)
	}
}
