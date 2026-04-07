# 🔨 Development Guide

Candela uses a **Nix-first** development workflow. This ensures that every developer has the exact same version of Go, Protobuf, Buf, and Node.js.

## 📦 Prerequisites

Ensure you have **Nix** with `flakes` enabled.

```bash
# Enter the nix dev shell
nix develop
```

This will automatically install:
- **Go 1.26**
- **Buf** (Protobuf management)
- **Node.js 22** & **pnpm**
- **Python 3.12** & **uv**
- **cloudflared** (Cloudflare tunnel for Cursor 3 integration)
- **docker-compose**

---

## 🏗️ Building & Running

### 1. Generating Protobuf Code
Candela is proto-first. All service boundaries are defined in `proto/`.

```bash
# From the root directory
cd proto && buf generate
```

This will populate `gen/go/` and `gen/ts/` (for the UI).

### 2. Running the Backend (Local Dev)
The server defaults to **DuckDB** and uses the port from `config.yaml` (default **8080**).

```bash
go run ./cmd/candela-server
```

You can point your browser at `http://localhost:8181/healthz` to verify it's running.

### 3. Running the UI
The web interface is a Next.js 16 app in `ui/`.

```bash
cd ui && npm install && npm run dev
```

The UI will be available at `http://localhost:3000`.

### 4. Testing
We use standard Go testing. Candela includes integration tests for the Proxy and Storage backends.

```bash
# Run all tests
go test ./...

# Run proxy tests (requires internet for some providers, or mocks)
go test ./pkg/proxy -v
```

---

## 🖥️ Editor Quick Start

### Zed

```bash
# Terminal 1: Start Candela
nix develop -c go run ./cmd/candela-server

# Then launch Zed with the API key set
OPENAI_API_KEY=candela open -a Zed
```

Configure Zed settings to point at Candela's proxy routes.

### OpenCode

```bash
# Terminal 1: Start Candela
nix develop -c go run ./cmd/candela-server

# Terminal 2: Launch OpenCode (picks up opencode.json from project root)
npx -y opencode-ai
```

Use `/connect` → Other → `candela-anthropic` → key `candela`, then `/models` to select a model.

See [docs/proxy.md](proxy.md) for full setup instructions for both editors.

---

## 🗄️ Storage Backends

### DuckDB (Default)
No setup required. The database will be created as `candela.duckdb` in your current directory. DuckDB is OLAP-optimized for high-throughput analytics queries.

### SQLite
Lightweight alternative for minimal setups. Set in `config.yaml`:

```yaml
storage:
  backend: "sqlite"
  sqlite:
    path: "candela.db"  # or ":memory:" for ephemeral
```

### BigQuery
For production-scale analytics. Requires a GCP project with BigQuery enabled:

```yaml
storage:
  backend: "bigquery"
  bigquery:
    project_id: "my-gcp-project"
    dataset: "candela"
    location: "US"
```

---

## 📡 API Interaction

Since we use **ConnectRPC**, you can interact with the API using `curl` (Connect protocol) or `grpcurl` (gRPC protocol).

**List Traces via `curl`:**
```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{}' \
  http://localhost:8181/candela.v1.TraceService/ListTraces
```

**List Traces via `grpcurl`:**
```bash
grpcurl -plaintext localhost:8181 candela.v1.TraceService/ListTraces
```
