# Changelog

All notable changes to Candela are documented here, organized by development phase. PRs are merged to `main`.

## v0.3.0 — 2026-05-14

### Auth Fix — User ADC Token Forwarding (#195)
- Fix "invalid authentication token" error in team mode by using `token.AccessToken` for user ADC credentials
- Ensures proper token forwarding when authenticating via Application Default Credentials

### Claude Code LLM Gateway (#194)
- Override `anthropic_version` to `vertex-2023-10-16` for Vertex AI passthrough
- Strip `model` field from request body to avoid Vertex AI validation errors
- Enables seamless Claude Code → Candela → Vertex AI routing

### Anthropic Prompt Caching (#192)
- Configurable `cache_control` breakpoint injection for Anthropic requests
- System prompt + last user message cached for ~10× cost reduction on multi-turn conversations
- Toggle via `vertex_ai.prompt_caching` in config
- Array-based system message support in Go translator

### Rust Proxy — P0 Sidecar Parity (#190)
- Line-by-line SSE token parser for streaming cache extraction
- W3C trace context propagation
- 120+ unit tests for parsers, attribution, and cost calculation
- Standalone `candela-proxy` binary release target

### Cost Accuracy Fixes (#188, #189)
- Fix 10× cost overcharge — cached tokens were double-counted as full-price input
- Fix trace duration reporting (was 0ms)
- Cache token visibility for all providers (OpenAI, Google, Anthropic)

### Documentation (#193)
- Remove all Docker/docker-compose references from docs
- Updated installation guides for native binary deployment

---

## Previous

### Documentation Audit & Site Expansion
- Added 3 missing proxy routes to README and docs site (`anthropic-vertex`, `anthropic-direct`, `gemini-oai`)
- New docs site pages: Claude Code IDE guide, Cursor/Windsurf IDE guide, Security & Auth, Multitenancy, Operations Runbook
- Updated Desktop component docs with Today View and Traces Screen features
- Added LICENSE files to `candela-desktop`, `candela-docs`, and `candela-protos` repos

### Token Counting & Cost Accuracy (#179)
- Fix Anthropic and Gemini over-reporting by correctly distinguishing cache/thinking tokens from standard usage
- Implement `strings.Builder` for streaming content concatenation (Gemini)
- Update test model pricing configurations

### Unified CLI — `candela-local` → `candela` (#178)

**⚠️ Breaking Change:**
- Renamed binary from `candela-local` to `candela` with `start`/`stop`/`status` subcommands
- Config path moved from `~/.candela.yaml` to `~/.config/candela/config.yaml` (legacy path still supported)
- Updated all internal docs, README, and Homebrew formula
- Homebrew: `brew install candelahq/tap/candela`

### Service Account Deny-by-Default (#177)

**⚠️ Breaking Change:**
- All service account tokens rejected with 403 unless explicitly allowlisted in `auth.allowed_service_accounts`
- Prevents unmetered cost vectors from automated systems

### Anthropic Vertex Provider — Claude Code Gateway (#176)
- New `/proxy/anthropic-vertex/` route for native Anthropic Messages API via Vertex AI
- New `/proxy/anthropic-direct/` route for direct Anthropic API passthrough
- New `/proxy/gemini-oai/` route for Gemini via OpenAI-compatible format
- Enables Claude Code integration via `ANTHROPIC_BASE_URL`

### Enrichment SDKs (#175)
- Zero-dependency SDKs for Python, TypeScript, Go, Kotlin, and Rust
- Inject `X-Candela-Tenant-Id`, `X-Candela-Job-Id`, and W3C Baggage headers
- Proxy strips enrichment headers before forwarding to upstream LLMs
- Per-tenant and per-job cost attribution in dashboard

### API Hardening & Proto Centralization (#113)

**⚠️ Breaking Changes:**
- `RuntimeStatus.status` (string) → `RuntimeStatus.state` (RuntimeState enum)
- `LoadModelResponse.status` (string) → `LoadModelResponse.state` (ModelLoadState enum)
- `PullModelResponse.status` field removed (async model)

