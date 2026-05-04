#!/usr/bin/env bash
set -euo pipefail

REPO="candelahq/candela"
MILESTONE="Rust Rewrite"

# Helper: create issue and print result
create_issue() {
  local title="$1"
  local labels="$2"
  local body="$3"
  
  echo "Creating: $title"
  gh issue create \
    --repo "$REPO" \
    --milestone "$MILESTONE" \
    --title "$title" \
    --label "$labels" \
    --body "$body"
  echo ""
}

# ── Ensure labels exist ──
echo "Creating labels..."
gh label create "rust-rewrite" --repo "$REPO" --color "DEA584" --description "Rust rewrite tracking" --force 2>/dev/null || true
gh label create "phase-0" --repo "$REPO" --color "0E8A16" --description "Foundation / setup" --force 2>/dev/null || true
gh label create "phase-1" --repo "$REPO" --color "1D76DB" --description "Phase 1: candela-sidecar" --force 2>/dev/null || true
gh label create "phase-2" --repo "$REPO" --color "5319E7" --description "Phase 2: Proxy hardening" --force 2>/dev/null || true
gh label create "phase-3" --repo "$REPO" --color "B60205" --description "Phase 3: candela-local" --force 2>/dev/null || true
gh label create "phase-4" --repo "$REPO" --color "D93F0B" --description "Phase 4: candela-server" --force 2>/dev/null || true
echo ""

# ════════════════════════════════════════════
# PHASE 0 — Foundation
# ════════════════════════════════════════════

create_issue \
  "[Rust] Create candela-rs repo + Cargo workspace scaffold" \
  "rust-rewrite,phase-0" \
'## Summary
Create the `candelahq/candela-rs` repository with the Cargo workspace scaffold for the incremental Rust rewrite.

## What
- Create new GitHub repo `candelahq/candela-rs`
- Initialize Cargo workspace with 4 crates + 1 binary:
  - `crates/candela-core` — domain types, `SpanWriter` trait
  - `crates/candela-proxy` — LLM reverse proxy engine
  - `crates/candela-processor` — batched span processor + cost calculator
  - `crates/candela-storage` — Pub/Sub + OTLP writer implementations
  - `bins/candela-sidecar` — main binary
- Add workspace-level `Cargo.toml` with shared dependencies
- Add `.gitignore`, `deny.toml`, `rustfmt.toml`, `clippy.toml`

## Why
Clean separation from the Go monorepo. Fits the existing multi-repo pattern (`candela`, `candela-protos`, `candela-desktop`, `candela-docs`). Each crate maps to a Go `pkg/` package for clear migration tracking.

## Acceptance Criteria
- [ ] `cargo check --workspace` passes with stub crates
- [ ] Workspace compiles on stable Rust (≥1.87)
- [ ] README documents the workspace layout and migration strategy'

create_issue \
  "[Rust] Set up Nix flake for Rust dev environment" \
  "rust-rewrite,phase-0" \
'## Summary
Create a `flake.nix` for the `candela-rs` repo that provides a reproducible Rust development environment.

## What
- Rust stable toolchain (rustc, cargo, clippy, rustfmt)
- `buf` CLI for proto codegen
- `protobuf` for protoc
- `cargo-deny` for license/vulnerability audit
- `cargo-watch` for dev iteration
- lefthook for pre-commit hooks

## Why
Consistent with the Go repo pattern (`candela/flake.nix`). All developers get identical toolchains via `nix develop`.

## Acceptance Criteria
- [ ] `nix develop -c cargo --version` works
- [ ] `nix develop -c buf --version` works
- [ ] `nix develop -c cargo deny --version` works'

create_issue \
  "[Rust] Set up Buf codegen for Rust (prost types from BSR)" \
  "rust-rewrite,phase-0" \
'## Summary
Configure `buf generate` to produce Rust types from the existing `buf.build/candelahq/protos` module, matching the Go/TypeScript codegen workflow.

## What
- Add `buf.gen.yaml` with remote BSR plugins:
  - `buf.build/community/neoeinstein-prost:v0.5.0` → struct generation
  - `buf.build/community/neoeinstein-prost-serde:v0.5.0` → JSON serialization
