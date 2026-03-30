# 🔀 LLM API Proxy

The Candela LLM Proxy is a transparent reverse proxy that adds instant observability, token metering, and cost calculation to any LLM application.

## 🚀 Quick Configuration

To use the proxy, update your LLM client's `base_url` (or `api_base`).

### 1. OpenAI
- **Endpoint**: `http://localhost:8080/proxy/openai/v1`
- **Upstream**: `https://api.openai.com`
- **Auth**: Standard `Authorization: Bearer sk-...` header.

### 2. Google Gemini
- **Endpoint**: `http://localhost:8080/proxy/google/`
- **Upstream**: `https://generativelanguage.googleapis.com`
- **Auth**: API Key (usually in query param `?key=...`).

### 3. Anthropic (via Vertex AI)
Candela defaults to routing Anthropic through **Google Cloud Vertex AI** for enterprise-grade security and reliability.
- **Endpoint**: `http://localhost:8080/proxy/anthropic/`
- **Upstream**: `https://us-central1-aiplatform.googleapis.com` (configurable)
- **Auth**: **GCP Bearer token** (ADC). The proxy expects a standard `Authorization: Bearer $(gcloud auth print-access-token)` header.

---

## 🛠️ Advanced Proxy Config

Configuration is done via `config.yaml`:

```yaml
proxy:
  enabled: true
  project_id: "my-gcp-project" # Required for Anthropic (Vertex AI)
  # Override default providers if needed
  providers:
    - name: "anthropic"
      upstream: "https://europe-west1-aiplatform.googleapis.com"
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

---

## 🔐 Security & Auth

The proxy **does not store** your API keys. It forwards your existing `Authorization` or `X-Api-Key` headers directly to the upstream provider while using them to identify the request/project internally.

For Anthropic (Vertex AI), ensure your local environment has `GOOGLE_APPLICATION_CREDENTIALS` or you are authenticated via `gcloud`.
