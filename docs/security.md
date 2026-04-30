# 🔐 Security & Authentication

Candela uses a **multi-strategy authentication** system designed to securely serve three distinct client types: browser users, developer CLI tools, and service accounts. This document covers the authentication architecture, authorization model, and security considerations.

## Authentication Architecture

```mermaid
flowchart TD
    REQ[Incoming Request] --> SKIP{Path = /healthz?}
    SKIP -->|yes| PASS[Pass through]
    SKIP -->|no| DEV{Dev Mode?}
    DEV -->|yes| SYNTH["Inject synthetic admin\n(admin@localhost)"]
    DEV -->|no| TOK{Has Bearer Token?}
    TOK -->|no| DENY["401 Unauthorized"]
    TOK -->|yes| S1["Strategy 1:\nFirebase ID Token"]
    S1 -->|valid| AUTH[Authenticated ✓]
    S1 -->|invalid| S2["Strategy 2:\nGoogle ID Token"]
    S2 -->|valid| AUTH
    S2 -->|invalid| S3["Strategy 3:\nOAuth2 Access Token\n(via userinfo API)"]
    S3 -->|valid| AUTH
    S3 -->|invalid| DENY
    AUTH --> CTX["User injected into\nrequest context"]
    CTX --> HANDLER[ConnectRPC Handler]
```

### Strategy Waterfall

The middleware (`pkg/auth/firebase.go`) tries three strategies in sequence. The first successful validation wins:

| # | Strategy | Client | Token Source | Validation Method |
|---|----------|--------|-------------|-------------------|
| 1 | **Firebase ID Token** | Browser UI | Firebase JS SDK (`onAuthStateChanged`) | `fbAuth.VerifyIDToken()` |
| 2 | **Google ID Token** | Service accounts, `candela-local` with `idtoken` | `idtoken.NewTokenSource(audience)` | `idtoken.Validate(token, audience)` |
| 3 | **OAuth2 Access Token** | `candela-local` with user ADC | `gcloud auth application-default login` | `googleapis.com/oauth2/v3/userinfo` |

> [!NOTE]
> Strategy 3 makes an HTTP call to Google's userinfo endpoint on every request. This adds ~50ms latency but is the only way to validate user-scoped Application Default Credentials (ADC) that `candela-local` uses when `gcloud auth application-default login` provides an access token rather than an ID token.

### Auth Bypass

These paths skip authentication entirely:
- `/healthz` — health check endpoint (Cloud Run readiness probe)

---

## Authorization — Role-Based Access Control (RBAC)

### Roles

| Role | Enum | Description |
|------|------|-------------|
| `developer` | `USER_ROLE_DEVELOPER = 1` | Use proxy, view own traces/costs, self-service RPCs |
| `admin` | `USER_ROLE_ADMIN = 2` | Full access: manage users, budgets, view all data |

### Admin Guard Interceptor

The `AdminInterceptor` (`pkg/auth/admin.go`) is a ConnectRPC unary interceptor that enforces admin-only access:

```
Request → Auth Middleware → Admin Interceptor → Handler
                               │
                               ├── Self-service RPC? → Pass through
                               └── Admin RPC? → Look up user role
                                                   ├── admin → Pass through
                                                   └── !admin → 403 PermissionDenied
```

### RPC Access Matrix

#### Self-Service RPCs (any authenticated user)
| RPC | Description |
|-----|-------------|
| `GetCurrentUser` | Returns the caller's own profile, budget, and active grants |
| `GetMyBudget` | Returns the caller's budget and current-period spending |

#### Admin-Only RPCs (13 RPCs)
| Category | RPCs |
|----------|------|
| **Users** | `CreateUser`, `ListUsers`, `GetUser`, `UpdateUser`, `DeactivateUser`, `ReactivateUser` |
| **Budgets** | `SetBudget`, `GetBudget`, `ResetSpend` |
| **Grants** | `CreateGrant`, `ListGrants`, `RevokeGrant` |
| **Audit** | `ListAuditLog` |

#### Unguarded Services (Data-Level Scoping)
| Service | Scoping Method |
|---------|----------------|
| `TraceService` | `scopeUserID()` injects user filter into storage queries; `GetTrace` uses post-fetch auth gate |
| `DashboardService` | `scopeUserID()` injects user filter into usage/model queries |
| `IngestionService` | Write-only — validated by project API key, not user identity |
| `ProjectService` | Currently unguarded (future: project-level RBAC) |

---

## Data Isolation — Per-User Trace Scoping

Non-admin developers can only see their own traces and spans. This is enforced at two levels:

### Query-Based Endpoints (ListTraces, SearchSpans, Dashboard)

The `scopeUserID()` helper (`pkg/connecthandlers/scope.go`) determines the caller's identity:
- **Admin** → returns `""` (empty string = no filter, sees all data)
- **Developer** → returns sanitized email (e.g., `alice@example.com`)

This value is injected into `TraceQuery.UserID`, `SpanQuery.UserID`, or `UsageQuery.UserID`. All storage backends (BigQuery, DuckDB, SQLite) apply the filter in SQL:

```sql
AND (? = '' OR user_id = ?)
```

### GetTrace (Direct Access by Trace ID)

`GetTrace` cannot pre-filter because it queries by `trace_id`, not user. Instead, it uses a **post-fetch authorization gate**:

1. Fetch the full trace from storage
2. Extract the trace owner via `traceUserID()` — checks root span first, then falls back to any span with `user_id`
3. Compare against `scopeUserID(ctx)`
4. If mismatch → `PermissionDenied`
5. If no `user_id` on any span (legacy data) → allow access, log for backfill visibility

