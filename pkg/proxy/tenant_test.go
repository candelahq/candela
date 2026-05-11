package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
)

// ── Unit Tests: parseBaggage ───────────────────────────────────────────────

// UNIT-1: Valid candela.tenant_id values are correctly extracted from Baggage.
func TestParseBaggage_ValidTenantID(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"simple", "candela.tenant_id=acme-corp", "acme-corp"},
		{"with other keys", "svc.version=1.0,candela.tenant_id=tenant-42,foo=bar", "tenant-42"},
		{"with property", "candela.tenant_id=dot.tenant;ttl=100", "dot.tenant"},
		{"underscore", "candela.tenant_id=tenant_42", "tenant_42"},
		{"dot domain style", "candela.tenant_id=acme.corp", "acme.corp"},
		{"spaces around", "  candela.tenant_id = acme-corp ", "acme-corp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBaggage(tc.header)
			if got != tc.want {
				t.Errorf("parseBaggage(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

// UNIT-2: Invalid candela.tenant_id values are silently discarded.
func TestParseBaggage_InvalidTenantID_Discarded(t *testing.T) {
	cases := []struct {
		name   string
		header string
	}{
		{"space in value", "candela.tenant_id=acme corp"},
		{"at sign", "candela.tenant_id=acme@corp"},
		{"too long", "candela.tenant_id=" + strings.Repeat("a", 129)},
		{"slash injection", "candela.tenant_id=../../etc"},
		{"empty value", "candela.tenant_id="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBaggage(tc.header)
			if got != "" {
				t.Errorf("parseBaggage(%q) = %q, want empty (invalid value should be discarded)", tc.header, got)
			}
		})
	}
}

// UNIT-3: Empty and no-match baggage headers return empty string without panicking.
func TestParseBaggage_EmptyAndMissing(t *testing.T) {
	if got := parseBaggage(""); got != "" {
		t.Errorf(`parseBaggage("") = %q, want ""`, got)
	}
	if got := parseBaggage("svc.version=1.0,other=stuff"); got != "" {
		t.Errorf("parseBaggage(no tenant key) = %q, want \"\"", got)
	}
}

// UNIT-4: CRIT-4 fix — invalid first duplicate entry is skipped; valid second entry wins.
func TestParseBaggage_InvalidFirstValidSecond(t *testing.T) {
	// First value has a space (invalid), second is valid — should return second.
	header := "candela.tenant_id=bad value,candela.tenant_id=good-tenant"
	got := parseBaggage(header)
	if got != "good-tenant" {
		t.Errorf("parseBaggage(%q) = %q, want good-tenant (invalid first entry should be skipped)", header, got)
	}
}

// UNIT-4b: parseBaggageHeaders joins multiple header values (W3C allows multiple
// Baggage: header instances in a single HTTP request).
func TestParseBaggageHeaders_MultipleHeaders(t *testing.T) {
	// Simulate two separate Baggage header lines — r.Header.Values returns both.
	values := []string{"svc.version=1.0", "candela.tenant_id=multi-tenant,other=val"}
	got := parseBaggageHeaders(values)
	if got != "multi-tenant" {
		t.Errorf("parseBaggageHeaders(%v) = %q, want multi-tenant", values, got)
	}
}

// UNIT-4c: Baggage key matching is case-insensitive per RFC 8941.
func TestParseBaggage_CaseInsensitiveKey(t *testing.T) {
	cases := []string{
		"Candela.Tenant_Id=acme-corp",
		"CANDELA.TENANT_ID=acme-corp",
		"candela.TENANT_ID=acme-corp",
	}
	for _, h := range cases {
		if got := parseBaggage(h); got != "acme-corp" {
			t.Errorf("parseBaggage(%q) = %q, want acme-corp (key should be case-insensitive)", h, got)
		}
	}
}

// UNIT-5: tenantIDPattern enforces the allowed character set and length limits.
func TestTenantIDPattern(t *testing.T) {
	valid := []string{
		"acme-corp", "tenant_42", "dot.tenant", "A1b2C3",
		strings.Repeat("a", 128), // exactly max length
	}
	invalid := []string{
		"", "acme corp", "acme@corp", "../../etc",
		strings.Repeat("a", 129), // one over max
		"acme/corp", "acme:corp",
	}
	for _, v := range valid {
		if !tenantIDPattern.MatchString(v) {
			t.Errorf("tenantIDPattern should match %q", v)
		}
	}
	for _, v := range invalid {
		if tenantIDPattern.MatchString(v) {
			t.Errorf("tenantIDPattern should NOT match %q", v)
		}
	}
}

// ── Integration Tests: tenant_id propagated through proxy span ────────────
// These reuse the mockSubmitter and helpers defined in proxy_test.go (same package).

// waitForSpan polls the mock submitter until at least one span is available or timeout.
func waitForSpan(sub *mockSubmitter) bool {
	for i := 0; i < 50; i++ {
		if len(sub.getSpans()) > 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// INTEG-1: tenant_id from X-Candela-Tenant-Id header is persisted in the emitted span.
func TestProxy_TenantID_HeaderPropagatedToSpan(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-t","object":"chat.completion","model":"gpt-4o",
			"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop","index":0}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`))
	}))
	defer upstream.Close()

	sub := &mockSubmitter{}
	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "proj-test",
	}, sub, costcalc.New())

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("X-Candela-Tenant-Id", "acme-corp")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if !waitForSpan(sub) {
		t.Fatal("no spans submitted within timeout")
	}
	spans := sub.getSpans()
	if spans[0].TenantID != "acme-corp" {
		t.Errorf("span.TenantID = %q, want acme-corp", spans[0].TenantID)
	}
}

// INTEG-2: candela.tenant_id in W3C Baggage takes precedence over the explicit header.
func TestProxy_TenantID_BaggageWinsOverHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-b","object":"chat.completion","model":"gpt-4o",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop","index":0}],
			"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}
		}`))
	}))
	defer upstream.Close()

	sub := &mockSubmitter{}
	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "proj-test",
	}, sub, costcalc.New())

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Baggage", "candela.tenant_id=baggage-tenant,svc=myapp")
	req.Header.Set("X-Candela-Tenant-Id", "header-tenant") // must be overridden

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if !waitForSpan(sub) {
		t.Fatal("no spans submitted within timeout")
	}
	if got := sub.getSpans()[0].TenantID; got != "baggage-tenant" {
		t.Errorf("TenantID = %q, want baggage-tenant (Baggage must win over header)", got)
	}
}

