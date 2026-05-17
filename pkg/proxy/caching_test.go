package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestCachingMode_DefaultIsAuto(t *testing.T) {
	ft := &AnthropicFormatTranslator{}
	if got := ft.GetCachingMode(); got != CachingAuto {
		t.Errorf("default caching mode = %q, want %q", got, CachingAuto)
	}
}

func TestCachingMode_SetGet(t *testing.T) {
	ft := &AnthropicFormatTranslator{}
	modes := []CachingMode{CachingOff, CachingAuto, CachingSystemOnly, CachingOff}
	for _, mode := range modes {
		ft.SetCachingMode(mode)
		if got := ft.GetCachingMode(); got != mode {
			t.Errorf("GetCachingMode() = %q after SetCachingMode(%q)", got, mode)
		}
	}
}

func TestCachingMode_ConcurrentAccess(t *testing.T) {
	ft := &AnthropicFormatTranslator{}
	var wg sync.WaitGroup
	modes := []CachingMode{CachingOff, CachingAuto, CachingSystemOnly}

	// Concurrent writers.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ft.SetCachingMode(modes[i%len(modes)])
		}(i)
	}

	// Concurrent readers — should never panic.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mode := ft.GetCachingMode()
			switch mode {
			case CachingOff, CachingAuto, CachingSystemOnly:
				// valid
			default:
				t.Errorf("unexpected mode: %q", mode)
			}
		}()
	}
	wg.Wait()
}

func TestProxy_SetCachingMode_PropagatesAllProviders(t *testing.T) {
	ft1 := &AnthropicFormatTranslator{}
	ft2 := &AnthropicFormatTranslator{}
	p := &Proxy{
		providers: map[string]Provider{
			"anthropic":    {Name: "anthropic", FormatTranslator: ft1},
			"anthropic-v2": {Name: "anthropic-v2", FormatTranslator: ft2},
			"google":       {Name: "google"},
		},
	}

	p.SetCachingMode(CachingSystemOnly)

	if ft1.GetCachingMode() != CachingSystemOnly {
		t.Errorf("ft1 mode = %q, want system-only", ft1.GetCachingMode())
	}
	if ft2.GetCachingMode() != CachingSystemOnly {
		t.Errorf("ft2 mode = %q, want system-only", ft2.GetCachingMode())
	}
}

func TestProxy_GetCachingMode_ReturnsFirst(t *testing.T) {
	ft := &AnthropicFormatTranslator{}
	ft.SetCachingMode(CachingSystemOnly)
	p := &Proxy{
		providers: map[string]Provider{
			"google":    {Name: "google"},
			"anthropic": {Name: "anthropic", FormatTranslator: ft},
		},
	}
	if got := p.GetCachingMode(); got != CachingSystemOnly {
		t.Errorf("Proxy.GetCachingMode() = %q, want system-only", got)
	}
}

func TestProxy_GetCachingMode_NoTranslators(t *testing.T) {
	p := &Proxy{
		providers: map[string]Provider{
			"google": {Name: "google"},
		},
	}
	if got := p.GetCachingMode(); got != CachingOff {
		t.Errorf("Proxy.GetCachingMode() with no translators = %q, want off", got)
	}
}

func TestCachingHeader_PerRequestOverride(t *testing.T) {
	ft := &AnthropicFormatTranslator{} // default: auto
	provider := Provider{
		Name:             "anthropic",
		FormatTranslator: ft,
	}

	// Simulate the header override logic from proxy.go.
	r := httptest.NewRequest(http.MethodPost, "/proxy/anthropic/v1/chat/completions", nil)
	r.Header.Set(CachingHeader, "off")

	// Apply override.
	override := r.Header.Get(CachingHeader)
	if override == "" {
		t.Fatal("expected X-Candela-Caching header")
	}
	original := ft.GetCachingMode()
	ft.SetCachingMode(ParseCachingMode(override))
	r.Header.Del(CachingHeader)

	// During request processing, mode should be "off".
	if ft.GetCachingMode() != CachingOff {
		t.Errorf("mode during override = %q, want off", ft.GetCachingMode())
	}
	// Header should be stripped.
	if r.Header.Get(CachingHeader) != "" {
		t.Error("X-Candela-Caching header should be stripped after override")
	}

	// Restore.
	ft.SetCachingMode(original)
	if ft.GetCachingMode() != CachingAuto {
		t.Errorf("mode after restore = %q, want auto", ft.GetCachingMode())
	}

	_ = provider // suppress unused
}