### Error Sanitization

All handlers use `internalError()` (`pkg/connecthandlers/errors.go`) for storage failures. This logs the real error server-side via `slog.Error` and returns a generic `"internal error"` to clients, preventing leakage of SQL errors, file paths, or infrastructure details.

---

## User Identity & Context Propagation

### User Struct

```go
// pkg/auth/context.go
type User struct {
    ID    string // Firebase UID or Google subject (`sub` claim)
    Email string // Verified email claim (lowercased)
}
```

### Context Flow

The authenticated user is attached to the request context and available throughout the handler stack:

```go
// In any handler:
user := auth.FromContext(ctx)        // *auth.User or nil
userID := auth.IDFromContext(ctx)    // string (empty if no user)
email := auth.EmailFromContext(ctx)  // string (empty if no user)
```

The proxy uses context-propagated user identity for:
- **Per-user span attribution** — `span.UserID = caller.ID`
- **Budget deduction** — `users.DeductSpend(ctx, span.UserID, cost, tokens)`
- **User-scoped trace queries** — developers only see their own traces

---

## Dev Mode

When `CANDELA_DEV_MODE=true` or `auth.dev_mode: true` in config:

- **No token validation** — all requests succeed
- **Synthetic admin user** injected: `{ID: "dev-admin", Email: "admin@localhost"}`
- All admin RPCs are accessible
- All traces are visible (no user scoping)

> [!WARNING]
> Never run dev mode in production. There is no authentication bypass — all requests get full admin access.

---

## `candela-local` Authentication

### Solo Mode
No authentication needed. All requests to `:1234` and `:8181` are unauthenticated.

### Solo + Cloud Mode
Uses **Application Default Credentials (ADC)** to call Vertex AI directly:
```bash
gcloud auth application-default login
```
No server-side auth needed — ADC tokens are used for upstream cloud calls only.

### Team Mode
`candela-local` injects OIDC tokens into requests to the Candela Cloud Run server:

```
IDE → candela-local (:1234)
         │
         ├── Local model → Ollama (no auth)
         │
         └── Cloud model → Candela Server (Cloud Run)
                              │
              ┌───────────────┘
              │ Authorization: Bearer <google-id-token>
              ▼
         Auth Middleware (Strategy 2 or 3)
```

Token acquisition flow:
1. `candela-local` reads `~/.candela.yaml` for `remote` and `audience`
2. Uses `google.DefaultTokenSource()` to get an ADC token
3. Injects `Authorization: Bearer <token>` on every proxied request

---

## IAP Authentication (Legacy)

The `IAPMiddleware` (`pkg/auth/iap.go`) validates Cloud IAP JWT assertions from the `x-goog-iap-jwt-assertion` header. This is the **original** auth path when Candela ran behind Cloud IAP.

The current production setup uses `FirebaseAuthMiddleware` instead, which provides more flexible multi-strategy auth. The IAP middleware remains available for deployments behind Cloud IAP.

---

## Input Validation

All `UserService` requests are validated server-side using [`protovalidate`](https://github.com/bufbuild/protovalidate). The validation interceptor runs **before** the admin guard:

```
Request → protovalidate → AdminInterceptor → Handler
```

See [docs/user-management.md](user-management.md) for the full validation rule reference.

---

## Security Hardening Checklist

| Item | Status | Notes |
|------|--------|-------|
| Token validation on all non-health endpoints | ✅ | 3-strategy waterfall |
| Email claim normalization (lowercase) | ✅ | Prevents case-based identity bypass |
| Admin role enforcement via ConnectRPC interceptor | ✅ | Per-RPC ACL |
| Per-user trace/span data isolation | ✅ | `scopeUserID()` + `traceUserID()` auth gate |
| Internal error message sanitization | ✅ | `internalError()` logs server-side, returns generic message |
| Rate limiting per user | ✅ | `CheckRateLimit()` in `UserStore` |
| Budget enforcement before proxy calls | ✅ | `CheckBudget()` pre-flight |
| Secrets not baked into container images | ✅ | `entrypoint.sh` generates config from env vars |
| ADC token auto-refresh | ✅ | `oauth2.TokenSource` handles refresh |
| API key hashing (bcrypt) | ✅ | `APIKey.KeyHash` never exposed |
| Proxy does not store upstream API keys | ✅ | Forwarded transparently |
| CORS origin allowlist | ✅ | Configurable, defaults to localhost |
| Firebase authorized domains | ⚠️ | Must be configured in Firebase Console |
| HTTPS in production | ⚠️ | Handled by Cloud Run / load balancer |
| Audit logging for admin actions | ✅ | Firestore `audit_log` collection |

---

## Implementation Files

| File | Purpose |
|------|---------|
| `pkg/auth/context.go` | `User` struct, context get/set helpers |
| `pkg/auth/firebase.go` | `FirebaseAuthMiddleware` — 3-strategy waterfall |
| `pkg/auth/iap.go` | `IAPMiddleware` — Cloud IAP JWT validation (legacy) |
| `pkg/auth/admin.go` | `AdminInterceptor` — ConnectRPC admin guard |
| `pkg/connecthandlers/scope.go` | `scopeUserID()` — per-user data scoping helper |
| `pkg/connecthandlers/errors.go` | `internalError()` — sanitized error responses |
| `cmd/candela-server/main.go` | Middleware wiring, Firebase init, dev mode |
| `cmd/candela-local/main.go` | ADC token injection for Team Mode |
