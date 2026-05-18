package proxy_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/proxy"
)

// TestFunctional_TTL_TranslationOutput is a functional test that exercises the
// full translation pipeline end-to-end and verifies the exact JSON output
// that would be sent to the Anthropic API.
func TestFunctional_TTL_TranslationOutput(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are a helpful coding assistant. Follow best practices."},
			{"role": "user", "content": "Write a Go function that sorts a slice"},
			{"role": "assistant", "content": "Here's a sorting function..."},
			{"role": "user", "content": "Can you add error handling?"}
		]
	}`)

	t.Run("default_5m_no_ttl_field_in_output", func(t *testing.T) {
		translator := &proxy.AnthropicFormatTranslator{}
		// Default: auto mode, 5m TTL.
		result, _, err := translator.TranslateRequest(body)
		if err != nil {
			t.Fatalf("TranslateRequest: %v", err)
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(result, &raw); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		// System prompt should have cache_control: {"type":"ephemeral"} — NO ttl.
		sysBlocks := raw["system"].([]interface{})
		sysCC := sysBlocks[len(sysBlocks)-1].(map[string]interface{})["cache_control"].(map[string]interface{})
		if _, has := sysCC["ttl"]; has {
			t.Errorf("5m TTL: system cache_control should NOT have 'ttl' field, got: %v", sysCC)
		}
		if sysCC["type"] != "ephemeral" {
			t.Errorf("type = %v, want ephemeral", sysCC["type"])
		}

		// Last user message cache_control should also not have ttl.
		msgs := raw["messages"].([]interface{})
		lastMsg := msgs[len(msgs)-1].(map[string]interface{})
		lastContent := lastMsg["content"].([]interface{})
		lastCC := lastContent[len(lastContent)-1].(map[string]interface{})["cache_control"].(map[string]interface{})
		if _, has := lastCC["ttl"]; has {
			t.Errorf("5m TTL: user msg cache_control should NOT have 'ttl' field, got: %v", lastCC)
		}

		t.Logf("✅ 5m TTL output:\n%s", prettyJSON(t, result))
	})

	t.Run("1h_includes_ttl_field", func(t *testing.T) {
		translator := &proxy.AnthropicFormatTranslator{}
		translator.SetCacheTTL(proxy.CacheTTL1h)

		result, _, err := translator.TranslateRequest(body)
		if err != nil {
			t.Fatalf("TranslateRequest: %v", err)
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(result, &raw); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		// System prompt: {"type":"ephemeral","ttl":"1h"}
		sysBlocks := raw["system"].([]interface{})
		sysCC := sysBlocks[len(sysBlocks)-1].(map[string]interface{})["cache_control"].(map[string]interface{})
		if sysCC["type"] != "ephemeral" {
			t.Errorf("type = %v, want ephemeral", sysCC["type"])
		}
		if sysCC["ttl"] != "1h" {
			t.Errorf("system ttl = %v, want '1h'", sysCC["ttl"])
		}

		// Last user message: {"type":"ephemeral","ttl":"1h"}
		msgs := raw["messages"].([]interface{})
		lastMsg := msgs[len(msgs)-1].(map[string]interface{})
		lastContent := lastMsg["content"].([]interface{})
		lastCC := lastContent[len(lastContent)-1].(map[string]interface{})["cache_control"].(map[string]interface{})
		if lastCC["ttl"] != "1h" {
			t.Errorf("user msg ttl = %v, want '1h'", lastCC["ttl"])
		}

		t.Logf("✅ 1h TTL output:\n%s", prettyJSON(t, result))
	})

	t.Run("per_request_header_override_1h", func(t *testing.T) {
		translator := &proxy.AnthropicFormatTranslator{} // default: auto, 5m

		// Per-request override: use 1h TTL via TranslateRequestWithModeAndTTL.
		result, _, err := translator.TranslateRequestWithModeAndTTL(body, proxy.CachingAuto, proxy.CacheTTL1h)
		if err != nil {
			t.Fatalf("TranslateRequestWithModeAndTTL: %v", err)
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(result, &raw); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		sysBlocks := raw["system"].([]interface{})
		sysCC := sysBlocks[len(sysBlocks)-1].(map[string]interface{})["cache_control"].(map[string]interface{})
		if sysCC["ttl"] != "1h" {
			t.Errorf("override: system ttl = %v, want '1h'", sysCC["ttl"])
		}

		// After per-request override, shared state should still be 5m.
		if translator.GetCacheTTL() != proxy.CacheTTL5m {
			t.Errorf("shared state leaked! TTL = %q, want 5m", translator.GetCacheTTL())
		}

		// Next normal request should NOT have ttl field.
		result2, _, _ := translator.TranslateRequest(body)
		var raw2 map[string]interface{}
		if err := json.Unmarshal(result2, &raw2); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		sysBlocks2 := raw2["system"].([]interface{})
		sysCC2 := sysBlocks2[len(sysBlocks2)-1].(map[string]interface{})["cache_control"].(map[string]interface{})
		if _, has := sysCC2["ttl"]; has {
			t.Errorf("normal request after override should NOT have ttl field, got: %v", sysCC2)
		}

		t.Logf("✅ Per-request override isolation verified")
	})

	t.Run("caching_off_no_cache_control_regardless_of_ttl", func(t *testing.T) {
		translator := &proxy.AnthropicFormatTranslator{}
		translator.SetCacheTTL(proxy.CacheTTL1h)
		translator.SetCachingMode(proxy.CachingOff)

		result, _, err := translator.TranslateRequest(body)
		if err != nil {
			t.Fatalf("TranslateRequest: %v", err)
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(result, &raw); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		// When caching is off, NO cache_control anywhere — even with 1h TTL.
		resultStr := string(result)
		if strings.Contains(resultStr, "cache_control") {
			t.Errorf("caching=off + TTL=1h: should NOT have any cache_control, got:\n%s", prettyJSON(t, result))
		}

		t.Logf("✅ caching=off correctly suppresses all cache_control")
	})

	t.Run("system_only_mode_with_1h_ttl", func(t *testing.T) {
		translator := &proxy.AnthropicFormatTranslator{}
		translator.SetCacheTTL(proxy.CacheTTL1h)
		translator.SetCachingMode(proxy.CachingSystemOnly)

		result, _, err := translator.TranslateRequest(body)
		if err != nil {
			t.Fatalf("TranslateRequest: %v", err)
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(result, &raw); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}

		// System should have ttl:1h.
		sysBlocks := raw["system"].([]interface{})
		sysCC := sysBlocks[len(sysBlocks)-1].(map[string]interface{})["cache_control"].(map[string]interface{})
		if sysCC["ttl"] != "1h" {
			t.Errorf("system-only: system ttl = %v, want '1h'", sysCC["ttl"])
		}

		// Last user message should NOT have cache_control at all.
		msgs := raw["messages"].([]interface{})
		lastMsg := msgs[len(msgs)-1].(map[string]interface{})
		if content, ok := lastMsg["content"].([]interface{}); ok {
			lastBlock := content[len(content)-1].(map[string]interface{})
			if _, hasCc := lastBlock["cache_control"]; hasCc {
				t.Errorf("system-only: user message should NOT have cache_control")
			}
		}

		t.Logf("✅ system-only + 1h TTL: only system prompt cached")
	})
}

func prettyJSON(t *testing.T, data []byte) string {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return string(data)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
