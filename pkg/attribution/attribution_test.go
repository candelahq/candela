package attribution

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── IDPattern ────────────────────────────────────────────────────────────────

func TestIDPattern_Valid(t *testing.T) {
	valid := []string{
		"my-tenant",
		"tenant_123",
		"abc.def.ghi",
		"a",
		"A",
		"123",
		"my-tenant.job_123",
		"a-b-c-d-e-f",
		"aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789-._",
	}
	for _, v := range valid {
		if !IDPattern.MatchString(v) {
			t.Errorf("IDPattern should match %q", v)
		}
	}
}

func TestIDPattern_Invalid(t *testing.T) {
	invalid := []string{
		"",                        // empty
		"has space",               // spaces
		"has/slash",               // path separator — injection risk
		"../traversal",            // path traversal
		"has\ttab",                // tab
		"has\nnewline",            // newline — log injection
		"has;semicolon",           // baggage delimiter injection
		"has,comma",               // baggage member separator
		"has=equals",              // baggage key-value separator
		string(make([]byte, 129)), // too long (129 chars)
		"<script>alert(1)</script>",
	}
	for _, v := range invalid {
		if IDPattern.MatchString(v) {
			t.Errorf("IDPattern should NOT match %q", v)
		}
	}
}

func TestIDPattern_MaxLength(t *testing.T) {
	maxLen := make([]byte, 128)
	for i := range maxLen {
		maxLen[i] = 'a'
	}
	if !IDPattern.MatchString(string(maxLen)) {
		t.Error("IDPattern should accept 128-char string")
	}
	tooLong := append(maxLen, 'a')
	if IDPattern.MatchString(string(tooLong)) {
		t.Error("IDPattern should reject 129-char string")
	}
}

// ── ParseBaggage ─────────────────────────────────────────────────────────────

func TestParseBaggage_Basic(t *testing.T) {
	tenantID, jobID := ParseBaggage("candela.tenant_id=my-tenant,candela.job_id=build-42")
	if tenantID != "my-tenant" {
		t.Errorf("tenant_id = %q, want %q", tenantID, "my-tenant")
	}
	if jobID != "build-42" {
		t.Errorf("job_id = %q, want %q", jobID, "build-42")
	}
}

func TestParseBaggage_Empty(t *testing.T) {
	tenantID, jobID := ParseBaggage("")
	if tenantID != "" || jobID != "" {
		t.Errorf("empty baggage should return empty, got %q/%q", tenantID, jobID)
	}
}

func TestParseBaggage_CaseInsensitive(t *testing.T) {
	// RFC 8941: Baggage keys are case-insensitive.
	tenantID, jobID := ParseBaggage("Candela.Tenant_ID=my-tenant,CANDELA.JOB_ID=build-42")
	if tenantID != "my-tenant" {
		t.Errorf("case-insensitive tenant_id = %q, want %q", tenantID, "my-tenant")
	}
	if jobID != "build-42" {
		t.Errorf("case-insensitive job_id = %q, want %q", jobID, "build-42")
	}
}

func TestParseBaggage_WithProperties(t *testing.T) {
	// W3C Baggage allows semicolon-separated properties: "key=val;prop1;prop2"
	tenantID, _ := ParseBaggage("candela.tenant_id=my-tenant;source=proxy")
	if tenantID != "my-tenant" {
		t.Errorf("properties should be stripped, got %q", tenantID)
	}
}

func TestParseBaggage_RightmostWins(t *testing.T) {
	// W3C spec: the right-most occurrence of a key wins.
	tenantID, _ := ParseBaggage("candela.tenant_id=first,candela.tenant_id=second")
	if tenantID != "second" {
		t.Errorf("right-most should win, got %q", tenantID)
	}
}

func TestParseBaggage_InvalidValueDiscarded(t *testing.T) {
	// Invalid values should be discarded, not crash.
	tenantID, _ := ParseBaggage("candela.tenant_id=valid-tenant,candela.tenant_id=invalid/path")
	if tenantID != "valid-tenant" {
		t.Errorf("should keep last valid, got %q", tenantID)
	}
}

func TestParseBaggage_OnlyTenantID(t *testing.T) {
	tenantID, jobID := ParseBaggage("candela.tenant_id=solo-tenant")
	if tenantID != "solo-tenant" {
		t.Errorf("tenant_id = %q, want %q", tenantID, "solo-tenant")
	}
	if jobID != "" {
		t.Errorf("job_id should be empty, got %q", jobID)
	}
}

func TestParseBaggage_WithOtherKeys(t *testing.T) {
	// Non-candela keys should be ignored.
	tenantID, jobID := ParseBaggage("other.key=val,candela.tenant_id=t1,random=noise")
	if tenantID != "t1" {
		t.Errorf("tenant_id = %q, want %q", tenantID, "t1")
	}
	if jobID != "" {
		t.Errorf("job_id should be empty, got %q", jobID)
	}
}

func TestParseBaggage_WhitespaceHandling(t *testing.T) {
	// Whitespace around members, keys, and values should be trimmed.
	tenantID, jobID := ParseBaggage(" candela.tenant_id = my-tenant , candela.job_id = build-42 ")
	if tenantID != "my-tenant" {
		t.Errorf("tenant_id = %q, want %q", tenantID, "my-tenant")
	}
	if jobID != "build-42" {
		t.Errorf("job_id = %q, want %q", jobID, "build-42")
	}
}

