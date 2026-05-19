package proxy

import (
	"encoding/json"
	"testing"
)

// ── Gemini parser: standard response ────────────────────────────────────────

func TestGeminiParser_ParseResponse_Basic(t *testing.T) {
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "Hello, world!"}]}}],
		"usageMetadata": {
			"promptTokenCount": 42,
			"candidatesTokenCount": 10,
			"totalTokenCount": 52
		}
	}`)

	parser := &googleParser{}
	content, inputTokens, outputTokens := parser.ParseResponse(body)

	if content != "Hello, world!" {
		t.Errorf("content = %q, want %q", content, "Hello, world!")
	}
	if inputTokens != 42 {
		t.Errorf("inputTokens = %d, want 42", inputTokens)
	}
	if outputTokens != 10 {
		t.Errorf("outputTokens = %d, want 10", outputTokens)
	}
}

func TestGeminiParser_ParseResponse_EmptyContent(t *testing.T) {
	body := []byte(`{
		"candidates": [],
		"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 0}
	}`)

	parser := &googleParser{}
	content, inputTokens, outputTokens := parser.ParseResponse(body)

	if content != "" {
		t.Errorf("content = %q, want empty", content)
	}
	if inputTokens != 10 {
		t.Errorf("inputTokens = %d, want 10", inputTokens)
	}
	if outputTokens != 0 {
		t.Errorf("outputTokens = %d, want 0", outputTokens)
	}
}

func TestGeminiParser_ParseResponse_InvalidJSON(t *testing.T) {
	parser := &googleParser{}
	content, inputTokens, outputTokens := parser.ParseResponse([]byte("not json"))

	if content != "" || inputTokens != 0 || outputTokens != 0 {
		t.Error("invalid JSON should return zero values")
	}
}

func TestGeminiParser_ParseResponse_MissingUsage(t *testing.T) {
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "hi"}]}}]
	}`)

	parser := &googleParser{}
	content, inputTokens, outputTokens := parser.ParseResponse(body)

	if content != "hi" {
		t.Errorf("content = %q, want %q", content, "hi")
	}
	if inputTokens != 0 || outputTokens != 0 {
		t.Errorf("missing usageMetadata should give 0 tokens, got in=%d out=%d",
			inputTokens, outputTokens)
	}
}

// ── Gemini thinking tokens (2.5+ reasoning models) ─────────────────────────

func TestGeminiParser_ThinkingTokens_AddedToOutput(t *testing.T) {
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "result"}]}}],
		"usageMetadata": {
			"promptTokenCount": 100,
			"candidatesTokenCount": 200,
			"thoughtsTokenCount": 3000,
			"totalTokenCount": 3300
		}
	}`)

	parser := &googleParser{}
	_, inputTokens, outputTokens := parser.ParseResponse(body)

	if inputTokens != 100 {
		t.Errorf("inputTokens = %d, want 100", inputTokens)
	}
	// Output must include thinking: 200 + 3000 = 3200
	if outputTokens != 3200 {
		t.Errorf("outputTokens = %d, want 3200 (200 candidates + 3000 thinking)", outputTokens)
	}
}

func TestGeminiParser_ThinkingTokens_StreamingAddedToOutput(t *testing.T) {
	data := []byte(`[
		{"candidates":[{"content":{"parts":[{"text":"step1"}]}}]},
		{"candidates":[{"content":{"parts":[{"text":"step2"}]}}],
		 "usageMetadata":{
			"promptTokenCount":50,
			"candidatesTokenCount":100,
			"thoughtsTokenCount":1500,
			"totalTokenCount":1650
		}}
	]`)

	parser := &googleParser{}
	content, inputTokens, outputTokens := parser.ParseStreamingResponse(data)

	if content != "step1step2" {
		t.Errorf("content = %q, want %q", content, "step1step2")
	}
	if inputTokens != 50 {
		t.Errorf("inputTokens = %d, want 50", inputTokens)
	}
	if outputTokens != 1600 {
		t.Errorf("outputTokens = %d, want 1600 (100 + 1500 thinking)", outputTokens)
	}
}

// ── Gemini implicit caching (passive — no injection needed) ─────────────────

func TestGeminiParser_ImplicitCaching_ResponseTokens(t *testing.T) {
	// Gemini 2.5+ automatically caches repeated prefixes (implicit caching).
	// The proxy doesn't inject anything — it just reads cachedContentTokenCount
	// from the response and passes it to the cost calculator.
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "cached result"}]}}],
		"modelVersion": "gemini-2.5-flash-preview-05-20",
		"usageMetadata": {
			"promptTokenCount": 10000,
			"cachedContentTokenCount": 8000,
			"candidatesTokenCount": 200,
			"totalTokenCount": 10200
		}
	}`)

	parser := &googleParser{}
	_, inputTokens, outputTokens := parser.ParseResponse(body)

	// Parser returns raw promptTokenCount (inclusive of cached tokens).
	if inputTokens != 10000 {
		t.Errorf("inputTokens = %d, want 10000 (raw, includes cached)", inputTokens)
	}
	if outputTokens != 200 {
		t.Errorf("outputTokens = %d, want 200", outputTokens)
	}

	// Verify cache token extraction separately.
	ct := extractCacheTokens("google", body)
	if ct.CacheReadTokens != 8000 {
		t.Errorf("CacheReadTokens = %d, want 8000", ct.CacheReadTokens)
	}
	if ct.CacheCreationTokens != 0 {
		t.Errorf("CacheCreationTokens = %d, want 0 (Google has no creation concept)", ct.CacheCreationTokens)
	}
}

