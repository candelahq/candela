package proxy

import (
	"strings"
	"sync"
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
)

// ─────────────────────────────────────────────────────────────
// matchSpendLimit tests
// ─────────────────────────────────────────────────────────────

func TestMatchSpendLimit_ExactMatch(t *testing.T) {
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 25},
		{Model: "claude-opus-4", MaxDailyUSD: 50},
	}
	got := matchSpendLimit("gpt-4.1", limits)
	if got == nil || got.Model != "gpt-4.1" {
		t.Fatalf("expected gpt-4.1 match, got %v", got)
	}
}

func TestMatchSpendLimit_PrefixMatch(t *testing.T) {
	limits := []SpendLimitConfig{
		{Model: "claude-opus-4", MaxDailyUSD: 50},
	}
	// Should match dated variant
	got := matchSpendLimit("claude-opus-4-20250514", limits)
	if got == nil || got.Model != "claude-opus-4" {
		t.Fatalf("expected claude-opus-4 prefix match, got %v", got)
	}
	// Should match extended version
	got = matchSpendLimit("claude-opus-4.7", limits)
	if got == nil || got.Model != "claude-opus-4" {
		t.Fatalf("expected claude-opus-4 prefix match for 4.7, got %v", got)
	}
}

func TestMatchSpendLimit_LongestPrefix(t *testing.T) {
	limits := []SpendLimitConfig{
		{Model: "claude-opus-4", MaxDailyUSD: 50},
		{Model: "claude-opus-4.7", MaxDailyUSD: 100},
	}
	got := matchSpendLimit("claude-opus-4.7-20260101", limits)
	if got == nil || got.Model != "claude-opus-4.7" {
		t.Fatalf("expected longest prefix claude-opus-4.7, got %v", got)
	}
	// Shorter model should still match the shorter prefix
	got = matchSpendLimit("claude-opus-4.6", limits)
	if got == nil || got.Model != "claude-opus-4" {
		t.Fatalf("expected claude-opus-4 for 4.6, got %v", got)
	}
}

func TestMatchSpendLimit_NoMatch(t *testing.T) {
	limits := []SpendLimitConfig{
		{Model: "claude-opus-4", MaxDailyUSD: 50},
	}
	got := matchSpendLimit("gpt-4.1", limits)
	if got != nil {
		t.Fatalf("expected no match, got %v", got)
	}
}

func TestMatchSpendLimit_CaseInsensitive(t *testing.T) {
	limits := []SpendLimitConfig{
		{Model: "Claude-Opus-4", MaxDailyUSD: 50},
	}
	got := matchSpendLimit("claude-opus-4-20250514", limits)
	if got == nil {
		t.Fatal("expected case-insensitive match")
	}
}

func TestMatchSpendLimit_ZeroLimit_Skipped(t *testing.T) {
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 0},        // invalid — should be skipped
		{Model: "gpt-4.1", MaxDailyUSD: -5.0},     // invalid — should be skipped
		{Model: "claude-opus-4", MaxDailyUSD: 50}, // valid
	}
	// gpt-4.1 with MaxDailyUSD=0 or negative should be skipped
	got := matchSpendLimit("gpt-4.1", limits)
	if got != nil {
		t.Fatalf("expected nil for zero/negative limit, got %+v", got)
	}
	// claude-opus-4 with valid limit should match
	got = matchSpendLimit("claude-opus-4", limits)
	if got == nil || got.MaxDailyUSD != 50 {
		t.Fatalf("expected claude-opus-4 with limit 50, got %v", got)
	}
}

func TestMatchSpendLimit_EmptyLimits(t *testing.T) {
	got := matchSpendLimit("gpt-4.1", nil)
	if got != nil {
		t.Fatalf("expected nil for empty limits, got %v", got)
	}
	got = matchSpendLimit("gpt-4.1", []SpendLimitConfig{})
	if got != nil {
		t.Fatalf("expected nil for empty slice, got %v", got)
	}
}

func TestMatchSpendLimit_EmptyModel(t *testing.T) {
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 25},
	}
	// Empty model name matches everything via HasPrefix("", "gpt-4.1") = false
	got := matchSpendLimit("", limits)
	if got != nil {
		t.Fatalf("expected nil for empty model, got %v", got)
	}
}

// ─────────────────────────────────────────────────────────────
// SpendTracker tests
// ─────────────────────────────────────────────────────────────

