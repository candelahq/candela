# 🔀 LLM API Proxy

The Candela LLM Proxy is a transparent reverse proxy that adds instant observability, token metering, and cost calculation to any LLM application.

## 🚀 Quick Configuration

To use the proxy, update your LLM client's `base_url` (or `api_base`).

### 1. OpenAI
- **Endpoint**: `http://localhost:8181/proxy/openai/v1`
- **Upstream**: `https://api.openai.com`
- **Auth**: Standard `Authorization: Bearer sk-...` header.

### 2. Google Gemini (Native API)
- **Endpoint**: `http://localhost:8181/proxy/google/`
- **Upstream**: `https://generativelanguage.googleapis.com`
- **Auth**: API Key (usually in query param `?key=...`).

### 3. Gemini via OpenAI-Compatible API
For clients that speak OpenAI format (like **Zed**).
- **Endpoint**: `http://localhost:8181/proxy/gemini-oai/v1`
- **Upstream**: `https://generativelanguage.googleapis.com/v1beta/openai`
- **Auth**: API Key via `Authorization: Bearer YOUR_GEMINI_KEY` header.
- **Models**: `gemini-2.5-pro`, `gemini-2.5-flash`, etc.

### 4. Anthropic (via Vertex AI)
Candela routes Anthropic through **Google Cloud Vertex AI** with automatic format translation and auth.
- **Endpoint**: `http://localhost:8181/proxy/anthropic/v1`
- **Upstream**: `https://{REGION}-aiplatform.googleapis.com` (configurable)
- **Auth**: **Automatic** — Candela injects GCP credentials from Application Default Credentials (ADC).
- **Format**: Candela translates OpenAI Chat Completions ↔ Anthropic Messages format automatically.
- **Models**: `claude-sonnet-4-20250514`, `claude-opus-4-20250514`, `claude-3-5-sonnet-20241022`, etc.
- **Display**: Model names are cleaned for the UI (e.g., `claude-sonnet-4-20250514` → `claude-sonnet-4`).

---

## 🖥️ Zed Integration

Zed connects directly to `localhost` — no tunnel needed.

### Prerequisites

1. **Candela running locally**: `nix develop -c go run ./cmd/candela-server`
2. **GCP ADC** (for Anthropic/Claude): `candela auth login` (or `gcloud auth application-default login`)

### Setup

Add the following to your Zed settings (`~/.config/zed/settings.json`):

#### Anthropic (Claude via Vertex AI)

```json
{
  "language_models": {
    "openai": {
      "api_url": "http://localhost:8181/proxy/anthropic/v1",
      "available_models": [
        {
          "name": "claude-sonnet-4-20250514",
          "display_name": "Claude Sonnet 4 (via Candela)",
          "max_tokens": 64000
        },
        {
          "name": "claude-opus-4-20250514",
          "display_name": "Claude Opus 4 (via Candela)",
          "max_tokens": 32000
        }
      ]
    }
  },
  "agent": {
    "default_model": {
      "provider": "openai",
      "model": "claude-sonnet-4-20250514"
    }
  }
}
```

#### Gemini

```json
{
  "language_models": {
    "openai": {
      "api_url": "http://localhost:8181/proxy/gemini-oai/v1",
      "available_models": [
        {
          "name": "gemini-2.5-pro",
          "display_name": "Gemini 2.5 Pro (via Candela)",
          "max_tokens": 65536
        },
        {
          "name": "gemini-2.5-flash",
          "display_name": "Gemini 2.5 Flash (via Candela)",
          "max_tokens": 65536
        }
      ]
    }
  },
  "agent": {
    "default_model": {
      "provider": "openai",
      "model": "gemini-2.5-pro"
    }
  }
}
```

### Setting the API Key

Launch Zed with the API key environment variable:

```bash
# For Anthropic (via Candela — key is a placeholder, ADC handles real auth):
OPENAI_API_KEY=candela open -a Zed

# For Gemini:
OPENAI_API_KEY=your-gemini-api-key open -a Zed

# For OpenAI:
OPENAI_API_KEY=sk-... open -a Zed
```

