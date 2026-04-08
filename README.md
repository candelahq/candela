<div align="center">
  <img src="assets/logo.png" alt="Candela Logo" width="200" />
  <h1>Candela</h1>
</div>
**Open-source, OTel-native LLM Observability & Engineering Platform.**

Candela is a production-grade observatory for your LLM applications. It captures every trace, calculates every cent, and evaluates every output with deep integration into **OpenTelemetry**, **Google Cloud (Vertex AI)**, and the wider GenAI ecosystem.

[![CI](https://github.com/candelahq/candela/actions/workflows/ci.yml/badge.svg)](https://github.com/candelahq/candela/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/candelahq/candela.svg)](https://pkg.go.dev/github.com/candelahq/candela)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

---

## 🚀 Two Ways to Get Observability

Candela offers a dual-mode ingestion strategy to fit any stage of your project:

### 1. Zero-Code Proxy Mode (Quick Start)
Drop Candela into your existing app by just changing your `base_url`. No instrumentation needed.
- **OpenAI**: `http://localhost:8080/proxy/openai/v1`
- **Google Gemini**: `http://localhost:8080/proxy/google/`
- **Anthropic (via Vertex AI)**: `http://localhost:8080/proxy/anthropic/`

### 2. OTel-Native Agent Mode (Production)
For deep observability into agent frameworks (**ADK**, **LangChain**, **CrewAI**), Candela ingests standard OTLP spans through a custom-built **OTel Collector distro**.

---

## ✨ Key Features

- **🕯️ OTel-Native**: OTLP is our native language. No proprietary SDKs.
- **💰 Real-time Cost Tracking**: Automatic token extraction and USD calculation for OpenAI, Google, and Anthropic.
- **🧪 LLM-as-Judge (Phase 3)**: Automated quality scoring and evaluation rubrics.
- **🗄️ Pluggable Storage**: **DuckDB** for high-performance local/edge; **BigQuery** for serverless production scale; **SQLite** for lightweight dev.
- **📡 SSE Streaming Support**: Captures full streaming responses without interfering with user latency.
- **📦 Single-Binary Edge-Ready**: In-process queuing and processing for low-overhead deployments.
- **🔀 Fan-out Architecture**: CQRS-based design allows writing to multiple sinks simultaneously (e.g., DuckDB + Pub/Sub).

---

## 🚀 Quick Start

You can get Candela running in less than 60 seconds using either a local binary or Docker.

### Option A: Local Binary (Fastest)
Ideal for local development. Uses **DuckDB** by default.

```bash
# Clone and enter the nix shell (or ensure Go 1.26 is installed)
nix develop

# Start the Candela server (defaults to DuckDB + Port 8080)
go run ./cmd/candela-server

# Start the UI (separate terminal)
cd ui && pnpm install && pnpm run dev
```

### Option B: Docker Compose (Full Stack)
Ideal for testing the full multi-service experience.

```bash
# Start all services (server + collector)
docker compose -f deploy/docker-compose.yml up
```

---

## 🛠️ Route an LLM Call

Once Candela is running, point your favorite LLM client at the Candela proxy (Port 8080) to start capturing observability data instantly.

### OpenAI Example
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/proxy/openai/v1",
    api_key="sk-..."
)

# Call as usual — Candela handles the rest
response = client.chat.completions.create(...)
```

### Anthropic (via Vertex AI) Example
```python
from anthropic import Anthropic

client = Anthropic(
    base_url="http://localhost:8080/proxy/anthropic",
    api_key="YOUR_GCP_TOKEN" # Uses ADC for GCP authentication
)

response = client.messages.create(...)
```

---

## 🏗️ Architecture

```mermaid
graph TD
    subgraph "Your Application"
        App[App Logic]
        SDK[OTel SDK / LLM Client]
    end

    subgraph "Candela Platform"
        Proxy[LLM API Proxy]
        Server[Go Backend Server]
        Processor[Span Processor<br/>Fan-out to Writers]
        DuckDB[(DuckDB<br/>SpanWriter + SpanReader)]
        BQ[(BigQuery<br/>SpanWriter + SpanReader)]
        PubSub[Pub/Sub<br/>SpanWriter Only]
    end

    subgraph "Upstream LLMs"
        VAI[Vertex AI / Google]
        ANT[Anthropic]
        OAI[OpenAI]
    end

    App -->|Proxy Mode| Proxy
    App -->|OTel Mode| Server
    Proxy -->|Forward| VAI
    Proxy -->|Forward| ANT
    Proxy -->|Forward| OAI
    Proxy -.->|Capture| Server
    Server --> Processor
    Processor -->|Write| DuckDB
    Processor -.->|Write| BQ
    Processor -.->|Write| PubSub
    DuckDB -->|Read| Server
```

### Storage Architecture (CQRS)

Candela uses a **Command Query Responsibility Segregation** pattern:

| Interface | Purpose | Implementations |
|-----------|---------|-----------------|
| `SpanWriter` | Write-only ingestion | DuckDB, SQLite, BigQuery, Pub/Sub |
| `SpanReader` | Read-only queries | DuckDB, SQLite, BigQuery |
| `TraceStore` | Convenience (both) | DuckDB, SQLite, BigQuery |

The processor fans out writes to **all configured writers** concurrently. Only one reader is active (the primary backend).

---

## ⚙️ Configuration

Candela is configured via `config.yaml` (or `$CANDELA_CONFIG`):

```yaml
server:
  host: "0.0.0.0"
  port: 8080

storage:
  backend: "duckdb"  # duckdb | sqlite | bigquery
  duckdb:
    path: "candela.duckdb"
  sqlite:
    path: "candela.db"
  bigquery:
    project_id: "my-gcp-project"
    dataset: "candela"
    table: "spans"         # default: "spans"
    location: "US"         # default: "US"

cors:
  allowed_origins:
    - "http://localhost:3000"
    - "http://localhost:8080"

sinks:
  pubsub:
    enabled: false
    project_id: "my-gcp-project"
    topic: "candela-spans"

proxy:
  enabled: true
  project_id: "default"
  providers:
    - openai
    - google
    - anthropic

worker:
  batch_size: 100
  flush_interval: "2s"
```

---

## 🖥️ UI Development

The web interface is a Next.js 16 app in `ui/` with a dark-themed dashboard.

```bash
cd ui
pnpm install          # install deps (included in nix shell)
pnpm run dev           # start dev server → http://localhost:3000
pnpm run build         # production build (includes TypeScript type-check)
pnpm run test:e2e      # run Playwright E2E tests (12 tests)
pnpm run test:e2e:ui   # Playwright interactive UI mode
```

The UI communicates with the backend via **ConnectRPC v2** on `localhost:8080`. Pages gracefully handle offline backend state.

> [!TIP]
> **Proto Generation**: We use **Buf Remote Generation**. Just run `buf generate` in the `proto/` directory—no local plugins required!

---

## 🗺️ Roadmap

- **Phase 1: Foundation** ✅ (Ingestion, Proxy, Cost Calc, Docs)
- **Phase 2: Storage & Architecture** ✅ (DuckDB, CQRS, BigQuery, Pub/Sub, CORS)
- **Phase 3: Visual Explorer** 🟡 (Next.js UI, Dashboard, Traces, Projects — Waterfall & Cost Charts next)
- **Phase 4: Platform & Evaluation** 📋 (Admin Panel, Token Metering, LLM-as-Judge)
- **Phase 5: Ecosystem & Polish** 📋 (Agent DAGs, Multi-tenant, Alerting)

---

## 📂 Project Structure

```
candela/
├── proto/                       # Protobuf definitions (Source of Truth)
├── gen/                         # Generated code (Go, TypeScript, Python)
├── cmd/candela-server/          # Server entry point
├── pkg/
│   ├── storage/                 # Storage interfaces (SpanWriter, SpanReader)
│   │   ├── duckdb/              # DuckDB driver (default, OLAP-optimized)
│   │   ├── sqlite/              # SQLite driver (lightweight)
│   │   ├── bigquery/            # BigQuery driver (production scale)
│   │   └── pubsub/              # Pub/Sub sink (write-only fan-out)
│   ├── proxy/                   # LLM API reverse proxy
│   ├── costcalc/                # Token cost calculation engine
│   ├── connecthandlers/         # ConnectRPC service handlers
│   └── ingestion/               # OTel span ingestion
├── collector/                   # Custom OTel Collector distro
├── docs/                        # Deep-dive documentation
├── ui/                          # Next.js 16 web interface
│   ├── src/app/                 # App Router pages (dashboard, traces, etc.)
│   ├── src/gen/                 # Generated TS proto stubs
│   ├── src/lib/                 # ConnectRPC transport config
│   ├── e2e/                     # Playwright E2E tests
│   └── playwright.config.ts     # Playwright config
├── .github/workflows/ci.yml    # CI pipeline (Go + UI + Playwright)
└── config.yaml                  # Server configuration
```
---

## 🤝 Contributing

We are in early development! See [CONTRIBUTING.md](./CONTRIBUTING.md) for local setup instructions and architectural deep dives.

## 📄 License

Apache License 2.0. See [LICENSE](./LICENSE) for details.
