package billing

import "testing"

func TestGrantRemaining_Normal(t *testing.T) {
	g := &GrantRecord{AmountUSD: 100, SpentUSD: 30}
	if got := g.Remaining(); got != 70 {
		t.Errorf("Remaining() = %v, want 70", got)
	}
}

func TestGrantRemaining_Overdraft(t *testing.T) {
	g := &GrantRecord{AmountUSD: 100, SpentUSD: 110}
	if got := g.Remaining(); got != 0 {
		t.Errorf("Remaining() = %v, want 0 (clamped)", got)
	}
}

func TestGrantRemaining_ExactlySpent(t *testing.T) {
	g := &GrantRecord{AmountUSD: 50, SpentUSD: 50}
	if got := g.Remaining(); got != 0 {
		t.Errorf("Remaining() = %v, want 0", got)
	}
}

func TestGrantRemaining_ZeroGrant(t *testing.T) {
	g := &GrantRecord{AmountUSD: 0, SpentUSD: 0}
	if got := g.Remaining(); got != 0 {
		t.Errorf("Remaining() = %v, want 0", got)
	}
}

func TestBudgetCheckResult_Reasons(t *testing.T) {
	// Verify constants are non-empty and distinct
	reasons := []string{ReasonAllowed, ReasonBudgetExhausted, ReasonSoftBlocked, ReasonNoBudget}
	seen := make(map[string]bool)
	for _, r := range reasons {
		if r == "" {
			t.Error("reason constant must not be empty")
		}
		if seen[r] {
			t.Errorf("duplicate reason: %s", r)
		}
		seen[r] = true
	}
}
