package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/auth"
	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// TestSpanContent_ScrubAndTruncateIntegration verifies that proxy span generation
// scrubs sensitive data (API keys, SSNs, emails) and truncates content to max_content_len (#590).
func TestSpanContent_ScrubAndTruncateIntegration(t *testing.T) {
	// Upstream responds with sensitive content.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Response with an API key, email, and long text.
		_, _ = fmt.Fprintf(w, `{
			"content": [{"type": "text", "text": "Result with email user@secretcorp.com and key sk-ant-api03-abcdef1234567890abcdef12345-AA and extra padding %s"}],
			"usage": {"input_tokens": 10, "output_tokens": 10},
			"model": "claude-sonnet-4-20250514"
		}`, strings.Repeat("A", 200))
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	// Configure proxy with max_content_len = 120.
	p, err := New(Config{
		Providers:     []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID:     "test-project",
		MaxContentLen: 120,
	}, submitter, calc)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if p.MaxContentLen() != 120 {
		t.Errorf("p.MaxContentLen() = %d, want 120", p.MaxContentLen())
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := &auth.User{ID: "usr-123", Email: "test@example.com"}
		mux.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), u)))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Send request with an API key and SSN in the prompt.
	reqBody := `{
		"model": "claude-sonnet-4-20250514",
		"messages": [{"role": "user", "content": "Please check SSN 123-45-6789 and secret key sk-1234567890abcdef1234567890abcdef for customer account details."}]
	}`

	req, err := http.NewRequest("POST", srv.URL+"/proxy/anthropic/v1/messages", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request creation failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Wait for async span creation.
	var spans []storage.Span
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		spans = submitter.getSpans()
		if len(spans) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if len(spans) == 0 {
		t.Fatal("no spans were submitted")
	}

	span := spans[0]
	if span.GenAI == nil {
		t.Fatal("span GenAI attributes are nil")
	}

	// 1. Verify prompt was scrubbed of SSN and OpenAI API key
	if strings.Contains(span.GenAI.InputContent, "123-45-6789") {
		t.Errorf("InputContent contains unredacted SSN: %q", span.GenAI.InputContent)
	}
	if !strings.Contains(span.GenAI.InputContent, "[REDACTED_SSN]") {
		t.Errorf("InputContent missing [REDACTED_SSN]: %q", span.GenAI.InputContent)
	}
	if strings.Contains(span.GenAI.InputContent, "sk-1234567890abcdef1234567890abcdef") {
		t.Errorf("InputContent contains unredacted API key: %q", span.GenAI.InputContent)
	}
	if !strings.Contains(span.GenAI.InputContent, "[REDACTED_API_KEY]") {
		t.Errorf("InputContent missing [REDACTED_API_KEY]: %q", span.GenAI.InputContent)
	}

	// 2. Verify prompt was truncated to 60 chars + "... [truncated]"
	if !strings.HasSuffix(span.GenAI.InputContent, "... [truncated]") {
		t.Errorf("InputContent was not truncated: %q", span.GenAI.InputContent)
	}

	// 3. Verify output was scrubbed of Anthropic API key and email
	if strings.Contains(span.GenAI.OutputContent, "user@secretcorp.com") {
		t.Errorf("OutputContent contains unredacted email: %q", span.GenAI.OutputContent)
	}
	if !strings.Contains(span.GenAI.OutputContent, "[REDACTED_EMAIL]") {
		t.Errorf("OutputContent missing [REDACTED_EMAIL]: %q", span.GenAI.OutputContent)
	}
	if strings.Contains(span.GenAI.OutputContent, "sk-ant-api03-abcdef1234567890abcdef12345-AA") {
		t.Errorf("OutputContent contains unredacted Anthropic key: %q", span.GenAI.OutputContent)
	}
	if !strings.Contains(span.GenAI.OutputContent, "[REDACTED_API_KEY]") {
		t.Errorf("OutputContent missing [REDACTED_API_KEY]: %q", span.GenAI.OutputContent)
	}

	// 4. Verify output was truncated
	if !strings.HasSuffix(span.GenAI.OutputContent, "... [truncated]") {
		t.Errorf("OutputContent was not truncated: %q", span.GenAI.OutputContent)
	}
}

// TestSpanContent_DefaultMaxContentLen verifies that New() defaults maxContentLen
// to DefaultMaxContentLen (1000) when 0 or unspecified (#590).
func TestSpanContent_DefaultMaxContentLen(t *testing.T) {
	p, err := New(Config{ProjectID: "test"}, &mockSubmitter{}, costcalc.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if p.MaxContentLen() != DefaultMaxContentLen {
		t.Errorf("default MaxContentLen = %d, want %d", p.MaxContentLen(), DefaultMaxContentLen)
	}

	// When set to -1, it becomes 0 (unlimited).
	pUnlimited, err := New(Config{ProjectID: "test", MaxContentLen: -1}, &mockSubmitter{}, costcalc.New())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if pUnlimited.MaxContentLen() != 0 {
		t.Errorf("unlimited MaxContentLen = %d, want 0", pUnlimited.MaxContentLen())
	}
}
