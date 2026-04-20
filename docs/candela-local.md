# candela-local — Developer Proxy & Runtime Manager

`candela-local` is a lightweight binary that runs on a developer's machine. It provides:

- **Unified model discovery** — one endpoint for local _and_ cloud models
- **Smart routing** — automatically sends requests to the right backend
- **Runtime management** — start/stop Ollama, pull models, manage state
- **Local observability** — capture every LLM call to SQLite with zero cloud dependencies

## Operating Modes

`candela-local` operates in one of two modes, determined entirely by your
`~/.candela.yaml` configuration:

### 🏠 Solo Mode

**For**: Individual developers who want to run local models with full
observability and zero cloud dependencies.

**Config**: Simply omit the `remote` field (or leave it empty).

```yaml
# ~/.candela.yaml — Solo Mode
port: 8181
lm_studio_port: 1234
runtime_backend: ollama
```

**What you get**:
- Local models via Ollama/vLLM/LM Studio on `:1234`
- Embedded observability — every call traced to `~/.candela/traces.db`
- Management UI at `http://localhost:8181/_local/`
- Model pulling, health monitoring, backend discovery
- **No cloud account, no authentication, no remote server needed**

**What you don't get**:
- Cloud models (GPT-4o, Claude, Gemini) — there's no remote to route to

**Architecture**:
```
JetBrains / Cline / curl
        │
        ▼
  LM Compat (:1234)
  /v1/models → local models only
  /v1/chat/completions
        │
        ▼
  spanCapture middleware
        │
        ├──▶ Ollama / vLLM (local inference)
        │
        ▼
  SpanProcessor → SQLite (~/.candela/traces.db)
        │
        ▼
  /_local/api/traces → Traces UI card
```

---

### 🌐 Team Mode

**For**: Developers on a team with a shared Candela cloud backend, who want
access to both local models _and_ cloud-hosted models (GPT-4o, Claude,
Gemini via Vertex AI).

**Config**: Set `remote` to your team's Candela server URL.

```yaml
# ~/.candela.yaml — Team Mode
port: 8181
lm_studio_port: 1234
runtime_backend: ollama

remote: https://candela-xxx.a.run.app
audience: "12345678.apps.googleusercontent.com"
```

**What you get**:
- Everything from Solo Mode, plus:
- Cloud models merged into `/v1/models` alongside local models
- Smart routing: local models run locally, cloud models route through the
  Candela server (with automatic OIDC auth injection via ADC)
- Team-wide cost tracking, budget enforcement, and traces on the cloud dashboard
- Cloud-hosted observability for remote model calls

**Architecture**:
```
JetBrains / Cline / curl
        │
        ▼
  LM Compat (:1234)
  /v1/models → local + remote models merged
  /v1/chat/completions
        │
        ├── local model ──▶ Ollama / vLLM
        │
        └── cloud model ──▶ Candela Server (Cloud Run)
                                │  (OIDC auto-injected)
                                ▼
                           OpenAI / Anthropic / Google
```

> [!TIP]
> A single developer _can_ use Team Mode without a team — you just need
> a Candela server deployed. This gives you access to cloud models
> (GPT-4o, Claude, Gemini) through the unified endpoint, with full
> cost tracking and observability.

---

## Installation

```bash
go install github.com/candelahq/candela/cmd/candela-local@latest
```

Or from the repo:

```bash
nix develop           # or ensure Go 1.26+ is installed
go run ./cmd/candela-local
```

## Configuration

`candela-local` reads `~/.candela.yaml` by default. Override with `--config`:

```bash
candela-local                          # reads ~/.candela.yaml
candela-local --config ./my-config.yaml
candela-local --remote https://... --audience 12345 --port 8181
```

### Full Config Reference

```yaml
# ── Required ──
runtime_backend: ollama             # ollama | vllm | lmstudio

# ── Optional: Network ──
port: 8181                          # main proxy port (default: 8181)
lm_studio_port: 1234                # LM compat listener (default: 1234)

# ── Optional: Team Mode (omit for Solo) ──
remote: https://candela-xxx.run.app # Candela server URL
audience: "12345678.apps..."        # IAP audience for OIDC auth

# ── Optional: Advanced ──
local_upstream: http://localhost:11434  # explicit local runtime URL
state_db_path: ~/.candela/state.db     # runtime state persistence
```

