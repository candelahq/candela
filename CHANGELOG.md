# Changelog

All notable changes to Candela are documented here, organized by development phase. PRs are merged to `main`.

## Latest

### OTLP SpanWriter — Universal Export Sink (#70)
- New `pkg/storage/otlpexporter` package implementing `storage.SpanWriter`
- Export Candela spans as standard OpenTelemetry traces via OTLP/HTTP
- Maps GenAI fields to OTel `gen_ai.*` semantic conventions
- Resource grouping by `(ProjectID, ServiceName)` per OTLP spec
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
