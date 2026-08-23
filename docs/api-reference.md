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
| `RuntimeService` | `v1/runtime_service.proto` | 10 | Local LLM runtime control (candela) |

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

Controls local LLM runtimes from `candela`. Served via ConnectRPC on the management port (`:8181`).

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

## REST Endpoints (non-Protobuf)

The following endpoints use standard REST conventions and return JSON directly. They do **not** require ConnectRPC framing.

> **Why REST?** The task spend endpoint is designed for lightweight, high-frequency polling (e.g., from CI runners or agent loops). A simple `GET` with no request body is easier to integrate than a ConnectRPC procedure.
>
> Task budget _management_ (create, get, delete) uses ConnectRPC via `UserService` (see above).

### `GET /api/v1/task-spend/{taskID}`

Returns the current spend snapshot for a task budget.

**Authentication:** Required (Bearer token via `Authorization` or `X-Candela-Auth` header).

**Authorization:** Owner + admin + service account.
- The task budget's **owner** can view their own task's spend.
- **Admins** (`role = "admin"`) can view any task's spend.
- **Service accounts** (allowlisted `*.iam.gserviceaccount.com`) can view any task's spend.
- All other callers receive `404 Not Found` (to prevent task ID enumeration).

**Response (200):**

```json
{
  "task_id": "task-abc-123",
  "limit_usd": 10.00,
  "spent_usd": 3.42,
  "remaining_usd": 6.58,
  "expired": false,
  "cached_at": "2025-01-15T12:00:05.123456789Z"
}
```

> **Note:** Responses may be cached for up to 5 seconds. The `cached_at` field indicates when the data was last fetched from Firestore.

**Status Codes:**

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Missing `taskID` in path |
| 401 | Authentication required (no valid token) |
| 404 | Task budget not found, or caller not authorized |
| 405 | Method not allowed (only GET is supported) |
| 500 | Internal server error |

**Example:**

```bash
# Poll task spend (replace TOKEN and TASK_ID)
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8181/api/v1/task-spend/$TASK_ID | jq .
```

### `X-Candela-Job-Id` Header

The `X-Candela-Job-Id` request header connects proxy requests to task budgets for spend tracking.

**Lifecycle:**

```text
1. Client sets X-Candela-Job-Id on proxy request
          │
2. Proxy pre-flight: CheckTaskBudget → enforce limits
          │
3. Proxy forwards to upstream LLM provider
          │
4. Proxy post-response: DeductTaskSpend
          │
5. Client polls GET /api/v1/task-spend/{taskID}
```

- Set this header on LLM proxy requests (e.g., `POST /v1/messages`) to associate spend with a task budget.
- The task budget must be created _before_ sending requests with its ID.
- If the header is set but no matching task budget exists, the proxy returns `402 Payment Required` with error type `task_budget_missing` (fail-closed).
- If the task budget is exhausted or expired, the proxy returns `402` with `task_budget_exhausted` or `task_budget_expired`.

```bash
# Proxy request with task budget tracking
curl -X POST http://localhost:8181/v1/messages \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Candela-Job-Id: task-abc-123" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-20250514","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}'
```

### Per-Model Daily Spend Limits

Candela supports per-model daily spend ceilings to prevent expensive models from consuming the entire budget.

#### Global Limits (YAML Config)

Global limits are enforced pre-flight via the in-memory `SpendTracker`. Set them in `config.yaml`:

```yaml
proxy:
  daily_limits:
    - model: "claude-opus-4"
      max_daily_usd: 50.00
    - model: "gpt-4o"
      max_daily_usd: 25.00
```

- **Model matching:** Uses longest-prefix matching. `"claude-opus-4"` matches `claude-opus-4`, `claude-opus-4-20250514`, etc.
- **Reset:** Counters reset at UTC midnight.
- **Scope:** Per-user — each user gets their own daily counter per model.
- **In-memory:** Counters are tracked in-memory and reset on proxy restart.

#### Per-User Limits (REST API)

Admins can set per-user model limits that override the global YAML config. These are stored in Firestore and persist across restarts. The proxy merges per-user limits with YAML defaults at request time.

> [!TIP]
> Per-user limits are cached for 60 seconds. Changes made via the REST API may take up to 60 seconds to take effect in the proxy.

### `PUT /api/v1/users/{userID}/model-limits/{modelPrefix}`

Create or update a per-user model limit.

**Authentication:** Required. **Authorization:** Admin only.

**Request body:**

```json
{"max_daily_usd": 10.00}
```

**Example:**

```bash
# Cap alice's Claude Opus usage to $10/day
curl -X PUT -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"max_daily_usd": 10.00}' \
  http://localhost:8181/api/v1/users/alice/model-limits/claude-opus-4
```

### `GET /api/v1/users/{userID}/model-limits`

List all per-user model limits.

**Authentication:** Required. **Authorization:** Admin only.

**Response (200):**

```json
{
  "user_id": "alice",
  "limits": [
    {"user_id": "alice", "model_prefix": "claude-opus-4", "max_daily_usd": 10.00, "created_at": "...", "updated_at": "..."},
    {"user_id": "alice", "model_prefix": "gpt-4o", "max_daily_usd": 25.00, "created_at": "...", "updated_at": "..."}
  ]
}
```

### `DELETE /api/v1/users/{userID}/model-limits/{modelPrefix}`

Remove a per-user model limit. The global YAML limit (if any) will apply after deletion.

**Authentication:** Required. **Authorization:** Admin only.

```bash
curl -X DELETE -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8181/api/v1/users/alice/model-limits/claude-opus-4
```

**Status Codes (all model limit endpoints):**

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Missing userID, modelPrefix, or invalid body |
| 401 | Authentication required |
| 403 | Admin access required |
| 405 | Method not allowed |
| 500 | Internal server error |

**Precedence:** Per-user Firestore limit > Global YAML limit > No limit.

---

### Budget Forecast (#719)

#### `GET /api/v1/users/{userID}/budget-forecast`

Returns budget forecast including burn rate, projected end-of-day spend, and
estimated budget exhaustion date based on 7-day historical spend.

**Authentication:** Required. **Authorization:** Self-service (own data) or admin.

**Algorithm:**
- **Burn rate:** today's spend ÷ hours elapsed since midnight UTC
- **Projected EOD:** burn rate × 24 hours
- **Average daily spend:** 7-day rolling average, excluding zero-spend days (weekends/holidays)
- **Exhaustion date:** remaining budget ÷ average daily spend

**Response:**

```json
{
  "burn_rate_usd_per_hour": 1.23,
  "projected_eod_spend_usd": 29.52,
  "will_exceed_budget": true,
  "avg_daily_spend_usd": 18.50,
  "estimated_exhaustion_date": "2026-08-28",
  "days_until_exhaustion": 5,
  "spend_history": [
    {"date": "2026-08-22", "spend_usd": 20.10, "token_count": 142},
    {"date": "2026-08-21", "spend_usd": 16.90, "token_count": 98}
  ]
}
```

When no budget is configured (`limit_usd = 0`), returns only `spend_history` with all
forecast fields zeroed and `days_until_exhaustion = -1`.

> [!TIP]
> Forecast results are cached for 5 minutes per user. Changes in spend may take
> up to 5 minutes to be reflected in the forecast.

| Status | Meaning |
|--------|---------|
| 200 | Forecast returned |
| 401 | Not authenticated |
| 404 | Not found (user accessing another user's data without admin role) |
| 500 | Internal server error |

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