- Input: `buf.build/candelahq/protos:v0.2.1`
- Output: `gen/src/` (committed to repo, same pattern as `candela/gen/go/`)
- Wire `candela-core` crate to re-export generated types via `proto.rs`

## Why
Same Buf workflow the team already knows. No raw `protoc`, no `build.rs` complexity. Proto types stay in sync with Go/TS via the shared BSR module.

## Reference
Go equivalent: `candela/buf.gen.yaml` (uses `buf.build/connectrpc/go` + `buf.build/protocolbuffers/go`)

## Acceptance Criteria
- [ ] `buf generate` produces Rust files in `gen/src/`
- [ ] Generated types compile and derive `serde::Serialize`/`Deserialize`
- [ ] `Span` proto type round-trips through JSON correctly'

create_issue \
  "[Rust] Set up GitHub Actions CI pipeline" \
  "rust-rewrite,phase-0" \
'## Summary
Configure GitHub Actions for the `candela-rs` repo with format checks, linting, testing, and container image builds.

## What
- **CI job** (on every push/PR):
  - `cargo fmt --check`
  - `cargo clippy --workspace -- -D warnings`
  - `cargo test --workspace`
  - `cargo deny check` (license + vulnerability audit)
- **Build job** (on main only):
  - Build `Dockerfile.sidecar`
  - Push to `us-central1-docker.pkg.dev/$PROJECT/candela/candela-sidecar-rs`
  - Authenticate via Workload Identity Federation
- Use `Swatinem/rust-cache@v2` for dependency caching

## Why
Same Artifact Registry as the Go sidecar, tagged with `-rs` suffix for side-by-side comparison during transition.

## Acceptance Criteria
- [ ] PRs get format + lint + test checks
- [ ] Main branch pushes build and push container image
- [ ] Build time < 10 minutes with caching'

# ════════════════════════════════════════════
# PHASE 1 — candela-sidecar
# ════════════════════════════════════════════

create_issue \
  "[Rust] Implement candela-core: domain types + SpanWriter trait" \
  "rust-rewrite,phase-1" \
'## Summary
Port the core domain types and storage interfaces from `pkg/storage/store.go` to the `candela-core` crate.

## What
- `Span` struct with all fields (span_id, trace_id, gen_ai, attributes, etc.)
- `GenAIAttributes` struct
- `SpanKind` and `SpanStatus` enums
- `SpanWriter` async trait (`ingest_spans(&self, spans: &[Span]) -> Result<()>`)
- Re-export buf-generated proto types for Pub/Sub serialization
- Full `serde` derive for JSON round-tripping