func TestSpendTracker_RecordAndCheck(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 10.0},
	}

	// Initially allowed
	allowed, spent, limit := tr.Check("user1", "gpt-4.1", limits, 0)
	if !allowed || spent != 0 || limit != 10.0 {
		t.Fatalf("expected allowed=true, spent=0, limit=10; got %v, %v, %v", allowed, spent, limit)
	}

	// Record some spend
	tr.Record("user1", "gpt-4.1", 3.0, limits)
	allowed, spent, _ = tr.Check("user1", "gpt-4.1", limits, 0)
	if !allowed || spent != 3.0 {
		t.Fatalf("expected allowed=true, spent=3; got %v, %v", allowed, spent)
	}

	// Record more, exceeding limit
	tr.Record("user1", "gpt-4.1", 8.0, limits)
	allowed, spent, _ = tr.Check("user1", "gpt-4.1", limits, 0)
	if allowed || spent != 11.0 {
		t.Fatalf("expected allowed=false, spent=11; got %v, %v", allowed, spent)
	}
}

func TestSpendTracker_CheckWithEstimatedCost(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 10.0},
	}

	// Record $8 of spend
	tr.Record("user1", "gpt-4.1", 8.0, limits)

	// Check with estimated $1 — should be allowed (8+1=9 <= 10)
	allowed, _, _ := tr.Check("user1", "gpt-4.1", limits, 1.0)
	if !allowed {
		t.Fatal("expected allowed=true for 8+1=9 <= 10")
	}

	// Check with estimated $3 — should be blocked (8+3=11 > 10)
	allowed, _, _ = tr.Check("user1", "gpt-4.1", limits, 3.0)
	if allowed {
		t.Fatal("expected allowed=false for 8+3=11 > 10")
	}

	// Check with estimated $2 — should be allowed (8+2=10 <= 10)
	allowed, _, _ = tr.Check("user1", "gpt-4.1", limits, 2.0)
	if !allowed {
		t.Fatal("expected allowed=true for 8+2=10 <= 10")
	}
}

func TestSpendTracker_CheckEstimatedCost_NoHistory(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 5.0},
	}

	// No spend history — check with estimated cost that exceeds limit
	allowed, _, _ := tr.Check("user1", "gpt-4.1", limits, 6.0)
	if allowed {
		t.Fatal("expected allowed=false when estimated cost alone exceeds limit")
	}

	// Estimated cost within limit
	allowed, _, _ = tr.Check("user1", "gpt-4.1", limits, 4.0)
	if !allowed {
		t.Fatal("expected allowed=true when estimated cost within limit")
	}
}

func TestSpendTracker_MultipleUsers(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 5.0},
	}

	tr.Record("user1", "gpt-4.1", 6.0, limits) // over limit

	// user2 should still be allowed
	allowed, spent, _ := tr.Check("user2", "gpt-4.1", limits, 0)
	if !allowed || spent != 0 {
		t.Fatalf("user2 should be independent, got allowed=%v spent=%v", allowed, spent)
	}
}

func TestSpendTracker_MultipleModels(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 5.0},
		{Model: "claude-opus-4", MaxDailyUSD: 10.0},
	}

	tr.Record("user1", "gpt-4.1", 6.0, limits)

	// claude should still be allowed
	allowed, _, _ := tr.Check("user1", "claude-opus-4", limits, 0)
	if !allowed {
		t.Fatal("claude-opus-4 should not be blocked by gpt-4.1 overspend")
	}
}

func TestSpendTracker_PrefixRecording(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "claude-opus-4", MaxDailyUSD: 5.0},
	}

	// Record with dated variant — should accumulate under "claude-opus-4" prefix
	tr.Record("user1", "claude-opus-4-20250514", 2.0, limits)
	tr.Record("user1", "claude-opus-4.7", 2.0, limits)

	// Check with yet another variant
	allowed, spent, _ := tr.Check("user1", "claude-opus-4.6", limits, 0)
	if !allowed || spent != 4.0 {
		t.Fatalf("expected all variants to accumulate, got allowed=%v spent=%v", allowed, spent)
	}
}

