# Candela Documentation

This directory contains **developer-facing documentation** for the Candela codebase.

## Relationship to Docs Site

The public-facing documentation lives in the [`candela-docs`](https://github.com/candelahq/candela-docs) repository and is published via Astro.

**Strategy: Option C — separation of concerns.**

- `candela/docs/` → **Internal developer docs**: architecture, operations, development guides, ADRs
- `candela-docs/` → **User-facing docs**: getting started, API reference, deployment guides, pricing

There is intentional overlap in some areas (e.g., deployment). The Astro site is the source of truth for user-facing content. This directory is the source of truth for internal/contributor content.

## Contents

| File | Description |
|------|-------------|
| `architecture.md` | System architecture overview |
| `api-reference.md` | Internal API documentation |
| `budgets.md` | Budget system design |
| `cost-calculation.md` | Cost calculation internals |
| `deployment.md` | Deployment procedures |
| `development.md` | Development setup |
| `env-vars.md` | Environment variables reference |
| `nix-setup.md` | Nix development environment |
| `operations.md` | Operational procedures |
| `proxy.md` | Proxy architecture |
| `security.md` | Security model |
| `testing.md` | Testing guide |
| `adr/` | Architecture Decision Records |
