package billing

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

// Property: Budget remaining = limit - spent (never negative)
func TestProperty_TaskBudgetRemaining(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.Float64Range(0, 1000).Draw(t, "limit")
		spent := rapid.Float64Range(0, 1500).Draw(t, "spent")

		budget := &TaskBudget{
			LimitUSD: limit,
			SpentUSD: spent,
		}

		remaining := budget.Remaining()
		if remaining < 0 {
			t.Fatalf("remaining cannot be negative: %f (limit=%f, spent=%f)", remaining, limit, spent)
		}

		expected := limit - spent
		if expected < 0 {
			expected = 0
		}
		if remaining != expected {
			t.Fatalf("remaining %f != expected %f", remaining, expected)
		}
	})
}

// Property: Deductions are additive (deduct a + deduct b = deduct a+b)
func TestProperty_TaskBudgetDeductionsAdditive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.Float64Range(10, 100).Draw(t, "limit")
		spentStart := rapid.Float64Range(0, 5).Draw(t, "spentStart")

		a := rapid.Float64Range(0, 10).Draw(t, "a")
		b := rapid.Float64Range(0, 10).Draw(t, "b")

		budget1 := &TaskBudget{LimitUSD: limit, SpentUSD: spentStart}
		budget2 := &TaskBudget{LimitUSD: limit, SpentUSD: spentStart}

		// Two separate deductions
		budget1.SpentUSD += a
		budget1.SpentUSD += b

		// One combined deduction
		budget2.SpentUSD += (a + b)

		if math.Abs(budget1.Remaining()-budget2.Remaining()) > 1e-9 {
			t.Fatalf("deductions not additive: %f != %f", budget1.Remaining(), budget2.Remaining())
		}
	})
}

// Property: Zero deduction doesn't change budget
func TestProperty_TaskBudgetZeroDeduction(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.Float64Range(0, 100).Draw(t, "limit")
		spent := rapid.Float64Range(0, 100).Draw(t, "spent")

		budget := &TaskBudget{LimitUSD: limit, SpentUSD: spent}
		remStart := budget.Remaining()

		budget.SpentUSD += 0

		if budget.Remaining() != remStart {
			t.Fatalf("zero deduction changed budget: %f -> %f", remStart, budget.Remaining())
		}
	})
}

// Also for GrantRecord
// Property: Budget remaining = limit - spent (never negative)
func TestProperty_GrantRemaining(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		amount := rapid.Float64Range(0, 1000).Draw(t, "amount")
		spent := rapid.Float64Range(0, 1500).Draw(t, "spent")

		grant := &GrantRecord{
			AmountUSD: amount,
			SpentUSD:  spent,
		}

		remaining := grant.Remaining()
		if remaining < 0 {
			t.Fatalf("remaining cannot be negative: %f (amount=%f, spent=%f)", remaining, amount, spent)
		}

		expected := amount - spent
		if expected < 0 {
			expected = 0
		}
		if remaining != expected {
			t.Fatalf("remaining %f != expected %f", remaining, expected)
		}
	})
}
