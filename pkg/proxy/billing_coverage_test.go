package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/auth"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// ====================================================================
// 1. TestToInt64 — table-driven, security-critical negative clamping
// ====================================================================

func TestToInt64(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want int64
	}{
		// --- float64 ---
		{"float64 positive", float64(42), 42},
		{"float64 zero", float64(0), 0},
		{"float64 fractional truncates", float64(9.99), 9},
		{"float64 large", float64(1e15), 1_000_000_000_000_000},
		{"float64 negative clamps to 0", float64(-100), 0},
		{"float64 negative fractional clamps to 0", float64(-0.5), 0},
		{"float64 max safe int", float64(1 << 53), 1 << 53},

		// --- int64 ---
		{"int64 positive", int64(100), 100},
		{"int64 zero", int64(0), 0},
		{"int64 max", int64(math.MaxInt64), math.MaxInt64},
		{"int64 negative clamps to 0", int64(-1), 0},
		{"int64 min clamps to 0", int64(math.MinInt64), 0},

		// --- json.Number ---
		{"json.Number valid", json.Number("12345"), 12345},
		{"json.Number zero", json.Number("0"), 0},
		{"json.Number invalid returns 0", json.Number("not-a-number"), 0},
		{"json.Number empty returns 0", json.Number(""), 0},
		{"json.Number negative clamps to 0", json.Number("-42"), 0},
		{"json.Number large", json.Number("9999999999999"), 9999999999999},

		// --- nil and unsupported types ---
		{"nil returns 0", nil, 0},
		{"string returns 0", "hello", 0},
		{"bool true returns 0", true, 0},
		{"bool false returns 0", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toInt64(tt.val)
			if got != tt.want {
				t.Errorf("toInt64(%v) = %d, want %d", tt.val, got, tt.want)
			}
		})
	}
}

func TestToInt64_NegativeClamping_SecurityCritical(t *testing.T) {
	// Negative token counts could inflate budget remaining or cause
	// incorrect cost calculations. Verify all numeric paths clamp to 0.
	negatives := []struct {
		name string
		val  interface{}
	}{
		{"float64 -1", float64(-1)},
		{"float64 -999999", float64(-999999)},
		{"int64 -1", int64(-1)},
		{"int64 MinInt64", int64(math.MinInt64)},
		{"json.Number -100", json.Number("-100")},
	}
	for _, tt := range negatives {
		t.Run(tt.name, func(t *testing.T) {
			got := toInt64(tt.val)
			if got < 0 {
				t.Errorf("toInt64(%v) = %d, MUST be >= 0 for billing safety", tt.val, got)
			}
		})
	}
}

// ====================================================================
// 2. TestExtractModelFromURLPath — Google API model extraction
// ====================================================================

