package firestoredb

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Property: Period keys for same day/week/month are equal
func TestProperty_PeriodKeyEqualForSamePeriod(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		periodType := rapid.SampledFrom([]string{"daily", "weekly", "monthly"}).Draw(t, "periodType")

		year := rapid.IntRange(2000, 2100).Draw(t, "year")
		month := time.Month(rapid.IntRange(1, 12).Draw(t, "month"))
		day := rapid.IntRange(1, 28).Draw(t, "day")

		hour1 := rapid.IntRange(0, 11).Draw(t, "hour1")
		hour2 := rapid.IntRange(12, 23).Draw(t, "hour2")

		t1 := time.Date(year, month, day, hour1, 0, 0, 0, time.UTC)
		t2 := time.Date(year, month, day, hour2, 0, 0, 0, time.UTC)

		key1 := currentPeriodKey(periodType, time.UTC, t1)
		key2 := currentPeriodKey(periodType, time.UTC, t2)

		if key1 != key2 {
			t.Fatalf("expected keys to be equal for same day: %s != %s", key1, key2)
		}
	})
}

// Property: Period keys are always non-empty strings
func TestProperty_PeriodKeyNonEmpty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		periodType := rapid.String().Draw(t, "periodType")
		unixTime := rapid.Int64().Draw(t, "unixTime")
		now := time.Unix(unixTime, 0)

		key := currentPeriodKey(periodType, time.UTC, now)
		if key == "" {
			t.Fatalf("period key should never be empty for type %q", periodType)
		}
	})
}

// Property: Period keys are sortable (lexicographic ordering = chronological)
func TestProperty_PeriodKeyChronologicalSort(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		periodType := rapid.SampledFrom([]string{"daily", "weekly", "monthly"}).Draw(t, "periodType")

		t1Unix := rapid.Int64Range(0, 2000000000).Draw(t, "t1Unix")
		offset := rapid.Int64Range(86400*32, 86400*365).Draw(t, "offset") // Ensure different period
		t2Unix := t1Unix + offset

		t1 := time.Unix(t1Unix, 0).UTC()
		t2 := time.Unix(t2Unix, 0).UTC()

		key1 := currentPeriodKey(periodType, time.UTC, t1)
		key2 := currentPeriodKey(periodType, time.UTC, t2)

		if key1 >= key2 {
			t.Fatalf("expected key1 < key2 for chronological sort, but got %s >= %s", key1, key2)
		}
	})
}
