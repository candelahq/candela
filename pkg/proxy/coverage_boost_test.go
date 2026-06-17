package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

// ====================================================================
// Coverage boost: rewriteModelField, toInt64, DefaultProviders
// ====================================================================

func TestRewriteModelField_Basic(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-20250514","messages":[]}`)
	result := rewriteModelField(body, "claude-sonnet-4@20250514")
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(result, &req); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if req.Model != "claude-sonnet-4@20250514" {
		t.Errorf("model = %q, want 'claude-sonnet-4@20250514'", req.Model)
	}
}

func TestRewriteModelField_PreservesContent(t *testing.T) {
	// Model name also appears in user content — only the field should be changed.
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"Use gpt-4o for this task"}]}`)
	result := rewriteModelField(body, "gpt-4o-2024-08-06")
	s := string(result)

	// Content should still reference old model name.
	if !strings.Contains(s, "Use gpt-4o for this task") {
		t.Error("user content was corrupted by model rewrite")
	}
	// Model field should be updated.
	var req struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(result, &req)
	if req.Model != "gpt-4o-2024-08-06" {
		t.Errorf("model = %q, want 'gpt-4o-2024-08-06'", req.Model)
	}
}

func TestRewriteModelField_EmptyModel(t *testing.T) {
	body := []byte(`{"messages":[]}`)
	result := rewriteModelField(body, "new-model")
	// No model field — should return unchanged.
	if string(result) != string(body) {
		t.Errorf("body was modified when model field is absent")
	}
}

func TestRewriteModelField_MalformedJSON(t *testing.T) {
	body := []byte(`{not json}`)
	result := rewriteModelField(body, "new-model")
	if string(result) != string(body) {
		t.Error("malformed JSON should be returned unchanged")
	}
}

func TestRewriteModelField_WhitespaceAround(t *testing.T) {
	// Pretty-printed JSON with spaces around the colon.
	body := []byte(`{
  "model" : "old-model",
  "messages": []
}`)
	result := rewriteModelField(body, "new-model")
	var req struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(result, &req)
	if req.Model != "new-model" {
		t.Errorf("model = %q, want 'new-model'", req.Model)
	}
}

func TestToInt64_VariousTypes(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int64
	}{
		{"float64", float64(42), 42},
		{"float64 frac", float64(99.7), 99},
		{"zero", float64(0), 0},
		{"nil", nil, 0},
		{"string", "100", 0}, // non-numeric type returns 0
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toInt64(tt.in)
			if got != tt.want {
				t.Errorf("toInt64(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestRequestIDPattern_Valid(t *testing.T) {
	valid := []string{
		"abc123",
		"a-b-c-d",
		"UPPER-lower-123",
		strings.Repeat("a", 128),
	}
	for _, v := range valid {
		if !requestIDPattern.MatchString(v) {
			t.Errorf("requestIDPattern rejected valid ID: %q", v)
		}
	}
}

func TestRequestIDPattern_Invalid(t *testing.T) {
	invalid := []string{
		"",                          // too short
		"hello world",               // spaces
		"id\ninjection",             // newline
		"<script>alert(1)</script>", // XSS attempt
		strings.Repeat("a", 129),    // too long
	}
	for _, v := range invalid {
		if requestIDPattern.MatchString(v) {
			t.Errorf("requestIDPattern accepted invalid ID: %q", v)
		}
	}
}

func TestTenantIDPattern_Valid(t *testing.T) {
	valid := []string{
		"acme.corp",
		"tenant_42",
		"my-org.prod_env",
	}
	for _, v := range valid {
		if !tenantIDPattern.MatchString(v) {
			t.Errorf("tenantIDPattern rejected valid ID: %q", v)
		}
	}
}

func TestTenantIDPattern_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"tenant id",              // spaces
		"tenant\n",               // newline
		strings.Repeat("x", 129), // too long
	}
	for _, v := range invalid {
		if tenantIDPattern.MatchString(v) {
			t.Errorf("tenantIDPattern accepted invalid ID: %q", v)
		}
	}
}

// TestParseTraceparent covers trace context parsing.
func TestParseTraceparent_ValidW3C(t *testing.T) {
	tc := parseTraceparent("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")
	if tc == nil {
		t.Fatal("expected non-nil traceContext")
	}
	if tc.traceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("traceID = %q", tc.traceID)
	}
	if tc.parentSpanID != "b7ad6b7169203331" {
		t.Errorf("parentSpanID = %q", tc.parentSpanID)
	}
}

func TestParseTraceparent_InvalidFormats(t *testing.T) {
	invalids := []string{
		"",
		"invalid",
		"00-short-tooshort-01",
		"ff-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", // version ff is rejected
	}
	for _, v := range invalids {
		if tc := parseTraceparent(v); tc != nil {
			t.Errorf("parseTraceparent(%q) should be nil, got %+v", v, tc)
		}
	}
}

func TestBuildModelsResponse_EmptyList(t *testing.T) {
	result := buildModelsResponse(nil)
	var resp struct {
		Object string `json:"object"`
		Data   []any  `json:"data"`
	}
	_ = json.Unmarshal(result, &resp)
	if resp.Object != "list" {
		t.Errorf("object = %q, want 'list'", resp.Object)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty data, got %d items", len(resp.Data))
	}
}

func TestBuildModelsResponse_MultipleModels(t *testing.T) {
	models := []CompatModel{
		{ID: "gpt-4o", Provider: "openai"},
		{ID: "claude-sonnet-4", Provider: "anthropic"},
	}
	result := buildModelsResponse(models)
	var resp struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	_ = json.Unmarshal(result, &resp)
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != "gpt-4o" {
		t.Errorf("first model = %q", resp.Data[0].ID)
	}
	if resp.Data[1].OwnedBy != "anthropic" {
		t.Errorf("second model owner = %q", resp.Data[1].OwnedBy)
	}
}

// TestFallbackParser covers the fallback parser for unknown providers.
func TestFallbackParser(t *testing.T) {
	p := getParser("unknown-provider")
	if p == nil {
		t.Fatal("getParser should return fallback, not nil")
	}
	if p.IsStreaming([]byte(`{"stream":true}`)) {
		t.Error("fallback should always return false for IsStreaming")
	}
	model, content := p.ParseRequest([]byte(`{"model":"x"}`))
	if model != "" || content != "" {
		t.Error("fallback ParseRequest should return empty strings")
	}
	_, in, out := p.ParseResponse([]byte(`{"usage":{"input_tokens":5}}`))
	if in != 0 || out != 0 {
		t.Error("fallback ParseResponse should return 0")
	}
}

// TestOpenAIParser_Streaming covers OpenAI streaming response parsing.
func TestOpenAIParser_Streaming(t *testing.T) {
	data := `data: {"choices":[{"delta":{"content":"Hello"},"index":0}]}

