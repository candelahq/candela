package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/proxy"
)

func TestConfigAPI_GetConfig_NilProxy(t *testing.T) {
	api := &localAPI{cloudProxy: nil}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_local/api/config", api.handleGetConfig)

	req := httptest.NewRequest(http.MethodGet, "/_local/api/config", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp cachingConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Caching.Anthropic != "off" {
		t.Errorf("caching.anthropic = %q, want 'off' when cloudProxy is nil", resp.Caching.Anthropic)
	}
	// Verify Gemini section is always present.
	if resp.Caching.Gemini.Mode != "implicit" {
		t.Errorf("caching.gemini.mode = %q, want 'implicit'", resp.Caching.Gemini.Mode)
	}
	if resp.Caching.Gemini.Info == "" {
		t.Error("caching.gemini.info should not be empty")
	}
}

func TestConfigAPI_GetConfig_WithProxy(t *testing.T) {
	ft := &proxy.AnthropicFormatTranslator{}
	ft.SetCachingMode(proxy.CachingSystemOnly)
	p := &proxy.Proxy{}
	// We can't easily set providers on Proxy (unexported), so test via the
	// localAPI directly by setting cloudProxy to a proxy with the mode.
	api := &localAPI{cloudProxy: p}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_local/api/config", api.handleGetConfig)

	req := httptest.NewRequest(http.MethodGet, "/_local/api/config", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp cachingConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	// Proxy with no providers returns "off".
	if resp.Caching.Anthropic != "off" {
		t.Errorf("caching.anthropic = %q, want 'off'", resp.Caching.Anthropic)
	}
	// Gemini is always implicit.
	if resp.Caching.Gemini.Mode != "implicit" {
		t.Errorf("caching.gemini.mode = %q, want 'implicit'", resp.Caching.Gemini.Mode)
	}
}

func TestConfigAPI_SetCaching_NilProxy_Returns503(t *testing.T) {
	api := &localAPI{cloudProxy: nil}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /_local/api/config/caching", api.handleSetCaching)

	req := httptest.NewRequest(http.MethodPost, "/_local/api/config/caching", strings.NewReader(`{"anthropic": "auto"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when cloudProxy is nil", w.Code)
	}
}

func TestConfigAPI_SetCaching_ValidModes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"anthropic": "auto"}`, "auto"},
		{`{"anthropic": "off"}`, "off"},
		{`{"anthropic": "system-only"}`, "system-only"},
		{`{"anthropic": "system"}`, "system-only"},
		{`{"anthropic": "true"}`, "auto"},
		{`{"anthropic": "false"}`, "off"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			// Use a real proxy with an Anthropic translator.
			ft := &proxy.AnthropicFormatTranslator{}
			p := proxy.NewProxyForTest(map[string]proxy.Provider{
				"anthropic": {Name: "anthropic", FormatTranslator: ft},
			})
			api := &localAPI{cloudProxy: p}
			mux := http.NewServeMux()
			mux.HandleFunc("POST /_local/api/config/caching", api.handleSetCaching)

			req := httptest.NewRequest(http.MethodPost, "/_local/api/config/caching", strings.NewReader(tt.input))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}

			var resp cachingConfigResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if resp.Caching.Anthropic != tt.expected {
				t.Errorf("caching.anthropic = %q, want %q", resp.Caching.Anthropic, tt.expected)
			}

			// Verify the mode actually propagated to the translator.
			if got := ft.GetCachingMode(); string(got) != tt.expected {
				t.Errorf("translator mode = %q, want %q", got, tt.expected)
			}

			// Verify Gemini section is always present in response.
			if resp.Caching.Gemini.Mode != "implicit" {
				t.Errorf("caching.gemini.mode = %q, want 'implicit'", resp.Caching.Gemini.Mode)
			}
		})
	}
}

func TestConfigAPI_SetCaching_BadJSON(t *testing.T) {
	api := &localAPI{cloudProxy: nil}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /_local/api/config/caching", api.handleSetCaching)

	req := httptest.NewRequest(http.MethodPost, "/_local/api/config/caching", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestConfigAPI_SetCaching_EmptyBody(t *testing.T) {
	// Empty JSON body with nil proxy → 503.
	api := &localAPI{cloudProxy: nil}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /_local/api/config/caching", api.handleSetCaching)

	req := httptest.NewRequest(http.MethodPost, "/_local/api/config/caching", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Nil proxy should return 503.
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