**Security & Validation:**
- Proto source centralized to `candela-protos` repo, consumed via BSR v0.2.1
- `buf.validate` constraints on all ProjectService fields (name, ID, description bounds)
- IngestSpansRequest batch capped at 1000 spans (prevents OOM)
- GenAI content fields capped at 1MB (prevents storage bombs)
- PaginationRequest `page_size` bounded [0, 1000]
- All ID fields bounded to 128 chars max
- CODEOWNERS requiring platform-leads approval for proto changes

**Testing:**
- 25 proto validation unit tests (protovalidate, round-trip, field stability)
- 8 integration tests (migration correctness, security hooks, BSR reference)

**Infrastructure:**
- Migrated from pre-commit to lefthook with security hooks (detect-private-key, check-merge-conflict)
- `magic-nix-cache-action` for faster CI
- `gofmt` scoped to staged files only

### OTLP SpanWriter — Universal Export Sink (#70)
- New `pkg/storage/otlpexporter` package implementing `storage.SpanWriter`
- Export Candela spans as standard OpenTelemetry traces via OTLP/HTTP
- Maps GenAI fields to OTel `gen_ai.*` semantic conventions
- Resource grouping by `(ProjectID, ServiceName, Environment)` per OTLP spec
- Gzip compression by default, configurable per-export timeout
- `sinks.otlp.required` flag for environments where export is mandatory
- 25 tests: unit, integration (httptest OTLP receiver), config validation
- Enables write-once-export-everywhere to Datadog, Grafana Tempo, Jaeger, Elastic, Honeycomb, etc.

### Per-Conversation Session Tracking (#71)
- Heuristic-based `SessionResolver` with message-prefix fingerprinting
- SQLite-backed session state persistence across restarts
- `X-Session-ID` header propagation to remote proxy

### UI Enhancements (#73)
- Collapsible trace tree with inline cost metrics

## Phase 4: Multi-User Platform ✅

### Today Budget Dashboard (#66)
- Real-time "Today" budget card with spend vs. limit visualization
- UTC-consistent date boundaries for budget calculations

### Configurable Default Daily Budget (#64)
- Admin-configurable default daily budget for new users
- Applied automatically at user creation via Firestore

### Daily Budget Period + Delete User (#61, #62)
- Migrate budget model from monthly to daily-only
- User deletion with Firestore subcollection cleanup via `BulkWriter`
- Global audit collection for persistent deletion logs
- Review feedback: null-safety, `DocumentRefs` optimization

### Admin Fixes (#60)
- Fix user count aggregation, `UpdateUser` `MergeAll` struct bug
- Budget UI spacing and display fixes

