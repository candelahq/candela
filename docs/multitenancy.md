# Multitenancy: Per-Tenant LLM Cost Attribution

Candela supports **first-class multitenant observability**: every LLM API call can
be attributed to a downstream customer (tenant), enabling you to answer:

> *"How much did we spend on LLM calls for Acme Corp this month?"*

This is accomplished by propagating a `tenant_id` through the proxy — either via
[W3C Baggage](https://www.w3.org/TR/baggage/) (preferred for ADK/OTel) or via an
explicit HTTP header.

---

## How It Works

```
  Your App (ADK agent / FastAPI / etc.)
       │
       │  Baggage: candela.tenant_id=acme-corp          ← preferred (propagates through hops)
       │  X-Candela-Tenant-Id: acme-corp                ← fallback (single-hop only)
       ▼
  Candela Proxy  ──────────────────────────────────────────────────────────
       │  Extracts tenant_id → validates → attaches to Span
       ▼
  Storage (DuckDB / SQLite / BigQuery)
       │  tenant_id column indexed for efficient GROUP BY queries
       ▼
  GetTenantLeaderboard RPC  →  Dashboard / Billing system
```

---

## Propagation Methods

### Option 1: W3C Baggage (Recommended for ADK)

The W3C Baggage header carries key-value pairs through distributed traces. ADK's
built-in OpenTelemetry propagator injects this header automatically — so if you
set `candela.tenant_id` in the baggage context at the *start* of an agent run,
it flows through every downstream LLM call automatically, even in multi-hop traces.

**Baggage takes precedence over the explicit header** (see precedence rules below).

```
Baggage: candela.tenant_id=acme-corp,svc.version=1.2.0
```

The proxy extracts **only** `candela.tenant_id` from baggage. All other baggage
entries are forwarded unchanged.

### Option 2: Explicit Header

For non-OTel callers (cURL, direct API clients, service-to-service without OTel):

```
X-Candela-Tenant-Id: acme-corp
```

If both Baggage and header are present, **Baggage wins**.

---

## Tenant ID Format

Tenant IDs must match the pattern `[a-zA-Z0-9\-._]{1,128}`. Examples:

| ✅ Valid             | ❌ Invalid                  |
|---------------------|-----------------------------|
| `acme-corp`         | `acme corp` (space)         |
| `acme.corp`         | `acme@corp` (@ sign)        |
| `tenant_42`         | `` (empty string)           |
| `trial-NCT01750580` | 129+ characters             |
| `customer.a.b.c`   | `../escape` (path traversal)|

Invalid values are **silently discarded** (not rejected with 4xx) to avoid
breaking clients that send malformed headers. The span is written without a
`tenant_id` in this case.

---

## ADK Integration

### Python: Setting Tenant Context

```python
from opentelemetry import baggage, context
from opentelemetry.propagate import inject
import httpx

def make_llm_request_with_tenant(tenant_id: str, payload: dict) -> dict:
    """Make an LLM request through Candela with tenant attribution."""
    # Set tenant_id in OTel baggage context.
    ctx = baggage.set_baggage("candela.tenant_id", tenant_id)

    # Inject baggage into outgoing headers.
    headers = {}
    inject(headers, context=ctx)
    # headers now contains: {"Baggage": "candela.tenant_id=acme-corp"}

    with httpx.Client() as client:
        resp = client.post(
            "http://candela-proxy/proxy/openai/v1/chat/completions",
            headers=headers,
            json=payload,
        )
        resp.raise_for_status()
        return resp.json()
```

### ADK Agent: Automatic Context Propagation

With the `CandlaContextPlugin` (see `docs/adk-integration.md`), tenant context
is injected automatically into every LLM call made by an ADK agent:

```python
from google.adk.agents import LlmAgent
from candela.adk import CandleaContextPlugin, CandelaContext

agent = LlmAgent(
    name="my-agent",
    model="gemini-2.0-flash",
    # ... tools, system_prompt, etc.
)

ctx = CandelaContext(tenant_id="acme-corp", session_id="session-42")
plugin = CandleaContextPlugin(ctx)

# The plugin injects Baggage headers on every underlying HTTP call.
# No code changes needed in the agent itself.
result = await agent.run_async(message, plugins=[plugin])
```

### Multi-Hop Traces: Why Baggage Matters

Consider a CTMS pipeline:

```
FastAPI handler
  └─ Orchestrator agent (Gemini call ①)  ← tenant_id set here
       └─ Tool: query_clinical_data
            └─ Sub-agent (Gemini call ②)  ← tenant_id auto-propagated via Baggage
                 └─ Tool: find_evidence
                      └─ Sub-agent (Gemini call ③)  ← tenant_id still here
```

With **Baggage**, `tenant_id=acme-corp` is set once and propagates through all
three Gemini calls automatically. With just the header, you'd have to manually
re-attach it at every hop.

### FunnelRequest Pattern (CTMS)

For the `intelligence-service`, you likely want to propagate **both** `tenant_id`
and `trial_id` to attribute costs to specific trials:

```python
from opentelemetry import baggage, context
from opentelemetry.propagate import inject

def make_funnel_baggage(tenant_id: str, trial_id: str) -> dict:
    """Build baggage headers for a funnel run."""
    ctx = context.get_current()
    ctx = baggage.set_baggage("candela.tenant_id", tenant_id, context=ctx)
    ctx = baggage.set_baggage("candela.job_id", trial_id, context=ctx)
    headers = {}
    inject(headers, context=ctx)
    return headers

# Example output:
# {"Baggage": "candela.tenant_id=acme-health,candela.job_id=NCT01750580"}
```

> **Note:** `candela.job_id` support is planned — see the roadmap section below.

---

## Querying Tenant Data

### Connect RPC: GetTenantLeaderboard

```python
import httpx

resp = httpx.post(
    "http://candela-server/candela.v1.DashboardService/GetTenantLeaderboard",
    headers={"Authorization": f"Bearer {admin_token}"},
    json={
        "project_id": "my-project",
        "limit": 10,
        "time_range": {
            "start": "2025-01-01T00:00:00Z",
            "end": "2025-02-01T00:00:00Z",
        }
    }
)

for tenant in resp.json()["tenants"]:
    print(f"{tenant['tenant_id']}: ${tenant['cost_usd']:.4f} ({tenant['call_count']} calls)")
```

Example output:
```
acme-corp:        $12.45 (342 calls)
trial-NCT01234:   $7.23  (198 calls)
acme-health-demo: $3.11  (89 calls)
```

This endpoint is **admin-only** (requires admin Firebase token).

### Direct SQL (DuckDB / BigQuery)

```sql
-- Per-tenant cost breakdown for the last 30 days
SELECT
    COALESCE(tenant_id, '(unattributed)') AS tenant,
    COUNT(*)                               AS calls,
    SUM(gen_ai_input_tokens)               AS input_tokens,
    SUM(gen_ai_output_tokens)              AS output_tokens,
    SUM(gen_ai_cost_usd)                   AS cost_usd
FROM spans
WHERE
    project_id = 'my-project'
    AND start_time >= NOW() - INTERVAL 30 DAY
GROUP BY 1
ORDER BY cost_usd DESC;
```

---

## OTLP Export

When the OTLP sink is enabled, `tenant_id` is exported as the `candela.tenant_id`
OTLP attribute on every span. This surfaces automatically in:

- **Datadog**: filterable via `@candela.tenant_id` in Log Explorer
- **Honeycomb**: group-by `candela.tenant_id` in query builder
- **Grafana / Tempo**: filter traces by `candela.tenant_id` attribute

---

## Schema Details

The `tenant_id` column is added via idempotent `ALTER TABLE` migration:

| Backend  | Migration strategy            | Index          |
|----------|-------------------------------|----------------|
| DuckDB   | `ALTER TABLE ... ADD COLUMN`  | Composite with `project_id, start_time` |
| SQLite   | `ALTER TABLE ... ADD COLUMN`  | Separate `idx_spans_tenant_id` |
| BigQuery | Schema update via Go client   | Clustering field |

The migration runs automatically on startup and is safe to run multiple times.

---

## Roadmap

| Feature | Status |
|---------|--------|
| `tenant_id` via Baggage + header | ✅ Implemented |
| `tenant_id` in DuckDB / SQLite / BigQuery | ✅ Implemented |
| `GetTenantLeaderboard` RPC | ✅ Implemented |
| OTLP export of `candela.tenant_id` | ✅ Implemented |
| ADK `CandleaContextPlugin` | ✅ Implemented |
| `job_id` / `trial_id` second attribution dimension | 🔜 Planned |
| Rust sidecar parity for tenant extraction | 🔜 Planned (after HTTP handler is wired) |
| UI: Tenant leaderboard dashboard widget | 🔜 Planned (see GitHub issues) |
| Per-tenant budget enforcement | 💡 Future |

---

## Security Considerations

- `tenant_id` is **set by the calling application**, not by Candela's auth system.
  It is an attribution label, not an access control boundary.
- Candela validates format but cannot verify that the caller is authorized to
  bill costs to that tenant. Enforce tenant authorization in your application layer.
- The `GetTenantLeaderboard` API is admin-only and requires a verified Firebase
  admin token.
- Tenant IDs are validated against `[a-zA-Z0-9\-._]{1,128}` to prevent log
  injection and other attacks.
