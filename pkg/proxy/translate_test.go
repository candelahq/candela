package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

// ====================================================================
// Model Name Parsing
// ====================================================================

func TestParseModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantVAI  string
		wantDisp string
	}{
		{
			name:     "claude-sonnet-4",
			input:    "claude-sonnet-4-20250514",
			wantVAI:  "claude-sonnet-4@20250514",
			wantDisp: "claude-sonnet-4",
		},
		{
			name:     "claude-opus-4",
			input:    "claude-opus-4-20250514",
			wantVAI:  "claude-opus-4@20250514",
			wantDisp: "claude-opus-4",
		},
		{
			name:     "claude-3-5-sonnet",
			input:    "claude-3-5-sonnet-20241022",
			wantVAI:  "claude-3-5-sonnet@20241022",
			wantDisp: "claude-3-5-sonnet",
		},
		{
			name:     "claude-3-5-haiku",
			input:    "claude-3-5-haiku-20241022",
			wantVAI:  "claude-3-5-haiku@20241022",
			wantDisp: "claude-3-5-haiku",
		},
		{
			name:     "no date suffix",
			input:    "claude-instant",
			wantVAI:  "claude-instant",
			wantDisp: "claude-instant",
		},
		{
			name:     "already has @version",
			input:    "claude-3-5-sonnet@20241022",
			wantVAI:  "claude-3-5-sonnet@20241022",
			wantDisp: "claude-3-5-sonnet@20241022",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ParseModelName(tt.input)
			if info.Raw != tt.input {
				t.Errorf("Raw = %q, want %q", info.Raw, tt.input)
			}
			if info.VertexAI != tt.wantVAI {
				t.Errorf("VertexAI = %q, want %q", info.VertexAI, tt.wantVAI)
			}
			if info.Display != tt.wantDisp {
				t.Errorf("Display = %q, want %q", info.Display, tt.wantDisp)
			}
		})
	}
}

// ====================================================================
// Request Translation: OpenAI → Anthropic
// ====================================================================

