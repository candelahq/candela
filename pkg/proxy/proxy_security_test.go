package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
)

// TestCompatRoute_ModelNameInjection verifies that user-supplied model names
// are JSON-encoded in error responses to prevent XSS/injection.
func TestCompatRoute_ModelNameInjection(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := costcalc.New()

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
	calc := costcalc.New()

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
	calc := costcalc.New()

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
	calc := costcalc.New()
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
	calc := costcalc.New()
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
	calc := costcalc.New()

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