// INTEG-3: No tenant headers → span.TenantID is empty string (not an error).
func TestProxy_TenantID_AbsentHeaderLeavesEmpty(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-n","object":"chat.completion","model":"gpt-4o",
			"choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop","index":0}],
			"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
		}`))
	}))
	defer upstream.Close()

	sub := &mockSubmitter{}
	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "proj-test",
	}, sub, costcalc.New())

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"ping"}]}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	// No tenant headers.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if !waitForSpan(sub) {
		t.Fatal("no spans submitted within timeout")
	}
	if got := sub.getSpans()[0].TenantID; got != "" {
		t.Errorf("TenantID = %q, want empty when no tenant headers provided", got)
	}
}

// INTEG-4: Invalid X-Candela-Tenant-Id is silently rejected; request still succeeds.
func TestProxy_TenantID_InvalidHeaderRejected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-i","object":"chat.completion","model":"gpt-4o",
			"choices":[{"message":{"role":"assistant","content":"safe"},"finish_reason":"stop","index":0}],
			"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}
		}`))
	}))
	defer upstream.Close()

	sub := &mockSubmitter{}
	p := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "proj-test",
	}, sub, costcalc.New())

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("X-Candela-Tenant-Id", "../../etc/passwd") // path injection attempt

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Must not return an error to the client — invalid tenant IDs are silently discarded.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (invalid tenant must not break the request)", resp.StatusCode)
	}

	if !waitForSpan(sub) {
		t.Fatal("no spans submitted within timeout")
	}
	if got := sub.getSpans()[0].TenantID; got != "" {
		t.Errorf("TenantID = %q, want empty (injection attempt must be rejected)", got)
	}
}
