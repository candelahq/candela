package proxy

import (
	"strings"
	"testing"
)

// FuzzOpenAIParseStreamingResponse exercises the OpenAI SSE parser with
// arbitrary data payloads. It must never panic regardless of input.
func FuzzOpenAIParseStreamingResponse(f *testing.F) {
	// Seed corpus: valid SSE events, malformed data, edge cases.
	f.Add([]byte(`data: {"choices":[{"delta":{"content":"hello"}}]}`))
	f.Add([]byte(`data: {"choices":[{"delta":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	f.Add([]byte("data: [DONE]\n"))
	f.Add([]byte("data: \n"))
	f.Add([]byte(""))
	f.Add([]byte("not a data line at all"))
	f.Add([]byte("data: {invalid json"))
	f.Add([]byte("data: null\n"))
	f.Add([]byte("data: {}\ndata: {}\ndata: [DONE]\n"))
	// Multi-line SSE stream.
	f.Add([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\ndata: [DONE]\n"))
	// Large token overflow attempt.
	f.Add([]byte(`data: {"usage":{"prompt_tokens":9999999999999999999,"completion_tokens":9999999999999999999}}`))
	// Nested JSON chaos.
	f.Add([]byte(`data: {"choices":[{"delta":{"content":"` + strings.Repeat("x", 10000) + `"}}]}`))

	parser := &openaiParser{}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic — that's the primary invariant.
		content, inputTokens, outputTokens := parser.ParseStreamingResponse(data)

		// Tokens must never be negative.
		if inputTokens < 0 {
			t.Errorf("inputTokens = %d, want >= 0", inputTokens)
		}
		if outputTokens < 0 {
			t.Errorf("outputTokens = %d, want >= 0", outputTokens)
		}

		// Content must be valid UTF-8 (Go strings always are, but check len sanity).
		_ = len(content)
	})
}

// FuzzAnthropicParseStreamingResponse exercises the Anthropic SSE parser.
func FuzzAnthropicParseStreamingResponse(f *testing.F) {
	f.Add([]byte(`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`))
	f.Add([]byte(`data: {"type":"message_delta","usage":{"output_tokens":50}}`))
	f.Add([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":100}}}`))
	f.Add([]byte("data: [DONE]\n"))
	f.Add([]byte(""))
	f.Add([]byte("data: {bad json"))
	f.Add([]byte("data: null"))
	f.Add([]byte(`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"` + strings.Repeat("a", 50000) + `"}}`))
	// Multi-event stream.
	f.Add([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5}}\n"))

	parser := &anthropicParser{}

	f.Fuzz(func(t *testing.T, data []byte) {
		content, inputTokens, outputTokens := parser.ParseStreamingResponse(data)

		if inputTokens < 0 {
			t.Errorf("inputTokens = %d, want >= 0", inputTokens)
		}
		if outputTokens < 0 {
			t.Errorf("outputTokens = %d, want >= 0", outputTokens)
		}
		_ = len(content)
	})
}

// FuzzOpenAIParseRequest exercises the OpenAI request parser.
func FuzzOpenAIParseRequest(f *testing.F) {
	f.Add([]byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}`))
	f.Add([]byte(`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(""))
	f.Add([]byte("{invalid"))
	f.Add([]byte(`{"model":"","messages":[]}`))
	f.Add([]byte(`{"model":"gpt-4o","messages":[{"role":"user","content":` + strings.Repeat(`"x"`, 1000) + `}]}`))

	parser := &openaiParser{}

	f.Fuzz(func(t *testing.T, data []byte) {
		model, content := parser.ParseRequest(data)
		_ = model
		_ = content
	})
}

// FuzzAnthropicParseRequest exercises the Anthropic request parser.
func FuzzAnthropicParseRequest(f *testing.F) {
	f.Add([]byte(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"Hello"}]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(""))
	f.Add([]byte("{bad"))
	f.Add([]byte(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`))

	parser := &anthropicParser{}

	f.Fuzz(func(t *testing.T, data []byte) {
		model, content := parser.ParseRequest(data)
		_ = model
		_ = content
	})
}

// FuzzOpenAIParseResponse exercises the OpenAI non-streaming response parser.
func FuzzOpenAIParseResponse(f *testing.F) {
	f.Add([]byte(`{"choices":[{"message":{"content":"Hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(""))
	f.Add([]byte("{bad"))
	f.Add([]byte(`{"usage":{"prompt_tokens":-1,"completion_tokens":-1}}`))

	parser := &openaiParser{}

	f.Fuzz(func(t *testing.T, data []byte) {
		content, inputTokens, outputTokens := parser.ParseResponse(data)
		_ = content
		if inputTokens < 0 {
			t.Errorf("inputTokens = %d, want >= 0", inputTokens)
		}
		if outputTokens < 0 {
			t.Errorf("outputTokens = %d, want >= 0", outputTokens)
		}
	})
}

// FuzzAnthropicParseResponse exercises the Anthropic non-streaming response parser.
func FuzzAnthropicParseResponse(f *testing.F) {
	f.Add([]byte(`{"content":[{"text":"Hello"}],"usage":{"input_tokens":10,"output_tokens":5}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(""))
	f.Add([]byte("{bad"))

	parser := &anthropicParser{}

	f.Fuzz(func(t *testing.T, data []byte) {
		content, inputTokens, outputTokens := parser.ParseResponse(data)
		_ = content
		if inputTokens < 0 {
			t.Errorf("inputTokens = %d, want >= 0", inputTokens)
		}
		if outputTokens < 0 {
			t.Errorf("outputTokens = %d, want >= 0", outputTokens)
		}
	})
}

// FuzzExtractModelFromStreamingResponse exercises model extraction from SSE data.
func FuzzExtractModelFromStreamingResponse(f *testing.F) {
	f.Add("openai", []byte(`data: {"model":"gpt-4o"}`))
	f.Add("anthropic", []byte(`data: {"type":"message_start","message":{"model":"claude-sonnet-4-20250514"}}`))
	f.Add("google", []byte(`data: {"modelVersion":"gemini-2.5-pro"}`))
	f.Add("unknown", []byte(""))
	f.Add("openai", []byte("data: {bad json"))
	f.Add("openai", []byte("not sse data"))

	f.Fuzz(func(t *testing.T, provider string, data []byte) {
		model := extractModelFromStreamingResponse(provider, data)
		_ = model
	})
}

// FuzzExtractStreamingCacheTokens exercises cache token extraction from SSE.
func FuzzExtractStreamingCacheTokens(f *testing.F) {
	f.Add("openai", []byte(`data: {"usage":{"prompt_tokens_details":{"cached_tokens":100}}}`))
	f.Add("anthropic", []byte(`data: {"type":"message_start","message":{"usage":{"cache_read_input_tokens":50,"cache_creation_input_tokens":25}}}`))
	f.Add("google", []byte(`data: {"usageMetadata":{"cachedContentTokenCount":200}}`))
	f.Add("unknown", []byte(""))
	f.Add("openai", []byte("data: {bad json"))

	f.Fuzz(func(t *testing.T, provider string, data []byte) {
		tokens := extractStreamingCacheTokens(provider, data)
		if tokens.CacheReadTokens < 0 {
			t.Errorf("CacheReadTokens = %d, want >= 0", tokens.CacheReadTokens)
		}
		if tokens.CacheCreationTokens < 0 {
			t.Errorf("CacheCreationTokens = %d, want >= 0", tokens.CacheCreationTokens)
		}
	})
}
