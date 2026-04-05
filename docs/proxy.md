# 🔀 LLM API Proxy

The Candela LLM Proxy is a transparent reverse proxy that adds instant observability, token metering, and cost calculation to any LLM application.

## 🚀 Quick Configuration

To use the proxy, update your LLM client's `base_url` (or `api_base`).

### 1. OpenAI
- **Endpoint**: `http://localhost:8080/proxy/openai/v1`
- **Upstream**: `https://api.openai.com`
- **Auth**: Standard `Authorization: Bearer sk-...` header.

### 2. Google Gemini (Native API)
- **Endpoint**: `http://localhost:8080/proxy/google/`
- **Upstream**: `https://generativelanguage.googleapis.com`
- **Auth**: API Key (usually in query param `?key=...`).

### 3. Gemini via OpenAI-Compatible API
For clients that only speak OpenAI format (like **Cursor**).
- **Endpoint**: `http://localhost:8080/proxy/gemini-oai/v1`
- **Upstream**: `https://generativelanguage.googleapis.com/v1beta/openai`
- **Auth**: API Key via `Authorization: Bearer YOUR_GEMINI_KEY` header.
- **Models**: `gemini-2.5-pro`, `gemini-2.5-flash`, etc.

### 4. Anthropic (via Vertex AI)
Candela routes Anthropic through **Google Cloud Vertex AI** with automatic format translation and auth.
- **Endpoint**: `http://localhost:8080/proxy/anthropic/v1`
- **Upstream**: `https://{REGION}-aiplatform.googleapis.com` (configurable)
- **Auth**: **Automatic** — Candela injects GCP credentials from Application Default Credentials (ADC).
- **Format**: Candela translates OpenAI Chat Completions ↔ Anthropic Messages format automatically.
- **Models**: `claude-sonnet-4-20250514`, `claude-opus-4-20250514`, `claude-3-5-sonnet-20241022`, etc.
- **Display**: Model names are cleaned for the UI (e.g., `claude-sonnet-4-20250514` → `claude-sonnet-4`).

---

## 🖱️ Cursor Integration

Cursor's BYOK mode speaks OpenAI Chat Completions format, so use these routes:

### Setup

1. **Start Candela**: `nix develop -c go run ./cmd/candela-server`
2. **Open Cursor Settings → Models**

### OpenAI Models
| Setting | Value |
|---------|-------|
| Base URL | `http://localhost:8080/proxy/openai/v1` |
| API Key | Your OpenAI `sk-...` key |

### Gemini Models
| Setting | Value |
|---------|-------|
| Base URL | `http://localhost:8080/proxy/gemini-oai/v1` |
| API Key | Your Gemini API key (from AI Studio) |
| Model | `gemini-2.5-pro`, `gemini-2.5-flash`, etc. |

### Anthropic (Claude) Models
| Setting | Value |
|---------|-------|
| Base URL | `http://localhost:8080/proxy/anthropic/v1` |
| API Key | `candela` (placeholder — ADC is auto-injected) |
| Model | `claude-sonnet-4-20250514`, `claude-opus-4-20250514`, etc. |

**Prerequisites for Anthropic**:
1. Run `gcloud auth application-default login`
2. Set `vertex_ai.project_id` in `config.yaml` to your GCP project
3. Enable the Vertex AI API and request Claude model access in Model Garden

> **⚠️ Important**: In Cursor, go to **Settings → Network → HTTP Compatibility Mode** and set it to **HTTP/1.1** for reliable localhost proxy connections.

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
- For Anthropic, ensure your local environment has `GOOGLE_APPLICATION_CREDENTIALS` or you are authenticated via `gcloud auth application-default login`.