func TestGeminiParser_ImplicitCaching_StreamingTokens(t *testing.T) {
	data := []byte(`[
		{"candidates":[{"content":{"parts":[{"text":"chunk1"}]}}]},
		{"candidates":[{"content":{"parts":[{"text":"chunk2"}]}}],
		 "modelVersion":"gemini-2.5-pro-preview-05-06",
		 "usageMetadata":{
			"promptTokenCount":20000,
			"cachedContentTokenCount":15000,
			"candidatesTokenCount":500,
			"thoughtsTokenCount":2000,
			"totalTokenCount":22500
		}}
	]`)

	// Verify streaming cache extraction.
	ct := extractStreamingCacheTokens("google", data)
	if ct.CacheReadTokens != 15000 {
		t.Errorf("streaming CacheReadTokens = %d, want 15000", ct.CacheReadTokens)
	}
}

// ── Gemini streaming: format variants ───────────────────────────────────────

func TestGeminiParser_Streaming_JSONArray(t *testing.T) {
	// Google streaming returns a JSON array of objects.
	data := []byte(`[
		{"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]},
		{"candidates":[{"content":{"parts":[{"text":" world"}]}}],
		 "usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}
	]`)

	parser := &googleParser{}
	content, in, out := parser.ParseStreamingResponse(data)

	if content != "Hello world" {
		t.Errorf("content = %q, want %q", content, "Hello world")
	}
	if in != 10 {
		t.Errorf("inputTokens = %d, want 10", in)
	}
	if out != 5 {
		t.Errorf("outputTokens = %d, want 5", out)
	}
}

func TestGeminiParser_Streaming_NDJSON(t *testing.T) {
	// Fallback: newline-delimited JSON (not a JSON array).
	data := []byte(`{"candidates":[{"content":{"parts":[{"text":"A"}]}}]}
{"candidates":[{"content":{"parts":[{"text":"B"}]}}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":8}}
`)

	parser := &googleParser{}
	content, in, out := parser.ParseStreamingResponse(data)

	if content != "AB" {
		t.Errorf("content = %q, want %q", content, "AB")
	}
	if in != 20 {
		t.Errorf("inputTokens = %d, want 20", in)
	}
	if out != 8 {
		t.Errorf("outputTokens = %d, want 8", out)
	}
}

func TestGeminiParser_Streaming_EmptyArray(t *testing.T) {
	parser := &googleParser{}
	content, in, out := parser.ParseStreamingResponse([]byte(`[]`))

	if content != "" || in != 0 || out != 0 {
		t.Error("empty array should return zero values")
	}
}

