package firestoredb

import (
	"testing"
	"time"
)

// ── Timezone-aware period key tests (#136) ───────────────────────────

func TestCurrentPeriodKey_UTC(t *testing.T) {
	key := currentPeriodKey("daily", time.UTC)
	expected := time.Now().UTC().Format("2006-01-02")
	if key != expected {
		t.Errorf("got %q, want %q", key, expected)
	}
}

func TestCurrentPeriodKey_CustomTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("failed to load timezone: %v", err)
	}
	key := currentPeriodKey("daily", loc)
	expected := time.Now().In(loc).Format("2006-01-02")
	if key != expected {
		t.Errorf("got %q, want %q", key, expected)
	}
}

func TestCurrentPeriodKey_MonthlyWithTimezone(t *testing.T) {
	key := currentPeriodKey("monthly", time.UTC)
	expected := time.Now().UTC().Format("2006-01")
	if key != expected {
		t.Errorf("got %q, want %q", key, expected)
	}
}

func TestCurrentPeriodKey_NilLocationDefaultsToUTC(t *testing.T) {
	key := currentPeriodKey("daily", nil)
	expected := time.Now().UTC().Format("2006-01-02")
	if key != expected {
		t.Errorf("nil loc: got %q, want %q", key, expected)
	}
}

func TestSetBudgetLocation(t *testing.T) {
	s := &Store{budgetLocation: time.UTC}

	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("failed to load timezone: %v", err)
	}
	s.SetBudgetLocation(loc)
	if s.budgetLocation != loc {
		t.Errorf("budgetLocation = %v, want Asia/Tokyo", s.budgetLocation)
	}

	// nil resets to UTC.
	s.SetBudgetLocation(nil)
	if s.budgetLocation != time.UTC {
		t.Errorf("budgetLocation after nil = %v, want UTC", s.budgetLocation)
	}
}
