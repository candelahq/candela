# Why Candela? — Comparison with Alternatives

> **Last updated:** August 2026

Candela is an **open-source LLM observability and cost management proxy**. This page honestly compares it to popular alternatives so you can decide what fits your stack.

## Quick Comparison

| Capability | Candela | Helicone | LangSmith | Portkey | LiteLLM |
|:---|:---:|:---:|:---:|:---:|:---:|
| **Open source** | ✅ AGPL-3.0 | ✅ Apache-2.0 | ❌ | ✅ Apache-2.0 (gateway) | ✅ Apache-2.0 |
| **Self-hosted** | ✅ | ✅ | Enterprise only | ✅ | ✅ |
| **Transparent proxy** | ✅ | ✅ | ❌ (SDK) | ✅ | ✅ |
| **Per-user budgets** | ✅ Real-time | ❌ | ❌ | ✅ | ❌ |
| **Multi-tenant** | ✅ Native | ❌ | ✅ | ✅ | ❌ |
| **Cost tracking** | ✅ Automatic | ✅ | ✅ | ✅ | ✅ |
| **Trace visualization** | ✅ Waterfall + DAG | ✅ | ✅ Best-in-class | ✅ | ❌ |
| **Prompt playground** | ❌ | ✅ | ✅ | ✅ | ❌ |
| **Eval framework** | ❌ | ❌ | ✅ | ❌ | ❌ |
| **Provider fallback** | ✅ | ❌ | N/A | ✅ | ✅ |
| **Format translation** | ✅ OpenAI ↔ Anthropic ↔ Gemini | ❌ | N/A | ✅ | ✅ |
| **OIDC / SSO auth** | ✅ Generic chain | ❌ | ✅ | ✅ | ❌ |
| **Deploy complexity** | Single binary + BQ | Docker + ClickHouse | SaaS or K8s (Enterprise) | Docker or K8s | Docker + Postgres |

## When to choose Candela

**Candela is ideal when you need:**

- **Real-time per-user budget enforcement** — Candela is the only proxy that blocks requests in real-time when a user hits their daily budget. Others track costs post-hoc.
- **Zero-integration observability** — Point your `OPENAI_BASE_URL` at Candela and every call is traced. No SDK changes, no decorator imports, no code changes.
- **Multi-provider translation** — Write OpenAI-format code, deploy against Anthropic or Gemini. Candela translates request/response formats bidirectionally.
- **Enterprise multi-tenancy** — Isolate teams, projects, or customers with native tenant scoping and separate budget pools.
- **Self-hosted on GCP** — Single Cloud Run service + BigQuery for storage. No ClickHouse, no Postgres, no Redis to operate.

## When to choose alternatives

**We're honest about where others excel:**

### Helicone
Choose Helicone if you want a **polished SaaS experience** with minimal setup. Their dashboard UX is more mature, and they offer features like prompt caching analytics and A/B testing that Candela doesn't have yet. **Note:** Helicone was acquired by Mintlify in early 2026 and is currently in maintenance mode (security patches only, no new features).

### LangSmith
Choose LangSmith if you're **deep in the LangChain ecosystem** and need tight integration with LangGraph, evaluations, and datasets. LangSmith's eval framework and annotation queue are best-in-class. Trade-off: self-hosting requires an Enterprise license, and integration is SDK-only (no proxy mode).

### Portkey
Choose Portkey if you need a **managed AI gateway** with enterprise support, semantic caching, and guardrails. Portkey's gateway features (load balancing, caching, guardrails) are more mature than Candela's proxy.

### LiteLLM
Choose LiteLLM if you need the **widest provider coverage** (100+ LLM APIs) and prefer Python-native tooling. LiteLLM is excellent as a unified API layer but lacks observability depth and budget enforcement.

## Architecture differences

```
┌─────────────────────────────────────────────────────┐
│                   Your Application                  │
│            OPENAI_BASE_URL=candela:8181              │
└────────────────────────┬────────────────────────────┘
                         │
                    ┌────▼────┐
                    │ Candela │  ← Budget check, auth, trace
                    │  Proxy  │     Format translation
                    └────┬────┘
                         │
            ┌────────────┼────────────┐
            │            │            │
       ┌────▼───┐  ┌─────▼────┐ ┌────▼─────┐
       │ OpenAI │  │Anthropic │ │  Google   │
       └────────┘  └──────────┘ └──────────┘
```

Unlike SDK-based tools (LangSmith), Candela operates as a **transparent proxy** — your code doesn't know it's there. This means:

- **Zero lock-in**: Remove Candela by changing one environment variable
- **Language-agnostic**: Works with Python, Go, Rust, TypeScript, curl — anything that speaks HTTP
- **Framework-agnostic**: Works with LangChain, CrewAI, AutoGen, raw API calls, or your custom code

## Data sovereignty

Candela stores traces in **your BigQuery dataset** in your GCP project. No data leaves your infrastructure. Compare:

| Tool | Data location |
|:---|:---|
| Candela | Your GCP project (BigQuery) |
| Helicone | Helicone's cloud (or self-hosted ClickHouse) |
| LangSmith | LangChain's cloud (US/EU/APAC) or self-hosted (Enterprise) |
| Portkey | Portkey's cloud (or self-hosted K8s) |
| LiteLLM | Your Postgres (self-hosted only) |

## Getting started

```bash
# 1. Run Candela
docker run -p 8181:8181 ghcr.io/candelahq/candela-server:latest

# 2. Point your app
export OPENAI_BASE_URL=http://localhost:8181/proxy/openai

# 3. Use normally — every call is now traced with cost tracking
python -c "import openai; print(openai.chat.completions.create(model='gpt-4o', messages=[{'role':'user','content':'hello'}]))"
```

---

*Missing a comparison? Open an issue or PR — we want this page to be accurate and fair.*
