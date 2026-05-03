# 🤖 Google ADK Integration

Route Google ADK agent LLM calls through Candela for instant observability — tokens, cost, latency, and budget enforcement — with unified trace correlation.

## Quick Start (Proxy Only)

Point ADK's built-in `Gemini` model at candela-sidecar using `base_url`:

```python
from google.adk.agents import Agent
from google.adk.models import Gemini

root_agent = Agent(
    model=Gemini(
        model="gemini-2.0-flash",
        base_url="http://localhost:8080/proxy/google",  # → candela-sidecar
    ),
    name="my_agent",
    instruction="You are a helpful assistant.",
)
```

That's it. The sidecar handles auth (ADC), token counting, cost calculation, and span export. No extra dependencies.

> **💡 Why does this work?** ADK's `Gemini` class passes `base_url` directly to `google.genai.Client(http_options=HttpOptions(base_url=...))`. The sidecar's `/proxy/google/` route speaks **native Gemini API format** — no translation layer needed.

### What You Get

| Metric | Source |
|--------|--------|
| Token usage (input/output/total) | Parsed from Gemini response |
| Cost (USD) | Calculated by Candela's cost engine |
| Latency & TTFB | Measured at the proxy |
| Budget enforcement | Per-user budget gating |
| Request/response content | Captured for debugging |

---

## Full Observability (Proxy + OTel)

For deep agent visibility — multi-span DAGs, tool calls, orchestration flow — combine the proxy with ADK's native OpenTelemetry instrumentation.

### Architecture

```
┌─────────────────────────────┐
│       ADK Agent             │
│  ┌───────────────────────┐  │
│  │ OTel SDK              │  │─── OTLP/HTTP ──→ Candela Collector (:4318)
│  │ (agent, tool spans)   │  │                  │ GenAI Processor
│  └───────────────────────┘  │                  │ (cost enrichment)
│  ┌───────────────────────┐  │                  ▼
│  │ httpx (instrumented)  │  │          Candela Backend
│  │  ↓ traceparent header │  │
│  └───────┬───────────────┘  │
└──────────┼──────────────────┘
           │ HTTP + traceparent
           ▼
┌─────────────────────────────┐
│   candela-sidecar (:8080)   │
│  ┌───────────────────────┐  │
│  │ Proxy span            │  │─── OTLP/Pub/Sub ──→ Candela Backend
│  │ (tokens, cost, TTFB)  │  │
│  │ parent = caller's     │  │
│  │ span from traceparent │  │
│  └───────────────────────┘  │
│           │                 │
└───────────┼─────────────────┘
            │ forwarded traceparent
            ▼
    Google Gemini API
```

### Trace Correlation

When ADK's `httpx` client sends an LLM request, the OTel instrumentation automatically injects a `traceparent` header. The sidecar extracts this header and uses the caller's trace ID — so the proxy span becomes a **child** of the ADK agent span:

```
Trace: 4bf92f3577b34da6a3ce929d0e0e4736
├─ invoke_agent (ADK)
│  ├─ call_llm (ADK)
│  │  ├─ generate_content (ADK)
│  │  │  └─ google.chat (candela-sidecar)     ← nested via traceparent
│  │  │     tokens: 150 in / 42 out
│  │  │     cost: $0.0023
│  │  │     ttfb: 234ms
```

Without trace correlation, the proxy spans would have a separate, unrelated trace ID.

### Setup

```python
"""Full observability: proxy + OTel unified traces."""
import os

# ADK reads these natively — no manual TracerProvider setup needed.
os.environ.setdefault("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://localhost:4318/v1/traces")
os.environ.setdefault("OTEL_SERVICE_NAME", "my-adk-agent")
os.environ.setdefault("OTEL_SEMCONV_STABILITY_OPT_IN", "gen_ai_latest_experimental")
os.environ.setdefault("OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT", "EVENT_ONLY")

from google.adk.agents import Agent
from google.adk.models import Gemini

SIDECAR = os.environ.get("CANDELA_SIDECAR_URL", "http://localhost:8080/proxy/google")

root_agent = Agent(
    model=Gemini(model="gemini-2.0-flash", base_url=SIDECAR),
    name="my_agent",
    instruction="You are a helpful assistant.",
)
```

**Extra dependency** — `opentelemetry-instrumentation-httpx` enables automatic `traceparent` injection on ADK's outgoing HTTP calls:

```bash
pip install opentelemetry-instrumentation-httpx
```

> **⚠️ Initialization order matters**: The httpx instrumentor must be active before ADK creates its httpx client. Setting the `OTEL_*` env vars before importing ADK ensures `maybe_set_otel_providers()` configures everything correctly.

---

## Running the Full Stack

See [`examples/adk-agent/launch.sh`](../examples/adk-agent/launch.sh) for a ready-to-run script. The components:

| Component | Port | Role |
|-----------|------|------|
| `candela-sidecar` | 8080 | LLM proxy (tokens, cost, budget, trace correlation) |
| Candela Collector | 4317 (gRPC) / 4318 (HTTP) | OTel span ingestion + GenAI cost enrichment |
| `adk web` | 8000 | ADK dev server |

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CANDELA_SIDECAR_URL` | `http://localhost:8080/proxy/google` | Sidecar proxy endpoint |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | — | Collector OTLP HTTP endpoint |
| `OTEL_SERVICE_NAME` | — | Service name for OTel spans |
| `OTEL_SEMCONV_STABILITY_OPT_IN` | — | Set to `gen_ai_latest_experimental` for GenAI conventions |

---

## Sidecar vs. Server

Both `candela-sidecar` and `candela-server` support the same proxy routes. Choose based on your environment:

| | `candela-sidecar` | `candela-server` |
|-|-------------------|------------------|
| **Best for** | Containers, CI, production | Local dev with UI |
| **Size** | <5MB binary | Full server |
| **Storage** | Pub/Sub + OTLP export | DuckDB, SQLite, BigQuery |
| **Budget enforcement** | Yes (via Firestore) | Yes (via local store) |
| **UI** | None | Dashboard at `:3000` |
| **ADK `base_url`** | `http://localhost:8080/proxy/google` | `http://localhost:8181/proxy/google` |

> **📍 Port difference**: Sidecar defaults to `:8080`, server defaults to `:8181`. Update `base_url` accordingly.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Agent works but no proxy spans | `base_url` not set or wrong port | Verify with `curl http://localhost:8080/healthz` |
| Proxy spans have separate trace ID | `traceparent` not propagated | Install `opentelemetry-instrumentation-httpx` and set OTel env vars |
| `401 Unauthorized` from sidecar | Missing ADC credentials | Run `gcloud auth application-default login` |
| Cost shows $0.00 | Unknown model in pricing table | Check collector logs for `⚠️ missing pricing` |
| ADK import error for `Gemini` | Old ADK version | Upgrade: `pip install --upgrade google-adk` |

---

## Related Docs

- [LLM Proxy](proxy.md) — All proxy routes, providers, and client integrations
- [OTel Collector](otel-collector.md) — Collector setup, GenAI processor, and framework instrumentation
- [Architecture](architecture.md) — CQRS storage, fan-out, and ingestion pipeline