func TestTranslateRequest_Basic(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "user", "content": "Hello"}
		],
		"max_tokens": 1024
	}`

	translated, model, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	if model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %q, want claude-sonnet-4-20250514", model)
	}

	var result anthropicRequest
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatalf("failed to parse translated body: %v", err)
	}

	// Model should NOT be in the body — Vertex AI gets it from the URL path.
	if result.Model != "" {
		t.Errorf("anthropic model should be empty (Vertex AI uses URL), got %q", result.Model)
	}
	if result.MaxTokens != 1024 {
		t.Errorf("max_tokens = %d, want 1024", result.MaxTokens)
	}
	if result.AnthropicVersion != "vertex-2023-10-16" {
		t.Errorf("anthropic_version = %q", result.AnthropicVersion)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Role != "user" {
		t.Errorf("message role = %q", result.Messages[0].Role)
	}
}

func TestTranslateRequest_SystemMessage_CachingOff(t *testing.T) {
	translator := &AnthropicFormatTranslator{}
	translator.SetCachingMode(CachingOff)

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	var result anthropicRequest
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// With CachingOff, system should be a plain string.
	sysStr, ok := result.System.(string)
	if !ok {
		t.Fatalf("system should be a string when CachingOff, got %T", result.System)
	}
	if sysStr != "You are a helpful assistant." {
		t.Errorf("system = %q, want 'You are a helpful assistant.'", sysStr)
	}
	// System message should NOT appear in messages array.
	if len(result.Messages) != 1 {
		t.Errorf("messages len = %d, want 1 (system extracted)", len(result.Messages))
	}
}

func TestTranslateRequest_PromptCaching(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there!"},
			{"role": "user", "content": "How are you?"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	// Parse as raw JSON to inspect cache_control injection.
	var raw map[string]interface{}
	if err := json.Unmarshal(translated, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// System should be a content block array with cache_control.
	sysBlocks, ok := raw["system"].([]interface{})
	if !ok {
		t.Fatalf("system should be []interface{} with caching enabled (default), got %T", raw["system"])
	}
	if len(sysBlocks) != 1 {
		t.Fatalf("system blocks = %d, want 1", len(sysBlocks))
	}
	sysBlock := sysBlocks[0].(map[string]interface{})
	if sysBlock["type"] != "text" {
		t.Errorf("system block type = %v, want text", sysBlock["type"])
	}
	if sysBlock["text"] != "You are a helpful assistant." {
		t.Errorf("system block text = %v", sysBlock["text"])
	}
	cc, ok := sysBlock["cache_control"].(map[string]interface{})
	if !ok {
		t.Fatal("system block missing cache_control")
	}
	if cc["type"] != "ephemeral" {
		t.Errorf("cache_control type = %v, want ephemeral", cc["type"])
	}

	// Last user message should have cache_control on its content.
	messages := raw["messages"].([]interface{})
	lastMsg := messages[len(messages)-1].(map[string]interface{})
	lastContent := lastMsg["content"].([]interface{})
	lastBlock := lastContent[len(lastContent)-1].(map[string]interface{})
	msgCC, ok := lastBlock["cache_control"].(map[string]interface{})
	if !ok {
		t.Fatal("last user message missing cache_control")
	}
	if msgCC["type"] != "ephemeral" {
		t.Errorf("last message cache_control type = %v, want ephemeral", msgCC["type"])
	}
}

func TestTranslateRequest_SystemMessageArray(t *testing.T) {
	// OpenAI allows system content as an array of content blocks.
	translator := &AnthropicFormatTranslator{}

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": [
				{"type": "text", "text": "You are a helpful assistant."},
				{"type": "text", "text": "Always be concise."}
			]},
			{"role": "user", "content": "Hello"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(translated, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// System should be passed through as an array of content blocks.
	sysBlocks, ok := raw["system"].([]interface{})
	if !ok {
		t.Fatalf("system should be []interface{} for array input, got %T", raw["system"])
	}
	if len(sysBlocks) != 2 {
		t.Fatalf("system blocks = %d, want 2", len(sysBlocks))
	}

	// Messages should not contain the system message.
	messages := raw["messages"].([]interface{})
	if len(messages) != 1 {
		t.Errorf("messages len = %d, want 1 (system extracted)", len(messages))
	}
}

func TestTranslateRequest_SystemMessageArrayWithCaching(t *testing.T) {
	// Array system content should add cache_control to last block by default.
	translator := &AnthropicFormatTranslator{}

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": [
				{"type": "text", "text": "You are a helpful assistant."},
				{"type": "text", "text": "Always be concise."}
			]},
			{"role": "user", "content": "Hello"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(translated, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	sysBlocks, ok := raw["system"].([]interface{})
	if !ok {
		t.Fatalf("system should be []interface{}, got %T", raw["system"])
	}
	if len(sysBlocks) != 2 {
		t.Fatalf("system blocks = %d, want 2", len(sysBlocks))
	}

	// First block should NOT have cache_control.
	firstBlock := sysBlocks[0].(map[string]interface{})
	if _, hasCc := firstBlock["cache_control"]; hasCc {
		t.Error("first block should not have cache_control")
	}

	// Last block should have cache_control.
	lastBlock := sysBlocks[1].(map[string]interface{})
	cc, ok := lastBlock["cache_control"].(map[string]interface{})
	if !ok {
		t.Fatal("last system block missing cache_control")
	}
	if cc["type"] != "ephemeral" {
		t.Errorf("cache_control type = %v, want ephemeral", cc["type"])
	}
}

func TestTranslateRequest_CachingSystemOnly(t *testing.T) {
	// system-only mode: system prompt gets cache_control, but last user message does NOT.
	translator := &AnthropicFormatTranslator{}
	translator.SetCachingMode(CachingSystemOnly)

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there!"},
			{"role": "user", "content": "How are you?"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(translated, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// System should have cache_control.
	sysBlocks, ok := raw["system"].([]interface{})
	if !ok {
		t.Fatalf("system should be []interface{} with CachingSystemOnly, got %T", raw["system"])
	}
	sysBlock := sysBlocks[0].(map[string]interface{})
	if _, hasCc := sysBlock["cache_control"]; !hasCc {
		t.Fatal("system block should have cache_control in system-only mode")
	}

	// Last user message should NOT have cache_control.
	messages := raw["messages"].([]interface{})
	lastMsg := messages[len(messages)-1].(map[string]interface{})
	lastContent, ok := lastMsg["content"].([]interface{})
	if ok {
		lastBlock := lastContent[len(lastContent)-1].(map[string]interface{})
		if _, hasCc := lastBlock["cache_control"]; hasCc {
			t.Error("last user message should NOT have cache_control in system-only mode")
		}
	}
}

func TestParseCachingMode(t *testing.T) {
	tests := []struct {
		input string
		want  CachingMode
	}{
		{"auto", CachingAuto},
		{"Auto", CachingAuto},
		{"AUTO", CachingAuto},
		{"true", CachingAuto},
		{"1", CachingAuto},
		{"system-only", CachingSystemOnly},
		{"system_only", CachingSystemOnly},
		{"system", CachingSystemOnly},
		{"off", CachingOff},
		{"false", CachingOff},
		{"0", CachingOff},
		{"", CachingOff},
		{"unknown", CachingOff},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseCachingMode(tt.input)
			if got != tt.want {
				t.Errorf("ParseCachingMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTranslateRequest_DefaultMaxTokens(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [{"role": "user", "content": "Hi"}]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	var result anthropicRequest
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.MaxTokens != 4096 {
		t.Errorf("max_tokens = %d, want 4096 (default)", result.MaxTokens)
	}
}

func TestTranslateRequest_Stream(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [{"role": "user", "content": "Hi"}],
		"stream": true
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	var result anthropicRequest
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !result.Stream {
		t.Error("stream should be true")
	}
}

// ====================================================================
// Response Translation: Anthropic → OpenAI
// ====================================================================

func TestTranslateResponse_Standard(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	anthResp := `{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "Hello! How can I help?"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 8}
	}`

	translated, err := translator.TranslateResponse([]byte(anthResp), "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("TranslateResponse failed: %v", err)
	}

	var result openAIResponse
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Object != "chat.completion" {
		t.Errorf("object = %q, want chat.completion", result.Object)
	}
	// Should use clean display name.
	if result.Model != "claude-sonnet-4" {
		t.Errorf("model = %q, want claude-sonnet-4 (cleaned)", result.Model)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("choices len = %d, want 1", len(result.Choices))
	}
	if result.Choices[0].Message.Content != "Hello! How can I help?" {
		t.Errorf("content = %q", result.Choices[0].Message.Content)
	}
	if result.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason = %q, want stop", result.Choices[0].FinishReason)
	}
	if result.Usage.PromptTokens != 10 {
		t.Errorf("prompt_tokens = %d, want 10", result.Usage.PromptTokens)
	}
	if result.Usage.CompletionTokens != 8 {
		t.Errorf("completion_tokens = %d, want 8", result.Usage.CompletionTokens)
	}
	if result.Usage.TotalTokens != 18 {
		t.Errorf("total_tokens = %d, want 18", result.Usage.TotalTokens)
	}
}

func TestTranslateResponse_MaxTokensStop(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	anthResp := `{
		"id": "msg_456",
		"type": "message",
		"role": "assistant",
		"content": [{"type": "text", "text": "truncated output"}],
		"stop_reason": "max_tokens",
		"usage": {"input_tokens": 5, "output_tokens": 100}
	}`

	translated, err := translator.TranslateResponse([]byte(anthResp), "claude-3-5-sonnet-20241022")
	if err != nil {
		t.Fatalf("TranslateResponse failed: %v", err)
	}

	var result openAIResponse
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Choices[0].FinishReason != "length" {
		t.Errorf("finish_reason = %q, want length", result.Choices[0].FinishReason)
	}
	if result.Model != "claude-3-5-sonnet" {
		t.Errorf("model = %q, want claude-3-5-sonnet", result.Model)
	}
}

// ====================================================================
// Stream Chunk Translation: Anthropic SSE → OpenAI SSE
// ====================================================================

func TestTranslateStreamChunk_ContentDelta(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	chunk := `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`

	translated, err := translator.TranslateStreamChunk([]byte(chunk), "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("TranslateStreamChunk failed: %v", err)
	}

	result := string(translated)
	if !strings.Contains(result, `"content":"Hello"`) {
		t.Errorf("expected OpenAI delta content, got: %s", result)
	}
	if !strings.Contains(result, `"model":"claude-sonnet-4"`) {
		t.Errorf("expected clean model name, got: %s", result)
	}
}

func TestTranslateStreamChunk_MessageStop(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

	chunk := `data: {"type":"message_stop"}`

	translated, err := translator.TranslateStreamChunk([]byte(chunk), "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("TranslateStreamChunk failed: %v", err)
	}

	if !strings.Contains(string(translated), "data: [DONE]") {
		t.Errorf("expected [DONE], got: %s", translated)
	}
}

// ====================================================================
// Vertex AI Path Rewriting
// ====================================================================

func TestVertexAIPathRewriter(t *testing.T) {
	rewriter := &VertexAIPathRewriter{
		ProjectID: "my-project",
		Region:    "us-central1",
	}

	tests := []struct {
		name      string
		model     string
		streaming bool
		want      string
	}{
		{
			name:      "standard request",
			model:     "claude-sonnet-4-20250514",
			streaming: false,
			want:      "/v1/projects/my-project/locations/us-central1/publishers/anthropic/models/claude-sonnet-4@20250514:rawPredict",
		},
		{
			name:      "streaming request",
			model:     "claude-sonnet-4-20250514",
			streaming: true,
			want:      "/v1/projects/my-project/locations/us-central1/publishers/anthropic/models/claude-sonnet-4@20250514:streamRawPredict",
		},
		{
			name:      "older model version",
			model:     "claude-3-5-sonnet-20241022",
			streaming: false,
			want:      "/v1/projects/my-project/locations/us-central1/publishers/anthropic/models/claude-3-5-sonnet@20241022:rawPredict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriter.RewritePath(tt.model, tt.streaming)
			if got != tt.want {
				t.Errorf("RewritePath = %q, want %q", got, tt.want)
			}
		})
	}
}
