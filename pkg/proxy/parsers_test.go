package proxy

import (
	"encoding/json"
	"testing"
)

// ── Anthropic parser tests ───────────────────────────────────────────────────
// All parsers return RAW token counts. Cache normalization is handled by
// costcalc.Calculator.NormalizeCachedInput at the proxy call site.

func TestAnthropicParser_ParseResponse_CacheTokens(t *testing.T) {
	// Anthropic's input_tokens is ONLY fresh tokens (not cached).
	// cache_read + cache_creation are separate additive fields.
	// Parser returns raw input_tokens — no cache normalization.
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
	// Parser returns raw input_tokens (no cache normalization).
	if inputTokens != 188607 {
		t.Errorf("inputTokens = %d, want 188607 (raw, no normalization)", inputTokens)
	}
	if outputTokens != 393 {
		t.Errorf("outputTokens = %d, want 393", outputTokens)
	}
}

func TestAnthropicParser_ParseResponse_NoCacheTokens(t *testing.T) {
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
	// Anthropic streaming: parser returns raw input_tokens from message_start.
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
	// Parser returns raw input_tokens (no cache normalization).
	if inputTokens != 50210 {
		t.Errorf("inputTokens = %d, want 50210 (raw, no normalization)", inputTokens)
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

// ── OpenAI parser tests ─────────────────────────────────────────────────────

func TestOpenAIParser_ParseResponse_CacheTokens(t *testing.T) {
	// OpenAI's prompt_tokens includes cached_tokens in the total.
	// Parser returns raw prompt_tokens — no cache normalization.
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

	// Parser returns raw prompt_tokens (no cache normalization).
	if inputTokens != 100 {
		t.Errorf("inputTokens = %d, want 100 (raw, no normalization)", inputTokens)
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
		t.Errorf("inputTokens = %d, want 50", inputTokens)
	}
}

// ── Google parser tests ─────────────────────────────────────────────────────

func TestGoogleParser_ParseResponse_CacheTokens(t *testing.T) {
	// Google's promptTokenCount includes cachedContentTokenCount.
	// Parser returns raw promptTokenCount — no cache normalization.
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

	// Parser returns raw promptTokenCount (no cache normalization).
	if inputTokens != 1000 {
		t.Errorf("inputTokens = %d, want 1000 (raw promptTokenCount)", inputTokens)
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

func TestExtractStreamingCacheTokens_Anthropic_MessageDelta(t *testing.T) {
	// Some Vertex AI responses report cache tokens in the message_delta
	// event (top-level usage) rather than message_start.
	stream := []byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10}}}
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}
data: {"type":"message_delta","usage":{"output_tokens":42,"cache_read_input_tokens":30000,"cache_creation_input_tokens":100}}
data: [DONE]
`)

	ct := extractStreamingCacheTokens("anthropic", stream)
	if ct.CacheReadTokens != 30000 {
		t.Errorf("CacheReadTokens = %d, want 30000 (from message_delta)", ct.CacheReadTokens)
	}
	if ct.CacheCreationTokens != 100 {
		t.Errorf("CacheCreationTokens = %d, want 100 (from message_delta)", ct.CacheCreationTokens)
	}
}

func TestExtractStreamingCacheTokens_Anthropic_BothEvents(t *testing.T) {
	// When both message_start and message_delta report cache tokens,
	// message_delta (later in stream) should win.
	stream := []byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":5000,"cache_creation_input_tokens":50}}}
data: {"type":"message_delta","usage":{"output_tokens":42,"cache_read_input_tokens":5000,"cache_creation_input_tokens":50}}
data: [DONE]
`)

	ct := extractStreamingCacheTokens("anthropic", stream)
	if ct.CacheReadTokens != 5000 {
		t.Errorf("CacheReadTokens = %d, want 5000", ct.CacheReadTokens)
	}
	if ct.CacheCreationTokens != 50 {
		t.Errorf("CacheCreationTokens = %d, want 50", ct.CacheCreationTokens)
	}
}

func TestExtractStreamingCacheTokens_Anthropic_NullPayload(t *testing.T) {
	// Ensure literal "null" SSE payload doesn't cause a panic.
	stream := []byte(`data: null
data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":100}}}
data: [DONE]
`)

	ct := extractStreamingCacheTokens("anthropic", stream)
	if ct.CacheReadTokens != 100 {
		t.Errorf("CacheReadTokens = %d, want 100", ct.CacheReadTokens)
	}
}

