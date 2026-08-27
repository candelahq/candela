package proxy

import (
	"encoding/json"
	"testing"
)

func TestClampMaxTokens_UnderLimit(t *testing.T) {
	body := `{"model":"gpt-4o","max_tokens":100}`
	result := clampMaxTokens([]byte(body), 200)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	got := int(parsed["max_tokens"].(float64))
	if got != 100 {
		t.Errorf("max_tokens = %d, want 100", got)
	}
}

func TestClampMaxTokens_OverLimit(t *testing.T) {
	body := `{"model":"gpt-4o","max_tokens":500}`
	result := clampMaxTokens([]byte(body), 200)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	got := int(parsed["max_tokens"].(float64))
	if got != 200 {
		t.Errorf("max_tokens = %d, want 200 (clamped)", got)
	}
}

func TestClampMaxTokens_ExactlyAtLimit(t *testing.T) {
	body := `{"model":"gpt-4o","max_tokens":200}`
	result := clampMaxTokens([]byte(body), 200)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	got := int(parsed["max_tokens"].(float64))
	if got != 200 {
		t.Errorf("max_tokens = %d, want 200", got)
	}
}

func TestClampMaxTokens_NoField(t *testing.T) {
	body := `{"model":"gpt-4o"}`
	result := clampMaxTokens([]byte(body), 200)
	if string(result) != body {
		t.Errorf("body changed when no max_tokens field: %s", result)
	}
}

func TestIsSafeModelName_Valid(t *testing.T) {
	safe := []string{
		"gpt-4o",
		"gpt-4o-mini",
		"claude-sonnet-4-20250514",
		"gemini-2.5-pro",
		"Qwen/Qwen2.5-Coder",
		"claude-sonnet-4@20250514",
		"meta-llama/Meta-Llama-3.1-8B",
	}
	for _, m := range safe {
		if !isSafeModelName(m) {
			t.Errorf("expected %q to be safe", m)
		}
	}
}

func TestIsSafeModelName_PathTraversal(t *testing.T) {
	unsafe := []string{
		"../../etc/passwd",
		"model/../../../evil",
		"model with spaces",
		"model;rm -rf",
		"model$(cmd)",
		"",
	}
	for _, m := range unsafe {
		if isSafeModelName(m) {
			t.Errorf("expected %q to be unsafe", m)
		}
	}
}

func TestExtractRequestInfo_EmptyBody(t *testing.T) {
	model, content := extractRequestInfo("openai", nil)
	if model != "" {
		t.Errorf("model = %q, want empty", model)
	}
	if content != "" {
		t.Errorf("content = %q, want empty", content)
	}
}

func TestExtractResponseInfo_EmptyBody(t *testing.T) {
	content, input, output := extractResponseInfo("openai", nil)
	if content != "" {
		t.Errorf("content = %q, want empty", content)
	}
	if input != 0 || output != 0 {
		t.Errorf("tokens = (%d, %d), want (0, 0)", input, output)
	}
}

func TestExtractRequestInfo_MalformedJSON(t *testing.T) {
	model, _ := extractRequestInfo("openai", []byte("not json"))
	if model != "" {
		t.Errorf("model = %q for malformed JSON, want empty", model)
	}
}

func TestExtractResponseInfo_MalformedJSON(t *testing.T) {
	_, input, output := extractResponseInfo("openai", []byte("{bad"))
	if input != 0 || output != 0 {
		t.Errorf("tokens = (%d, %d) for malformed JSON, want (0, 0)", input, output)
	}
}
