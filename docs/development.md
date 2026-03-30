# 🔨 Development Guide

Candela uses a **Nix-first** development workflow. This ensures that every developer has the exact same version of Go, Protobuf, Buf, and Node.js.

## 📦 Prerequisites

Ensure you have **Nix** with `flakes` enabled.

```bash
# Enter the nix dev shell
nix develop
```

This will automatically install:
- **Go 1.23**
- **Buf** (Protobuf management)
- **Node.js 22** & **pnpm**
- **Python 3.12** & **uv**
- **gh** (GitHub CLI)
- **docker-compose**

---

## 🏗️ Building & Running

### 1. Generating Protobuf Code
Candela is proto-first. All service boundaries are defined in `proto/`.

```bash
# From the root directory
cd proto && buf generate
```

This will populate `gen/go/` and `gen/ts/` (for the upcoming UI).

### 2. Running the Backend (Local Dev)
The server defaults to **SQLite** and **Port 8080**.

```bash
go run ./cmd/candela-server
```

You can point your browser at `http://localhost:8080/healthz` to verify it's running.

### 3. Testing
We use standard Go testing. Candela includes integration tests for the Proxy and Storage backends.

```bash
# Run all tests
go test ./...

# Run proxy tests (requires internet for some providers, or mocks)
go test ./pkg/proxy -v
```

---

## 🗄️ Storage Backends

### SQLite (Default)
No setup required. The database will be created as `candela.db` in your current directory.

### ClickHouse
For production-like testing, use the included Docker Compose file.

```bash
docker compose -f deploy/docker-compose.yml up clickhouse
```

Then update `config.yaml`:
```yaml
storage:
  backend: "clickhouse"
  clickhouse:
    addr: "localhost:9000"
    database: "candela"
```

---

## 📡 API Interaction

Since we use **ConnectRPC**, you can interact with the API using `curl` (Connect protocol) or `grpcurl` (gRPC protocol).

**List Traces via `curl`:**
```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{}' \
  http://localhost:8080/candela.v1.TraceService/ListTraces
```

**List Traces via `grpcurl`:**
```bash
grpcurl -plaintext localhost:8080 candela.v1.TraceService/ListTraces
```