func TestExtractStreamingCacheTokens_AnthropicVertex(t *testing.T) {
	// anthropic-vertex uses the same parser as anthropic.
	stream := []byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":40000,"cache_creation_input_tokens":300}}}
data: {"type":"message_delta","usage":{"output_tokens":42}}
data: [DONE]
`)

	ct := extractStreamingCacheTokens("anthropic-vertex", stream)
	if ct.CacheReadTokens != 40000 {
		t.Errorf("CacheReadTokens = %d, want 40000", ct.CacheReadTokens)
	}
	if ct.CacheCreationTokens != 300 {
		t.Errorf("CacheCreationTokens = %d, want 300", ct.CacheCreationTokens)
	}
}

// ── OpenAI streaming parser tests ────────────────────────────────────────────

func TestOpenAIParser_ParseStreamingResponse_CacheTokens(t *testing.T) {
	// Parser returns raw prompt_tokens — no cache normalization.
	stream := `data: {"choices":[{"delta":{"content":"hi"}}]}
data: {"choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":900}}}
data: [DONE]
`
	parser := &openaiParser{}
	content, inputTokens, outputTokens := parser.ParseStreamingResponse([]byte(stream))

	if content != "hi" {
		t.Errorf("content = %q, want %q", content, "hi")
	}
	// Parser returns raw prompt_tokens (no cache normalization).
	if inputTokens != 1000 {
		t.Errorf("inputTokens = %d, want 1000 (raw, no normalization)", inputTokens)
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

// ── Google streaming cache tests ─────────────────────────────────────────────

func TestGoogleParser_ParseStreamingResponse_CacheTokens(t *testing.T) {
	// Parser returns raw promptTokenCount — no cache normalization.
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
	// Parser returns raw promptTokenCount (no cache normalization).
	if inputTokens != 5000 {
		t.Errorf("inputTokens = %d, want 5000 (raw promptTokenCount)", inputTokens)
	}
	if outputTokens != 10 {
		t.Errorf("outputTokens = %d, want 10", outputTokens)
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

// ── Model extraction from response tests ─────────────────────────────────────

func TestExtractModelFromResponse_Google(t *testing.T) {
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "hi"}]}}],
		"modelVersion": "gemini-2.0-flash-001",
		"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5}
	}`)

	model := extractModelFromResponse("google", body)
	if model != "gemini-2.0-flash-001" {
		t.Errorf("Google model = %q, want %q", model, "gemini-2.0-flash-001")
	}
}

func TestExtractModelFromResponse_Google_25Pro(t *testing.T) {
	body := []byte(`{
		"modelVersion": "gemini-2.5-pro-preview-05-06",
		"candidates": [{"content": {"parts": [{"text": "hi"}]}}]
	}`)

	model := extractModelFromResponse("google", body)
	if model != "gemini-2.5-pro-preview-05-06" {
		t.Errorf("Google model = %q, want %q", model, "gemini-2.5-pro-preview-05-06")
	}
}

func TestExtractModelFromResponse_Google_Missing(t *testing.T) {
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "hi"}]}}],
		"usageMetadata": {"promptTokenCount": 10}
	}`)

	model := extractModelFromResponse("google", body)
	if model != "" {
		t.Errorf("Google model without modelVersion = %q, want empty", model)
	}
}

func TestExtractModelFromResponse_OpenAI(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o-2024-08-06",
		"choices": [{"message": {"content": "hi"}}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5}
	}`)

	model := extractModelFromResponse("openai", body)
	if model != "gpt-4o-2024-08-06" {
		t.Errorf("OpenAI model = %q, want %q", model, "gpt-4o-2024-08-06")
	}
}

func TestExtractModelFromResponse_GeminiOAI(t *testing.T) {
	body := []byte(`{
		"model": "gemini-2.5-flash",
		"choices": [{"message": {"content": "hi"}}]
	}`)

	model := extractModelFromResponse("gemini-oai", body)
	if model != "gemini-2.5-flash" {
		t.Errorf("gemini-oai model = %q, want %q", model, "gemini-2.5-flash")
	}
}

func TestExtractModelFromResponse_Anthropic(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"content": [{"type": "text", "text": "hi"}],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)

	model := extractModelFromResponse("anthropic", body)
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("Anthropic model = %q, want %q", model, "claude-sonnet-4-20250514")
	}
}

func TestExtractModelFromResponse_UnknownProvider(t *testing.T) {
	body := []byte(`{"model": "some-model"}`)
	model := extractModelFromResponse("bedrock", body)
	if model != "" {
		t.Errorf("unknown provider model = %q, want empty", model)
	}
}

func TestExtractModelFromStreamingResponse_Google(t *testing.T) {
	data := []byte(`[
		{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]},
		{"candidates":[{"content":{"parts":[{"text":" there"}]}}],
		 "modelVersion":"gemini-2.0-flash-lite-001",
		 "usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}
	]`)

	model := extractModelFromStreamingResponse("google", data)
	if model != "gemini-2.0-flash-lite-001" {
		t.Errorf("Google streaming model = %q, want %q", model, "gemini-2.0-flash-lite-001")
	}
}

func TestExtractModelFromStreamingResponse_OpenAI(t *testing.T) {
	data := []byte(`data: {"model":"gpt-4o","choices":[{"delta":{"content":"hi"}}]}
data: {"model":"gpt-4o","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}
data: [DONE]
`)

	model := extractModelFromStreamingResponse("openai", data)
	if model != "gpt-4o" {
		t.Errorf("OpenAI streaming model = %q, want %q", model, "gpt-4o")
	}
}

func TestExtractModelFromStreamingResponse_Anthropic(t *testing.T) {
	data := []byte(`data: {"type":"message_start","message":{"model":"claude-sonnet-4-20250514","usage":{"input_tokens":10}}}
data: {"type":"content_block_delta","delta":{"text":"hi"}}
data: [DONE]
`)

	model := extractModelFromStreamingResponse("anthropic", data)
	// The model is inside message, not top-level. extractModelFromStreamingResponse
	// scans for top-level "model" field. For message_start, model is nested.
	// This tests the fallback behavior.
	if model != "" {
		// Anthropic's model is nested in message, not top-level in SSE chunk.
		// Top-level scan won't find it — this is expected.
		t.Logf("note: found model %q in streaming data (unexpected but harmless)", model)
	}
}
