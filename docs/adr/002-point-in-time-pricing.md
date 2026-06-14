# ADR 002: Point-in-Time Pricing on Spans

**Status:** Accepted
**Date:** 2024-06-14

## Context

When model pricing changes, historical cost calculations become inaccurate. Auditors ask "what price was this request billed at?" and we couldn't answer.

## Decision

Record `InputPricePerMillion` and `OutputPricePerMillion` on every span at the time of cost calculation. This creates a permanent audit trail.

> **Note:** The `InputPricePerMillion` and `OutputPricePerMillion` proto fields were added to the span message in PR #356 and are already in production.

## Consequences

- **Audit-safe**: each span is self-contained with the exact rates used
- **Historical accuracy**: price changes don't retroactively alter past costs
- **Storage cost**: two extra float64 fields per span (~16 bytes)
- **Dashboard flexibility**: can recalculate costs with different rates without re-querying the catalog