## Go Source
- [`pkg/storage/store.go`](https://github.com/candelahq/candela/blob/main/pkg/storage/store.go) lines 40-97

## Why
Every other crate depends on these types. Establishing them first ensures a stable foundation. The Rust `enum` types provide exhaustive matching that the Go `iota` constants lack.

## Acceptance Criteria
- [ ] All types match the Go equivalents field-for-field
- [ ] `Span` round-trips through `serde_json` (serialize → deserialize → equal)
- [ ] `SpanKind`/`SpanStatus` enum values match proto definitions'

create_issue \
  "[Rust] Implement candela-proxy: LLM reverse proxy engine" \
  "rust-rewrite,phase-1" \
'## Summary
Port the LLM reverse proxy from `pkg/proxy/` to the `candela-proxy` crate. This is the largest and most critical crate — it handles all LLM API traffic.

## What
### Core proxy (`proxy.go` → `lib.rs`)
- `Proxy` struct with provider routing, circuit breakers, span capture
- `Provider` config (name, upstream URL, format translator, path rewriter, token source)
- Route registration (`/proxy/{provider}/...`)
- Request forwarding with body capture
- Streaming SSE response handling
- Span generation from captured request/response data

### Format translation (`translate.go` → `translate.rs`)
- `AnthropicFormatTranslator` — OpenAI ↔ Anthropic format conversion
- **Key improvement**: Fully typed `serde` structs replacing Go `map[string]interface{}`
- `TranslateRequest`: OpenAI Chat Completions → Anthropic Messages
- `TranslateResponse`: Anthropic → OpenAI (non-streaming)
- `TranslateStreamChunk`: Anthropic SSE → OpenAI SSE (streaming)
- Tool call translation (function calling)
- `VertexAIPathRewriter` — model name parsing + Vertex AI URL construction

### Supporting modules
- `parsers.go` → `parsers.rs` — response body parsing, token extraction
- `circuit_breaker.go` → `circuit.rs` — per-provider circuit breaker
- `ids.go` → `ids.rs` — span/trace ID generation

## Go Source
- [`pkg/proxy/proxy.go`](https://github.com/candelahq/candela/blob/main/pkg/proxy/proxy.go) (944 LOC)
- [`pkg/proxy/translate.go`](https://github.com/candelahq/candela/blob/main/pkg/proxy/translate.go) (585 LOC)
- [`pkg/proxy/parsers.go`](https://github.com/candelahq/candela/blob/main/pkg/proxy/parsers.go) (275 LOC)
- [`pkg/proxy/circuit_breaker.go`](https://github.com/candelahq/candela/blob/main/pkg/proxy/circuit_breaker.go) (141 LOC)
- [`pkg/proxy/ids.go`](https://github.com/candelahq/candela/blob/main/pkg/proxy/ids.go) (81 LOC)

## Why
The proxy is the hot path — every LLM request flows through it. Rust gives zero-copy body forwarding via `hyper::Body`/`bytes::Bytes`, no GC pauses during SSE streaming, and compile-time guarantees that every Anthropic event type is handled (no silent `default` fallthrough).

## Acceptance Criteria
- [ ] Proxy forwards requests to mock upstream and returns responses
- [ ] Anthropic ↔ OpenAI format translation passes golden-file tests
- [ ] SSE streaming translation produces correct OpenAI chunks
- [ ] Tool call messages translate correctly
- [ ] Circuit breaker trips after configured failure threshold
- [ ] Vertex AI path rewriting produces correct URLs'

create_issue \
  "[Rust] Implement candela-processor: batched span processor + cost calculator" \
  "rust-rewrite,phase-1" \
'## Summary
Port the span processing pipeline and cost calculator from `pkg/processor/` and `pkg/costcalc/` to the `candela-processor` crate.

## What
### Span Processor (`processor.go` → `lib.rs`)
- `SpanProcessor` with `tokio::sync::mpsc` channel (replaces Go channel)
- Configurable batch size + 2-second flush timer
- Fan-out to multiple `SpanWriter` sinks
- Cost enrichment before flush
- Backpressure handling (drop + log when buffer full)
- Graceful shutdown with drain

### Cost Calculator (`calculator.go` → `cost.rs`)
- Per-model, per-provider token pricing table
- `calculate(provider, model, input_tokens, output_tokens) -> f64`
- Support for OpenAI, Anthropic, Google models

## Go Source
- [`pkg/processor/processor.go`](https://github.com/candelahq/candela/blob/main/pkg/processor/processor.go) (137 LOC)
- [`pkg/costcalc/calculator.go`](https://github.com/candelahq/candela/blob/main/pkg/costcalc/calculator.go) (275 LOC)

## Why
The processor is the bridge between the proxy and storage. Batching reduces Pub/Sub API calls and amortizes serialization cost.

## Acceptance Criteria
- [ ] Spans submitted individually are batched and flushed together
- [ ] Timer-based flush works for low-traffic periods
- [ ] Cost enrichment calculates correct USD values for known models
- [ ] Backpressure drops spans and logs warning (not panic)
- [ ] Graceful shutdown drains all pending spans'

create_issue \
  "[Rust] Implement candela-storage: Pub/Sub + OTLP span writers" \
  "rust-rewrite,phase-1" \
'## Summary
Port the Pub/Sub and OTLP span export sinks to the `candela-storage` crate.

## What
### Pub/Sub Writer (`pubsub.go` → `pubsub.rs`)
- Uses **official** `google-cloud-pubsub` crate (first-party Google Cloud Rust SDK)
- Proto serialization (prost) and JSON serialization (serde) modes
- Message attributes: `trace_id`, `project_id`, `format`
- Batch publish with `PublishResult` awaiting
- `Close()` flushes pending messages

### OTLP Exporter (`otlpexporter/` → `otlp.rs`)
- HTTP/JSON export to configurable OTLP endpoint
- Gzip compression
- Custom headers (for auth tokens)
- Span → OTLP ResourceSpans conversion
- Configurable timeout

## Go Source
- [`cmd/candela-sidecar/pubsub.go`](https://github.com/candelahq/candela/blob/main/cmd/candela-sidecar/pubsub.go) (159 LOC)
- [`pkg/storage/otlpexporter/`](https://github.com/candelahq/candela/tree/main/pkg/storage/otlpexporter) (391 LOC)

## Why
These are fire-and-forget sinks — the sidecar writes spans and moves on. Using the official Google Cloud Rust crate gives us first-party ADC support and idiomatic async.

## Acceptance Criteria
- [ ] Pub/Sub writer publishes spans in proto format (verified with emulator)
- [ ] Pub/Sub writer publishes spans in JSON format
- [ ] OTLP exporter sends correct ResourceSpans payload
- [ ] OTLP exporter handles gzip compression and custom headers
- [ ] Both writers implement `SpanWriter` trait cleanly'

create_issue \
  "[Rust] Wire up candela-sidecar binary" \
  "rust-rewrite,phase-1" \
'## Summary
Port the sidecar `main.go` to Rust, wiring all Phase 1 crates together into a single deployable binary.

## What
- Env-var based configuration (PORT, GCP_PROJECT, VERTEX_REGION, PROVIDERS, etc.)
- Span writer initialization (Pub/Sub + OTLP based on env vars)
- Span processor startup (`tokio::spawn`)
- LLM proxy creation with provider configuration
- ADC token source for Vertex AI (`gcp-auth`)
- Anthropic format translator + Vertex AI path rewriter wiring
- Provider filtering
- `axum` HTTP server with routes:
  - `GET /healthz` — health check
  - `GET /readyz` — readiness check
  - `/proxy/{provider}/...` — LLM proxy routes
- CORS middleware via `tower-http`
- Graceful shutdown (`tokio::signal`)
- `Dockerfile.sidecar` — multi-stage build, distroless runtime

## Go Source
- [`cmd/candela-sidecar/main.go`](https://github.com/candelahq/candela/blob/main/cmd/candela-sidecar/main.go) (295 LOC)

## Why
This is the Phase 1 deliverable — a drop-in replacement for the Go sidecar. Same env vars, same behavior, smaller binary, lower latency.

## Acceptance Criteria
- [ ] Binary starts and responds to `/healthz`
- [ ] Proxy routes forward to upstream LLM APIs
- [ ] Spans are exported to Pub/Sub and/or OTLP
- [ ] Graceful shutdown flushes pending spans
- [ ] Docker image builds and runs on distroless
- [ ] Binary size < 10 MB (vs 26 MB Go sidecar)
- [ ] Integration test: send request → verify span in Pub/Sub emulator'

# ════════════════════════════════════════════
# PHASE 2 — Proxy Hardening
# ════════════════════════════════════════════

create_issue \
  "[Rust] Phase 2: Proxy test suite + benchmarks" \
  "rust-rewrite,phase-2" \
'## Summary
Port the Go test suite for the proxy and add Rust-specific benchmarks to validate performance gains.

## What
- Port all proxy unit tests (~1,500 LOC Go tests → `#[tokio::test]`)
- Golden-file tests for format translation (OpenAI ↔ Anthropic)
- SSE streaming tests with multi-chunk responses
- Integration tests with mock upstream servers
- `criterion` benchmarks for:
  - Format translation throughput
  - SSE chunk parsing latency
  - Span serialization (proto vs JSON)
- CI integration: benchmarks run on PRs with comparison

## Why
The proxy is the most complex crate. Comprehensive testing ensures the Rust port is behavior-identical to the Go version before deploying to production.

## Acceptance Criteria
- [ ] Test coverage ≥ Go test coverage for proxy package
- [ ] All golden-file tests pass
- [ ] Benchmarks show measurable improvement over Go baseline
- [ ] CI runs tests on every PR'

# ════════════════════════════════════════════
# PHASE 3 — candela-local
# ════════════════════════════════════════════

create_issue \
  "[Rust] Phase 3: candela-local with runtime management + embedded UI" \
  "rust-rewrite,phase-3" \
'## Summary
Port the developer-facing `candela-local` binary from Go to Rust.

## What
- **Runtime management** (`pkg/runtime/` → `candela-runtime` crate):
  - `Runtime` trait: Start, Stop, Health, ListModels, PullModel, LoadModel, UnloadModel, DeleteModel
  - Ollama backend (HTTP API client)
  - vLLM backend (process management via `tokio::process::Command`)
  - LM Studio backend (filesystem + HTTP)
  - Runtime registry + manager
- **ConnectRPC RuntimeService** via `connect-rust` crate
- **Embedded UI** via `rust-embed` (replaces Go `embed`)
- **SQLite traces** via `rusqlite`
- **OIDC token injection** for remote server communication
- **Solo mode** (local-only) vs remote-proxy mode
- **YAML configuration** via `serde_yaml`

## Go Source
- [`cmd/candela-local/main.go`](https://github.com/candelahq/candela/blob/main/cmd/candela-local/main.go) (637 LOC)
- [`pkg/runtime/`](https://github.com/candelahq/candela/tree/main/pkg/runtime) (~800 LOC)

## Depends On
- Phase 1 crates (candela-core, candela-proxy, candela-processor)
- Phase 2 (proxy tests passing)

## Acceptance Criteria
- [ ] Runtime management works for Ollama, vLLM, LM Studio
- [ ] ConnectRPC RuntimeService responds to Flutter desktop client
- [ ] Embedded UI serves correctly
- [ ] SQLite trace storage works
- [ ] Solo mode functions without remote server'

# ════════════════════════════════════════════
# PHASE 4 — candela-server
# ════════════════════════════════════════════

create_issue \
  "[Rust] Phase 4: candela-server with ConnectRPC + multi-backend storage" \
  "rust-rewrite,phase-4" \
'## Summary
Port the full team backend `candela-server` from Go to Rust. This is the final and largest phase.

## What
- **ConnectRPC services** (`candela-rpc` crate via `connect-rust`):
  - TraceService — trace queries
  - IngestionService — span ingestion
  - DashboardService — aggregations
  - ProjectService — project/API key management
  - UserService — user CRUD, budgets, grants
- **Firebase Auth middleware** (`candela-auth` crate):
  - JWT verification against Google JWKS endpoint (~200 LOC)
  - IAP JWT validation
  - Admin guard interceptor
- **Storage backends** (expand `candela-storage`):
  - DuckDB via `duckdb-rs` (SpanReader + SpanWriter)
  - BigQuery via `gcp-bigquery-client` (SpanReader)
  - Firestore via community `firestore` crate (UserStore, ProjectStore)
  - Firestore transactions for `DeductSpend` budget waterfall
- **Budget enforcement** in proxy (team mode)
- **h2c** (HTTP/2 cleartext) support via `hyper`

## Go Source
- [`cmd/candela-server/main.go`](https://github.com/candelahq/candela/blob/main/cmd/candela-server/main.go) (587 LOC)
- [`pkg/connecthandlers/`](https://github.com/candelahq/candela/tree/main/pkg/connecthandlers) (~1,200 LOC)
- [`pkg/auth/`](https://github.com/candelahq/candela/tree/main/pkg/auth) (~500 LOC)
- [`pkg/storage/`](https://github.com/candelahq/candela/tree/main/pkg/storage) (~3,000 LOC)

## Depends On
- All Phase 1-3 crates
- Decision on ConnectRPC wire compatibility with Flutter client

## Client Impact
> ⚠️ The Flutter desktop client (`candela-desktop`) uses `connectrpc-dart`. With `connect-rust`, the wire protocol should be identical — but this MUST be verified with conformance tests before shipping.

## Acceptance Criteria
- [ ] All 5 ConnectRPC services respond correctly
- [ ] Firebase JWT verification works
- [ ] DuckDB storage backend reads/writes spans
- [ ] Firestore user/project operations work
- [ ] Budget deduction with grants works transactionally
- [ ] Flutter desktop client connects without code changes'

echo ""
echo "✅ All issues created!"
