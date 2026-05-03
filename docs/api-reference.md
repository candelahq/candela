# 📡 Protobuf API Reference

Candela defines all service boundaries in Protobuf. The backend serves these via **ConnectRPC** (HTTP/JSON + gRPC dual-protocol). The UI consumes them via generated TypeScript stubs.

## Services Overview

| Service | Proto File | RPCs | Description |
|---------|-----------|------|-------------|
| `TraceService` | `v1/trace_service.proto` | 3 | Trace queries and detail views |
| `DashboardService` | `v1/dashboard_service.proto` | 4 | Usage analytics, model breakdown, leaderboard |
| `IngestionService` | `v1/ingestion_service.proto` | 1 | OTLP span ingestion |
| `UserService` | `v1/user_service.proto` | 15 | User CRUD, budgets, grants, audit |
| `ProjectService` | `v1/project_service.proto` | 5 | Project + API key management |
| `RuntimeService` | `v1/runtime_service.proto` | 10 | Local LLM runtime control (candela-local) |

## Type Packages

| Package | Proto File | Description |
|---------|-----------|-------------|
| `candela.types` | `types/trace.proto` | Span, Trace, TraceSummary, UsageSummary, ModelUsage |
| `candela.types` | `types/user.proto` | User, UserBudget, BudgetGrant, AuditEntry |
| `candela.types` | `types/project.proto` | Project, APIKey |
| `candela.types` | `types/common.proto` | Shared enums (TimeRange) |
| `candela.types` | `types/bq_span.proto` | BigQuery span schema (server-only) |

---

## TraceService

Provides trace querying for the dashboard and trace detail views.

### ListTraces

List traces with filtering and pagination.

```bash
curl -X POST http://localhost:8181/candela.v1.TraceService/ListTraces \
  -H "Content-Type: application/json" \
  -d '{
    "pageSize": 20,
    "timeRange": "TIME_RANGE_24H",
    "model": "gpt-4o"
  }'
```

**Filters**: `timeRange`, `model`, `provider`, `status`, `search`, `userId`

### GetTrace

Get a single trace with all spans.

```bash
curl -X POST http://localhost:8181/candela.v1.TraceService/GetTrace \
  -H "Content-Type: application/json" \
  -d '{"traceId": "abc123def456..."}'
```

### SearchSpans

Search individual spans across all traces.

```bash
curl -X POST http://localhost:8181/candela.v1.TraceService/SearchSpans \
  -H "Content-Type: application/json" \
  -d '{"nameContains": "openai.chat", "pageSize": 50}'
```

---

## DashboardService

Provides aggregated analytics for the dashboard.

### GetUsageSummary

Returns total traces, tokens, cost, avg latency, and error rate.

```bash
curl -X POST http://localhost:8181/candela.v1.DashboardService/GetUsageSummary \
  -H "Content-Type: application/json" \
  -d '{"timeRange": "TIME_RANGE_7D"}'
```

### GetModelBreakdown

Returns per-model usage metrics (calls, tokens, cost, latency).

```bash
curl -X POST http://localhost:8181/candela.v1.DashboardService/GetModelBreakdown \
  -H "Content-Type: application/json" \
  -d '{"timeRange": "TIME_RANGE_24H"}'
```

### GetDashboardSummary

Combined endpoint returning summary + time-series data + recent traces.

```bash
curl -X POST http://localhost:8181/candela.v1.DashboardService/GetDashboardSummary \
  -H "Content-Type: application/json" \
  -d '{"timeRange": "TIME_RANGE_24H"}'
```

### GetUserLeaderboard

Per-user usage ranking (admin only, team mode).

```bash
curl -X POST http://localhost:8181/candela.v1.DashboardService/GetUserLeaderboard \
  -H "Content-Type: application/json" \
  -d '{"timeRange": "TIME_RANGE_7D", "limit": 10}'
```

---

## IngestionService

Receives OTLP trace spans.

### IngestSpans

```bash
curl -X POST http://localhost:8181/candela.v1.IngestionService/IngestSpans \
  -H "Content-Type: application/json" \
  -d '{
    "spans": [{
      "spanId": "abc123",
      "traceId": "def456",
      "name": "openai.chat",
      "kind": "SPAN_KIND_LLM",
      "startTime": "2026-04-20T15:00:00Z",
      "endTime": "2026-04-20T15:00:02Z",
      "genAi": {
        "model": "gpt-4o",
        "provider": "openai",
        "inputTokens": 150,
        "outputTokens": 42
      }
    }]
  }'
```