func TestTranslateRequest_CachingOff_NoBreakpoints(t *testing.T) {
	translator := &AnthropicFormatTranslator{}
	translator.SetCachingMode(CachingOff)

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi!"},
			{"role": "user", "content": "How are you?"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	// With CachingOff, NO cache_control should appear anywhere.
	if containsCacheControl(translated) {
		t.Error("CachingOff: translated body should not contain cache_control")
	}
}

func TestTranslateRequest_CachingAuto_TwoBreakpoints(t *testing.T) {
	translator := &AnthropicFormatTranslator{} // default: auto

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi!"},
			{"role": "user", "content": "How are you?"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	// Auto mode: both system prompt and last user message should have cache_control.
	raw := parseJSON(t, translated)

	// System breakpoint.
	sysBlocks := raw["system"].([]interface{})
	sysBlock := sysBlocks[0].(map[string]interface{})
	if _, ok := sysBlock["cache_control"]; !ok {
		t.Error("auto mode: system prompt missing cache_control")
	}

	// Last user message breakpoint.
	messages := raw["messages"].([]interface{})
	lastMsg := messages[len(messages)-1].(map[string]interface{})
	lastContent := lastMsg["content"].([]interface{})
	lastBlock := lastContent[len(lastContent)-1].(map[string]interface{})
	if _, ok := lastBlock["cache_control"]; !ok {
		t.Error("auto mode: last user message missing cache_control")
	}
}

func TestTranslateRequest_CachingSystemOnly_OnlySystemBreakpoint(t *testing.T) {
	translator := &AnthropicFormatTranslator{}
	translator.SetCachingMode(CachingSystemOnly)

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi!"},
			{"role": "user", "content": "How are you?"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	raw := parseJSON(t, translated)

	// System breakpoint should exist.
	sysBlocks := raw["system"].([]interface{})
	sysBlock := sysBlocks[0].(map[string]interface{})
	if _, ok := sysBlock["cache_control"]; !ok {
		t.Error("system-only mode: system prompt missing cache_control")
	}

	// Last user message should NOT have cache_control.
	messages := raw["messages"].([]interface{})
	lastMsg := messages[len(messages)-1].(map[string]interface{})
	// Content might be string (no breakpoint) or array.
	if content, ok := lastMsg["content"].([]interface{}); ok {
		lastBlock := content[len(content)-1].(map[string]interface{})
		if _, hasCc := lastBlock["cache_control"]; hasCc {
			t.Error("system-only mode: last user message should NOT have cache_control")
		}
	}
}

func TestTranslateRequest_CachingAuto_ArraySystemPrompt(t *testing.T) {
	translator := &AnthropicFormatTranslator{} // default: auto

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": [
				{"type": "text", "text": "Rule 1"},
				{"type": "text", "text": "Rule 2"},
				{"type": "text", "text": "Rule 3"}
			]},
			{"role": "user", "content": "Hello"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	raw := parseJSON(t, translated)
	sysBlocks := raw["system"].([]interface{})
	if len(sysBlocks) != 3 {
		t.Fatalf("expected 3 system blocks, got %d", len(sysBlocks))
	}

	// Only the LAST block should have cache_control.
	for i, block := range sysBlocks {
		b := block.(map[string]interface{})
		_, hasCc := b["cache_control"]
		if i < 2 && hasCc {
			t.Errorf("system block %d should NOT have cache_control", i)
		}
		if i == 2 && !hasCc {
			t.Error("last system block should have cache_control")
		}
	}
}

func TestTranslateRequest_CachingOff_EmptySystemHandled(t *testing.T) {
	translator := &AnthropicFormatTranslator{}
	translator.SetCachingMode(CachingOff)

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": ""},
			{"role": "user", "content": "Hello"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	raw := parseJSON(t, translated)
	// Empty system content should result in nil/absent system field.
	if raw["system"] != nil {
		t.Errorf("empty system content should be nil, got %v", raw["system"])
	}
}

func TestTranslateRequest_ModeSwitch_Sequential(t *testing.T) {
	// Verify mode switching produces correct results sequentially.
	translator := &AnthropicFormatTranslator{}
	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hi"}
		]
	}`

	// Auto → should have cache_control.
	translated, _, _ := translator.TranslateRequest([]byte(body))
	if !containsCacheControl(translated) {
		t.Error("auto mode should contain cache_control")
	}

	// Switch to off → should NOT have cache_control.
	translator.SetCachingMode(CachingOff)
	translated, _, _ = translator.TranslateRequest([]byte(body))
	if containsCacheControl(translated) {
		t.Error("off mode should NOT contain cache_control")
	}

	// Switch back to system-only → should have cache_control on system only.
	translator.SetCachingMode(CachingSystemOnly)
	translated, _, _ = translator.TranslateRequest([]byte(body))
	if !containsCacheControl(translated) {
		t.Error("system-only mode should contain cache_control on system prompt")
	}
}

// --- Helpers ---

func containsCacheControl(data []byte) bool {
	return strings.Contains(string(data), "cache_control")
}

func parseJSON(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	return raw
}
