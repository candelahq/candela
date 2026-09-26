# Architectural Evaluation: Float64 Currency Precision vs. Integer Micro-USD

**Issue:** [#645](https://github.com/candelahq/candela/issues/645)
**Status:** Decided & Implemented
**Date:** 2026-09-26
**Authors:** Candela Systems & Core Billing Team

---

## 1. Executive Summary

This document evaluates the trade-offs of migrating Candela's currency representation from IEEE 754 `float64` to fixed-point integer micro-USD (`int64` microdollars: $1.00 = 1,000,000 µUSD), as requested in issue [#645](https://github.com/candelahq/candela/issues/645).

### Decision
**We retain `float64` as the canonical currency type across Candela while implementing strict precision boundary guardrails, subtraction cancellation clamping (`SafeSubUSD`), equality tolerance (`EpsilonUSD = 1e-9`), and continuous property-based precision bounds testing.**

### Summary of Rationale
1. **Sub-Microdollar Token Pricing:** Modern high-efficiency LLM models cost fractions of a microdollar per token (e.g., Gemini 2.5 Flash Lite at $0.10/1M tokens = 0.1 µUSD/token; DeepSeek V3 cache hits at $0.014/1M tokens = 0.014 µUSD/token). Integer micro-USD cannot represent single-token transactions without truncation to 0 (100% loss) or artificial rounding up (multi-hundred-percent distortion).
2. **IEEE 754 Precision Headroom:** At balance scales up to $10,000.00, `float64` provides 53 bits of significand (15–17 decimal digits), resolving differences down to `1e-12` USD (pico-dollars)—orders of magnitude finer than integer micro-USD.
3. **Targeted Precision Hazards:** The only realistic precision hazards in `float64` billing are:
   - **Subtraction cancellation:** `limit - spent` yielding `-1e-17` or negative zero `-0.0` when spent equals limit.
   - **Equality comparison drift:** Direct `==` comparisons without epsilon tolerance.
   Both are completely mitigated by `SafeSubUSD()` and `EqualUSD()` in `pkg/billing/precision.go`.
4. **Blast Radius & API Compatibility:** A transition to integer micro-USD or nano-USD would break Protobuf schemas (`user_service.proto`, `trace.proto`), BigQuery telemetry columns (`cost_usd FLOAT64`), Firestore balance documents, REST APIs, and UI dashboard visualization.

---

## 2. Quantitative Precision Comparison

| Attribute | Integer Micro-USD (`int64`) | Integer Nano-USD (`int64`) | IEEE 754 `float64` (with Candela guardrails) |
| :--- | :--- | :--- | :--- |
| **Smallest Unit** | $0.000001 (1 µUSD) | $0.000000001 (1 nUSD) | Continuous (~$1e-15 at $1.00 balance) |
| **1 Token @ $0.10 / 1M** | Truncates to 0 µUSD ❌ | 100 nUSD ✅ | $0.00000010 ✅ |
| **1 Token @ $0.014 / 1M** | Truncates to 0 µUSD ❌ | 14 nUSD ✅ | $0.000000014 ✅ |
| **Accumulation Drift (100k txs)** | 0 (exact integer) | 0 (exact integer) | < 1e-9 USD (empirically verified by PBT) |
| **Protobuf Compatibility** | Breaking (change float to int64) | Breaking | Fully Backward Compatible |
| **Firestore Compatibility** | Migration required | Migration required | Seamless (numeric float) |
| **BigQuery Schema** | Requires migration/cast | Requires migration/cast | Native `FLOAT64` |
| **UI Formatting** | Needs `/ 1e6` everywhere | Needs `/ 1e9` everywhere | Direct dollar formatting |

---

## 3. Analysis of Failure Modes & Mitigations

### 3.1 Subtraction Cancellation
**Hazard:** When a user with a $5.00 grant spends exactly $5.00 across multiple requests, floating-point subtraction can evaluate to `-1.1102230246251565e-16`. In naive code (`if remaining < 0`), this negative residual could trigger false overdraft warnings or reduce adjacent balances in waterfall deductions (BILL-3).

**Mitigation:** `SafeSubUSD(limit, spent float64) float64`:
```go
func SafeSubUSD(limit, spent float64) float64 {
    rem := limit - spent
    if rem <= 0 || math.IsNaN(rem) {
        return 0
    }
    return rem
}
```
Standardized across all budget records:
- `BudgetRecord.Remaining()`
- `GrantRecord.Remaining()`
- `TaskBudget.Remaining()`

### 3.2 Micro-Transaction Accumulation Drift
**Hazard:** Repeated addition of fractional costs `sum(Calculate(in_i, out_i))` might diverge from bulk calculation `Calculate(sum(in_i), sum(out_i))`.

**Verification:** Implemented Rapid property-based test `TestProperty_AccumulatedSpendPrecision` in `pkg/costcalc/precision_test.go`:
- Simulates bursts of small prompt/completion requests across hundreds of iterations.
- Proves that over arbitrary sequences of micro-transactions, accumulated drift is strictly bounded within `1e-9` USD (1 nano-dollar).

### 3.3 Equality Comparisons
**Hazard:** Using `a == b` for floating point balances can fail due to insignificant least-bit differences.

**Mitigation:** `EqualUSD(a, b float64) bool` using `EpsilonUSD = 1e-9`:
```go
func EqualUSD(a, b float64) bool {
    if math.IsNaN(a) || math.IsNaN(b) {
        return false
    }
    return math.Abs(a-b) <= EpsilonUSD
}
```

---

## 4. Implemented Guardrails & Test Suite

The following packages establish the currency precision framework:

1. **`pkg/billing/precision.go`**:
   - `SafeSubUSD(limit, spent float64) float64`: Clamped non-negative subtraction.
   - `ClampNonNegative(val float64) float64`: Cleans negative zero and NaN.
   - `RoundUSD(usd float64, decimals int) float64`: Decimal rounding utility.
   - `EqualUSD(a, b float64) bool`: Epsilon comparison.
   - `ToMicroUSD(usd float64) int64` & `FromMicroUSD(micro int64) float64`: Micro-USD conversion utilities for export or interoperability with external accounting systems.

2. **`pkg/billing/precision_test.go`**:
   - Unit tests for edge cases (microscopic residuals, cancellation, negative values).
   - Rapid property test `TestProperty_SafeSubUSD_NonNegative`.
   - Rapid property test `TestProperty_BudgetRecordRemaining`.
   - Rapid property test `TestProperty_MicroUSDRoundtrip`.

3. **`pkg/costcalc/precision_test.go`**:
   - Rapid property test `TestProperty_CostPrecisionBounds` across flagship and sub-cent models.
   - Rapid property test `TestProperty_AccumulatedSpendPrecision` verifying `< 1e-9` drift under randomized transaction bursts.
   - Unit test `TestProperty_SubCentTokenGranularity` verifying sub-cent model resolution down to individual tokens.

---

## 5. Conclusion & Action Items

- **Evaluation Result:** Retaining `float64` with `SafeSubUSD` and nano-dollar epsilon comparisons provides greater precision for modern sub-cent LLM models than integer micro-USD, without incurring the systemic risk and breaking changes of a data migration.
- **Issue Status:** Criteria for [#645](https://github.com/candelahq/candela/issues/645) are completely fulfilled.
