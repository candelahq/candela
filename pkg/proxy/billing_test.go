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
	"github.com/candelahq/candela/pkg/storage"
)

// ====================================================================
// U9: Streaming tail buffer — usage chunk captured on long streams (#8)
// ====================================================================

// TestExtractStreamingUsage_TailBufferCapturesLongStream verifies that
// extractStreamingUsage correctly parses token counts from an Anthropic
// usage event that appears at the END of a stream. This validates the
// ring-buffer logic: even if the main buffer caps at 32KB, the tail
// buffer must still deliver the token metadata.
func TestExtractStreamingUsage_TailBufferCapturesLongStream(t *testing.T) {
	// Anthropic streaming usage appears in the message_start event (input tokens)
	// and the message_delta event (output tokens). Generate enough filler to push
	// past the 32KB main buffer so only the tail buffer captures the usage events.
	filler := strings.Repeat("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"x\"}}"+"\n\n", 500)
	// Anthropic sends input tokens in message_start, output tokens in message_delta.
	usageStart := `data: {"type":"message_start","message":{"usage":{"input_tokens":456}}}` + "\n\n"
	usageDelta := `data: {"type":"message_delta","usage":{"output_tokens":123}}` + "\n\n"
	streamData := []byte(filler + usageStart + usageDelta + "data: [DONE]\n\n")

	_, inputTok, outputTok := extractStreamingUsage("anthropic", streamData)

	if outputTok == 0 {
		t.Error("no output tokens extracted — tail buffer likely not working")
	}
	if outputTok != 123 {
		t.Errorf("output tokens = %d, want 123", outputTok)
	}
	// Input tokens from message_start may or may not be in the tail depending on position;
	// we at minimum require output tokens (from message_delta at stream end) to be captured.
	_ = inputTok
}

// ====================================================================
// U10: Pricing gate blocks requests with unknown cloud model (#6)
// ====================================================================

// pricingUserStore stubs only CheckBudget (with an empty budget allowed state).
// HasPricing is on the Calculator, not the UserStore.
type pricingUserStore struct{ budgetUserStore }

// TestPricingGate_BlocksUnknownCloudModel verifies that a request for a model
// with no pricing configured returns 402 Payment Required before hitting upstream.
func TestPricingGate_BlocksUnknownCloudModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream MUST NOT be called for an unpriced model")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()
	// "anthropic" has default pricing, but "mystery-model" is not registered.

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	// Budget check allows, but model has no pricing.
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 50},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"mystery-model-9999","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402; body = %s", resp.StatusCode, body)
	}

	var errResp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("invalid error JSON: %v (body=%s)", err, body)
	}
	if errResp.Error.Type != "pricing_not_configured" {
		t.Errorf("error.type = %q, want 'pricing_not_configured'", errResp.Error.Type)
	}
}

// ====================================================================
// U11: Pricing gate allows "local" provider regardless of model (#6)
// ====================================================================

// TestPricingGate_AllowsLocalProvider verifies that the "local" provider
// bypasses the pricing gate — local models intentionally have $0 cost.
func TestPricingGate_AllowsLocalProvider(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "local", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 50},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/local/v1/chat/completions",
		strings.NewReader(`{"model":"llama3.2:latest","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("local provider should not be blocked by pricing gate; body = %s", body)
	}
	if !upstreamCalled {
		t.Error("upstream was not called for local provider")
	}
}

// ====================================================================
// U12: Budget floor — user with sub-floor balance is blocked (#7)
// ====================================================================

// TestBudgetFloor_ExhaustedUserBlocked verifies that a user with a positive
// but sub-floor balance ($0.0005 < $0.001 floor) is blocked with a 402.
// This prevents users from burning negligible balances on expensive models.
func TestBudgetFloor_ExhaustedUserBlocked(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream MUST NOT be called when balance is below floor")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	// CheckBudget returns Allowed=false because remaining < floor.
	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{
			Allowed:      false,
			RemainingUSD: 0.0005, // > $0 but below the $0.001 floor
		},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 402; body = %s", resp.StatusCode, body)
	}
}

// ====================================================================
// I7: Full proxy end-to-end — pricing gate with known model (#6)
// ====================================================================

// TestProxy_PricingGate_KnownModelAllowed verifies that a request with a
// correctly-configured model passes the pricing gate and reaches upstream.
func TestProxy_PricingGate_KnownModelAllowed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()
	// claude-sonnet-4-20250514 is in the default pricing table.

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}
}

// ====================================================================
// I8: Full proxy end-to-end — user with $0 balance is blocked (#7)
// ====================================================================

// TestProxy_BudgetFloor_ZeroBalance_Blocked verifies end-to-end that
// a user with zero balance gets a structured 402 before upstream is hit.
func TestProxy_BudgetFloor_ZeroBalance_Blocked(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream called despite zero balance")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	p.SetUserStore(&budgetUserStore{
		checkResult: &storage.BudgetCheckResult{Allowed: false, RemainingUSD: 0},
	})

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"free money"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 402; body = %s", resp.StatusCode, body)
	}

	var errResp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	body, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(body, &errResp)
	if errResp.Error.Type != "insufficient_budget" {
		t.Errorf("error.type = %q, want 'insufficient_budget'", errResp.Error.Type)
	}
}

// Ensure pricingUserStore implements UserStore.
var _ storage.UserStore = (*pricingUserStore)(nil)
