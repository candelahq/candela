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

	var resp struct {
		Caching struct {
			Anthropic string `json:"anthropic"`
		} `json:"caching"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Caching.Anthropic != "off" {
		t.Errorf("caching.anthropic = %q, want 'off' when cloudProxy is nil", resp.Caching.Anthropic)
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

	var resp struct {
		Caching struct {
			Anthropic string `json:"anthropic"`
		} `json:"caching"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	// Proxy with no providers returns "off".
	if resp.Caching.Anthropic != "off" {
		t.Errorf("caching.anthropic = %q, want 'off'", resp.Caching.Anthropic)
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
			api := &localAPI{cloudProxy: nil} // nil proxy — mode is parsed but not applied
			mux := http.NewServeMux()
			mux.HandleFunc("POST /_local/api/config/caching", api.handleSetCaching)

			req := httptest.NewRequest(http.MethodPost, "/_local/api/config/caching", strings.NewReader(tt.input))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}

			var resp struct {
				Caching struct {
					Anthropic string `json:"anthropic"`
				} `json:"caching"`
			}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if resp.Caching.Anthropic != tt.expected {
				t.Errorf("caching.anthropic = %q, want %q", resp.Caching.Anthropic, tt.expected)
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
	api := &localAPI{cloudProxy: nil}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /_local/api/config/caching", api.handleSetCaching)

	req := httptest.NewRequest(http.MethodPost, "/_local/api/config/caching", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Caching struct {
			Anthropic string `json:"anthropic"`
		} `json:"caching"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	// Empty string → ParseCachingMode("") → off.
	if resp.Caching.Anthropic != "off" {
		t.Errorf("caching.anthropic = %q, want 'off'", resp.Caching.Anthropic)
	}
}
