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
	// Verify that TranslateRequestWithMode does NOT mutate shared state.
	ft := &AnthropicFormatTranslator{} // default: auto

	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`)

	// Simulate per-request override: header says "off".
	r := httptest.NewRequest(http.MethodPost, "/proxy/anthropic/v1/chat/completions", nil)
	r.Header.Set(CachingHeader, "off")

	override := r.Header.Get(CachingHeader)
	r.Header.Del(CachingHeader)

	// Use TranslateRequestWithMode — shared state should NOT change.
	result, _, err := ft.TranslateRequestWithMode(body, ParseCachingMode(override))
	if err != nil {
		t.Fatalf("TranslateRequestWithMode failed: %v", err)
	}

	// Result should have NO cache_control (mode=off was used).
	if containsCacheControl(result) {
		t.Error("override=off: result should not contain cache_control")
	}

	// Header should be stripped.
	if r.Header.Get(CachingHeader) != "" {
		t.Error("X-Candela-Caching header should be stripped")
	}

	// Shared translator state should be UNCHANGED (still auto).
	if ft.GetCachingMode() != CachingAuto {
		t.Errorf("shared state changed! mode = %q, want auto", ft.GetCachingMode())
	}

	// A normal request (no override) should still use auto mode.
	result2, _, _ := ft.TranslateRequest(body)
	if !containsCacheControl(result2) {
		t.Error("normal request after override should still have cache_control (auto mode)")
	}
}

func TestCachingHeader_ConcurrentIsolation(t *testing.T) {
	// Verify that concurrent requests with different overrides don't interfere.
	ft := &AnthropicFormatTranslator{} // default: auto

	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`)

	var wg sync.WaitGroup
	errCh := make(chan string, 200)

	// 50 requests with override=off (should NOT have cache_control).
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _, err := ft.TranslateRequestWithMode(body, CachingOff)
			if err != nil {
				errCh <- "off request failed: " + err.Error()
				return
			}
			if containsCacheControl(result) {
				errCh <- "override=off request got cache_control (race!)"
			}
		}()
	}

	// 50 requests with no override (should use auto, HAVE cache_control).
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _, err := ft.TranslateRequest(body)
			if err != nil {
				errCh <- "auto request failed: " + err.Error()
				return
			}
			if !containsCacheControl(result) {
				errCh <- "auto request missing cache_control (race!)"
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for e := range errCh {
		t.Error(e)
	}

	// Shared state should still be auto after all requests complete.
	if ft.GetCachingMode() != CachingAuto {
		t.Errorf("shared state changed after concurrent requests! mode = %q", ft.GetCachingMode())
	}
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

func TestTranslateRequest_ConcurrentTranslateWithModeSwitch(t *testing.T) {
	// Verify that translating requests while modes change concurrently
	// never panics or produces invalid JSON.
	translator := &AnthropicFormatTranslator{}
	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`)

	var wg sync.WaitGroup
	modes := []CachingMode{CachingOff, CachingAuto, CachingSystemOnly}

	// 50 concurrent translators.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _, err := translator.TranslateRequest(body)
			if err != nil {
				t.Errorf("concurrent TranslateRequest failed: %v", err)
				return
			}
			// Must be valid JSON.
			var raw map[string]interface{}
			if err := json.Unmarshal(result, &raw); err != nil {
				t.Errorf("concurrent TranslateRequest produced invalid JSON: %v", err)
			}
		}()
	}

	// 50 concurrent mode switchers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			translator.SetCachingMode(modes[i%len(modes)])
		}(i)
	}
	wg.Wait()
}

func TestTranslateRequest_CachingAuto_NoSystemPrompt(t *testing.T) {
	// Even without a system message, caching should apply to last user message.
	translator := &AnthropicFormatTranslator{} // auto

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
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

	// No system field.
	if raw["system"] != nil {
		t.Error("should not have system field when no system message provided")
	}

	// Last user message should still have cache_control in auto mode.
	messages := raw["messages"].([]interface{})
	lastMsg := messages[len(messages)-1].(map[string]interface{})
	lastContent := lastMsg["content"].([]interface{})
	lastBlock := lastContent[len(lastContent)-1].(map[string]interface{})
	if _, ok := lastBlock["cache_control"]; !ok {
		t.Error("auto mode: last user message should have cache_control even without system prompt")
	}
}

func TestTranslateRequest_CachingOff_NoSystemPrompt(t *testing.T) {
	translator := &AnthropicFormatTranslator{}
	translator.SetCachingMode(CachingOff)

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "user", "content": "Hello"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	if containsCacheControl(translated) {
		t.Error("CachingOff should never inject cache_control")
	}
}

func TestTranslateRequest_CachingAuto_SingleUserMessage(t *testing.T) {
	// Single user message — cache_control should still be applied.
	translator := &AnthropicFormatTranslator{}

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "Be concise."},
			{"role": "user", "content": "Hello"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	if !containsCacheControl(translated) {
		t.Error("auto mode should inject cache_control even with single user message")
	}
}

func TestTranslateRequest_CachingAuto_WithToolMessages(t *testing.T) {
	// Tool results are merged into user messages — caching should not break.
	translator := &AnthropicFormatTranslator{}

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "What time is it?"},
			{"role": "assistant", "content": "", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "get_time", "arguments": "{}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "3:45 PM"},
			{"role": "user", "content": "Thanks!"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	raw := parseJSON(t, translated)

	// System should have cache_control.
	sysBlocks := raw["system"].([]interface{})
	sysBlock := sysBlocks[0].(map[string]interface{})
	if _, ok := sysBlock["cache_control"]; !ok {
		t.Error("system prompt should have cache_control")
	}

	// Last user message "Thanks!" should have cache_control.
	messages := raw["messages"].([]interface{})
	lastMsg := messages[len(messages)-1].(map[string]interface{})
	lastContent := lastMsg["content"].([]interface{})
	lastBlock := lastContent[len(lastContent)-1].(map[string]interface{})
	if _, ok := lastBlock["cache_control"]; !ok {
		t.Error("last user message should have cache_control after tool messages")
	}
}

func TestTranslateRequest_CachingSystemOnly_ArraySystem(t *testing.T) {
	translator := &AnthropicFormatTranslator{}
	translator.SetCachingMode(CachingSystemOnly)

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": [
				{"type": "text", "text": "Rule A"},
				{"type": "text", "text": "Rule B"}
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

	// Last system block should have cache_control.
	lastSys := sysBlocks[len(sysBlocks)-1].(map[string]interface{})
	if _, ok := lastSys["cache_control"]; !ok {
		t.Error("system-only mode: last system block should have cache_control")
	}

	// User message should NOT have cache_control.
	messages := raw["messages"].([]interface{})
	lastMsg := messages[len(messages)-1].(map[string]interface{})
	if content, ok := lastMsg["content"].([]interface{}); ok {
		lastBlock := content[len(content)-1].(map[string]interface{})
		if _, hasCc := lastBlock["cache_control"]; hasCc {
			t.Error("system-only: user message should NOT have cache_control")
		}
	}
}

func TestParseCachingMode_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		input string
		want  CachingMode
	}{
		{"  auto  ", CachingAuto},
		{"\toff\n", CachingOff},
		{" system-only ", CachingSystemOnly},
		{"  TRUE  ", CachingAuto},
	}
	for _, tt := range tests {
		got := ParseCachingMode(tt.input)
		if got != tt.want {
			t.Errorf("ParseCachingMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCachingHeader_Constant(t *testing.T) {
	if CachingHeader != "X-Candela-Caching" {
		t.Errorf("CachingHeader = %q, want X-Candela-Caching", CachingHeader)
	}
}

// --- Helpers ---

func containsCacheControl(data []byte) bool {
	return strings.Contains(string(data), "cache_control")
}

func containsTTL1h(data []byte) bool {
	return strings.Contains(string(data), `"ttl":"1h"`) || strings.Contains(string(data), `"ttl": "1h"`)
}

func parseJSON(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	return raw
}

// ====================================================================
// Cache TTL Tests
// ====================================================================

func TestCacheTTL_DefaultIs5m(t *testing.T) {
	ft := &AnthropicFormatTranslator{}
	if got := ft.GetCacheTTL(); got != CacheTTL5m {
		t.Errorf("default cache TTL = %q, want %q", got, CacheTTL5m)
	}
}

func TestCacheTTL_SetGet(t *testing.T) {
	ft := &AnthropicFormatTranslator{}
	ttls := []CacheTTL{CacheTTL1h, CacheTTL5m, CacheTTL1h}
	for _, ttl := range ttls {
		ft.SetCacheTTL(ttl)
		if got := ft.GetCacheTTL(); got != ttl {
			t.Errorf("GetCacheTTL() = %q after SetCacheTTL(%q)", got, ttl)
		}
	}
}

func TestCacheTTL_ConcurrentAccess(t *testing.T) {
	ft := &AnthropicFormatTranslator{}
	var wg sync.WaitGroup
	ttls := []CacheTTL{CacheTTL5m, CacheTTL1h}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ft.SetCacheTTL(ttls[i%len(ttls)])
		}(i)
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ttl := ft.GetCacheTTL()
			switch ttl {
			case CacheTTL5m, CacheTTL1h:
				// valid
			default:
				t.Errorf("unexpected TTL: %q", ttl)
			}
		}()
	}
	wg.Wait()
}

func TestParseCacheTTL(t *testing.T) {
	tests := []struct {
		input string
		want  CacheTTL
	}{
		{"1h", CacheTTL1h},
		{"1hr", CacheTTL1h},
		{"1hour", CacheTTL1h},
		{"60m", CacheTTL1h},
		{"5m", CacheTTL5m},
		{"5min", CacheTTL5m},
		{"", CacheTTL5m},
		{"default", CacheTTL5m},
		{"  1h  ", CacheTTL1h},
		{"  5M  ", CacheTTL5m},
		{"unknown", CacheTTL5m},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseCacheTTL(tt.input)
			if got != tt.want {
				t.Errorf("ParseCacheTTL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildCacheControl_5m_OmitsTTL(t *testing.T) {
	cc := buildCacheControl(CacheTTL5m)
	if cc["type"] != "ephemeral" {
		t.Errorf("type = %q, want ephemeral", cc["type"])
	}
	if _, hasTTL := cc["ttl"]; hasTTL {
		t.Error("5m TTL should NOT include ttl field (backward compat)")
	}
}

func TestBuildCacheControl_1h_IncludesTTL(t *testing.T) {
	cc := buildCacheControl(CacheTTL1h)
	if cc["type"] != "ephemeral" {
		t.Errorf("type = %q, want ephemeral", cc["type"])
	}
	if cc["ttl"] != "1h" {
		t.Errorf("ttl = %q, want 1h", cc["ttl"])
	}
}

func TestCacheTTL_1h_InjectsCorrectCacheControl(t *testing.T) {
	translator := &AnthropicFormatTranslator{}
	translator.SetCacheTTL(CacheTTL1h)

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	// Should contain ttl: 1h in the output.
	if !containsTTL1h(translated) {
		t.Error("TTL=1h: translated body should contain \"ttl\":\"1h\"")
	}

	// Verify the structure: system prompt cache_control should have ttl.
	raw := parseJSON(t, translated)
	sysBlocks := raw["system"].([]interface{})
	sysBlock := sysBlocks[0].(map[string]interface{})
	cc := sysBlock["cache_control"].(map[string]interface{})
	if cc["type"] != "ephemeral" {
		t.Errorf("system cache_control.type = %v, want ephemeral", cc["type"])
	}
	if cc["ttl"] != "1h" {
		t.Errorf("system cache_control.ttl = %v, want 1h", cc["ttl"])
	}

	// Last user message should also have ttl.
	messages := raw["messages"].([]interface{})
	lastMsg := messages[len(messages)-1].(map[string]interface{})
	lastContent := lastMsg["content"].([]interface{})
	lastBlock := lastContent[0].(map[string]interface{})
	msgCc := lastBlock["cache_control"].(map[string]interface{})
	if msgCc["ttl"] != "1h" {
		t.Errorf("last user message cache_control.ttl = %v, want 1h", msgCc["ttl"])
	}
}

func TestCacheTTL_5m_OmitsTTLField(t *testing.T) {
	translator := &AnthropicFormatTranslator{}
	// default TTL is 5m

	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`

	translated, _, err := translator.TranslateRequest([]byte(body))
	if err != nil {
		t.Fatalf("TranslateRequest failed: %v", err)
	}

	// Should NOT contain ttl field (backward compat).
	if containsTTL1h(translated) {
		t.Error("TTL=5m (default): translated body should NOT contain ttl field")
	}

	// Should still contain cache_control with type: ephemeral.
	if !containsCacheControl(translated) {
		t.Error("TTL=5m: should still have cache_control")
	}
}

func TestCacheTTL_PerRequestOverride(t *testing.T) {
	translator := &AnthropicFormatTranslator{} // default: auto, 5m

	body := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		]
	}`)

	// Per-request TTL override: 1h
	result, _, err := translator.TranslateRequestWithModeAndTTL(body, CachingAuto, CacheTTL1h)
	if err != nil {
		t.Fatalf("TranslateRequestWithModeAndTTL failed: %v", err)
	}

	if !containsTTL1h(result) {
		t.Error("per-request TTL override: result should contain ttl:1h")
	}

	// Shared translator state should be UNCHANGED (still 5m).
	if translator.GetCacheTTL() != CacheTTL5m {
		t.Errorf("shared TTL state changed! got %q, want 5m", translator.GetCacheTTL())
	}

	// Normal request should NOT have ttl field.
	result2, _, _ := translator.TranslateRequest(body)
	if containsTTL1h(result2) {
		t.Error("normal request after override should NOT contain ttl:1h")
	}
}

func TestProxy_SetCacheTTL_PropagatesAllProviders(t *testing.T) {
	ft1 := &AnthropicFormatTranslator{}
	ft2 := &AnthropicFormatTranslator{}
	p := &Proxy{
		providers: map[string]Provider{
			"anthropic":    {Name: "anthropic", FormatTranslator: ft1},
			"anthropic-v2": {Name: "anthropic-v2", FormatTranslator: ft2},
			"google":       {Name: "google"},
		},
	}

	p.SetCacheTTL(CacheTTL1h)

	if ft1.GetCacheTTL() != CacheTTL1h {
		t.Errorf("ft1 TTL = %q, want 1h", ft1.GetCacheTTL())
	}
	if ft2.GetCacheTTL() != CacheTTL1h {
		t.Errorf("ft2 TTL = %q, want 1h", ft2.GetCacheTTL())
	}
}

func TestProxy_GetCacheTTL_ReturnsFirst(t *testing.T) {
	ft := &AnthropicFormatTranslator{}
	ft.SetCacheTTL(CacheTTL1h)
	p := &Proxy{
		providers: map[string]Provider{
			"google":    {Name: "google"},
			"anthropic": {Name: "anthropic", FormatTranslator: ft},
		},
	}
	if got := p.GetCacheTTL(); got != CacheTTL1h {
		t.Errorf("Proxy.GetCacheTTL() = %q, want 1h", got)
	}
}

func TestProxy_GetCacheTTL_NoTranslators(t *testing.T) {
	p := &Proxy{
		providers: map[string]Provider{
			"google": {Name: "google"},
		},
	}
	if got := p.GetCacheTTL(); got != CacheTTL5m {
		t.Errorf("Proxy.GetCacheTTL() with no translators = %q, want 5m", got)
	}
}

func TestCacheTTLHeader_Constant(t *testing.T) {
	if CacheTTLHeader != "X-Candela-Cache-TTL" {
		t.Errorf("CacheTTLHeader = %q, want X-Candela-Cache-TTL", CacheTTLHeader)
	}
}

func TestCacheTTL_ModeSwitch_WithTTL(t *testing.T) {
	// Verify TTL applies correctly across mode switches.
	translator := &AnthropicFormatTranslator{}
	translator.SetCacheTTL(CacheTTL1h)
	body := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hi"}
		]
	}`

	// Auto + 1h → should have ttl.
	translated, _, _ := translator.TranslateRequest([]byte(body))
	if !containsTTL1h(translated) {
		t.Error("auto+1h: should contain ttl:1h")
	}

	// Off + 1h → should NOT have any cache_control.
	translator.SetCachingMode(CachingOff)
	translated, _, _ = translator.TranslateRequest([]byte(body))
	if containsCacheControl(translated) {
		t.Error("off+1h: should NOT contain cache_control at all")
	}

	// System-only + 1h → should have ttl on system but not on user msg.
	translator.SetCachingMode(CachingSystemOnly)
	translated, _, _ = translator.TranslateRequest([]byte(body))
	if !containsTTL1h(translated) {
		t.Error("system-only+1h: should contain ttl:1h on system prompt")
	}
}