func TestParseBaggage_MalformedEntries(t *testing.T) {
	// Entries without = should be skipped.
	tenantID, _ := ParseBaggage("noequals,candela.tenant_id=ok,=nokey,justvalue")
	if tenantID != "ok" {
		t.Errorf("should skip malformed, got %q", tenantID)
	}
}

// ── FromRequest ──────────────────────────────────────────────────────────────

func TestFromRequest_BaggagePrecedence(t *testing.T) {
	// Baggage should take precedence over X-Candela-* headers.
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Baggage", "candela.tenant_id=baggage-tenant")
	req.Header.Set("X-Candela-Tenant-Id", "header-tenant")

	attr := FromRequest(req)
	if attr.TenantID != "baggage-tenant" {
		t.Errorf("Baggage should win, got %q", attr.TenantID)
	}
}

func TestFromRequest_HeaderFallback(t *testing.T) {
	// Without Baggage, X-Candela-* headers are used.
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-Candela-Tenant-Id", "header-tenant")
	req.Header.Set("X-Candela-Job-Id", "header-job")

	attr := FromRequest(req)
	if attr.TenantID != "header-tenant" {
		t.Errorf("tenant_id = %q, want %q", attr.TenantID, "header-tenant")
	}
	if attr.JobID != "header-job" {
		t.Errorf("job_id = %q, want %q", attr.JobID, "header-job")
	}
}

func TestFromRequest_NoAttribution(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	attr := FromRequest(req)
	if attr.TenantID != "" || attr.JobID != "" {
		t.Errorf("should be empty, got %+v", attr)
	}
}

func TestFromRequest_InvalidHeaderDiscarded(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-Candela-Tenant-Id", "../traversal")
	req.Header.Set("X-Candela-Job-Id", "valid-job")

	attr := FromRequest(req)
	if attr.TenantID != "" {
		t.Errorf("invalid tenant should be discarded, got %q", attr.TenantID)
	}
	if attr.JobID != "valid-job" {
		t.Errorf("job_id = %q, want %q", attr.JobID, "valid-job")
	}
}

func TestFromRequest_MultipleBaggageHeaders(t *testing.T) {
	// W3C allows multiple Baggage headers in a single request.
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Add("Baggage", "candela.tenant_id=t1")
	req.Header.Add("Baggage", "candela.job_id=j1")

	attr := FromRequest(req)
	if attr.TenantID != "t1" {
		t.Errorf("tenant_id = %q, want %q", attr.TenantID, "t1")
	}
	if attr.JobID != "j1" {
		t.Errorf("job_id = %q, want %q", attr.JobID, "j1")
	}
}

func TestFromRequest_BaggageTenantOnly_HeaderJob(t *testing.T) {
	// Baggage has tenant, header has job — both should be used.
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Baggage", "candela.tenant_id=baggage-t")
	req.Header.Set("X-Candela-Job-Id", "header-j")

	attr := FromRequest(req)
	if attr.TenantID != "baggage-t" {
		t.Errorf("tenant_id = %q, want %q", attr.TenantID, "baggage-t")
	}
	if attr.JobID != "header-j" {
		t.Errorf("job_id = %q, want %q", attr.JobID, "header-j")
	}
}

// ── ParseBaggageHeaders ─────────────────────────────────────────────────────

func TestParseBaggageHeaders_JoinsMultiple(t *testing.T) {
	values := []string{
		"candela.tenant_id=t1",
		"candela.job_id=j1",
	}
	tenantID, jobID := ParseBaggageHeaders(values)
	if tenantID != "t1" {
		t.Errorf("tenant_id = %q, want %q", tenantID, "t1")
	}
	if jobID != "j1" {
		t.Errorf("job_id = %q, want %q", jobID, "j1")
	}
}

func TestParseBaggageHeaders_Empty(t *testing.T) {
	tenantID, jobID := ParseBaggageHeaders(nil)
	if tenantID != "" || jobID != "" {
		t.Errorf("nil should return empty, got %q/%q", tenantID, jobID)
	}
}

// ── Security: injection resistance ──────────────────────────────────────────

func TestIDPattern_InjectionResistance(t *testing.T) {
	injections := []struct {
		name  string
		input string
	}{
		{"SQL injection", "'; DROP TABLE spans; --"},
		{"LDAP injection", "*)(&(objectClass=*))"},
		{"XSS", "<img src=x onerror=alert(1)>"},
		{"null byte", "tenant\x00id"},
		{"path traversal", "../../etc/passwd"},
		{"Firestore path", "a/b/c"},
		{"command injection", "$(whoami)"},
		{"newline log injection", "tenant\nINFO fake log line"},
	}
	for _, tt := range injections {
		t.Run(tt.name, func(t *testing.T) {
			if IDPattern.MatchString(tt.input) {
				t.Errorf("IDPattern should block injection: %q", tt.input)
			}
		})
	}
}

// Ensure FromRequest works with a real http.Request from net/http.
func TestFromRequest_RealHTTPRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attr := FromRequest(r)
		if attr.TenantID != "real-tenant" {
			t.Errorf("tenant_id = %q, want %q", attr.TenantID, "real-tenant")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Baggage", "candela.tenant_id=real-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
