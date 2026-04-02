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

	if result.Model != "claude-sonnet-4-20250514" {
		t.Errorf("anthropic model = %q", result.Model)
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

func TestTranslateRequest_SystemMessage(t *testing.T) {
	translator := &AnthropicFormatTranslator{}

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

	if result.System != "You are a helpful assistant." {
		t.Errorf("system = %q, want 'You are a helpful assistant.'", result.System)
	}
	// System message should NOT appear in messages array.
	if len(result.Messages) != 1 {
		t.Errorf("messages len = %d, want 1 (system extracted)", len(result.Messages))
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
