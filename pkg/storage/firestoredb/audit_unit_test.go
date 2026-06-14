package firestoredb

import (
	"testing"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: currentPeriodKey tests — Issue #4 (timezone consistency)
// ──────────────────────────────────────────────────────────────────────────────

func TestAudit_CurrentPeriodKey_Daily(t *testing.T) {
	now := time.Date(2026, 6, 13, 23, 59, 59, 0, time.UTC)
	got := currentPeriodKey("daily", time.UTC, now)
	if got != "2026-06-13" {
		t.Errorf("currentPeriodKey(daily) = %q, want %q", got, "2026-06-13")
	}
}

func TestAudit_CurrentPeriodKey_Monthly(t *testing.T) {
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	got := currentPeriodKey("monthly", time.UTC, now)
	if got != "2026-01" {
		t.Errorf("currentPeriodKey(monthly) = %q, want %q", got, "2026-01")
	}
}

func TestAudit_CurrentPeriodKey_Weekly(t *testing.T) {
	// 2026-01-01 is a Thursday, ISO week 1
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	got := currentPeriodKey("weekly", time.UTC, now)
	if got != "2026-W01" {
		t.Errorf("currentPeriodKey(weekly) = %q, want %q", got, "2026-W01")
	}
}

func TestCurrentPeriodKey_WeeklyISOBoundary(t *testing.T) {
	// 2024-12-30 is a Monday in ISO week 1 of 2025
	now := time.Date(2024, 12, 30, 12, 0, 0, 0, time.UTC)
	got := currentPeriodKey("weekly", time.UTC, now)
	// ISO week should use the year from ISOWeek(), not calendar year.
	if got != "2025-W01" {
		t.Errorf("currentPeriodKey(weekly, 2024-12-30) = %q, want %q", got, "2025-W01")
	}
}

func TestCurrentPeriodKey_UnknownType(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	got := currentPeriodKey("invalid", time.UTC, now)
	// Unknown types fall back to daily.
	if got != "2026-06-13" {
		t.Errorf("currentPeriodKey(invalid) = %q, want daily format %q", got, "2026-06-13")
	}
}

func TestCurrentPeriodKey_NilLocation(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	got := currentPeriodKey("daily", nil, now)
	if got != "2026-06-13" {
		t.Errorf("currentPeriodKey(daily, nil loc) = %q, want %q", got, "2026-06-13")
	}
}

func TestCurrentPeriodKey_TimezoneShift(t *testing.T) {
	// Test that timezone affects date: 2026-06-14 01:00 UTC = 2026-06-13 in US/Pacific
	now := time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC)

	pacificLoc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("timezone not available:", err)
	}
	got := currentPeriodKey("daily", pacificLoc, now)
	if got != "2026-06-13" {
		t.Errorf("currentPeriodKey(daily, pacific) = %q, want %q (timezone shift)", got, "2026-06-13")
	}
}

func TestCurrentPeriodKey_LeapYear(t *testing.T) {
	// 2024 is a leap year, Feb 29 exists.
	now := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)
	got := currentPeriodKey("daily", time.UTC, now)
	if got != "2024-02-29" {
		t.Errorf("currentPeriodKey(daily, leap year) = %q, want %q", got, "2024-02-29")
	}
}

func TestCurrentPeriodKey_MonthBoundary(t *testing.T) {
	// End of month.
	now := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
	got := currentPeriodKey("monthly", time.UTC, now)
	if got != "2026-01" {
		t.Errorf("currentPeriodKey(monthly, month-end) = %q, want %q", got, "2026-01")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: sanitizeID edge cases
// ──────────────────────────────────────────────────────────────────────────────

func TestSanitizeID_NormalEmail(t *testing.T) {
	got := sanitizeID("User@Example.Com")
	if got != "user@example.com" {
		t.Errorf("sanitizeID(User@Example.Com) = %q, want %q", got, "user@example.com")
	}
}

func TestSanitizeID_SlashReplacement(t *testing.T) {
	got := sanitizeID("org/user@example.com")
	if got != "org_user@example.com" {
		t.Errorf("sanitizeID with slash = %q, want %q", got, "org_user@example.com")
	}
}

func TestAudit_SanitizeID_ConsecutiveDots(t *testing.T) {
	got := sanitizeID("user..test@example.com")
	if got != "user._test@example.com" {
		t.Errorf("sanitizeID with consecutive dots = %q, want %q", got, "user._test@example.com")
	}
}

func TestSanitizeID_ReservedPrefix(t *testing.T) {
	// Firestore reserves __.*__ pattern.
	got := sanitizeID("__admin__")
	if got != "u___admin__" {
		t.Errorf("sanitizeID(__admin__) = %q, want %q", got, "u___admin__")
	}
}

func TestSanitizeID_LongID(t *testing.T) {
	// Create ID longer than 1500 bytes.
	longID := ""
	for i := 0; i < 1600; i++ {
		longID += "a"
	}
	got := sanitizeID(longID)
	if len(got) > 1500 {
		t.Errorf("sanitizeID produced ID of length %d, want <= 1500", len(got))
	}
}

func TestSanitizeID_EmptyString(t *testing.T) {
	got := sanitizeID("")
	if got != "" {
		t.Errorf("sanitizeID('') = %q, want %q", got, "")
	}
}