func TestSpendTracker_NoLimitNoTracking(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 5.0},
	}

	// Record for a model with no limit — should not track
	tr.Record("user1", "gemini-2.5-flash", 100.0, limits)

	// And it should always be allowed
	allowed, spent, limit := tr.Check("user1", "gemini-2.5-flash", limits, 0)
	if !allowed || spent != 0 || limit != 0 {
		t.Fatalf("model without limit should always be allowed, got %v %v %v", allowed, spent, limit)
	}
}

func TestSpendTracker_Concurrent(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 10000.0},
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.Record("user1", "gpt-4.1", 1.0, limits)
			tr.Check("user1", "gpt-4.1", limits, 0)
		}()
	}
	wg.Wait()

	_, spent, _ := tr.Check("user1", "gpt-4.1", limits, 0)
	if spent != 100.0 {
		t.Fatalf("expected spent=100 after 100 concurrent records, got %v", spent)
	}
}

func TestSpendTracker_NegativeCostIgnored(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 10.0},
	}

	tr.Record("user1", "gpt-4.1", -5.0, limits) // negative — ignored
	tr.Record("user1", "gpt-4.1", 0.0, limits)  // zero — ignored
	tr.Record("user1", "gpt-4.1", 3.0, limits)  // valid

	_, spent, _ := tr.Check("user1", "gpt-4.1", limits, 0)
	if spent != 3.0 {
		t.Fatalf("expected only positive costs tracked, got %v", spent)
	}
}

func TestSpendTracker_LocalFallback(t *testing.T) {
	// Simulates solo mode where effectiveUserID is empty.
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 5.0},
	}

	// Use "local" as the solo-mode fallback user
	tr.Record("local", "gpt-4.1", 6.0, limits)

	allowed, spent, _ := tr.Check("local", "gpt-4.1", limits, 0)
	if allowed || spent != 6.0 {
		t.Fatalf("expected blocked for local user, got allowed=%v spent=%v", allowed, spent)
	}

	// A real user should be independent
	allowed, spent, _ = tr.Check("user1", "gpt-4.1", limits, 0)
	if !allowed || spent != 0 {
		t.Fatalf("real user should be independent of local, got allowed=%v spent=%v", allowed, spent)
	}
}

func TestSpendTracker_EvictStaleEntries(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 10.0},
	}

	// Manually inject a stale entry from yesterday
	tr.mu.Lock()
	tr.entries["user1\x00gpt-4.1"] = &spendEntry{spent: 100, date: "2020-01-01"}
	tr.entries["user2\x00gpt-4.1"] = &spendEntry{spent: 200, date: "2020-01-01"}
	tr.mu.Unlock()

	if tr.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", tr.Len())
	}

	// Record for today — should trigger eviction of stale entries
	tr.Record("user1", "gpt-4.1", 1.0, limits)

	// Stale user2 entry should be gone, user1 should have new entry
	if tr.Len() != 1 {
		t.Fatalf("expected 1 entry after eviction, got %d", tr.Len())
	}

	// user1 should have today's spend only (1.0, not 101.0)
	_, spent, _ := tr.Check("user1", "gpt-4.1", limits, 0)
	if spent != 1.0 {
		t.Fatalf("expected fresh spend=1.0 after date change, got %v", spent)
	}
}

// ─────────────────────────────────────────────────────────────
// estimateRequestCost tests
// ─────────────────────────────────────────────────────────────

func TestEstimateRequestCost(t *testing.T) {
	calc := costcalc.New()

	tests := []struct {
		name     string
		provider string
		model    string
		body     []byte
		wantZero bool
	}{
		{
			name:     "normal request",
			provider: "openai",
			model:    "gpt-4.1",
			body:     []byte(strings.Repeat("x", 4000)),
			wantZero: false,
		},
		{
			name:     "empty body",
			provider: "openai",
			model:    "gpt-4.1",
			body:     nil,
			wantZero: true,
		},
		{
			name:     "no model",
			provider: "openai",
			model:    "",
			body:     []byte(strings.Repeat("x", 4000)),
			wantZero: true,
		},
		{
			name:     "nil calculator",
			provider: "openai",
			model:    "gpt-4.1",
			body:     []byte(strings.Repeat("x", 4000)),
			wantZero: true,
		},
		{
			name:     "local provider",
			provider: "local",
			model:    "llama3",
			body:     []byte(strings.Repeat("x", 4000)),
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := calc
			if tt.name == "nil calculator" {
				c = nil
			}
			got := estimateRequestCost(c, tt.provider, tt.model, tt.body)
			if tt.wantZero && got != 0 {
				t.Errorf("expected 0, got %f", got)
			}
			if !tt.wantZero && got <= 0 {
				t.Errorf("expected positive cost, got %f", got)
			}
		})
	}
}