func TestExtractModelFromURLPath_Billing(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/v1beta/models/gemini-2.5-flash:generateContent", "gemini-2.5-flash"},
		{"/v1/models/gemini-2.5-pro:streamGenerateContent", "gemini-2.5-pro"},
		{"/v1beta/models/gemini-2.0-flash-lite:generateContent", "gemini-2.0-flash-lite"},
		{"/v1/health", ""},
		{"", ""},
		{"/v1/chat/completions", ""},
		{"/models/", ""},
		{"/v1/models/model-name", "model-name"},
		// Multiple /models/ — should use LastIndex
		{"/v1/models/namespace/models/actual-model:generate", "actual-model"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractModelFromURLPath(tt.path)
			if got != tt.want {
				t.Errorf("extractModelFromURLPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// ====================================================================
// 3. TestIsUtilityEndpoint — non-generative API paths
// ====================================================================

func TestIsUtilityEndpoint_Billing(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/count_tokens", true},
		{"/v1/count_tokens", true},
		{"/tokenize", true},
		{"/v1/tokenize", true},
		{"/models", true},
		{"/v1/models", true},
		{"/v1/chat/completions", false},
		{"/v1/messages", false},
		{"/v1/embeddings", false},
		{"/v1beta/models/gemini:generateContent", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isUtilityEndpoint(tt.path)
			if got != tt.want {
				t.Errorf("isUtilityEndpoint(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// ====================================================================
// 4. Streaming usage extraction — OpenAI
// ====================================================================

func TestExtractStreamingUsage_OpenAI_Billing(t *testing.T) {
	t.Run("standard usage chunk", func(t *testing.T) {
		stream := []byte(`data: {"choices":[{"delta":{"content":"hi"}}]}` + "\n\n" +
			`data: {"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}` + "\n\n" +
			"data: [DONE]\n\n")
		_, input, output := extractStreamingUsage("openai", stream)
		if input != 100 {
			t.Errorf("input = %d, want 100", input)
		}
		if output != 50 {
			t.Errorf("output = %d, want 50", output)
		}
	})

	t.Run("missing usage returns zeros", func(t *testing.T) {
		stream := []byte(`data: {"choices":[{"delta":{"content":"hi"}}]}` + "\n\n" +
			"data: [DONE]\n\n")
		_, input, output := extractStreamingUsage("openai", stream)
		if input != 0 || output != 0 {
			t.Errorf("missing usage: input=%d output=%d, want 0,0", input, output)
		}
	})

	t.Run("malformed JSON returns zeros", func(t *testing.T) {
		stream := []byte("data: {broken json\n\ndata: [DONE]\n\n")
		_, input, output := extractStreamingUsage("openai", stream)
		if input != 0 || output != 0 {
			t.Errorf("malformed JSON: input=%d output=%d, want 0,0", input, output)
		}
	})
}

// ====================================================================
// 5. Streaming usage extraction — Anthropic
// ====================================================================

func TestExtractStreamingUsage_Anthropic_Billing(t *testing.T) {
	t.Run("message_start + message_delta", func(t *testing.T) {
		stream := []byte(
			`data: {"type":"message_start","message":{"usage":{"input_tokens":200}}}` + "\n\n" +
				`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
				`data: {"type":"message_delta","usage":{"output_tokens":75}}` + "\n\n" +
				"data: [DONE]\n\n")
		_, input, output := extractStreamingUsage("anthropic", stream)
		if input != 200 {
			t.Errorf("input = %d, want 200", input)
		}
		if output != 75 {
			t.Errorf("output = %d, want 75", output)
		}
	})

	t.Run("anthropic-direct alias", func(t *testing.T) {
		stream := []byte(
			`data: {"type":"message_start","message":{"usage":{"input_tokens":50}}}` + "\n\n" +
				`data: {"type":"message_delta","usage":{"output_tokens":30}}` + "\n\n")
		_, input, output := extractStreamingUsage("anthropic-direct", stream)
		if input != 50 {
			t.Errorf("input = %d, want 50", input)
		}
		if output != 30 {
			t.Errorf("output = %d, want 30", output)
		}
	})
}

// ====================================================================
// 6. Empty/nil stream edge cases
// ====================================================================

func TestExtractStreamingUsage_EmptyAndNil(t *testing.T) {
	providers := []string{"openai", "anthropic", "anthropic-direct", "google", "bedrock", "local", ""}
	for _, p := range providers {
		t.Run("nil/"+p, func(t *testing.T) {
			_, in, out := extractStreamingUsage(p, nil)
			if in != 0 || out != 0 {
				t.Errorf("nil data: input=%d output=%d, want 0,0", in, out)
			}
		})
		t.Run("empty/"+p, func(t *testing.T) {
			_, in, out := extractStreamingUsage(p, []byte{})
			if in != 0 || out != 0 {
				t.Errorf("empty data: input=%d output=%d, want 0,0", in, out)
			}
		})
	}
}

// ====================================================================
// 7. Cost calculation E2E — proxy → upstream → span with correct cost
// ====================================================================

func TestProxy_CostCalculation_E2E(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Anthropic response format with known token counts.
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":1000,"output_tokens":500},"model":"claude-sonnet-4-20250514"}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

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
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"cost test"}]}`))
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

	// Wait for async span submission.
	if !waitForSpan(submitter) {
		t.Fatal("no span submitted")
	}
	spans := submitter.getSpans()
	span := spans[0]

	if span.GenAI == nil {
		t.Fatal("span.GenAI is nil")
	}

	// claude-sonnet-4: $3/M input, $15/M output
	// Expected: (1000 * 3 / 1_000_000) + (500 * 15 / 1_000_000) = 0.003 + 0.0075 = 0.0105
	expectedCost := 0.0105
	if math.Abs(span.GenAI.CostUSD-expectedCost) > 0.001 {
		t.Errorf("cost = %f, want ~%f", span.GenAI.CostUSD, expectedCost)
	}
}

// ====================================================================
// 8. Service account bypasses budget deduction
// ====================================================================

// trackingDeductUserStore tracks DeductSpend calls with an atomic counter.
type trackingDeductUserStore struct {
	budgetUserStore
	deductCount atomic.Int64
}

func (s *trackingDeductUserStore) DeductSpend(_ context.Context, _ string, _ float64, _ int64) error {
	s.deductCount.Add(1)
	return nil
}

func TestProxy_ServiceAccountBypassesBudget(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":10,"output_tokens":5},"model":"claude-sonnet-4-20250514"}`)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p := New(Config{
		Providers: []Provider{{Name: "anthropic", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	store := &trackingDeductUserStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
	}
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	// Inject a service account identity via auth.NewContext.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sa := &auth.User{ID: "sa@test.iam.gserviceaccount.com", Email: "sa@test.iam.gserviceaccount.com"}
		mux.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), sa)))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"SA test"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Give async processing a moment.
	time.Sleep(200 * time.Millisecond)

	if got := store.deductCount.Load(); got != 0 {
		t.Errorf("DeductSpend called %d times for SA, want 0", got)
	}
}

// ====================================================================
// 9. Pricing gate prefix matching — date-suffixed models pass
// ====================================================================

func TestProxy_PricingGate_PrefixMatch(t *testing.T) {
	models := []struct {
		provider string
		model    string
	}{
		{"anthropic", "claude-sonnet-4-20250514"},
	}

	for _, tt := range models {
		t.Run(tt.model, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":10,"output_tokens":5},"model":"%s"}`, tt.model)
			}))
			defer upstream.Close()

			submitter := &mockSubmitter{}
			calc := costcalc.New()

			p := New(Config{
				Providers: []Provider{{Name: tt.provider, UpstreamURL: upstream.URL}},
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
				srv.URL+"/proxy/"+tt.provider+"/v1/messages",
				strings.NewReader(fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"prefix test"}]}`, tt.model)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer tok")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode == http.StatusPaymentRequired {
				t.Errorf("model %q blocked by pricing gate — prefix matching failed", tt.model)
			}
		})
	}
}

// ====================================================================
// 10. deductBudget unit tests
// ====================================================================

func TestDeductBudget_SkipsZeroCostZeroTokens(t *testing.T) {
	store := &trackingDeductUserStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
	}
	calc := costcalc.New()
	p := New(Config{ProjectID: "test"}, &mockSubmitter{}, calc)
	p.SetUserStore(store)

	// Zero tokens + zero cost → should skip DeductSpend.
	p.deductBudget(context.Background(), Provider{Name: "local"}, "llama3", "user@test.com", 0, 0)

	if got := store.deductCount.Load(); got != 0 {
		t.Errorf("DeductSpend called %d times for zero tokens, want 0", got)
	}
}

func TestDeductBudget_CallsForPositiveTokens(t *testing.T) {
	store := &trackingDeductUserStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
	}
	calc := costcalc.New()
	p := New(Config{ProjectID: "test"}, &mockSubmitter{}, calc)
	p.SetUserStore(store)

	// Positive tokens → should call DeductSpend.
	p.deductBudget(context.Background(), Provider{Name: "anthropic"}, "claude-sonnet-4-20250514", "user@test.com", 100, 50)

	if got := store.deductCount.Load(); got != 1 {
		t.Errorf("DeductSpend called %d times, want 1", got)
	}
}

func TestDeductBudget_SkipsServiceAccount(t *testing.T) {
	store := &trackingDeductUserStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
	}
	calc := costcalc.New()
	p := New(Config{ProjectID: "test"}, &mockSubmitter{}, calc)
	p.SetUserStore(store)

	p.deductBudget(context.Background(), Provider{Name: "anthropic"}, "claude-sonnet-4-20250514",
		"my-sa@project.iam.gserviceaccount.com", 1000, 500)

	if got := store.deductCount.Load(); got != 0 {
		t.Errorf("DeductSpend called %d times for SA, want 0", got)
	}
}

func TestDeductBudget_SkipsEmptyUserID(t *testing.T) {
	store := &trackingDeductUserStore{
		budgetUserStore: budgetUserStore{
			checkResult: &storage.BudgetCheckResult{Allowed: true, RemainingUSD: 100},
		},
	}
	calc := costcalc.New()
	p := New(Config{ProjectID: "test"}, &mockSubmitter{}, calc)
	p.SetUserStore(store)

	p.deductBudget(context.Background(), Provider{Name: "anthropic"}, "claude-sonnet-4-20250514", "", 1000, 500)

	if got := store.deductCount.Load(); got != 0 {
		t.Errorf("DeductSpend called %d times for empty userID, want 0", got)
	}
}

// ====================================================================
// 11. GrantRecord.Remaining() — BILL-3 clamping verification
// ====================================================================

func TestGrantRecord_Remaining_BillingClamp(t *testing.T) {
	tests := []struct {
		name   string
		amount float64
		spent  float64
		want   float64
	}{
		{"normal grant", 100.0, 40.0, 60.0},
		{"fully spent", 50.0, 50.0, 0.0},
		{"overdraft clamped", 10.0, 15.0, 0.0},
		{"large overdraft clamped", 100.0, 999.0, 0.0},
		{"tiny float overdraft", 1.0, 1.0000000000001, 0.0},
		{"zero grant", 0.0, 0.0, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &storage.GrantRecord{AmountUSD: tt.amount, SpentUSD: tt.spent}
			got := g.Remaining()
			if got != tt.want {
				t.Errorf("Remaining() = %f, want %f", got, tt.want)
			}
			if got < 0 {
				t.Errorf("Remaining() = %f, MUST be >= 0 for billing safety", got)
			}
		})
	}
}
