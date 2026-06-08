package proxy

import (
	"sync"
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
)

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

func TestSpendTracker_RecordAndCheck(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 10.0},
	}

	// Initially allowed
	allowed, spent, limit := tr.Check("user1", "gpt-4.1", limits)
	if !allowed || spent != 0 || limit != 10.0 {
		t.Fatalf("expected allowed=true, spent=0, limit=10; got %v, %v, %v", allowed, spent, limit)
	}

	// Record some spend
	tr.Record("user1", "gpt-4.1", 3.0, limits)
	allowed, spent, _ = tr.Check("user1", "gpt-4.1", limits)
	if !allowed || spent != 3.0 {
		t.Fatalf("expected allowed=true, spent=3; got %v, %v", allowed, spent)
	}

	// Record more, exceeding limit
	tr.Record("user1", "gpt-4.1", 8.0, limits)
	allowed, spent, _ = tr.Check("user1", "gpt-4.1", limits)
	if allowed || spent != 11.0 {
		t.Fatalf("expected allowed=false, spent=11; got %v, %v", allowed, spent)
	}
}

func TestSpendTracker_MultipleUsers(t *testing.T) {
	tr := NewSpendTracker()
	limits := []SpendLimitConfig{
		{Model: "gpt-4.1", MaxDailyUSD: 5.0},
	}

	tr.Record("user1", "gpt-4.1", 6.0, limits) // over limit

	// user2 should still be allowed
	allowed, spent, _ := tr.Check("user2", "gpt-4.1", limits)
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
	allowed, _, _ := tr.Check("user1", "claude-opus-4", limits)
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
	allowed, spent, _ := tr.Check("user1", "claude-opus-4.6", limits)
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
	allowed, spent, limit := tr.Check("user1", "gemini-2.5-flash", limits)
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
			tr.Check("user1", "gpt-4.1", limits)
		}()
	}
	wg.Wait()

	_, spent, _ := tr.Check("user1", "gpt-4.1", limits)
	if spent != 100.0 {
		t.Fatalf("expected spent=100 after 100 concurrent records, got %v", spent)
	}
}

func TestEstimateRequestCost(t *testing.T) {
	calc := costcalc.New()

	tests := []struct {
		name     string
		provider string
		model    string
		bodySize int
		wantZero bool
	}{
		{
			name:     "normal request",
			provider: "openai",
			model:    "gpt-4.1",
			bodySize: 4000, // ~1000 input tokens, ~2000 output tokens
			wantZero: false,
		},
		{
			name:     "empty body",
			provider: "openai",
			model:    "gpt-4.1",
			bodySize: 0,
			wantZero: true,
		},
		{
			name:     "no model",
			provider: "openai",
			model:    "",
			bodySize: 4000,
			wantZero: true,
		},
		{
			name:     "nil calculator",
			provider: "openai",
			model:    "gpt-4.1",
			bodySize: 4000,
			wantZero: true,
		},
		{
			name:     "local provider",
			provider: "local",
			model:    "llama3",
			bodySize: 4000,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := calc
			if tt.name == "nil calculator" {
				c = nil
			}
			got := estimateRequestCost(c, tt.provider, tt.model, tt.bodySize)
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

	// Very large body: 1MB = ~250K input tokens, output would be 500K uncapped.
	// With 32K cap on output, the cost should be lower.
	largeCost := estimateRequestCost(calc, "openai", "gpt-4.1", 1_000_000)
	// 250K input + 32K output (capped) for gpt-4.1 ($2/M input, $8/M output)
	// = 250K * $2/M + 32K * $8/M = $0.50 + $0.256 = $0.756
	if largeCost < 0.5 || largeCost > 1.5 {
		t.Errorf("expected large request cost in [0.5, 1.5], got %f", largeCost)
	}

	// Compare: small body (4KB = ~1K tokens) should cost much less
	smallCost := estimateRequestCost(calc, "openai", "gpt-4.1", 4_000)
	if smallCost >= largeCost {
		t.Errorf("small body (%f) should cost less than large body (%f)", smallCost, largeCost)
	}
}
