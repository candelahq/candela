package proxy

import (
	"encoding/json"
	"testing"
)

// ── Anthropic cache token tests ──────────────────────────────────────────────

func TestAnthropicParser_ParseResponse_CacheTokens(t *testing.T) {
	// Anthropic's input_tokens INCLUDES cache_read + cache_creation in the total.
	// input_tokens = 21 (new) + 188086 (cache_read) + 500 (cache_creation) = 188607
	body := []byte(`{
		"id": "msg_01",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "hello"}],
		"usage": {
			"input_tokens": 188607,
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
	// nonCached = 188607 - 188086 - 500 = 21
	// Cost-equivalent = 21 + round(188086 * 0.1) + round(500 * 1.25)
	//                = 21 + 18809 + 625 = 19455
	if inputTokens != 19455 {
		t.Errorf("inputTokens = %d, want 19455 (21 non-cached + 18809 cache_read@0.1x + 625 cache_creation@1.25x)", inputTokens)
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
	// In streaming, input tokens (including cache) come in message_start.message.usage.
	// Anthropic's input_tokens = 50210 (total: 10 new + 50000 cache_read + 200 cache_creation).
	stream := `data: {"type":"message_start","message":{"usage":{"input_tokens":50210,"cache_read_input_tokens":50000,"cache_creation_input_tokens":200}}}
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}
data: {"type":"message_delta","usage":{"output_tokens":42}}
data: [DONE]
`

	parser := &anthropicParser{}
	content, inputTokens, outputTokens := parser.ParseStreamingResponse([]byte(stream))

	if content != "hello" {
		t.Errorf("content = %q, want %q", content, "hello")
	}
	// nonCached = 50210 - 50000 - 200 = 10
	// Cost-equivalent = 10 + round(50000 * 0.1) + round(200 * 1.25)
	//                = 10 + 5000 + 250 = 5260
	if inputTokens != 5260 {
		t.Errorf("inputTokens = %d, want 5260 (10 non-cached + 5000 cache_read@0.1x + 250 cache_creation@1.25x)", inputTokens)
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

// ── OpenAI cache token tests ─────────────────────────────────────────────────

func TestOpenAIParser_ParseResponse_CacheTokens(t *testing.T) {
	// OpenAI's prompt_tokens includes cached_tokens in the total.
	// prompt_tokens = 100 (total: 10 new + 90 cached)
	body := []byte(`{
		"choices": [{"message": {"role": "assistant", "content": "ok"}}],
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 20,
			"prompt_tokens_details": {"cached_tokens": 90}
		}
	}`)

	parser := &openaiParser{}
	_, inputTokens, outputTokens := parser.ParseResponse(body)

	// nonCached = 100 - 90 = 10
	// Cost-equivalent = 10 + round(90 * 0.5) = 10 + 45 = 55
	if inputTokens != 55 {
		t.Errorf("inputTokens = %d, want 55 (10 non-cached + 45 cached@0.5x)", inputTokens)
	}
	if outputTokens != 20 {
		t.Errorf("outputTokens = %d, want 20", outputTokens)
	}
}

func TestOpenAIParser_ParseResponse_NoCacheTokens(t *testing.T) {
	body := []byte(`{
		"choices": [{"message": {"role": "assistant", "content": "hi"}}],
		"usage": {"prompt_tokens": 50, "completion_tokens": 10}
	}`)

	parser := &openaiParser{}
	_, inputTokens, _ := parser.ParseResponse(body)

	if inputTokens != 50 {
		t.Errorf("inputTokens = %d, want 50 (no cache, unchanged)", inputTokens)
	}
}

// ── Google cache token tests ─────────────────────────────────────────────────

func TestGoogleParser_ParseResponse_CacheTokens(t *testing.T) {
	// Google's promptTokenCount includes cachedContentTokenCount.
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "result"}]}}],
		"usageMetadata": {
			"promptTokenCount": 1000,
			"cachedContentTokenCount": 800,
			"candidatesTokenCount": 50,
			"totalTokenCount": 1050
		}
	}`)

	parser := &googleParser{}
	_, inputTokens, outputTokens := parser.ParseResponse(body)

	// nonCached = 1000 - 800 = 200
	// Cost-equivalent = 200 + round(800 * 0.25) = 200 + 200 = 400
	if inputTokens != 400 {
		t.Errorf("inputTokens = %d, want 400 (200 non-cached + 200 cached@0.25x)", inputTokens)
	}
	if outputTokens != 50 {
		t.Errorf("outputTokens = %d, want 50", outputTokens)
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

// ── OpenAI streaming cache normalization ─────────────────────────────────────

func TestOpenAIParser_ParseStreamingResponse_CacheTokens(t *testing.T) {
	// OpenAI streaming includes usage in the final chunk when
	// stream_options.include_usage is set.
	stream := `data: {"choices":[{"delta":{"content":"hi"}}]}
data: {"choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":900}}}
data: [DONE]
`
	parser := &openaiParser{}
	content, inputTokens, outputTokens := parser.ParseStreamingResponse([]byte(stream))

	if content != "hi" {
		t.Errorf("content = %q, want %q", content, "hi")
	}
	// nonCached = 1000 - 900 = 100
	// Cost-equivalent = 100 + round(900 * 0.5) = 100 + 450 = 550
	if inputTokens != 550 {
		t.Errorf("inputTokens = %d, want 550 (100 non-cached + 450 cached@0.5x)", inputTokens)
	}
	if outputTokens != 50 {
		t.Errorf("outputTokens = %d, want 50", outputTokens)
	}
}

func TestOpenAIParser_ParseStreamingResponse_NoCacheTokens(t *testing.T) {
	stream := `data: {"choices":[{"delta":{"content":"ok"}}]}
data: {"choices":[],"usage":{"prompt_tokens":200,"completion_tokens":30}}
data: [DONE]
`
	parser := &openaiParser{}
	_, inputTokens, _ := parser.ParseStreamingResponse([]byte(stream))

	if inputTokens != 200 {
		t.Errorf("inputTokens = %d, want 200 (no cache, unchanged)", inputTokens)
	}
}

// ── Google streaming cache normalization ─────────────────────────────────────

func TestGoogleParser_ParseStreamingResponse_CacheTokens(t *testing.T) {
	// Google streaming returns a JSON array; the last chunk has usageMetadata.
	stream := `[
		{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]},
		{"candidates":[{"content":{"parts":[{"text":" world"}]}}],
		 "usageMetadata":{"promptTokenCount":5000,"cachedContentTokenCount":4000,"candidatesTokenCount":10}}
	]`

	parser := &googleParser{}
	content, inputTokens, outputTokens := parser.ParseStreamingResponse([]byte(stream))

	if content != "hello world" {
		t.Errorf("content = %q, want %q", content, "hello world")
	}
	// nonCached = 5000 - 4000 = 1000
	// Cost-equivalent = 1000 + round(4000 * 0.25) = 1000 + 1000 = 2000
	if inputTokens != 2000 {
		t.Errorf("inputTokens = %d, want 2000 (1000 non-cached + 1000 cached@0.25x)", inputTokens)
	}
	if outputTokens != 10 {
		t.Errorf("outputTokens = %d, want 10", outputTokens)
	}
}

// ── Helper function edge cases ───────────────────────────────────────────────

func TestNormalizeCachedInput_ZeroCached(t *testing.T) {
	// No cached tokens — should return raw input unchanged.
	result := normalizeCachedInput(500, 0, 0.5)
	if result != 500 {
		t.Errorf("normalizeCachedInput(500, 0, 0.5) = %d, want 500", result)
	}
}

func TestNormalizeCachedInput_AllCached(t *testing.T) {
	// All tokens are cached (e.g. repeated identical prompt).
	// nonCached = 100 - 100 = 0
	// result = 0 + round(100 * 0.5) = 50
	result := normalizeCachedInput(100, 100, 0.5)
	if result != 50 {
		t.Errorf("normalizeCachedInput(100, 100, 0.5) = %d, want 50", result)
	}
}

func TestNormalizeCachedInput_CachedExceedsRaw(t *testing.T) {
	// API inconsistency: cached > raw. Should clamp nonCached to 0.
	result := normalizeCachedInput(50, 100, 0.5)
	if result != 50 {
		t.Errorf("normalizeCachedInput(50, 100, 0.5) = %d, want 50 (clamped)", result)
	}
}

func TestNormalizeCachedInput_NegativeCached(t *testing.T) {
	// Negative cached tokens should be treated as zero.
	result := normalizeCachedInput(500, -10, 0.5)
	if result != 500 {
		t.Errorf("normalizeCachedInput(500, -10, 0.5) = %d, want 500", result)
	}
}

func TestNormalizeAnthropicInput_BothCacheTypes(t *testing.T) {
	// 100 total, 80 cache_read, 10 cache_creation, 10 new
	result := normalizeAnthropicInput(100, 80, 10)
	// nonCached = 100 - 80 - 10 = 10
	// result = 10 + round(80*0.1) + round(10*1.25) = 10 + 8 + 13 = 31
	if result != 31 {
		t.Errorf("normalizeAnthropicInput(100, 80, 10) = %d, want 31", result)
	}
}

func TestNormalizeAnthropicInput_NoCaching(t *testing.T) {
	result := normalizeAnthropicInput(100, 0, 0)
	if result != 100 {
		t.Errorf("normalizeAnthropicInput(100, 0, 0) = %d, want 100", result)
	}
}

// ── Stream usage option injection ────────────────────────────────────────────

func TestInjectStreamUsageOption_OpenAI(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	result := injectStreamUsageOption("openai", body)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	so, ok := parsed["stream_options"].(map[string]interface{})
	if !ok {
		t.Fatal("stream_options not injected")
	}
	if so["include_usage"] != true {
		t.Errorf("include_usage = %v, want true", so["include_usage"])
	}
}

func TestInjectStreamUsageOption_GeminiOAI(t *testing.T) {
	body := []byte(`{"model":"gemini-2.5-pro","stream":true}`)
	result := injectStreamUsageOption("gemini-oai", body)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if _, ok := parsed["stream_options"]; !ok {
		t.Fatal("stream_options not injected for gemini-oai")
	}
}

func TestInjectStreamUsageOption_Anthropic_NoOp(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	result := injectStreamUsageOption("anthropic", body)

	// Anthropic should not be modified.
	if string(result) != string(body) {
		t.Errorf("anthropic body was modified: %s", string(result))
	}
}

func TestInjectStreamUsageOption_PreservesExisting(t *testing.T) {
	body := []byte(`{"stream":true,"stream_options":{"include_usage":false}}`)
	result := injectStreamUsageOption("openai", body)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	so := parsed["stream_options"].(map[string]interface{})
	if so["include_usage"] != false {
		t.Errorf("should not override existing stream_options, got include_usage=%v", so["include_usage"])
	}
}
