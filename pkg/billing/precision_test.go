package billing

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

func TestPrecision_SafeSubUSD(t *testing.T) {
	// Standard subtraction
	if got := SafeSubUSD(10.0, 4.5); !EqualUSD(got, 5.5) {
		t.Errorf("SafeSubUSD(10.0, 4.5) = %f, want 5.5", got)
	}

	// Exact cancellation must return exactly 0
	if got := SafeSubUSD(5.0, 5.0); got != 0 {
		t.Errorf("SafeSubUSD(5.0, 5.0) = %f, want 0", got)
	}

	// Overdraft must return 0
	if got := SafeSubUSD(5.0, 10.0); got != 0 {
		t.Errorf("SafeSubUSD(5.0, 10.0) = %f, want 0", got)
	}

	// Microscopic negative residuals from floating point cancellation (e.g. 1e-16)
	if got := SafeSubUSD(1.0000000000000002, 1.0000000000000004); got != 0 {
		t.Errorf("SafeSubUSD with micro-underflow = %f, want 0", got)
	}

	// NaN and Inf handling
	if got := SafeSubUSD(math.NaN(), 5.0); got != 0 {
		t.Errorf("SafeSubUSD(NaN) = %f, want 0", got)
	}
}

func TestPrecision_ClampNonNegative(t *testing.T) {
	if got := ClampNonNegative(math.Copysign(0, -1)); got != 0 {
		t.Errorf("ClampNonNegative(negative zero) = %f, want 0", got)
	}
	if got := ClampNonNegative(-100.5); got != 0 {
		t.Errorf("ClampNonNegative(-100.5) = %f, want 0", got)
	}
	if got := ClampNonNegative(math.NaN()); got != 0 {
		t.Errorf("ClampNonNegative(NaN) = %f, want 0", got)
	}
	if got := ClampNonNegative(42.5); got != 42.5 {
		t.Errorf("ClampNonNegative(42.5) = %f, want 42.5", got)
	}
}

func TestPrecision_RoundUSD(t *testing.T) {
	tests := []struct {
		val      float64
		decimals int
		want     float64
	}{
		{0.123456789, 2, 0.12},
		{0.125, 2, 0.13},
		{0.123456789, 6, 0.123457},
		{0.000075, 6, 0.000075},
		{0.000075123, 6, 0.000075},
		{0.000075123, 8, 0.00007512},
		{math.NaN(), 2, 0},
		{math.Inf(1), 2, 0},
		{10.5, -1, 11},
	}
	for _, tc := range tests {
		got := RoundUSD(tc.val, tc.decimals)
		if !EqualUSD(got, tc.want) {
			t.Errorf("RoundUSD(%f, %d) = %f, want %f", tc.val, tc.decimals, got, tc.want)
		}
	}
}

func TestPrecision_EqualUSD(t *testing.T) {
	if !EqualUSD(10.0, 10.0000000001) {
		t.Errorf("expected EqualUSD for difference < EpsilonUSD")
	}
	if EqualUSD(10.0, 10.000001) {
		t.Errorf("did not expect EqualUSD for 1 microdollar difference")
	}
	if EqualUSD(math.NaN(), math.NaN()) {
		t.Errorf("NaN should not equal NaN in EqualUSD")
	}
}

func TestPrecision_MicroUSDConversions(t *testing.T) {
	usd := 12.345678
	micro := ToMicroUSD(usd)
	if micro != 12345678 {
		t.Errorf("ToMicroUSD(%f) = %d, want 12345678", usd, micro)
	}

	back := FromMicroUSD(micro)
	if !EqualUSD(back, usd) {
		t.Errorf("FromMicroUSD(%d) = %f, want %f", micro, back, usd)
	}

	// Negative and zero cases
	if got := ToMicroUSD(-5.0); got != 0 {
		t.Errorf("ToMicroUSD(-5.0) = %d, want 0", got)
	}
	if got := FromMicroUSD(-100); got != 0 {
		t.Errorf("FromMicroUSD(-100) = %f, want 0", got)
	}
}

// Property: SafeSubUSD is strictly non-negative for all inputs
func TestProperty_SafeSubUSD_NonNegative(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.Float64Range(-1000, 10000).Draw(t, "limit")
		spent := rapid.Float64Range(-1000, 10000).Draw(t, "spent")

		res := SafeSubUSD(limit, spent)
		if res < 0 || math.IsNaN(res) {
			t.Fatalf("SafeSubUSD(%f, %f) = %f, must be >= 0", limit, spent, res)
		}
		if limit > spent && spent >= 0 && limit >= 0 {
			expected := limit - spent
			if math.Abs(res-expected) > EpsilonUSD {
				t.Fatalf("SafeSubUSD(%f, %f) = %f != expected %f", limit, spent, res, expected)
			}
		}
	})
}

// Property: BudgetRecord.Remaining is non-negative and bounded by LimitUSD
func TestProperty_BudgetRecordRemaining(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		limit := rapid.Float64Range(0, 5000).Draw(t, "limit")
		spent := rapid.Float64Range(0, 7000).Draw(t, "spent")

		b := &BudgetRecord{
			LimitUSD: limit,
			SpentUSD: spent,
		}

		rem := b.Remaining()
		if rem < 0 || math.IsNaN(rem) {
			t.Fatalf("budget remaining cannot be negative or NaN: %f", rem)
		}
		if rem > limit {
			t.Fatalf("budget remaining %f cannot exceed limit %f", rem, limit)
		}
		if limit > spent {
			if math.Abs(rem-(limit-spent)) > EpsilonUSD {
				t.Fatalf("remaining %f != expected %f", rem, limit-spent)
			}
		} else {
			if rem != 0 {
				t.Fatalf("overspent budget must have remaining == 0, got %f", rem)
			}
		}
	})
}

// Property: MicroUSD roundtrip preserves values up to 6 decimal places
func TestProperty_MicroUSDRoundtrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw integer microdollar amount up to $100,000.00
		originalMicro := rapid.Int64Range(0, 100_000_000_000).Draw(t, "micro")
		usd := FromMicroUSD(originalMicro)
		convertedMicro := ToMicroUSD(usd)

		if originalMicro != convertedMicro {
			t.Fatalf("roundtrip mismatch: original %d != converted %d (usd=%f)", originalMicro, convertedMicro, usd)
		}
	})
}
