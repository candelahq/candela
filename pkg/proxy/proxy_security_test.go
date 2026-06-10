package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCompatRoute_ModelNameInjection verifies that user-supplied model names
// are JSON-encoded in error responses to prevent XSS/injection.
func TestCompatRoute_ModelNameInjection(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterCompatRoutes(mux, []CompatModel{{ID: "gpt-4o", Provider: "openai"}})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Send a request with a malicious model name containing JSON-breaking characters.
	maliciousModel := `", "injected": "true`
	body := fmt.Sprintf(`{"model": %q}`, maliciousModel)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	// The response body must be valid JSON.
	respBody, _ := io.ReadAll(resp.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		t.Fatalf("response is not valid JSON (injection succeeded): body=%s, err=%v", respBody, err)
	}

	// Verify the model name is properly contained in the error message.
	errObj, ok := parsed["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object, got: %s", respBody)
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, maliciousModel) {
		t.Errorf("error message should contain model name, got: %s", msg)
	}
}

// TestHandleProxy_MaxBytesError413 verifies that oversized request bodies
// return 413 (not 400) in the main proxy path.
func TestHandleProxy_MaxBytesError413(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be reached for oversized body")
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Create a body larger than the 10MB limit.
	bigBody := strings.Repeat("x", 11<<20) // 11MB
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(bigBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d (413)", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
}

// TestHandleProxy_UnknownProviderSanitized verifies that unknown provider names
// are not reflected raw in error responses.
func TestHandleProxy_UnknownProviderSanitized(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://localhost:1"}},
		ProjectID: "test",
	}, submitter, calc)

	// Use ServeHTTP directly (bypasses mux pattern matching) to reach the
	// "unknown provider" code path with a malicious provider name in the URL.
	maliciousProvider := "<script>alert(1)</script>"
	req := httptest.NewRequest("POST",
		fmt.Sprintf("/proxy/%s/v1/chat", maliciousProvider),
		strings.NewReader(`{"model":"test"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	body := w.Body.String()

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	// The response body must NOT contain the raw malicious input.
	if strings.Contains(body, "<script>") {
		t.Errorf("malicious provider name reflected in response: %s", body)
	}
}

// TestRequestID_UsesTraceIDFormat verifies that auto-generated request IDs
// use the 32-char hex trace ID format (not concatenated span IDs).
func TestRequestID_UsesTraceIDFormat(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if len(rid) != 32 {
			t.Errorf("upstream received X-Request-ID with len=%d, want 32", len(rid))
		}
		// Verify it's valid lowercase hex.
		for _, c := range rid {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				t.Errorf("X-Request-ID contains non-hex char: %c in %q", c, rid)
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()
	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")
	// No X-Request-ID → should be auto-generated.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	rid := resp.Header.Get("X-Request-ID")
	if len(rid) != 32 {
		t.Errorf("response X-Request-ID len=%d, want 32: %q", len(rid), rid)
	}
}

// TestHandleProxy_EmptyPath verifies that a malformed proxy path returns 400.
func TestHandleProxy_EmptyPath(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()
	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://localhost:1"}},
		ProjectID: "test",
	}, submitter, calc)

	// Call handleProxy directly with a bad path.
	req := httptest.NewRequest("POST", "/proxy/openai", nil)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestHandleProxy_GETModelsRoute verifies the synthetic /v1/models route.
func TestHandleProxy_GETModelsRoute(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := newCalcWithTestModels()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: "http://localhost:1"}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/proxy/anthropic/v1/models")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("response is not valid JSON: %s", body)
	}

	if result["object"] != "list" {
		t.Errorf("expected object=list, got %v", result["object"])
	}
}

// ── Model Name Sanitization (SSRF Prevention) ───────────────────────────────
// These tests validate safeModelNameRe which guards against path traversal
// in Vertex AI URL construction. See the regex definition in proxy.go for
// the full threat model.

func TestSafeModelNameRe_ValidModels(t *testing.T) {
	// All real model names the proxy handles should pass.
	validModels := []string{
		"gpt-4o",
		"gpt-4o-mini",
		"claude-sonnet-4-20250514",
		"claude-sonnet-4@20250514",
		"gemini-2.5-flash",
		"gemini-2.5-pro-preview-06-05",
		"mistral-large-2411",
		"deepseek-ai/deepseek-chat",
		"Qwen/Qwen2.5-Coder-32B-Instruct",
		"meta-llama/Llama-3.1-405B",
		"o1-preview",
		"text-embedding-3-small",
	}
	for _, model := range validModels {
		if !isSafeModelName(model) {
			t.Errorf("safeModelNameRe rejected valid model: %q", model)
		}
	}
}

func TestSafeModelNameRe_PathTraversal(t *testing.T) {
	// Path traversal attempts — all must be rejected.
	attacks := []struct {
		name  string
		model string
	}{
		{"dot-dot-slash", "../../other-project/locations/us-central1/models/gemini-2.5-flash"},
		{"backslash traversal", `..\..\other-project\models\gemini-2.5-flash`},
		{"url encoded dots", "%2e%2e/evil"},
		{"null byte", "gemini-2.5-flash\x00../../evil"},
		{"newline injection", "gemini-2.5-flash\n../../evil"},
		{"space injection", "gemini 2.5-flash"},
		{"query string", "gemini-2.5-flash?param=evil"},
		{"fragment", "gemini-2.5-flash#evil"},
		{"html tag", "<script>alert(1)</script>"},
		{"semicolon", "gemini-2.5-flash;rm -rf /"},
		{"pipe", "gemini-2.5-flash|cat /etc/passwd"},
		{"backtick", "gemini-2.5-flash`id`"},
	}
	for _, tc := range attacks {
		t.Run(tc.name, func(t *testing.T) {
			if isSafeModelName(tc.model) {
				t.Errorf("safeModelNameRe ALLOWED dangerous model name: %q", tc.model)
			}
		})
	}
}

func TestSafeModelNameRe_EmptyString(t *testing.T) {
	// Empty model names should be rejected by the regex (the proxy handles
	// empty separately before reaching the regex check).
	if isSafeModelName("") {
		t.Error("isSafeModelName should reject empty string")
	}
}

func TestSafeModelName_DotDotWithSlash(t *testing.T) {
	// This is the specific attack vector: ".." combined with "/" forms
	// a path traversal. Both chars are individually valid (dots for versions,
	// slashes for org/model), but together they're dangerous.
	attacks := []string{
		"../gemini-2.5-flash",
		"deepseek-ai/../../../evil",
		"Qwen/..%2f../../evil",
	}
	for _, model := range attacks {
		if isSafeModelName(model) {
			t.Errorf("isSafeModelName ALLOWED dot-dot attack: %q", model)
		}
	}
}

func TestSafeModelName_SingleDotAllowed(t *testing.T) {
	// Single dots are fine — used in version numbers like "Qwen2.5".
	models := []string{
		"Qwen2.5-Coder",
		"text-embedding-3.small",
		"v1.0-release",
	}
	for _, model := range models {
		if !isSafeModelName(model) {
			t.Errorf("isSafeModelName rejected valid dotted model: %q", model)
		}
	}
}
