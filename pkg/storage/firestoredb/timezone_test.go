package firestoredb

import (
	"testing"
	"time"
)

// ── Timezone-aware period key tests (#136) ───────────────────────────

// refTime is a fixed point in time used across all period key tests.
// Using a fixed time eliminates midnight-boundary flakiness when the
// date rolls over between currentPeriodKey and the expected value.
var refTime = time.Date(2026, 6, 13, 14, 30, 0, 0, time.UTC)

func TestCurrentPeriodKey_UTC(t *testing.T) {
	key := currentPeriodKey("daily", time.UTC, refTime)
	expected := "2026-06-13"
	if key != expected {
		t.Errorf("got %q, want %q", key, expected)
	}
}

func TestCurrentPeriodKey_CustomTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("failed to load timezone: %v", err)
	}
	key := currentPeriodKey("daily", loc, refTime)
	// 14:30 UTC = 10:30 EDT, still June 13
	expected := "2026-06-13"
	if key != expected {
		t.Errorf("got %q, want %q", key, expected)
	}
}

func TestCurrentPeriodKey_CustomTimezone_DateShift(t *testing.T) {
	// Use a time near midnight UTC so that a far-east timezone shifts to the next day.
	nearMidnight := time.Date(2026, 6, 13, 23, 30, 0, 0, time.UTC)
	loc, err := time.LoadLocation("Asia/Tokyo") // UTC+9
	if err != nil {
		t.Fatalf("failed to load timezone: %v", err)
	}
	key := currentPeriodKey("daily", loc, nearMidnight)
	// 23:30 UTC = 08:30 JST next day
	expected := "2026-06-14"
	if key != expected {
		t.Errorf("got %q, want %q", key, expected)
	}
}

func TestCurrentPeriodKey_MonthlyWithTimezone(t *testing.T) {
	key := currentPeriodKey("monthly", time.UTC, refTime)
	expected := "2026-06"
	if key != expected {
		t.Errorf("got %q, want %q", key, expected)
	}
}

func TestCurrentPeriodKey_NilLocationDefaultsToUTC(t *testing.T) {
	key := currentPeriodKey("daily", nil, refTime)
	expected := "2026-06-13"
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
