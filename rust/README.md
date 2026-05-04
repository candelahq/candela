# Candela Rust Workspace

Rust implementation of the [Candela](https://github.com/candelahq/candela) LLM observability platform, colocated with the Go codebase for unified development and star accumulation.

> **Status**: Phase 1 — core types, processor, proxy, and sidecar scaffold are implemented with **38+ tests** (unit + integration). See the [Rust Rewrite milestone](https://github.com/candelahq/candela/milestone/1) for tracking.

## Workspace

| Crate | Description | Go Equivalent |
|---|---|---|
| `candela-core` | Domain types, `SpanWriter` trait | `pkg/storage/store.go` |
| `candela-proxy` | LLM reverse proxy engine | `pkg/proxy/` |
| `candela-processor` | Batched span processor + cost calculator | `pkg/processor/` + `pkg/costcalc/` |
| `candela-storage` | Pub/Sub + OTLP span writers | `cmd/candela-sidecar/pubsub.go` + `pkg/storage/otlpexporter/` |
| `candela-sidecar` | Minimal production proxy binary | `cmd/candela-sidecar/` |

## Quick Start

```bash
# From the repo root, enter the Rust dev shell
cd rust
nix develop

# Check workspace compiles
cargo check --workspace

# Run tests
cargo test --workspace

# Run sidecar locally
cargo run -p candela-sidecar
```

## Architecture

```
candela-sidecar (binary)
├── candela-proxy       (LLM routing, format translation, circuit breaking)
├── candela-processor   (span batching, cost enrichment)
├── candela-storage     (Pub/Sub, OTLP sinks)
└── candela-core        (Span, SpanWriter trait, domain types)
```

## Migration Strategy

This is an **incremental rewrite** from Go. Each phase produces an independently deployable binary:

1. **Phase 1**: `candela-sidecar` — drop-in replacement for the Go sidecar
2. **Phase 2**: Harden proxy with full test suite + benchmarks
3. **Phase 3**: `candela-local` — developer desktop binary
4. **Phase 4**: `candela-server` — full team backend with ConnectRPC

The Go code (`cmd/`, `pkg/`) continues running alongside — zero downtime, zero client breakage.
