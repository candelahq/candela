package proxy

import (
	"encoding/json"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: isSafeModelName tests
// ──────────────────────────────────────────────────────────────────────────────

func TestIsSafeModelName_Audit(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{"normal", "gemini-2.5-flash", true},
		{"with_at", "claude-sonnet-4@20250514", true},
		{"with_slash", "deepseek-ai/deepseek-chat", true},
		{"with_dots", "claude-opus-4.7", true},
		{"path_traversal", "../../../etc/passwd", false},
		{"double_dot_middle", "model..hack", false},
		{"empty", "", false},
		{"with_space", "model name", false},
		{"with_newline", "model\nname", false},
		{"with_semicolon", "model;rm -rf /", false},
		{"unicode", "модель", false},
		{"colon_prefix", "ft:gpt-4.1:org:custom:id", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSafeModelName(tt.model)
			if got != tt.want {
				t.Errorf("isSafeModelName(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: isUtilityEndpoint tests
// ──────────────────────────────────────────────────────────────────────────────

func TestIsUtilityEndpoint_Audit(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/v1/models", true},
		{"/v1beta/models/gemini-2.5-flash/count_tokens", true},
		{"/v1/tokenize", true},
		{"/proxy/openai/v1/models", true},
		{"/proxy/openai/v1/chat/completions", false},
		{"/v1/chat/completions", false},
		{"/healthz", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isUtilityEndpoint(tt.path)
			if got != tt.want {
				t.Errorf("isUtilityEndpoint(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: extractModelFromURLPath tests
// ──────────────────────────────────────────────────────────────────────────────

func TestExtractModelFromURLPath_Audit(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"gemini_generate", "/v1beta/models/gemini-2.5-flash:generateContent", "gemini-2.5-flash"},
		{"gemini_stream", "/v1/models/gemini-2.5-pro:streamGenerateContent", "gemini-2.5-pro"},
		{"no_method", "/v1/models/gemini-2.5-pro", "gemini-2.5-pro"},
		{"no_models", "/v1/chat/completions", ""},
		{"empty", "", ""},
		{"multiple_models", "/v1/models/old/v2/models/new:gen", "new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractModelFromURLPath(tt.path)
			if got != tt.want {
				t.Errorf("extractModelFromURLPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: pricingProvider tests
// ──────────────────────────────────────────────────────────────────────────────

func TestPricingProvider_Audit(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"gemini-vertex", "google"},
		{"gemini-oai", "google"},
		{"anthropic-vertex", "anthropic"},
		{"anthropic-direct", "anthropic"},
		{"anthropic-bedrock", "anthropic"},
		{"openai", "openai"},
		{"google", "google"},
		{"unknown", "unknown"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := pricingProvider(tt.provider)
			if got != tt.want {
				t.Errorf("pricingProvider(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: buildModelsResponse empty slice — Issue #9
// ──────────────────────────────────────────────────────────────────────────────

func TestBuildModelsResponse_EmptySlice(t *testing.T) {
	result := buildModelsResponse(nil)

	var parsed struct {
		Object string          `json:"object"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Issue #9: "data" must be [] not null, for OpenAI-compatible clients.
	if string(parsed.Data) == "null" {
		t.Error("buildModelsResponse(nil) produced \"data\":null, want \"data\":[]")
	}
	if string(parsed.Data) != "[]" {
		t.Errorf("buildModelsResponse(nil) data = %s, want []", parsed.Data)
	}
}

func TestBuildModelsResponse_WithModels(t *testing.T) {
	models := []CompatModel{
		{ID: "gemini-2.5-pro", Provider: "google"},
		{ID: "claude-sonnet-4", Provider: "anthropic"},
	}
	result := buildModelsResponse(models)

	var parsed struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(parsed.Data) != 2 {
		t.Errorf("buildModelsResponse returned %d models, want 2", len(parsed.Data))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: rewriteModelField tests — Issue #6
// ──────────────────────────────────────────────────────────────────────────────

func TestRewriteModelField_BasicRewrite(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[]}`)
	got := rewriteModelField(body, "gpt-4.1")
	var parsed struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if parsed.Model != "gpt-4.1" {
		t.Errorf("rewriteModelField model = %q, want %q", parsed.Model, "gpt-4.1")
	}
}

func TestRewriteModelField_NoModelKey(t *testing.T) {
	body := []byte(`{"messages":[]}`)
	got := rewriteModelField(body, "gpt-4.1")
	// Should return body unchanged when no model key.
	if string(got) != string(body) {
		t.Errorf("rewriteModelField without model key changed body")
	}
}

func TestRewriteModelField_PrettyPrinted(t *testing.T) {
	body := []byte(`{
  "model" : "gpt-4o",
  "messages": []
}`)
	got := rewriteModelField(body, "claude-sonnet-4")
	var parsed struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if parsed.Model != "claude-sonnet-4" {
		t.Errorf("rewriteModelField pretty = %q, want %q", parsed.Model, "claude-sonnet-4")
	}
}