// ── Gemini IsStreaming always false (streaming is URL-based) ─────────────────

func TestGeminiParser_IsStreaming_AlwaysFalse(t *testing.T) {
	parser := &googleParser{}

	// Google uses separate endpoints for streaming, not a body param.
	bodies := [][]byte{
		[]byte(`{"contents":[{"parts":[{"text":"hi"}]}]}`),
		[]byte(`{"contents":[{"parts":[{"text":"hi"}]}],"stream":true}`),
		[]byte(`{}`),
	}
	for _, body := range bodies {
		if parser.IsStreaming(body) {
			t.Errorf("IsStreaming should always be false for Google, got true for %s", body)
		}
	}
}

// ── Gemini ParseRequest ─────────────────────────────────────────────────────

func TestGeminiParser_ParseRequest(t *testing.T) {
	body := []byte(`{
		"contents": [
			{"role": "user", "parts": [{"text": "What is the meaning of life?"}]}
		],
		"generationConfig": {"temperature": 0.7}
	}`)

	parser := &googleParser{}
	model, content := parser.ParseRequest(body)

	// Google doesn't include model in the request body (it's in the URL path).
	if model != "" {
		t.Errorf("model = %q, want empty (Google has model in URL, not body)", model)
	}
	if content == "" {
		t.Error("content should not be empty")
	}
}

// ── Model version extraction ────────────────────────────────────────────────

func TestGeminiModelVersion_StandardResponse(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantModel string
	}{
		{
			name:      "2.5 Pro preview",
			body:      `{"modelVersion":"gemini-2.5-pro-preview-05-06","candidates":[]}`,
			wantModel: "gemini-2.5-pro-preview-05-06",
		},
		{
			name:      "2.5 Flash",
			body:      `{"modelVersion":"gemini-2.5-flash","candidates":[]}`,
			wantModel: "gemini-2.5-flash",
		},
		{
			name:      "2.0 Flash Lite",
			body:      `{"modelVersion":"gemini-2.0-flash-lite-001","candidates":[]}`,
			wantModel: "gemini-2.0-flash-lite-001",
		},
		{
			name:      "3.0 Flash Preview",
			body:      `{"modelVersion":"gemini-3-flash-preview","candidates":[]}`,
			wantModel: "gemini-3-flash-preview",
		},
		{
			name:      "3.0 Pro Preview",
			body:      `{"modelVersion":"gemini-3-pro-preview","candidates":[]}`,
			wantModel: "gemini-3-pro-preview",
		},
		{
			name:      "3.1 Pro",
			body:      `{"modelVersion":"gemini-3.1-pro","candidates":[]}`,
			wantModel: "gemini-3.1-pro",
		},
		{
			name:      "3.1 Flash",
			body:      `{"modelVersion":"gemini-3.1-flash","candidates":[]}`,
			wantModel: "gemini-3.1-flash",
		},
		{
			name:      "missing modelVersion",
			body:      `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`,
			wantModel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractModelFromResponse("google", []byte(tt.body))
			if got != tt.wantModel {
				t.Errorf("extractModelFromResponse = %q, want %q", got, tt.wantModel)
			}
		})
	}
}

func TestGeminiModelVersion_StreamingResponse(t *testing.T) {
	// modelVersion appears in the last chunk of a streaming response.
	data := []byte(`[
		{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]},
		{"candidates":[],
		 "modelVersion":"gemini-2.5-flash-preview-05-20",
		 "usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}
	]`)

	model := extractModelFromStreamingResponse("google", data)
	if model != "gemini-2.5-flash-preview-05-20" {
		t.Errorf("streaming model = %q, want %q", model, "gemini-2.5-flash-preview-05-20")
	}
}

// ── gemini-oai (OpenAI-compat) uses openaiParser ────────────────────────────

