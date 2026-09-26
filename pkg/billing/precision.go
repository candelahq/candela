package billing

import (
	"math"
)

// Precision constants for currency throughout Candela's billing pipeline.
const (
	// EpsilonUSD is the tolerance used for floating-point equality comparisons (1 nano-dollar: $0.000000001).
	EpsilonUSD = 1e-9

	// MicroUSD is the multiplier / divisor for integer micro-USD representations ($0.000001).
	MicroUSD = 1e-6

	// SubCentDecimals is the number of decimal places preserved for sub-cent LLM billing.
	// Most modern LLM tokens cost between $0.00000005 and $0.00005 per token.
	SubCentDecimals = 8
)

// SafeSubUSD subtracts spent from limit or balance, clamping negative residuals to zero.
// This prevents floating-point subtraction cancellation (e.g., 5.0 - 5.0 = -1e-17)
// from producing negative balances or false overdraft signals.
func SafeSubUSD(limit, spent float64) float64 {
	rem := limit - spent
	if rem <= 0 || math.IsNaN(rem) {
		return 0
	}
	return rem
}

// ClampNonNegative returns 0 if val is negative, negative zero, or NaN; otherwise returns val.
func ClampNonNegative(val float64) float64 {
	if val <= 0 || math.IsNaN(val) {
		return 0
	}
	return val
}

// RoundUSD rounds a USD amount to the specified number of decimal places.
// For example, RoundUSD(0.123456789, 6) returns 0.123457 (rounded to microdollars).
func RoundUSD(usd float64, decimals int) float64 {
	if math.IsNaN(usd) || math.IsInf(usd, 0) {
		return 0
	}
	if decimals < 0 {
		decimals = 0
	}
	pow := math.Pow(10, float64(decimals))
	return math.Round(usd*pow) / pow
}

// EqualUSD reports whether two USD values are equal within the nano-dollar EpsilonUSD tolerance.
func EqualUSD(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	return math.Abs(a-b) <= EpsilonUSD
}

// ToMicroUSD converts a float64 USD amount to integer micro-USD ($0.000001), rounding to the nearest microdollar.
func ToMicroUSD(usd float64) int64 {
	if math.IsNaN(usd) || math.IsInf(usd, 0) || usd <= 0 {
		return 0
	}
	return int64(math.Round(usd * 1_000_000))
}

// FromMicroUSD converts integer micro-USD ($0.000001) to float64 USD.
func FromMicroUSD(micro int64) float64 {
	if micro <= 0 {
		return 0
	}
	return float64(micro) / 1_000_000.0
}
