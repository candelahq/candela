package proxy

import (
	"net/http"
	"testing"
)

func TestIsAllowedResponseHeader(t *testing.T) {
	allowed := []string{
		"Content-Type",
		"Content-Length",
		"Content-Encoding",
		"Cache-Control",
		"Vary",
		"Date",
		"Transfer-Encoding",
		"Connection",
		"Access-Control-Allow-Origin",
		"Access-Control-Expose-Headers",
		"Access-Control-Allow-Credentials",
		"Openai-Organization",
		"Openai-Processing-Ms",
		"X-Request-Id",
	}

	for _, h := range allowed {
		if !isAllowedResponseHeader(h) {
			t.Errorf("expected %q to be allowed", h)
		}
	}

	// Case-insensitive check.
	if !isAllowedResponseHeader("content-type") {
		t.Error("expected case-insensitive match for content-type")
	}
	if !isAllowedResponseHeader("CONTENT-TYPE") {
		t.Error("expected case-insensitive match for CONTENT-TYPE")
	}
}

func TestBlockedResponseHeaders(t *testing.T) {
	blocked := []string{
		"Set-Cookie",
		"Server",
		"X-Ratelimit-Limit-Requests",
		"X-Ratelimit-Remaining-Requests",
		"X-Ratelimit-Limit-Tokens",
		"X-Ratelimit-Remaining-Tokens",
		"X-Ratelimit-Reset-Requests",
		"X-Ratelimit-Reset-Tokens",
		"Openai-Version",
		"Anthropic-Ratelimit-Requests-Limit",
		"Anthropic-Ratelimit-Tokens-Limit",
		"Cf-Ray",
		"Cf-Cache-Status",
		"X-Cloud-Trace-Context",
		"Alt-Svc",
		"X-Goog-Request-Params",
		"Strict-Transport-Security",
		"X-Frame-Options",
		"X-Content-Type-Options",
	}

	for _, h := range blocked {
		if isAllowedResponseHeader(h) {
			t.Errorf("expected %q to be blocked but it was allowed", h)
		}
	}
}

func TestResponseHeaderAllowlist_Integration(t *testing.T) {
	upstreamHeaders := http.Header{
		"Content-Type":                   {"application/json"},
		"Content-Length":                 {"42"},
		"Server":                         {"openai/nginx"},
		"Set-Cookie":                     {"session=abc; HttpOnly"},
		"X-Ratelimit-Limit-Requests":     {"3500"},
		"X-Ratelimit-Remaining-Requests": {"3499"},
		"Cache-Control":                  {"no-cache"},
		"Openai-Processing-Ms":           {"150"},
		"Cf-Ray":                         {"abc123-SJC"},
		"Strict-Transport-Security":      {"max-age=31536000"},
	}

	forwarded := make(http.Header)
	for k, vv := range upstreamHeaders {
		if !isAllowedResponseHeader(k) {
			continue
		}
		for _, v := range vv {
			forwarded.Add(k, v)
		}
	}

	// Allowed headers present.
	if forwarded.Get("Content-Type") != "application/json" {
		t.Error("Content-Type should be forwarded")
	}
	if forwarded.Get("Content-Length") != "42" {
		t.Error("Content-Length should be forwarded")
	}
	if forwarded.Get("Cache-Control") != "no-cache" {
		t.Error("Cache-Control should be forwarded")
	}
	if forwarded.Get("Openai-Processing-Ms") != "150" {
		t.Error("Openai-Processing-Ms should be forwarded")
	}

	// Blocked headers absent.
	if forwarded.Get("Server") != "" {
		t.Error("Server should be stripped")
	}
	if forwarded.Get("Set-Cookie") != "" {
		t.Error("Set-Cookie should be stripped")
	}
	if forwarded.Get("X-Ratelimit-Limit-Requests") != "" {
		t.Error("rate limit headers should be stripped")
	}
	if forwarded.Get("Cf-Ray") != "" {
		t.Error("Cloudflare headers should be stripped")
	}
	if forwarded.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS should be stripped")
	}
}