---

## UserService

Full user lifecycle management with RBAC and budgets.

### Self-Service RPCs (any authenticated user)

#### GetCurrentUser
Returns the caller's own profile, budget, and active grants.

```bash
curl -X POST http://localhost:8181/candela.v1.UserService/GetCurrentUser \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{}'
```

#### GetMyBudget
Returns the caller's own budget and spending.

```bash
curl -X POST http://localhost:8181/candela.v1.UserService/GetMyBudget \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{}'
```

### Admin RPCs (requires `admin` role)

| RPC | Description | Key Fields |
|-----|-------------|------------|
| `CreateUser` | Pre-provision a user | `email` (required, validated), `role`, `monthlyBudgetUsd` |
| `ListUsers` | Paginated user list | `statusFilter`, `limit`, `offset` |
| `GetUser` | Get user by ID | `userId` (required, min_len: 1) |
| `UpdateUser` | Update mutable fields | `userId`, `displayName`, `role` |
| `DeactivateUser` | Set status to inactive | `userId` |
| `ReactivateUser` | Set status to active | `userId` |
| `SetBudget` | Create/update monthly budget | `userId`, `limitUsd` (> 0) |
| `GetBudget` | Get user's budget | `userId` |
| `ResetSpend` | Zero current-period spending | `userId` |
| `CreateGrant` | Issue one-time bonus | `userId`, `amountUsd` (> 0), `reason`, `startsAt`, `expiresAt` |
| `ListGrants` | List user's grants | `userId`, `activeOnly` |
| `RevokeGrant` | Cancel active grant | `userId`, `grantId` |
| `ListAuditLog` | View admin action log | `userId`, `limit` (0–500) |

### Validation Rules

All requests are validated server-side via `protovalidate`:

| Field | Rule |
|-------|------|
| `email` | Must be valid email format |
| `monthlyBudgetUsd` | ≥ 0 |
| `limitUsd` (SetBudget) | > 0 |
| `amountUsd` (CreateGrant) | > 0 |
| `userId`, `id` | min_len: 1 (required) |
| `limit` (ListAuditLog) | 0 ≤ x ≤ 500 |
| `expiresAt` vs `startsAt` | CEL: expires > starts |

---

## ProjectService

Manages projects and API keys for multi-tenant span isolation.

| RPC | Description |
|-----|-------------|
| `CreateProject` | Create a new project |
| `ListProjects` | List all projects (paginated) |
| `GetProject` | Get project by ID |
| `UpdateProject` | Update name, description, environment |
| `DeleteProject` | Delete project and its API keys |

API key management:

| RPC | Description |
|-----|-------------|
| `CreateAPIKey` | Generate API key for a project (hash stored, full key returned once) |
| `ListAPIKeys` | List keys with prefix + status (never the full key) |
| `RevokeAPIKey` | Deactivate a key |

---

## RuntimeService

Controls local LLM runtimes from `candela-local`. Served via ConnectRPC on the management port (`:8181`).

| RPC | Description |
|-----|-------------|
| `GetHealth` | Runtime status (running/stopped/error), uptime, version |
| `StartRuntime` | Start the configured runtime (Ollama/vLLM/LM Studio) |
| `StopRuntime` | Stop the runtime |
| `ListModels` | List loaded models with size, family, quantization |
| `LoadModel` | Load a model into memory |
| `UnloadModel` | Unload a model from memory |
| `DeleteModel` | Delete model from disk |
| `PullModel` | Download a model (streaming progress) |
| `CancelPull` | Cancel an in-progress download |
| `ListBackends` | Auto-detect installed runtimes with install hints |

---

## Interacting with gRPC

Since Candela uses ConnectRPC (which supports both HTTP/JSON and gRPC), you can also use `grpcurl`:

```bash
# List services
grpcurl -plaintext localhost:8181 list

# List RPCs
grpcurl -plaintext localhost:8181 list candela.v1.TraceService

# Call an RPC
grpcurl -plaintext -d '{}' localhost:8181 candela.v1.DashboardService/GetUsageSummary
```

---

## Proto Generation

Proto definitions live in [`candelahq/candela-protos`](https://github.com/candelahq/candela-protos)
and are published to BSR as [`buf.build/candelahq/protos`](https://buf.build/candelahq/protos).

```bash
# Generate Go + TypeScript stubs from BSR
nix develop -c buf generate
```

Output:
- Go stubs → `gen/go/candela/`
- TypeScript stubs → `ui/src/gen/`
- BigQuery schemas → `gen/bq/candela/`