### Environment & Auth

In Team Mode, `candela-local` uses **Application Default Credentials** (ADC)
to authenticate with the Candela server:

```bash
# Set up ADC (one-time)
gcloud auth application-default login
```

---

## Unified Model Discovery

The LM-compatible listener on `:1234` provides a single `/v1/models` endpoint
that merges local and remote models:

```bash
curl http://localhost:1234/v1/models
```

**Solo Mode** returns only local models:
```json
{
  "data": [
    {"id": "llama3.2:3b", "owned_by": "ollama"},
    {"id": "mistral:7b", "owned_by": "ollama"}
  ]
}
```

**Team Mode** returns local + cloud models:
```json
{
  "data": [
    {"id": "llama3.2:3b", "owned_by": "ollama"},
    {"id": "gpt-4o", "owned_by": "openai"},
    {"id": "claude-3.5-sonnet", "owned_by": "anthropic"}
  ]
}
```

### Smart Routing

`/v1/chat/completions` automatically routes based on model name:

| Request model | Mode | Where it runs |
|---------------|------|---------------|
| `llama3.2:3b` | Solo or Team | Local (Ollama) |
| `gpt-4o` | Team | Cloud (via Candela server) |
| `gpt-4o` | Solo | 404 — no remote configured |
| `unknown-model` | Team | Cloud (fallback) |
| `unknown-model` | Solo | 404 |

> [!NOTE]
> Model matching is tag-aware: requesting `llama3.2` will match
> `llama3.2:3b` if that's the only local variant loaded.

---

## Local Observability (Solo Mode)

In Solo Mode, every LLM call through `:1234` is captured:

- **Token extraction** from both streaming (SSE) and non-streaming responses
- **Cost calculation** via the shared `costcalc` engine
- **SQLite storage** at `~/.candela/traces.db`
- **REST API** at `/_local/api/traces`
- **UI card** in the management dashboard with auto-refresh

### Traces API

```bash
# Recent traces (default: 50, max: 200)
curl http://localhost:8181/_local/api/traces?limit=20
```

```json
{
  "spans": [
    {
      "span_id": "abc123...",
      "model": "llama3.2:3b",
      "provider": "local",
      "input_tokens": 150,
      "output_tokens": 42,
      "total_tokens": 192,
      "cost_usd": 0.0,
      "duration_ms": 1230,
      "status": "ok",
      "timestamp": "2026-04-19T22:15:00Z"
    }
  ],
  "count": 1
}
```

---

## Management UI

Access at `http://localhost:8181/_local/`:

| Card | Description |
|------|-------------|
| **Health** | Runtime status, start/stop controls, uptime |
| **Models** | Loaded models with size, family, quantization |
| **Pull Model** | Download new models with progress tracking |
| **Traces** | Recent LLM calls with tokens, cost, duration |
| **Backends** | Auto-detected runtimes with install hints |
| **Settings** | State DB path, reset |

---

## IDE Integration

### JetBrains (IntelliJ, PyCharm, etc.)

1. Settings → AI Assistant → Enable "LM Studio"
2. URL is pre-configured to `http://localhost:1234` — just works!
3. Select any model from the dropdown (local + cloud)

### VS Code (Continue, Cline)

```json
{
  "models": [{
    "title": "Candela Local",
    "provider": "openai",
    "apiBase": "http://localhost:1234/v1",
    "model": "llama3.2:3b"
  }]
}
```

### curl

```bash
curl http://localhost:1234/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2:3b",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| "model not found locally and no remote server configured" | Solo Mode + cloud model | Add `remote` to config, or use a local model |
| "audience is required when remote is set" | Missing `audience` | Add IAP `audience` to `~/.candela.yaml` |
| Traces card shows "Traces not available" | Not in Solo Mode (Team Mode traces go to cloud) | Expected — check the cloud dashboard |
| No models in `/v1/models` | Runtime not started | Start Ollama: `ollama serve` |
