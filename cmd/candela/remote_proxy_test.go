package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/candelahq/candela/pkg/proxy"
)

func TestInjectCacheHeaders(t *testing.T) {
	tests := []struct {
		name           string
		cachingMode    string
		cacheTTL       string
		existingHeader map[string]string // pre-set headers on request
		wantCaching    string            // expected X-Candela-Caching value ("" = absent)
		wantCacheTTL   string            // expected X-Candela-Cache-TTL value ("" = absent)
	}{
		{
			name:         "both set, no existing headers",
			cachingMode:  "auto",
			cacheTTL:     "1h",
			wantCaching:  "auto",
			wantCacheTTL: "1h",
		},
		{
			name:         "only caching mode set",
			cachingMode:  "system-only",
			wantCaching:  "system-only",
			wantCacheTTL: "",
		},
		{
			name:         "only cache TTL set",
			cacheTTL:     "5m",
			wantCaching:  "",
			wantCacheTTL: "5m",
		},
		{
			name:         "empty config — no headers injected",
			wantCaching:  "",
			wantCacheTTL: "",
		},
		{
			name:           "existing TTL header — not overwritten (SDK override wins)",
			cacheTTL:       "1h",
			existingHeader: map[string]string{proxy.CacheTTLHeader: "5m"},
			wantCacheTTL:   "5m", // original preserved
			wantCaching:    "",
		},
		{
			name:           "existing caching header — not overwritten",
			cachingMode:    "auto",
			existingHeader: map[string]string{proxy.CachingHeader: "off"},
			wantCaching:    "off", // original preserved
			wantCacheTTL:   "",
		},
		{
			name:        "both existing — config does not overwrite either",
			cachingMode: "auto",
			cacheTTL:    "1h",
			existingHeader: map[string]string{
				proxy.CachingHeader:  "off",
				proxy.CacheTTLHeader: "5m",
			},
			wantCaching:  "off",
			wantCacheTTL: "5m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/messages", nil)
			for k, v := range tt.existingHeader {
				req.Header.Set(k, v)
			}

			var cfg VertexAIConfig
			cfg.Anthropic.CachingMode = tt.cachingMode
			cfg.Anthropic.CacheTTL = tt.cacheTTL

			injectCacheHeaders(req, cfg)

			gotCaching := req.Header.Get(proxy.CachingHeader)
			gotTTL := req.Header.Get(proxy.CacheTTLHeader)

			if gotCaching != tt.wantCaching {
				t.Errorf("X-Candela-Caching = %q, want %q", gotCaching, tt.wantCaching)
			}
			if gotTTL != tt.wantCacheTTL {
				t.Errorf("X-Candela-Cache-TTL = %q, want %q", gotTTL, tt.wantCacheTTL)
			}
		})
	}
}