func TestGeminiOAI_ParseResponse(t *testing.T) {
	// Gemini's OpenAI-compat endpoint returns standard OpenAI format.
	body := []byte(`{
		"model": "gemini-2.5-flash",
		"choices": [{"message": {"role": "assistant", "content": "Gemini response"}}],
		"usage": {
			"prompt_tokens": 100,
			"completion_tokens": 50,
			"total_tokens": 150
		}
	}`)

	parser := getParser("gemini-oai")
	content, inputTokens, outputTokens := parser.ParseResponse(body)

	if content != "Gemini response" {
		t.Errorf("content = %q, want %q", content, "Gemini response")
	}
	if inputTokens != 100 {
		t.Errorf("inputTokens = %d, want 100", inputTokens)
	}
	if outputTokens != 50 {
		t.Errorf("outputTokens = %d, want 50", outputTokens)
	}
}

func TestGeminiOAI_StreamUsageInjection(t *testing.T) {
	body := []byte(`{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	result := injectStreamUsageOption("gemini-oai", body)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	so, ok := parsed["stream_options"].(map[string]interface{})
	if !ok {
		t.Fatal("stream_options should be injected for gemini-oai")
	}
	if so["include_usage"] != true {
		t.Errorf("include_usage = %v, want true", so["include_usage"])
	}
}

// ── gemini-oai cache tokens via OpenAI format ───────────────────────────────

func TestGeminiOAI_CacheTokens(t *testing.T) {
	// Gemini OAI-compat reports cached tokens in the OpenAI format.
	body := []byte(`{
		"usage": {
			"prompt_tokens": 5000,
			"completion_tokens": 100,
			"prompt_tokens_details": {"cached_tokens": 4000}
		}
	}`)

	ct := extractCacheTokens("gemini-oai", body)
	if ct.CacheReadTokens != 4000 {
		t.Errorf("CacheReadTokens = %d, want 4000", ct.CacheReadTokens)
	}
}

func TestGeminiOAI_StreamingCacheTokens(t *testing.T) {
	stream := []byte(`data: {"choices":[{"delta":{"content":"hi"}}]}
data: {"choices":[],"usage":{"prompt_tokens":5000,"completion_tokens":100,"prompt_tokens_details":{"cached_tokens":4000}}}
data: [DONE]
`)

	ct := extractStreamingCacheTokens("gemini-oai", stream)
	if ct.CacheReadTokens != 4000 {
		t.Errorf("streaming CacheReadTokens = %d, want 4000", ct.CacheReadTokens)
	}
}

// ── Gemini: isAnthropicProvider should NOT match ────────────────────────────

func TestGeminiProviders_NotAnthropic(t *testing.T) {
	for _, name := range []string{"google", "gemini-oai", "gemini-direct"} {
		if isAnthropicProvider(name) {
			t.Errorf("isAnthropicProvider(%q) = true, want false", name)
		}
	}
}

// ── Gemini: stream_options injection only for OAI-compat ────────────────────

func TestGeminiNative_NoStreamUsageInjection(t *testing.T) {
	// Native Google API doesn't use stream_options (it always returns usage).
	body := []byte(`{"contents":[{"parts":[{"text":"hi"}]}]}`)
	result := injectStreamUsageOption("google", body)

	// Should be unchanged — injectStreamUsageOption only fires for openai/gemini-oai.
	if string(result) != string(body) {
		t.Error("google native body should not be modified by injectStreamUsageOption")
	}
}

// ── Multi-part response (multiple text parts) ───────────────────────────────

func TestGeminiParser_MultiPartResponse(t *testing.T) {
	body := []byte(`{
		"candidates": [{
			"content": {
				"parts": [
					{"text": "Part 1. "},
					{"text": "Part 2."}
				]
			}
		}],
		"usageMetadata": {
			"promptTokenCount": 30,
			"candidatesTokenCount": 15,
			"totalTokenCount": 45
		}
	}`)

	parser := &googleParser{}
	content, _, _ := parser.ParseResponse(body)

	// googleParser concatenates all text parts from multi-part responses.
	if content != "Part 1. Part 2." {
		t.Errorf("content = %q, want %q (concatenated parts)", content, "Part 1. Part 2.")
	}
}
