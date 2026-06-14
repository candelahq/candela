# ADR 001: Catalog Pick-One Backend

**Status:** Accepted
**Date:** 2024-06-14

## Context

The model catalog needs a persistent store. We considered:
1. **Layered config** — stack multiple sources (config file + database + API)
2. **Pick-one backend** — select exactly one backend via config

Layered configs add complexity: conflict resolution, precedence rules, and debugging which layer "won".

## Decision

Use a **pick-one backend** pattern: `catalog.backend` selects exactly one of `config`, `firestore`, or `postgres` (future). The `ModelCatalogStore` interface ensures all backends are interchangeable.

## Consequences

- **Simpler debugging**: one source of truth, no precedence ambiguity
- **Clean interface**: `List`, `Get`, `Update`, `Source()`, `Writable()` — each backend implements all or returns `ErrReadOnly`
- **No `Delete` by design**: catalog entries are soft-disabled (via `Update`) rather than hard-deleted, preserving referential integrity for historical spans and audit trails
- **Migration path**: switch backends by changing one config field
- **Limitation**: can't mix config-based models with database-managed ones in the same deployment
