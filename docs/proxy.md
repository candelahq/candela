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
For clients that only speak OpenAI format (like **Cursor**).
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

## 🖱️ Cursor 3 Integration

Cursor 3 routes API requests through their cloud infrastructure, which means it
**cannot connect to `localhost` directly** (SSRF protection). You must expose
Candela via a public tunnel.

### Prerequisites

1. **Candela running locally**: `nix develop -c go run ./cmd/candela-server`
2. **Cloudflare tunnel** (included in the nix dev shell):

```bash
# Open a second terminal — this creates a temporary public URL
nix develop -c cloudflared tunnel --url http://localhost:8181
```

Cloudflared will print a URL like:
```
https://random-words-here.trycloudflare.com
```

Copy that URL — you'll use it as the base for Cursor's model settings below.

> **⚠️ Security**: This exposes your Candela proxy to the public internet.
> Since API keys are forwarded from the client (not stored), your keys aren't
> at risk. But anyone who discovers the URL could route traffic through your
> proxy. For daily-driver use, consider a [named Cloudflare tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)
> with access policies, or deploy Candela to **Cloud Run** for a stable endpoint.

### Cursor Settings

Open **Cursor Settings → Models** and configure your providers:

#### OpenAI Models
| Setting | Value |
|---------|-------|
| Override OpenAI Base URL | `https://<tunnel-url>/proxy/openai/v1` |
| API Key | Your OpenAI `sk-...` key |
| Models | `gpt-4o`, `gpt-4.1`, `o3-mini`, etc. |

#### Gemini Models
| Setting | Value |
|---------|-------|
| Override OpenAI Base URL | `https://<tunnel-url>/proxy/gemini-oai/v1` |
| API Key | Your Gemini API key (from AI Studio) |
| Models | `gemini-2.5-pro`, `gemini-2.5-flash`, etc. |

#### Anthropic (Claude) Models
| Setting | Value |
|---------|-------|
| Override OpenAI Base URL | `https://<tunnel-url>/proxy/anthropic/v1` |
| API Key | `candela` (placeholder — ADC is auto-injected by Candela) |
| Models | `claude-sonnet-4-20250514`, `claude-opus-4-20250514`, etc. |

**Anthropic prerequisites**:
1. Run `gcloud auth application-default login`
2. Set `vertex_ai.project_id` in `config.yaml` to your GCP project
3. Enable the Vertex AI API and request Claude model access in Model Garden

### Network Settings

In **Cursor Settings → Network**, set **HTTP Compatibility Mode** to **HTTP/1.1**
for reliable streaming through the tunnel.

### Known Limitations

- **Agent Mode**: Cursor 3's Agent Mode sends requests in OpenAI's **Responses
  API** format (`input` field instead of `messages`), which differs from the
  standard Chat Completions format. Candela does not yet translate this format.
  Use **Ask** or **Plan** mode for full observability. Agent Mode support is on
  the roadmap.
- **Override is global**: Cursor's "Override OpenAI Base URL" applies globally —
  you can only point one provider route at a time unless you use separate Cursor
  profiles.
- **Tunnel URL changes on restart**: The Cloudflare quick tunnel URL is
  temporary. You'll need to update Cursor settings each time you restart the
  tunnel. Use a named tunnel or Cloud Run deployment for a stable URL.

### Alternative: Cloud Run Deployment

For a stable, always-on proxy without tunnels, deploy Candela to Google Cloud Run:

```bash
# Build and deploy (uses deploy/Dockerfile.server)
gcloud run deploy candela \
  --source . \
  --dockerfile deploy/Dockerfile.server \
  --region us-central1 \
  --allow-unauthenticated
```

This gives you a permanent `https://candela-<hash>.run.app` URL. Cloud Run's
service account handles Vertex AI auth automatically — no ADC setup needed.

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
