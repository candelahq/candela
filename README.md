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
- **OpenAI**: `http://localhost:8181/proxy/openai/v1`
- **Google Gemini**: `http://localhost:8181/proxy/google/`
- **Anthropic (via Vertex AI)**: `http://localhost:8181/proxy/anthropic/`

> **📍 Port Configuration**: Candela uses port `8181` when running with a config file (recommended), or `8080` when using defaults. See [Port Configuration](#-port-configuration) for details.

### 2. OTel-Native Agent Mode (Production)
For deep observability into agent frameworks (**ADK**, **LangChain**, **CrewAI**), Candela ingests standard OTLP spans through a custom-built **OTel Collector distro**.

---

## ✨ Key Features

- **🕯️ OTel-Native**: OTLP is our native language. No proprietary SDKs.
- **💰 Real-time Cost Tracking**: Automatic token extraction and USD calculation for OpenAI, Google, and Anthropic.
- **🔐 Role-Based Access Control**: Admin vs Developer roles with budget enforcement and grant-based spending.
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

# Start the Candela server (defaults to DuckDB + Port 8181)
nix develop -c go run ./cmd/candela-server

# Start the UI (separate terminal)
cd ui && pnpm install && pnpm run dev
```

> **💡 Quick Start Tip**: Copy `config.example.yaml` to `config.yaml` to use port 8181 and enable all features.

### Option B: Docker Compose (Full Stack)
Ideal for testing the full multi-service experience.

```bash
# Start all services (server + collector)
docker compose -f deploy/docker-compose.yml up
```

---

## 🛠️ Route an LLM Call

Once Candela is running, point your favorite LLM client at the Candela proxy (Port 8181) to start capturing observability data instantly.

> **🔧 Setup Required**: For Anthropic/Claude via Vertex AI, see [GCP Setup Guide](#-gcp-setup-for-anthropic) below.

### OpenAI Example
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8181/proxy/openai/v1",
    api_key="sk-..."
)

# Call as usual — Candela handles the rest
response = client.chat.completions.create(...)
```

### Anthropic (via Vertex AI) Example
```python
from anthropic import Anthropic

client = Anthropic(
    base_url="http://localhost:8181/proxy/anthropic",
    api_key="YOUR_GCP_TOKEN" # Uses ADC for GCP authentication
)

response = client.messages.create(...)
```

---

## 🏗️ Architecture

```mermaid
graph TD
    subgraph "Developer Tools"
        JB[JetBrains / Cline]
        App[App / Agent]
        SDK[OTel SDK]
    end

    subgraph "candela-local"
        LM["LM Compat Listener (:1234)<br/>Unified /v1/models"]
        SC[Span Capture]
        LP[Local Proxy]
        RP[Remote Proxy + Auth]
        SP["SpanProcessor<br/>(Solo Mode)"]
        SQLite[(traces.db)]
    end

    subgraph "Local Runtimes"
        Ollama["Ollama (:11434)"]
        VLLM["vLLM (:8000)"]
        LMS["LM Studio (:1234)"]
    end

    subgraph "Candela Cloud (Team Mode)"
        Server[Go Backend Server]
        Proxy[LLM API Proxy]
        Processor[Span Processor<br/>Fan-out to Writers]
        DuckDB[(DuckDB)]
        BQ[(BigQuery)]
    end

    subgraph "Upstream LLMs"
        VAI[Vertex AI / Google]
        ANT[Anthropic]
        OAI[OpenAI]
    end

    JB -->|/v1/models<br/>/v1/chat/completions| LM
    LM -->|Local model| SC
    SC --> LP
    SC -.->|Capture| SP
    SP --> SQLite
    LM -->|Remote model| RP
    LP --> Ollama
    LP -.-> VLLM
    LP -.-> LMS
    RP -->|OIDC Auth| Server
    App -->|Proxy Mode| RP
    App -->|OTel Mode| SDK --> Server
    Proxy -->|Forward| VAI
    Proxy -->|Forward| ANT
    Proxy -->|Forward| OAI
    Proxy -.->|Capture| Processor
    Processor -->|Write| DuckDB
    Processor -.->|Write| BQ
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

## 🔧 Port Configuration

Candela uses port `8181` by default.

| Setup | Port | When Used |
|-------|------|-----------|
| **Default** | `8181` | Running `go run ./cmd/candela-server` |
| **Docker Compose** | `8181` | As specified in `docker-compose.yml` |

### Quick Setup
```bash
# Copy example config to enable all features on port 8181
cp config.example.yaml config.yaml

# Or create minimal config
cat > config.yaml << EOF
server:
  port: 8181
storage:
  backend: "duckdb"
EOF
```

---

## ⚙️ Configuration

Candela is configured via `config.yaml` (or `$CANDELA_CONFIG`):

```yaml
server:
  host: "0.0.0.0"
  port: 8181

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
    - "http://localhost:8181"

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
pnpm run test:e2e      # run Playwright E2E tests (27+ tests)
pnpm run test:e2e:ui   # Playwright interactive UI mode
```

The UI communicates with the backend via **ConnectRPC v2** on the configured port (`8181` with config file, `8080` without). Pages gracefully handle offline backend state.

> [!TIP]
> **Proto Generation**: We use **Buf Remote Generation**. Just run `buf generate` in the `proto/` directory—no local plugins required!

---

## 🕹️ candela-local — Local Development Proxy

`candela-local` is an auth-injecting proxy + runtime manager for developer machines.
It operates in three modes:

### 🏠 Solo Mode (Zero-Config)

Run local models with full observability — **no cloud account needed**.

```yaml
# ~/.candela.yaml
runtime_backend: ollama
```

```bash
candela-local   # starts on :8181 (proxy) + :1234 (LM compat)
```

- Local models via Ollama/vLLM on `:1234`
- Every call traced to `~/.candela/traces.db` (SQLite)
- Management UI at `http://localhost:8181/_local/`

### ☁️ Solo + Cloud Mode (Direct Cloud)

Call Gemini and Claude directly using your **Google identity** — no server deployment needed.

```yaml
# ~/.candela.yaml
runtime_backend: ollama

providers:
  - name: google
    models: [gemini-2.5-pro, gemini-2.0-flash]
  - name: anthropic
    models: [claude-sonnet-4-20250514]

vertex_ai:
  project: my-gcp-project
```

- Local + cloud models merged into `/v1/models`
- Smart routing: local stays local, cloud goes direct to Vertex AI via ADC
- All calls (local + cloud) traced to SQLite — full observability
- Prerequisites: `gcloud auth application-default login`

### 🌐 Team Mode (Governance + Budgets)

Connect to a shared Candela server for **budget enforcement, RBAC, and team-wide cost tracking**.

```yaml
# ~/.candela.yaml
runtime_backend: ollama
remote: https://candela-xxx.a.run.app
audience: "12345678.apps.googleusercontent.com"
```

- Local + cloud models merged into `/v1/models`
- Smart routing: local models stay local, cloud models route through Candela server
- OIDC auth injected automatically via ADC
- Team-wide cost tracking, budget enforcement, and centralized governance

> [!TIP]
> See [docs/candela-local.md](docs/candela-local.md) for the full setup guide.

### Unified Model Discovery

The LM-compatible listener on `:1234` merges **local**, **cloud**, and **remote** models into a single `/v1/models` response:

```bash
# JetBrains, Cline, or any OpenAI-compatible client sees everything:
curl http://localhost:1234/v1/models
# → local: llama3.2:3b, mistral:7b (from Ollama)
# → cloud: gemini-2.5-pro, claude-sonnet-4 (direct via ADC)  [Solo + Cloud]
# → remote: gpt-4o (from Cloud Run)  [Team Mode]
```

`/v1/chat/completions` automatically routes to the right backend:
- **Local model** → Ollama/vLLM/LM Studio (no round-trip)
- **Cloud model** → Vertex AI directly (via Google ADC)
- **Remote model** → Cloud Run proxy (with OIDC auth injection)

### Runtime Management UI

Embedded management UI at `/_local/` for controlling local LLM runtimes:

- **Runtime control** — Start/stop Ollama, vLLM, or LM Studio; health monitoring with auto-polling
- **Model management** — List, load, unload, and **delete** models from disk
- **Pull models** — Download models with real-time progress tracking and **cancel** support
- **Traces** — Recent LLM calls with tokens, cost, duration (Solo Mode)
- **Backend discovery** — Auto-detect installed runtimes and show install hints
- **State persistence** — Settings, pull history, and runtime state persisted to `~/.candela/state.db`

All features are accessible via **ConnectRPC** (`RuntimeService`) and the embedded vanilla JS dashboard.

---

## 🗺️ Roadmap

- **Phase 1: Foundation** ✅ (Ingestion, Proxy, Cost Calc, Docs)
- **Phase 2: Storage & Architecture** ✅ (DuckDB, CQRS, BigQuery, Pub/Sub, CORS)
- **Phase 3: Visual Explorer** ✅ (Next.js UI, Dashboard, Traces, Costs, Settings, E2E Tests)
- **Phase 4: Multi-User Platform** ✅ (IAP Auth, Token Budgets, Admin Panel, RBAC)
  - Terraform infrastructure (Cloud Run, BigQuery, Firestore, IAP)
  - Proto-first user/budget/grant schemas with `protovalidate`
  - Role-based access control (admin guard interceptor)
  - Admin UI: user management, budget explainer, audit logs
  - Client-side form validation (`@bufbuild/protovalidate`)
  - Budget enforcement with grant-first waterfall
  - `candela-local` auth-injecting proxy for developer machines
  - `candela-local` embedded runtime management UI (`/_local/`)
- **Phase 5: Ecosystem & Polish** 📋 (Agent DAGs, Multi-region, Alerting, Google Workspace Sync)

---

## 📂 Project Structure

```
candela/
├── proto/                       # Protobuf definitions (Source of Truth)
├── gen/                         # Generated code (Go, TypeScript, Python)
├── cmd/candela-server/          # Server entry point
├── cmd/candela-local/           # Local dev proxy + runtime manager
│   ├── lm_handler.go            # Smart model routing (local ↔ cloud)
│   ├── span_capture.go          # Request/response capture middleware
│   ├── traces_handler.go        # /_local/api/traces REST endpoint
│   └── ui/                      # Embedded management UI (vanilla JS)
├── pkg/
│   ├── processor/               # Shared span processor (Solo + Server)
│   ├── storage/                 # Storage interfaces (SpanWriter, SpanReader)
│   │   ├── duckdb/              # DuckDB driver (default, OLAP-optimized)
│   │   ├── sqlite/              # SQLite driver (lightweight)
│   │   ├── bigquery/            # BigQuery driver (production scale)
│   │   └── pubsub/              # Pub/Sub sink (write-only fan-out)
│   ├── proxy/                   # LLM API reverse proxy
│   ├── costcalc/                # Token cost calculation engine
│   ├── connecthandlers/         # ConnectRPC service handlers
│   ├── runtime/                 # Local LLM runtime abstraction (Ollama, vLLM, LM Studio)
│   └── ingestion/               # OTel span ingestion
├── terraform/                   # GCP infrastructure (OpenTofu/Terraform)
│   ├── cloud_run.tf             # Cloud Run + IAP
│   ├── bigquery.tf              # Spans storage
│   ├── firestore.tf             # Users, budgets, grants
│   └── iam.tf                   # Service accounts + roles
├── collector/                   # Custom OTel Collector distro
├── docs/                        # Deep-dive documentation
├── ui/                          # Next.js 16 web interface
│   ├── src/app/                 # App Router pages (dashboard, traces, admin, etc.)
│   ├── src/gen/                 # Generated TS proto stubs
│   ├── src/hooks/               # React hooks (useCurrentUser, useProtoValidation)
│   ├── src/lib/                 # ConnectRPC transport config
│   ├── e2e/                     # Playwright E2E tests (app + admin)
│   └── playwright.config.ts     # Playwright config
├── .github/workflows/ci.yml    # CI pipeline (Go + UI + Playwright)
└── config.yaml                  # Server configuration
```

---

## 🌩️ GCP Setup for Anthropic

To use Claude models via Vertex AI, follow these steps:

### 1. GCP Project Setup
```bash
# Install gcloud CLI if needed
curl https://sdk.cloud.google.com | bash

# Create a new project (or use existing)
gcloud projects create YOUR-PROJECT-ID

# Set as default project
gcloud config set project YOUR-PROJECT-ID

# Enable required APIs
gcloud services enable \
  aiplatform.googleapis.com \
  compute.googleapis.com
```

### 2. Authentication
```bash
# Set up Application Default Credentials
gcloud auth application-default login

# Verify your credentials
gcloud auth list
```

### 3. Enable Claude Models
1. Go to [Vertex AI Model Garden](https://console.cloud.google.com/vertex-ai/model-garden)
2. Search for "Claude"
3. Enable Claude 3.5 Sonnet and Claude 3 Opus
4. Accept Google's terms for Anthropic models

### 4. Update Config
```yaml
# In your config.yaml
proxy:
  enabled: true
  vertex_ai:
    project_id: "YOUR-PROJECT-ID"  # Replace with your actual project
    region: "us-east5"             # or us-central1, us-west1
```

### 5. Test Connection
```bash
# Start Candela
go run ./cmd/candela-server

# Test in another terminal
curl -X POST http://localhost:8181/proxy/anthropic/v1/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test" \
  -d '{
    "model": "claude-3-51022",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'
```

> **💡 Project ID Tips**:
> - Use `gcloud config get project` to see your current project
> - Project IDs must be globally unique (numbers/letters/hyphens only)
> - Billing must be enabled for Vertex AI usage

---

## 🐛 Troubleshooting

### Common Issues

#### Port Already in Use
```bash
# Check what's using the port
lsof -i :8181  # or :8080

# Kill the process or use a different port
kill -9 <PID>
```

#### Backend Not Starting
```bash
# Check if config file is valid
go run ./cmd/candela-server --help

# Verify with minimal config
echo "server:\n  port: 8181" > test-config.yaml
CANDELA_CONFIG=test-config.yaml go run ./cmd/candela-server
```

#### UI Can't Connect to Backend
1. **Check backend is running**: Visit `http://localhost:8181/healthz`
2. **Verify port configuration**: Backend and UI must use same port
3. **Check CORS settings**: Add your UI URL to `cors.allowed_origins`

#### Anthropic/Vertex AI Errors
```bash
# Verify ADC is working
gcloud auth application-default print-access-token

# Check project/region settings
gcloud config list

# Test Vertex AI access
gcloud ai models list --region=us-east5
```

#### Permission Denied (GCP)
```bash
# Check IAM roles
gcloud projects get-iam-policy YOUR-PROJECT-ID

# Add required role if missing
gcloud projects add-iam-policy-binding YOUR-PROJECT-ID \
  --member="user:YOUR-EMAIL" \
  --role="roles/aiplatform.user"
```

### Getting Help
- **GitHub Issues**: [Report bugs](https://github.delahq/candela/issues)
- **Documentation**: Check `docs/` directory for detailed guides
- **Logs**: Run with `CANDELA_LOG_LEVEL=debug` for verbose output

---
## 🤝 Contributing

We are in early development! See [CONTRIBUTING.md](./CONTRIBUTING.md) for local setup instructions and architectural deep dives.

## 📄 License

Apache License 2.0. See [LICENSE](./LICENSE) for details.
