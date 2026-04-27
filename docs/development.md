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
The server defaults to **DuckDB** and listens on the port from `config.yaml` (default **8181**).

```bash
nix develop -c go run ./cmd/candela-server
```

Verify it's running: `curl http://localhost:8181/healthz`

### 3. Running the UI
The web interface is a Next.js 16 app in `ui/`.

```bash
cd ui && pnpm install && pnpm run dev
```

The UI will be available at `http://localhost:3000`.

### 4. Testing
We use standard Go testing. Candela includes integration tests for the Proxy and Storage backends.

```bash
# Run all tests (always use nix develop -c for toolchain access)
nix develop -c go test ./...

# With race detector and verbose output
nix develop -c go test ./... -v -race -count=1

# Run proxy tests
nix develop -c go test ./pkg/proxy -v
```

See [docs/testing.md](testing.md) for the full testing guide.

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

### OTLP Export Sink
Forward traces to any OTel-compatible backend (Datadog, Grafana Tempo, Jaeger, etc.) alongside your primary storage:

```yaml
sinks:
  otlp:
    enabled: true
    endpoint: "http://localhost:4318"
    compression: "gzip"  # default
```

See [docs/architecture.md](architecture.md) for full configuration options.

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

See [docs/api-reference.md](api-reference.md) for the full API reference.

---

## 🐛 Debugging

### Log Level Control

Candela uses Go's `slog` with JSON output. The default level is `INFO`. To enable debug logging:

```go
// In cmd/candela-server/main.go, change the handler options:
logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
    Level: slog.LevelDebug,  // Change from LevelInfo
}))
```

Debug-level logs include:
- Every proxied LLM call with model, tokens, cost, latency
- Auth strategy evaluation (which strategy succeeded/failed)
- Span processor flush events
- Stream chunk translation details

### IDE Debugger (GoLand / VS Code)

Set the working directory to the repo root so `config.yaml` is found:

```json
// VS Code launch.json
{
  "type": "go",
  "request": "launch",
  "name": "Candela Server",
  "program": "${workspaceFolder}/cmd/candela-server",
  "cwd": "${workspaceFolder}"
}
```

### Delve (CLI)

```bash
nix develop -c dlv debug ./cmd/candela-server
```

---

## 🔥 Firestore Emulator

For full-stack local dev with user management, run the Firestore emulator:

```bash
# Start the emulator (included in gcloud SDK via Nix)
nix develop -c gcloud emulators firestore start --host-port=localhost:8282

# In another terminal, set the emulator env var
export FIRESTORE_EMULATOR_HOST=localhost:8282
nix develop -c go run ./cmd/candela-server
```

With the emulator, Firestore operations (users, budgets, grants, audit) work locally without a GCP project.

---

## 🔄 Proto Generation

Candela uses **Buf Remote Generation** — no local protoc plugins needed.

```bash
# Generate Go + TypeScript stubs
cd proto && nix develop -c buf generate
```

This requires a `BUF_TOKEN` for remote generation. Options:
- **CI**: Set as a GitHub Actions secret
- **Local**: Add to `~/.netrc`: `machine buf.build login <user> password <token>`
- **Local (env)**: `BUF_TOKEN=<token> buf generate`

After generation, copy TS stubs to the UI:
```bash
cp -r gen/ts/candela/* ui/src/gen/
rm -f ui/src/gen/types/bq_span_pb.ts  # BigQuery schema — server-only
```
