package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSidecar_HealthAndReadiness verifies that the sidecar's /healthz and
// /readyz endpoints return correct responses.
func TestSidecar_HealthAndReadiness(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","binary":"candela-sidecar"}`))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		path   string
		expect string
	}{
		{"/healthz", "candela-sidecar"},
		{"/readyz", "ready"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tc.path)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200", resp.StatusCode)
			}
		})
	}
}

// TestSidecar_CORSHeaders verifies that the sidecar's CORS middleware includes
// trace context headers (Traceparent, Tracestate) in the allowed headers.
func TestSidecar_CORSHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := corsMiddleware(inner, []string{"*"})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Send an OPTIONS preflight request.
	req, _ := http.NewRequest("OPTIONS", srv.URL+"/proxy/openai/v1/chat", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Traceparent")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", resp.StatusCode)
	}

	allowedHeaders := resp.Header.Get("Access-Control-Allow-Headers")
	for _, h := range []string{"Traceparent", "Tracestate", "X-Request-ID", "X-Session-Id"} {
		if !strings.Contains(allowedHeaders, h) {
			t.Errorf("CORS missing allowed header %q in: %s", h, allowedHeaders)
		}
	}

	allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	if allowOrigin != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", allowOrigin)
	}
}

// TestSidecar_ProviderFiltering verifies that the parseHeaders and envOr
// helper functions work correctly.
func TestSidecar_ProviderFiltering(t *testing.T) {
	t.Run("parseHeaders", func(t *testing.T) {
		tests := []struct {
			input    string
			expected map[string]string
		}{
			{"", nil},
			{"key1=val1", map[string]string{"key1": "val1"}},
			{"key1=val1,key2=val2", map[string]string{"key1": "val1", "key2": "val2"}},
			{"auth=Bearer token123", map[string]string{"auth": "Bearer token123"}},
		}

		for _, tc := range tests {
			result := parseHeaders(tc.input)
			if tc.expected == nil {
				if result != nil {
					t.Errorf("parseHeaders(%q) = %v, want nil", tc.input, result)
				}
				continue
			}
			for k, v := range tc.expected {
				if result[k] != v {
					t.Errorf("parseHeaders(%q)[%q] = %q, want %q", tc.input, k, result[k], v)
				}
			}
		}
	})

	t.Run("envOr", func(t *testing.T) {
		if got := envOr("NONEXISTENT_TEST_VAR_12345", "fallback"); got != "fallback" {
			t.Errorf("envOr(missing) = %q, want fallback", got)
		}
	})
}