### Anthropic Prerequisites

1. Run `candela auth login` (or `gcloud auth application-default login`)
2. Set `vertex_ai.project_id` in `config.yaml` to your GCP project
3. Enable the Vertex AI API and request Claude model access in Model Garden

### Verify

Send a message in Zed's Agent Panel (`Cmd+Shift+A`). You should see:
- The response from the model in Zed
- A trace in the Candela dashboard at `http://localhost:3000`

---

## 💻 OpenCode Integration

[OpenCode](https://opencode.ai/) is an open-source terminal AI coding agent. It connects directly
to `localhost` — no tunnel needed.

### Prerequisites

1. **Candela running locally**: `nix develop -c go run ./cmd/candela-server`
2. **OpenCode installed**: `npm install -g opencode-ai` (or use `npx -y opencode-ai`)
3. **GCP ADC** (for Anthropic/Claude): `candela auth login` (or `gcloud auth application-default login`)

### Step 1: Create `opencode.json`

Create `opencode.json` in your project root (not `.opencode.json`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "candela-anthropic": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Claude via Candela (Vertex AI)",
      "options": {
        "baseURL": "http://localhost:8181/proxy/anthropic/v1"
      },
      "models": {
        "claude-sonnet-4-20250514": {
          "name": "Claude Sonnet 4"
        },
        "claude-opus-4-20250514": {
          "name": "Claude Opus 4"
        }
      }
    },
    "candela-gemini": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Gemini via Candela",
      "options": {
        "baseURL": "http://localhost:8181/proxy/gemini-oai/v1"
      },
      "models": {
        "gemini-2.5-pro": {
          "name": "Gemini 2.5 Pro"
        },
        "gemini-2.5-flash": {
          "name": "Gemini 2.5 Flash"
        }
      }
    },
    "candela-openai": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "OpenAI via Candela",
      "options": {
        "baseURL": "http://localhost:8181/proxy/openai/v1"
      },
      "models": {
        "gpt-4o": {
          "name": "GPT-4o"
        },
        "o3-mini": {
          "name": "o3-mini"
        }
      }
    }
  }
}
```

### Step 2: Register credentials via `/connect`

Launch OpenCode and register each provider:

```bash
npx -y opencode-ai
```

Inside the TUI:

1. Type `/connect`
2. Scroll to **"Other"** and select it
3. Enter provider ID: `candela-anthropic`
4. Enter API key: `candela` (placeholder — Candela injects ADC for Vertex AI)

Repeat for other providers if needed:
- Provider ID: `candela-gemini`, API key: your Gemini API key
- Provider ID: `candela-openai`, API key: your OpenAI `sk-...` key

### Step 3: Select a model

Type `/models` in the TUI and select a model under **"Claude via Candela (Vertex AI)"**,
**"Gemini via Candela"**, or **"OpenAI via Candela"**.

### Verify

Send a message in the OpenCode TUI. You should see:
- The response from the model in OpenCode
- A trace in the Candela dashboard at `http://localhost:3000`

### Anthropic Prerequisites

1. Run `candela auth login` (or `gcloud auth application-default login`)
2. Set `vertex_ai.project_id` in `config.yaml` to your GCP project
3. Enable the Vertex AI API and request Claude model access in Model Garden

---

## 💎 Gemini CLI Integration

