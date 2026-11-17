---
name: verify-cache-cost
description: Checks that the cache charge Candela reports reconciles with the input cost the calculator bills. Use when changing pkg/costcalc pricing, cache discounts, cache token normalization, or the cache cost recorded on a span.
---

# Verify cache cost

**The property.** For a request on a provider with cache pricing, the cache charge plus the charge for the fresh tokens equals what `Calculate` charges for the whole normalized prompt, and the cache charge never exceeds that total.

**The harm.** A break attributes to cache dollars the provider never charged, or hides dollars it did charge. Either way the cache cost shown in Candela Desktop and the budget the user sees stop matching the bill.

**Run it.**

```
nix develop --command .agents/skills/verify-cache-cost/run.sh
```

from the repository root. It runs the property over generated requests, then 20 seconds of fuzzing. Takes under a minute. Outside the flake, run the script directly with the Go toolchain on PATH.

**What it covers.** `pkg/costcalc/cache_cost_property_test.go`, which drives the real functions in `pkg/costcalc/calculator.go`:

- `CacheCostUSD`
- `NormalizeCachedInputWithTTL` and `normalizeCachedInput`
- `cacheConfigFor`
- `inputCostUSD` and `discounted`
- `Calculate`, as the reference for the total

Provider and model cases live in `cacheCostCases` in the test: anthropic with its three route aliases, openai, google with its two aliases, and one provider with no cache pricing. Token counts are generated up to 2,000,000 fresh, 1,000,000 cache read and 1,000,000 cache write, with both cache TTLs.

**Last confirmed against** `ff819ddcc4aba8f0028b529eb4be2968f6c3f267`. When a covered file changes after that commit, rerun before trusting the result, and move this commit forward only after you confirm the checks still describe the code.

**Bounds and what it does not cover.**

- Tiered models are excluded from the reconciliation. `CacheCostUSD` allocates cache as a share of the tiered input cost, so a fresh-token charge computed at base rates cannot add back to the total by construction. For those models the check only asserts the charge is non-negative and never above the input cost.
- It does not cover `promptTokensFor` in `pkg/proxy/proxy.go`, the storage sums, or the desktop. Those have their own tests.
- It checks the calculator, not the provider. If a provider changes its real cache pricing, the constants in `defaultCacheDiscounts` and `googleCacheReadDiscount` are wrong before this check ever runs.

**Reading a failure.** The message names the provider, the model, the token counts, the TTL, and both charges. A real bug looks like a systematic shortfall or overshoot for one provider family or one TTL. A modeling error looks like a case whose token semantics do not match the case table, for example a new inclusive provider that reports cache writes, or a prompt count smaller than its own cached tokens. Fix the case table for the second kind, never the assertion.

**When you change the property.** If a new provider or a new cache pricing shape lands, add a case to `cacheCostCases` with its real token semantics, then confirm the new case fails against a deliberately broken `CacheCostUSD` before trusting it to pass.
