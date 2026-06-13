# 🤝 Contributing to Candela

Welcome! We're thrilled you're interested in contributing to Candela. Whether you're fixing a typo, squashing a bug, or building a brand-new feature — every contribution makes a difference, and we're grateful for your time.

Candela is an open-source, OTel-native LLM observability platform. We believe in building in the open, and we want this to be a project where **anyone** can jump in, learn, and make an impact — regardless of experience level.

> **First time contributing to open source?** That's awesome! We've marked some issues with the [`good first issue`](https://github.com/candelahq/candela/labels/good%20first%20issue) label — they're a great place to start. Don't hesitate to ask questions in the issue comments or open a draft PR early for feedback.

---

## 📋 Table of Contents

- [Prerequisites](#-prerequisites)
- [Getting Started](#-getting-started)
- [Project Structure](#-project-structure)
- [Development Workflow](#-development-workflow)
- [Code Style](#-code-style)
- [Pull Request Process](#-pull-request-process)
- [Architecture Overview](#-architecture-overview)
- [Getting Help](#-getting-help)
- [Code of Conduct](#-code-of-conduct)
- [License](#-license)

---

## 🧰 Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| **Nix** | Latest | **Recommended.** Provides all dependencies in a reproducible shell. |
| **Go** | 1.26+ | Included in the Nix dev shell. |
| **Node.js** | 22+ | For the Next.js UI. Included in the Nix dev shell. |
| **pnpm** | Latest | For UI dependency management. Included in the Nix dev shell. |

> [!TIP]
> **We strongly recommend using Nix.** Running `nix develop` gives you Go, Node.js, pnpm, golangci-lint, buf, lefthook, and everything else you need — no manual installs, no version conflicts. See [`docs/nix-setup.md`](docs/nix-setup.md) for details.

---

## 🚀 Getting Started

```bash
# 1. Fork and clone the repo
git clone https://github.com/<your-username>/candela.git
cd candela

# 2. Enter the Nix dev shell (installs all dependencies)
nix develop

# 3. Copy the example config
cp config.example.yaml config.yaml

# 4. Run the tests to make sure everything works
go test ./...

# 5. Start the server locally
go run ./cmd/candela-server

# 6. (Optional) Start the UI in a separate terminal
cd ui && pnpm install && pnpm run dev
```

You should now have the Candela server running on `http://localhost:8181` and the UI on `http://localhost:3000`. 🎉

---

## 🗂️ Project Structure

```
candela/
├── cmd/                        # Application entry points
│   ├── candela-server/         #   Production server binary
│   ├── candela/                #   Local dev proxy + runtime manager
│   ├── candela-sidecar/        #   Lightweight production sidecar
│   ├── backfill-budget-config/ #   Budget config migration tool
│   └── migrate-schema/         #   Schema migration tool
│
├── pkg/                        # Shared Go packages (the core library)
│   ├── proxy/                  #   LLM API reverse proxy
│   ├── costcalc/               #   Token cost calculation engine
│   ├── storage/                #   Storage backends (DuckDB, SQLite, BigQuery, Pub/Sub)
│   ├── processor/              #   Span processing pipeline
│   ├── connecthandlers/        #   ConnectRPC service handlers
│   ├── ingestion/              #   OTel span ingestion
│   ├── runtime/                #   Local LLM runtime abstraction
│   ├── session/                #   Session resolution
│   └── notify/                 #   Budget threshold notifications
│
├── ui/                         # Next.js 16 web interface
│   ├── src/app/                #   App Router pages
│   ├── src/gen/                #   Generated TypeScript proto stubs
│   ├── src/hooks/              #   React hooks
│   ├── src/lib/                #   ConnectRPC transport config
│   └── e2e/                    #   Playwright E2E tests
│
├── gen/                        # Generated code (Go + TypeScript proto stubs, BQ schemas)
├── deploy/                     # Dockerfiles, Helm charts, docker-compose
├── docs/                       # In-depth documentation (architecture, security, ops)
├── rust/                       # Rust workspace (incremental rewrite of sidecar)
├── sdks/                       # Enrichment SDKs (Python, TypeScript, Go, Kotlin, Rust)
├── collector/                  # Custom OpenTelemetry Collector distro
├── test/functional/            # Language-agnostic Hurl HTTP tests
├── terraform/                  # GCP infrastructure (Cloud Run, BigQuery, IAP)
│
├── flake.nix                   # Nix dev shell definition
├── lefthook.yml                # Pre-commit hook configuration
├── buf.gen.yaml                # Buf code generation config
├── config.example.yaml         # Example server configuration
└── go.mod                      # Go module definition
```

### Key directories to know

- **`cmd/`** — Start here to understand how the binaries are wired up.
- **`pkg/`** — The heart of the codebase. Most feature work happens here.
- **`ui/`** — The Next.js dashboard. If you're a frontend contributor, this is your home.
- **`deploy/`** — Docker and Helm configs for production deployments.
- **`docs/`** — Deep-dive docs on architecture, security, proxy internals, and more.
- **`gen/`** — Auto-generated code from protobuf definitions. **Don't edit these files directly** — they're regenerated from [`candelahq/candela-protos`](https://github.com/candelahq/candela-protos) via `buf generate`.

---

## 🔄 Development Workflow

### Branch naming

Create a branch from `main` using one of these prefixes:

| Prefix | Use for |
|--------|---------|
| `feat/` | New features (e.g., `feat/bedrock-cost-calc`) |
| `fix/` | Bug fixes (e.g., `fix/streaming-timeout`) |
| `docs/` | Documentation changes (e.g., `docs/contributing-rewrite`) |
| `test/` | Test additions or improvements (e.g., `test/proxy-race-conditions`) |

### Commit conventions

We follow [**Conventional Commits**](https://www.conventionalcommits.org/en/v1.0.0/):

```
<type>(<optional scope>): <description>

[optional body]

[optional footer(s)]
```

**Examples:**

```
feat(proxy): add Anthropic Bedrock provider support
fix(costcalc): correct prompt caching multiplier for 1h TTL
docs: update deployment guide for Helm chart v2
test(storage): add race condition tests for DuckDB writer
chore: bump golangci-lint to v2.3
```

### Pre-commit hooks (Lefthook)

We use [**Lefthook**](https://github.com/evilmartians/lefthook) for pre-commit hooks. They run **automatically** on every `git commit` — no setup needed if you're in the Nix shell. The hooks include:

- ✅ `gofmt` — Go formatting
- ✅ `go vet` — Go static analysis
- ✅ `golangci-lint` — Comprehensive Go linting
- ✅ `go test` — Unit tests
- ✅ `go mod tidy` — Module tidiness check
- ✅ Trailing whitespace and end-of-file checks
- ✅ YAML and JSON validation
- ✅ Private key detection (security)
- ✅ Merge conflict marker detection
- ✅ `cargo fmt`, `cargo clippy`, `cargo test` (for Rust changes)

> [!IMPORTANT]
> **Never skip the hooks** with `--no-verify`. If a hook is failing, fix the underlying issue. If you believe a hook is broken, open an issue.

### Running tests

```bash
# Unit tests
go test ./...

# Unit tests with race detector (CI runs this too)
go test -race ./...

# Single package
go test ./pkg/costcalc/...

# Functional tests (language-agnostic HTTP tests via Hurl)
./test/functional/run.sh --go

# UI E2E tests
cd ui && pnpm run test:e2e
```

### Linting

```bash
# Go linting
golangci-lint run

# Go formatting check
gofmt -l .

# Proto linting (if you've changed proto definitions)
buf lint
buf format -d
```

---

## 🎨 Code Style

### Go

- Format with **`gofmt`** (enforced by pre-commit hooks).
- Follow [`golangci-lint`](https://golangci-lint.run/) rules — the full config is in the repo root.
- Use **table-driven tests** where applicable.
- Place tests in the same package as the code they test (e.g., `proxy_test.go` alongside `proxy.go`).
- Every new feature **must** include tests.
- Use `go.uber.org/zap` for structured logging.

### Protobuf

- Proto definitions live in the separate [`candelahq/candela-protos`](https://github.com/candelahq/candela-protos) repo.
- Run `buf generate` from the repo root to regenerate Go and TypeScript stubs.
- Use `buf lint` and `buf format` to validate proto changes.
- Generated code lands in `gen/` — **do not edit generated files by hand**.

### TypeScript (UI)

- The UI uses **Next.js 16** with App Router.
- Follow **ESLint** + **Prettier** rules (configured in `ui/`).
- Use the generated ConnectRPC client stubs from `src/gen/`.
- Write Playwright E2E tests for new UI features in `ui/e2e/`.

---

## 🔀 Pull Request Process

### 1. Open a draft PR early

Don't wait until your code is perfect — **open a draft PR** as soon as you have a direction. This lets us give feedback early and avoids wasted effort.

### 2. Reference the issue

Link your PR to the relevant issue using GitHub keywords:

```
Closes #123
Fixes #456
```

### 3. Fill out the PR description

Explain:
- **What** changed and **why**
- Any design decisions or trade-offs
- How to test the change

### 4. CI must pass

All of the following checks must be green before merge:

- ✅ Go tests (`go test -race ./...`)
- ✅ Go linting (`golangci-lint run`)
- ✅ UI build (`pnpm run build` — includes TypeScript type-check)
- ✅ Playwright E2E tests
- ✅ Functional tests (Hurl)

### 5. Code review

- **One approval** is required to merge.
- Be responsive to feedback — we aim to keep PRs moving.
- We may suggest changes, ask questions, or request tests. This is collaborative, not adversarial. 🤝

### 6. Merge

Once approved and CI is green, a maintainer will merge your PR. We use **squash merges** to keep the commit history clean.

---

## 🏗️ Architecture Overview

Understanding the high-level architecture will help you navigate the codebase.

### Request Pipeline

```
Client Request
    │
    ▼
┌─────────┐     ┌───────────────┐     ┌─────────┐
│  Proxy  │ ──▶ │ Cost Calculator│ ──▶ │ Storage │
└─────────┘     └───────────────┘     └─────────┘
```

1. **Proxy** (`pkg/proxy/`) — Intercepts LLM API calls (OpenAI, Google, Anthropic), injects auth, captures request/response pairs, and propagates W3C Trace Context.
2. **Cost Calculator** (`pkg/costcalc/`) — Extracts token counts and calculates USD cost using the model catalog (including prompt caching adjustments).
3. **Storage** (`pkg/storage/`) — Persists spans via the CQRS pattern: `SpanWriter` (write-only), `SpanReader` (read-only), `TraceStore` (convenience wrapper). Supports DuckDB, SQLite, BigQuery, and Pub/Sub fan-out.

### Model Catalog

The cost calculator uses a **catalog store interface** to look up per-model pricing. This keeps pricing data decoupled from the calculation logic and makes it easy to add new model providers.

### Auth Middleware

Authentication is handled at two levels:

- **Firebase Auth** — For the web UI and API (user identity, RBAC roles).
- **Identity-Aware Proxy (IAP)** — For team-mode deployments behind Google Cloud IAP, using a dual-token strategy.

Both are implemented as middleware interceptors in `pkg/connecthandlers/`.

### Fan-out Architecture

The processor uses CQRS to fan out writes to **multiple sinks simultaneously** — e.g., DuckDB for local queries + Pub/Sub for downstream pipelines + OTLP export to Datadog/Tempo/Jaeger. Each sink is isolated with its own timeout.

---

## 💬 Getting Help

Stuck? Have questions? We're here to help:

- **💡 [GitHub Discussions](https://github.com/candelahq/candela/discussions)** — Ask questions, share ideas, or just say hi.
- **🐛 [GitHub Issues](https://github.com/candelahq/candela/issues)** — Report bugs or request features.
- **📖 [Documentation](docs/)** — Deep-dive guides on architecture, security, deployment, and more.

When reporting a bug, please include:
- A clear description of the issue
- Steps to reproduce
- Your environment (Go version, OS, storage backend)
- Relevant logs or error messages

---

## 📜 Code of Conduct

We are committed to providing a welcoming and inclusive experience for everyone. All participants are expected to uphold our [Code of Conduct](https://github.com/candelahq/candela/blob/main/CODE_OF_CONDUCT.md).

**In short:** Be kind, be respectful, and assume good intentions.

---

## ⚖️ License

By contributing to Candela, you agree that your contributions will be licensed under the [**Apache License 2.0**](LICENSE).

---

**Thank you for helping make Candela better!** 🕯️ Every contribution — big or small — helps us build the observability platform the LLM ecosystem deserves.