[Gemini CLI](https://geminicli.com/) is Google's official terminal AI coding assistant. It supports custom API
endpoints via environment variables — route all traffic through Candela with two exports.

### Prerequisites

1. **Candela running locally**: `nix develop -c go run ./cmd/candela-server`
2. **Gemini CLI installed**: `npm install -g @google/gemini-cli` (or see [geminicli.com](https://geminicli.com/docs/get-started/installation))
3. **Gemini API key**: Required for `GEMINI_API_KEY`

### Setup

Set two environment variables in your shell profile (`~/.zshrc`, `~/.bashrc`, etc.):

```bash
# Route all Gemini CLI API traffic through Candela
export GOOGLE_GEMINI_BASE_URL="http://localhost:8181/proxy/google"

# Your Gemini API key (forwarded by Candela to upstream)
export GEMINI_API_KEY="your-gemini-api-key"
```

That's it. Launch `gemini` as usual — every request is now proxied through Candela.

> **💡 Why does this work?** Gemini CLI uses `GOOGLE_GEMINI_BASE_URL` to override the default
> `https://generativelanguage.googleapis.com` endpoint. Candela's `/proxy/google/` route speaks
> the **native Gemini API format** — no translation needed. The CLI allows plain `http://` for
> `localhost` / `127.0.0.1` / `[::1]`, so no TLS is required for local proxying.

### Optional: Export Gemini CLI Telemetry to Candela

Gemini CLI has built-in OTLP telemetry support. You can point it at Candela's OTel Collector to
get **client-side spans** (tool use, session duration) alongside **proxy spans** (tokens, cost, TTFB):

```bash
# Enable Gemini CLI's native telemetry export
export GEMINI_TELEMETRY_ENABLED=true
export GEMINI_TELEMETRY_TARGET=local
export GEMINI_TELEMETRY_USE_COLLECTOR=true
export GEMINI_TELEMETRY_OTLP_ENDPOINT="http://localhost:4318"
export GEMINI_TELEMETRY_OTLP_PROTOCOL=http
```

This gives you end-to-end visibility: client-side session spans correlated with server-side
LLM proxy spans in a single trace tree.

### Verify

1. Run `gemini` in any project directory
2. Send a prompt — you should see the response in your terminal
3. Check the Candela dashboard at `http://localhost:3000` for a new trace

### Using with Anthropic (Claude via Vertex AI)

Gemini CLI natively speaks the Gemini API. To use **Claude** through Gemini CLI, you would need a
separate tool (see [OpenCode Integration](#-opencode-integration) or [Zed Integration](#️-zed-integration))
since Gemini CLI does not support OpenAI-compatible endpoints.

---

## 🤖 Google ADK Integration

Route [ADK](https://adk.dev/) agent LLM calls through Candela with one line:

```python
from google.adk.agents import Agent
from google.adk.models import Gemini

agent = Agent(
    model=Gemini(
        model="gemini-2.0-flash",
        base_url="http://localhost:8080/proxy/google",  # → candela-sidecar
    ),
    name="my_agent",
    instruction="You are a helpful assistant.",
)
```

For full observability with unified OTel traces (agent DAG + proxy spans in one trace tree), see the complete [ADK Integration Guide](adk-integration.md).

---

## 🛠️ Advanced Proxy Config

Configuration is done via `config.yaml`:

```yaml
proxy:
  enabled: true
  project_id: "my-gcp-project"

  # Vertex AI (required for Anthropic)
  vertex_ai:
    project_id: "my-gcp-project"
    region: "us-central1"
    caching_mode: "auto"  # off | auto | system-only

  # Selective provider activation — only listed providers are registered.
  # If omitted or empty, all providers are enabled.
  # Valid values: openai, google, anthropic, gemini-oai
  providers:
    - openai
    - google
    - anthropic
    - gemini-oai
```

---

## 🗃️ Anthropic Prompt Caching

Anthropic's [prompt caching](https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching)
reduces costs up to **90%** and latency up to **85%** for multi-turn conversations by caching
the system prompt and conversation prefix on Anthropic's servers.

Candela automatically injects `cache_control` breakpoints into translated Anthropic requests.
This is controlled by the **caching mode**.

### Caching Modes

| Mode | System Prompt | Last User Message | Best For |
|------|:---:|:---:|---|
| `auto` (default) | ✅ cached | ✅ cached | Most use cases — maximum cost savings |
| `system-only` | ✅ cached | ❌ not cached | Frequently changing conversation history |
| `off` | ❌ no caching | ❌ no caching | Debugging, or when you manage caching yourself |

### Configuration

#### Via `config.yaml` (sidecar)

```yaml
vertex_ai:
  project: "my-project"
  region: "us-central1"
  caching_mode: "auto"  # off | auto | system-only
```

#### Via `config.yaml` (Cloud Run server)

```yaml
proxy:
  vertex_ai:
    project_id: "my-project"
    region: "us-central1"
    caching_mode: "auto"  # off | auto | system-only
```

### Runtime Toggling (No Restart Required)

The caching mode can be changed at runtime via the sidecar's management API:

```bash
# Check current mode
curl http://localhost:8080/_local/api/config

# Switch to system-only
curl -X POST http://localhost:8080/_local/api/config/caching \
  -H "Content-Type: application/json" \
  -d '{"anthropic": "system-only"}'

# Disable caching
curl -X POST http://localhost:8080/_local/api/config/caching \
  -H "Content-Type: application/json" \
  -d '{"anthropic": "off"}'
```

The Candela Desktop app also provides a segmented control in **Settings → Performance**
to toggle between modes with immediate effect.

### Per-Request Override via Header

Clients can override the server's default caching mode on individual requests
using the `X-Candela-Caching` header:

```bash
# Force caching off for this one request
curl http://localhost:8080/proxy/anthropic/v1/chat/completions \
  -H "X-Candela-Caching: off" \
  -H "Content-Type: application/json" \
  -d '{"model": "claude-sonnet-4-20250514", "messages": [...]}'
```

Valid header values: `off`, `auto`, `system-only`.

The header is stripped before forwarding to the upstream provider and does not
persist — the server's configured default is restored after each request.

> **💡 Team Mode**: In team deployments (Desktop → Cloud Run → Vertex AI), the
> `X-Candela-Caching` header lets each developer control their own caching
> strategy without affecting the server's default for other users.

### How It Works

When caching is enabled, Candela injects `cache_control: {"type": "ephemeral"}`
breakpoints into the translated Anthropic request:

1. **System prompt breakpoint** — caches the system instructions (both `auto` and `system-only`)
2. **Last user message breakpoint** — caches the conversation prefix (`auto` only)

This follows [Anthropic's recommended two-breakpoint pattern](https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching#cache-limitations)
for maximum cache reuse across turns.

### Backward Compatibility

- `caching_mode: "auto"` is the default (caching ON) — no config change needed
- The old `disable_prompt_caching: true` is equivalent to `caching_mode: "off"`
- Boolean values (`true`/`false`) are accepted: `true` → `auto`, `false` → `off`

---

## 📡 What is Captured?

For every request, Candela creates a **span** containing:
- **Attributes**: `llm.model`, `llm.provider`, `llm.usage.total_tokens`, `candela.cost_usd`.
- **Events**: The full `request.body` and `response.body`.
- **Status**: Error rates and latency metrics.

### SSE Streaming
If your request specifies `stream: true`, Candela will:
1. Proxy the streaming response to your client immediately (minimal latency impact).
2. Buffer the chunks in memory.
3. Once the stream ends, it parses the buffered chunks to extract final token usage and completion content.
4. Asynchronously saves the trace to the backend.

### Format Translation (Anthropic)
For translated providers, Candela:
1. Translates the OpenAI-format request to Anthropic Messages format.
2. Rewrites the URL path for Vertex AI's project-scoped endpoint.
3. Injects ADC credentials automatically.
4. Translates the Anthropic response back to OpenAI format before returning to the client.
5. Observability spans capture the **raw upstream** data for accurate token counting.

---

## 🔐 Security & Auth

- **OpenAI / Gemini**: The proxy **forwards** your existing `Authorization` header to the upstream provider. It does not store keys.
- **Anthropic (Vertex AI)**: The proxy **injects** a GCP access token from Application Default Credentials. The token auto-refreshes — no manual token management needed.
- For Anthropic, ensure your local environment has `GOOGLE_APPLICATION_CREDENTIALS` or you are authenticated via `candela auth login` (or `gcloud auth application-default login`).