### Auth & Storage Fixes (#55, #58, #59)
- Migrate to email-as-ID and fix `MergeAll` struct serialization (#55)
- Handle Firestore reserved `__.*__` document ID pattern (#58)
- Pass Firebase build args to Docker, improve login error handling (#59)

### Documentation (#54, #65)
- Comprehensive documentation deep-dive: architecture, development, deployment, proxy, env-vars, testing, cost-calculation, security, budgets, user-management, otel-collector, candela-local (#54)
- `pkg/runtime` README: interface contract, registry pattern, backend implementations (#65)

### Housekeeping (#57)
- Remove binaries and temp files from git tracking

### Team Mode Frontend Enhancements (#53)
- Per-user usage attribution in trace views
- Budget gauge component with threshold alerts
- User leaderboard with cost ranking
- Team mode E2E test suite (`team_mode.spec.ts`)

### Team Mode Backend — Budget Alerts & Leaderboard (#52)
- `BudgetChecker` with configurable thresholds (80%, 90%, 100%)
- `LogNotifier` for Cloud Logging–based alerts
- `GetUserLeaderboard` RPC for per-user cost ranking
- Budget deduction wired into proxy pipeline
- Deduplication: one notification per threshold per period

### Embedded Local Observability (#51)
- Local SQLite trace storage at `~/.candela/traces.db`
- `SpanProcessor` shared between solo and server modes
- Traces REST API at `/_local/api/traces`
- Traces card in management UI with auto-refresh

### SpanProcessor Extraction (#50)
- Extracted shared `pkg/processor` package
- Fan-out to multiple `SpanWriter` sinks
- Configurable batch size and flush interval
- Used by both `candela-server` and `candela-local`

### Solo Mode (#49)
- `candela-local` operates without a remote server
- Local models via Ollama/vLLM with full observability
- No cloud account required

### Unified Model Discovery (#48)
- `/v1/models` merges local, cloud, and remote models
- Smart routing: local stays local, cloud goes direct or via server
- Solo + Cloud mode with Vertex AI ADC integration

### Model Management Polish (#47)
- UI refinements for model management
- Health status auto-polling

### Delete Model, Cancel Pull, State DB (#46)
- Delete models from disk
- Cancel in-progress model downloads
- State persistence to `~/.candela/state.db`

### RuntimeService Proto + Management UI (#45)
- Full ConnectRPC service for runtime control
- Embedded vanilla JS management dashboard at `/_local/`

### Management API (#44)
- REST API at `/_local/*` for runtime control
- Start/stop, health monitoring, model listing

### pkg/runtime — Interface, Registry, Backends (#43)
- Abstract `Runtime` interface
- Registry with backend auto-discovery
- Ollama, vLLM, and LM Studio implementations

### Local Provider for Runtimes (#42)
- Local model proxy forwarding to Ollama/vLLM/LM Studio

### LM Studio Compat Listener (#41)
- Secondary listener on `:1234` for IntelliJ compatibility
- `/v1/models` and `/v1/chat/completions` at root path
- LM Studio native API (`/api/v0/models`) support

### LM Studio Compatibility Mode (#40)
- OpenAI-compatible routes at `/v1/` (no `/proxy/` prefix)
- Model-to-provider routing via config

### Expandable Prompt/Completion Views (#39)
- JSON-formatted prompt and completion display in trace detail
- Collapsible sections for large payloads

### OAuth2 Access Token Auth (#38)
- Strategy 3: Google OAuth2 access token validation via userinfo API
- Enables `candela-local` with user ADC

## Phase 3: Visual Explorer ✅

- Next.js 16 web dashboard with dark theme
- Dashboard with summary cards, time-series charts, recent traces
- Trace waterfall view with span-level detail
- Cost analytics page with model breakdown
- Usage metrics with provider filtering
- Project management with API key creation
- Settings page with backend status
- 27+ Playwright E2E tests
- Firebase Auth integration (Google Sign-In)
- Admin panel: user management, budgets, audit log
- ConnectRPC v2 client transport

## Phase 2: Storage & Architecture ✅

- DuckDB storage backend (OLAP-optimized, Appender API)
- SQLite storage backend (CGO-free, lightweight)
- BigQuery storage backend (serverless, time-partitioned)
- Pub/Sub write-only sink for fan-out
- OTLP export sink for universal OTel backend forwarding
- CQRS interfaces: `SpanWriter`, `SpanReader`, `TraceStore`
- CORS middleware with configurable origins
- Structured logging with `slog` (JSON)

## Phase 1: Foundation ✅

- LLM API proxy with observability (OpenAI, Google, Anthropic)
- SSE streaming support with response buffering
- Token extraction from provider-specific response formats
- Cost calculation engine with hardcoded pricing
- OpenTelemetry span ingestion (OTLP)
- Custom OTel Collector with GenAI processor
- Anthropic ↔ OpenAI format translation for Vertex AI routing
- ADC credential injection for GCP providers
- Circuit breaker for observability pipeline resilience
- Terraform infrastructure (Cloud Run, BigQuery, Firestore, IAP)
- Nix flake for reproducible dev environment
- Pre-commit hooks (golangci-lint, buf, go vet, go test)
- CI pipeline (GitHub Actions)
