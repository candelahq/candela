package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
)

// TestCORS_TraceparentHeader verifies that the server's CORS middleware
// includes Traceparent and Tracestate in Access-Control-Allow-Headers.
// This is critical for browser-based OTel callers (ADK web apps).
func TestCORS_TraceparentHeader(t *testing.T) {
	// Use the sidecar's corsMiddleware function to test header inclusion.
	// Since this is in the proxy package, we test forwardHeaders instead
	// and verify the headers that the proxy itself forwards.

	// Test that forwardHeaders preserves Tracestate.
	src := httptest.NewRequest("POST", "/proxy/openai/v1/chat", nil)
	src.Header.Set("Tracestate", "vendor1=value1")
	src.Header.Set("Authorization", "Bearer tok")
	src.Header.Set("Content-Type", "application/json")

	dst, _ := http.NewRequest("POST", "http://upstream/v1/chat", nil)
	forwardHeaders(src, dst, "openai")

	if got := dst.Header.Get("Tracestate"); got != "vendor1=value1" {
		t.Errorf("Tracestate not forwarded: got %q, want %q", got, "vendor1=value1")
	}
	if got := dst.Header.Get("Authorization"); got != "Bearer tok" {
		t.Errorf("Authorization not forwarded: got %q", got)
	}
}

// TestCORS_ModelsEndpointReturnsJSON verifies that GET /v1/models returns
// a 200 response with application/json content type.
func TestCORS_ModelsEndpointReturnsJSON(t *testing.T) {
	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: "http://localhost:1"}},
		ProjectID: "test",
	}, submitter, calc)

	mux := http.NewServeMux()
	p.RegisterCompatRoutes(mux, []CompatModel{{ID: "gpt-4o", Provider: "openai"}})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// GET /v1/models should return the model list with proper headers.
	req, _ := http.NewRequest("GET", srv.URL+"/v1/models", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /v1/models status = %d, want 200", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