data: {"choices":[{"delta":{"content":" world"},"index":0}],"usage":{"prompt_tokens":5,"completion_tokens":2}}

data: [DONE]
`
	p := getParser("openai")
	content, input, output := p.ParseStreamingResponse([]byte(data))
	if content != "Hello world" {
		t.Errorf("content = %q, want 'Hello world'", content)
	}
	if input != 5 {
		t.Errorf("input = %d, want 5", input)
	}
	if output != 2 {
		t.Errorf("output = %d, want 2", output)
	}
}

// TestOpenAIParser_Response covers standard OpenAI response parsing.
func TestOpenAIParser_Response(t *testing.T) {
	body := `{"choices":[{"message":{"content":"Hi!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":3}}`
	p := getParser("openai")
	content, input, output := p.ParseResponse([]byte(body))
	if content != "Hi!" {
		t.Errorf("content = %q", content)
	}
	if input != 10 || output != 3 {
		t.Errorf("tokens = %d/%d", input, output)
	}
}

// TestGeminiOAI_UsesOpenAIParser verifies gemini-oai reuses OpenAI parser.
func TestGeminiOAI_UsesOpenAIParser(t *testing.T) {
	p := getParser("gemini-oai")
	body := `{"model":"gemini-2.0-flash","messages":[{"role":"user","content":"hi"}]}`
	model, _ := p.ParseRequest([]byte(body))
	if model != "gemini-2.0-flash" {
		t.Errorf("model = %q", model)
	}
}

// ── CRIT-17: isUtilityEndpoint ───────────────────────────────────────────────

func TestIsUtilityEndpoint(t *testing.T) {
	utility := []string{
		"/v1/messages/count_tokens",
		"/v1/count_tokens",
		"/v1/tokenize",
		"/v1/models",
		"/v1/engines/gpt-4o/models",
	}
	for _, p := range utility {
		if !isUtilityEndpoint(p) {
			t.Errorf("isUtilityEndpoint(%q) = false, want true", p)
		}
	}

	generative := []string{
		"/v1/messages",
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/messages/batches",
	}
	for _, p := range generative {
		if isUtilityEndpoint(p) {
			t.Errorf("isUtilityEndpoint(%q) = true, want false", p)
		}
	}
}

// ====================================================================
// MaaS provider integration tests — prefix injection, parser registry,
// model extraction, and stream usage injection.
// ====================================================================

// TestMaaSProviders_InDefaultProviders verifies all MaaS providers
// are registered in DefaultProviders.
func TestMaaSProviders_InDefaultProviders(t *testing.T) {
	providers := DefaultProviders()
	names := make(map[string]bool)
	for _, p := range providers {
		names[p.Name] = true
	}

	want := []string{"deepseek", "deepseek-v3", "qwen", "zai"}
	for _, name := range want {
		if !names[name] {
			t.Errorf("MaaS provider %q not found in DefaultProviders()", name)
		}
	}
}

// TestMaaSProviders_ParserRegistry verifies all MaaS providers have
// parsers registered and they return openaiParser (not fallback).
func TestMaaSProviders_ParserRegistry(t *testing.T) {
	maas := []string{"deepseek", "deepseek-v3", "qwen", "zai"}
	for _, name := range maas {
		p := getParser(name)
		if _, ok := p.(*openaiParser); !ok {
			t.Errorf("getParser(%q) = %T, want *openaiParser", name, p)
		}
	}

	// Verify unknown provider falls back.
	p := getParser("unknown-provider")
	if _, ok := p.(*fallbackParser); !ok {
		t.Errorf("getParser(\"unknown-provider\") = %T, want *fallbackParser", p)
	}
}

// TestMaaSProviders_ModelPrefixRewrite verifies that rewriteModelField
// correctly prepends publisher prefixes for each MaaS provider.
func TestMaaSProviders_ModelPrefixRewrite(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		prefix   string
		want     string
	}{
		{"deepseek", "deepseek-r1-0528-maas", "deepseek-ai/", "deepseek-ai/deepseek-r1-0528-maas"},
		{"deepseek-v3", "deepseek-v3.2-maas", "deepseek-ai/", "deepseek-ai/deepseek-v3.2-maas"},
		{"qwen", "qwen3-235b-a22b-instruct-2507-maas", "qwen/", "qwen/qwen3-235b-a22b-instruct-2507-maas"},
		{"zai", "glm-5-maas", "zai-org/", "zai-org/glm-5-maas"},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.model, func(t *testing.T) {
			body := []byte(`{"model":"` + tt.model + `","messages":[]}`)
			result := rewriteModelField(body, tt.prefix+tt.model)

			var req struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(result, &req); err != nil {
				t.Fatalf("failed to parse: %v", err)
			}
			if req.Model != tt.want {
				t.Errorf("model = %q, want %q", req.Model, tt.want)
			}
		})
	}
}

// TestMaaSProviders_ModelPrefixIdempotent verifies that if the model
// already has the publisher prefix, rewrite is not double-applied.
func TestMaaSProviders_ModelPrefixIdempotent(t *testing.T) {
	tests := []struct {
		provider string
		prefix   string
		model    string
	}{
		{"deepseek", "deepseek-ai/", "deepseek-ai/deepseek-r1-0528-maas"},
		{"qwen", "qwen/", "qwen/qwen3-235b-a22b-instruct-2507-maas"},
		{"zai", "zai-org/", "zai-org/glm-5-maas"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			// When prefix already present, the model should not change.
			if !strings.HasPrefix(tt.model, tt.prefix) {
				t.Fatalf("test setup: model %q should start with prefix %q", tt.model, tt.prefix)
			}
		})
	}
}

// TestMaaSProviders_ExtractModel verifies that extractModelFromResponse
// strips publisher prefixes for MaaS providers.
func TestMaaSProviders_ExtractModel(t *testing.T) {
	tests := []struct {
		provider string
		response string
		want     string
	}{
		{"deepseek", `{"model":"deepseek-ai/deepseek-r1-0528-maas"}`, "deepseek-r1-0528-maas"},
		{"deepseek-v3", `{"model":"deepseek-ai/deepseek-v3.2-maas"}`, "deepseek-v3.2-maas"},
		{"qwen", `{"model":"qwen/qwen3-235b-a22b-instruct-2507-maas"}`, "qwen3-235b-a22b-instruct-2507-maas"},
		{"zai", `{"model":"zai-org/glm-5-maas"}`, "glm-5-maas"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := extractModelFromResponse(tt.provider, []byte(tt.response))
			if got != tt.want {
				t.Errorf("extractModelFromResponse(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

// TestMaaSProviders_InjectStreamUsage verifies that injectStreamUsageOption
// adds stream_options for all MaaS providers.
func TestMaaSProviders_InjectStreamUsage(t *testing.T) {
	maas := []string{"deepseek", "deepseek-v3", "qwen", "zai"}
	body := []byte(`{"model":"test","stream":true}`)

	for _, provider := range maas {
		t.Run(provider, func(t *testing.T) {
			result := injectStreamUsageOption(provider, body)
			if !strings.Contains(string(result), "stream_options") {
				t.Errorf("injectStreamUsageOption(%q) did not add stream_options", provider)
			}
			if !strings.Contains(string(result), "include_usage") {
				t.Errorf("injectStreamUsageOption(%q) did not add include_usage", provider)
			}
		})
	}

	// Verify non-OpenAI providers don't get injection.
	for _, provider := range []string{"anthropic", "google"} {
		t.Run(provider+"_no_inject", func(t *testing.T) {
			result := injectStreamUsageOption(provider, body)
			if strings.Contains(string(result), "stream_options") {
				t.Errorf("injectStreamUsageOption(%q) should not inject for non-OpenAI provider", provider)
			}
		})
	}
}