func TestEstimateRequestCost_OutputCap(t *testing.T) {
	calc := costcalc.New()

	// Very large body: 1MB text = ~250K input tokens, output would be 500K uncapped.
	largeBody := []byte(strings.Repeat("a", 1_000_000))
	largeCost := estimateRequestCost(calc, "openai", "gpt-4.1", largeBody)
	// 250K input + 32K output (capped) for gpt-4.1 ($2/M input, $8/M output)
	// = 250K * $2/M + 32K * $8/M = $0.50 + $0.256 = $0.756
	if largeCost < 0.5 || largeCost > 1.5 {
		t.Errorf("expected large request cost in [0.5, 1.5], got %f", largeCost)
	}

	// Compare: small body (4KB = ~1K tokens) should cost much less
	smallBody := []byte(strings.Repeat("a", 4_000))
	smallCost := estimateRequestCost(calc, "openai", "gpt-4.1", smallBody)
	if smallCost >= largeCost {
		t.Errorf("small body (%f) should cost less than large body (%f)", smallCost, largeCost)
	}
}

// ─────────────────────────────────────────────────────────────
// Multimodal / base64 detection tests
// ─────────────────────────────────────────────────────────────

func TestCountBase64Images(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		count int
	}{
		{"no images", `{"messages":[{"content":"hello"}]}`, 0},
		{"one openai image", `{"messages":[{"content":[{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,/9j/4AAQ..."}}]}]}`, 1},
		{"two openai images", `data:image/png;base64,abc data:image/jpeg;base64,def`, 2},
		{"anthropic base64", `{"type":"base64","media_type":"image/jpeg","data":"abc..."}`, 1},
		{"anthropic base64 spaced", `{"type": "base64", "media_type": "image/jpeg"}`, 1},
		{"mixed", `data:image/png;base64,x "type":"base64" data:image/jpeg;base64,y`, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countBase64Images([]byte(tt.body))
			if got != tt.count {
				t.Errorf("countBase64Images = %d, want %d", got, tt.count)
			}
		})
	}
}

func TestEstimateRequestCost_MultimodalDiscount(t *testing.T) {
	calc := costcalc.New()

	// Simulate a request with a base64-encoded image (~1MB of base64 data)
	// plus a short text prompt (~100 bytes).
	textPart := `{"messages":[{"role":"user","content":[{"type":"text","text":"describe this image"},{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,`
	base64Data := strings.Repeat("A", 1_000_000) // ~1MB of base64
	closePart := `"}}]}]}`
	multimodalBody := []byte(textPart + base64Data + closePart)

	// Same total size but pure text (no images)
	textOnlyBody := []byte(strings.Repeat("x", len(multimodalBody)))

	multimodalCost := estimateRequestCost(calc, "openai", "gpt-4.1", multimodalBody)
	textOnlyCost := estimateRequestCost(calc, "openai", "gpt-4.1", textOnlyBody)

	// The multimodal request should cost less than the same-size text-only request
	// because the base64 data is discounted to ~1000 tokens per image instead of
	// 250K tokens (1MB/4).
	if multimodalCost >= textOnlyCost {
		t.Errorf("multimodal cost ($%.4f) should be less than text-only cost ($%.4f) for same body size",
			multimodalCost, textOnlyCost)
	}

	// The multimodal input estimate should be dramatically lower:
	// Text-only: ~250K input tokens. Multimodal: ~1K (image) + tiny text = ~1K.
	// But output cap (32K) dominates both, so we just check multimodal < text.
	t.Logf("multimodal=$%.4f text=$%.4f ratio=%.2fx", multimodalCost, textOnlyCost, textOnlyCost/multimodalCost)
}

func TestEstimateRequestCost_MultimodalSmallBody(t *testing.T) {
	calc := costcalc.New()

	// A very small multimodal request where text is most of the body.
	// The discount should not produce negative tokens.
	body := []byte(`{"messages":[{"content":"data:image/png;base64,tiny"}]}`)
	cost := estimateRequestCost(calc, "openai", "gpt-4.1", body)
	if cost < 0 {
		t.Errorf("cost should never be negative, got %f", cost)
	}
}
