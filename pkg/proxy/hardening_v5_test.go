package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
)

// ──────────────────────────────────────────
// U-1: Request ID validation rejects injection
// ──────────────────────────────────────────

func TestRequestID_ValidationRejectsInvalid(t *testing.T) {
	// C-1: Crafted X-Request-ID with newlines/special chars must be rejected.
	invalid := []string{
		"abc\ndef",                  // newline injection
		"abc\r\ndef",                // CRLF injection
		"<script>alert(1)</script>", // XSS
		strings.Repeat("a", 200),    // too long
		"abc def",                   // space
		"abc;DROP TABLE spans",      // SQL injection
	}
	for _, id := range invalid {
		if requestIDPattern.MatchString(id) {
			t.Errorf("requestIDPattern should reject %q", id)
		}
	}
}

// ──────────────────────────────────────────
// U-2: Valid request IDs are accepted
// ──────────────────────────────────────────

func TestRequestID_AcceptsValidHex(t *testing.T) {
	valid := []string{
		"abc123",
		"550e8400-e29b-41d4-a716-446655440000", // UUID
		strings.Repeat("a", 128),               // max length
		"A1B2C3",                               // uppercase
	}
	for _, id := range valid {
		if !requestIDPattern.MatchString(id) {
			t.Errorf("requestIDPattern should accept %q", id)
		}
	}
}

// ──────────────────────────────────────────
// U-3: Upstream error does not leak URLs
// ──────────────────────────────────────────

func TestUpstreamError_DoesNotLeakURL(t *testing.T) {
	// C-2: Error response should not contain internal URLs.
	// Set up a proxy with a provider that has an unreachable upstream.
	calc := newTestCalc()
	proxy := New(Config{
		Providers: []Provider{
			{Name: "openai", UpstreamURL: "http://192.168.0.1:99999"},
		},
	}, nil, calc)

	req := httptest.NewRequest("POST", "/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	proxy.ServeHTTP(rr, req)

	body := rr.Body.String()
	if strings.Contains(body, "192.168.0.1") {
		t.Errorf("response body leaks internal IP: %s", body)
	}
	if strings.Contains(body, "dial tcp") {
		t.Errorf("response body leaks error details: %s", body)
	}
	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rr.Code)
	}
}

// ──────────────────────────────────────────
// U-4: Circuit breaker failure count is capped
// ──────────────────────────────────────────

func TestCircuitBreaker_FailureCountCapped(t *testing.T) {
	// H-2: failures should not grow beyond threshold+1.
	cb := NewCircuitBreaker(CircuitBreakerConfig{Threshold: 3})
	for i := 0; i < 1000; i++ {
		cb.RecordFailure()
	}
	cb.mu.Lock()
	if cb.failures > cb.threshold+1 {
		t.Errorf("failures=%d, should be capped at %d", cb.failures, cb.threshold+1)
	}
	cb.mu.Unlock()
	if cb.State() != CircuitOpen {
		t.Errorf("expected CircuitOpen after threshold failures")
	}
}

// ──────────────────────────────────────────
// U-5: Stream ID uniqueness under concurrency
// ──────────────────────────────────────────

func TestStreamID_UniqueUnderConcurrency(t *testing.T) {
	// H-4: Stream IDs should not collide when generated concurrently.
	const n = 1000
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ids[idx] = "chatcmpl-" + generateSpanID()
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate stream ID: %s", id)
		}
		seen[id] = true
	}
}

// ──────────────────────────────────────────
// U-6: ParseModelName with no date suffix
// ──────────────────────────────────────────

func TestParseModelName_NoDateSuffix(t *testing.T) {
	info := ParseModelName("gpt-4-turbo")
	if info.Display != "gpt-4-turbo" {
		t.Errorf("Display=%q, want gpt-4-turbo", info.Display)
	}
	if info.VertexAI != "gpt-4-turbo" {
		t.Errorf("VertexAI=%q, want gpt-4-turbo", info.VertexAI)
	}
}

// ──────────────────────────────────────────
// U-7: ParseModelName with multiple dashes
// ──────────────────────────────────────────

func TestParseModelName_MultipleDashes(t *testing.T) {
	info := ParseModelName("claude-3-5-sonnet-20241022")
	if info.Display != "claude-3-5-sonnet" {
		t.Errorf("Display=%q, want claude-3-5-sonnet", info.Display)
	}
	if info.VertexAI != "claude-3-5-sonnet@20241022" {
		t.Errorf("VertexAI=%q, want claude-3-5-sonnet@20241022", info.VertexAI)
	}
}

// ──────────────────────────────────────────
// U-8: Fallback parser returns defaults
// ──────────────────────────────────────────

func TestFallbackParser_ReturnsDefaults(t *testing.T) {
	p := getParser("unknown-provider")
	model, content := p.ParseRequest([]byte(`{"model":"x"}`))
	if model != "" || content != "" {
		t.Errorf("fallback should return empty, got model=%q content=%q", model, content)
	}
	if p.IsStreaming(nil) {
		t.Error("fallback should not report streaming")
	}
	c, in, out := p.ParseResponse(nil)
	if c != "" || in != 0 || out != 0 {
		t.Errorf("fallback response parse should return zeros")
	}
}

// ──────────────────────────────────────────
// U-9: Google parser streaming falls back to standard
// ──────────────────────────────────────────

func TestGoogleParser_StreamingFallback(t *testing.T) {
	body := []byte(`{
		"candidates": [{"content": {"parts": [{"text": "hello"}]}}],
		"usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5}
	}`)
	p := &googleParser{}
	content, inTok, outTok := p.ParseStreamingResponse(body)
	if content != "hello" {
		t.Errorf("content=%q, want hello", content)
	}
	if inTok != 10 || outTok != 5 {
		t.Errorf("tokens in=%d out=%d, want 10 and 5", inTok, outTok)
	}
}

// ──────────────────────────────────────────
// U-10: Anthropic parser with missing usage
// ──────────────────────────────────────────

func TestAnthropicParser_EmptyUsage(t *testing.T) {
	body := []byte(`{"content": [{"type": "text", "text": "hi"}]}`)
	p := &anthropicParser{}
	content, in, out := p.ParseResponse(body)
	if content != "hi" {
		t.Errorf("content=%q, want hi", content)
	}
	if in != 0 || out != 0 {
		t.Errorf("expected zero tokens, got in=%d out=%d", in, out)
	}
}

// ──────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────

func newTestCalc() *costcalc.Calculator {
	return costcalc.New()
}
