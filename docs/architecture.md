# 🏗️ Architecture Architecture

Candela is designed to be an **OTel-native LLM observability platform**. This means every piece of data captured internally is stored as an OpenTelemetry-compatible span, even if it comes from the LLM Proxy.

## 1. Dual-Mode Ingestion

### A. LLM API Proxy (Zero-Code)
The proxy is the fastest way to get visibility. It acts as a transparent middleman between your app and the provider.
- **How it works**: Your app sends requests to `candela:8080/proxy/{provider}` instead of the direct provider URL.
- **Observability**: Candela captures the request/response, extracts tokens, calculates cost, and creates a "span" in the background before forwarding the response.
- **Streaming**: For SSE streaming (`stream: true`), Candela uses a tee-like buffer to forward chunks immediately to keep latency low while still capturing the full completion.

### B. OTel Agent Mode (Production-Grade)
For complex apps using frameworks like **ADK** or **LangChain**, you want to see the *entire* trace (e.g., retrieval → embedding → LLM call → tool execution).
- **How it works**: Use standard OpenTelemetry instrumentation libraries (like `openinference`). Point them to the Candela OTel Collector.
- **GenAI Processor**: Candela includes a custom OTel Collector distro that enriches generic spans with token-to-USD pricing in real-time.

---

## 2. Server Internals

### Single-Binary Backend
To simplify self-hosting, Candela merges the ingestion worker and the query API into one Go process.
- **Span Processor**: Incoming spans (from the Proxy or Collector) go into a high-speed memory channel.
- **Batching**: Spans are batched by count or time before being flushed to the permanent `TraceStore`.
- **ConnectRPC**: The API layer uses ConnectRPC, which supports gRPC, gRPC-Web, and a pure JSON/HTTP protocol on a single port.

---

## 3. Storage Backends

Candela uses a pluggable `TraceStore` interface:

| Backend | Best For | Status |
|---------|----------|--------|
| **SQLite** | Local dev, small teams, easy setup | ✅ Implemented |
| **ClickHouse** | High-volume production, deep analytics | ✅ Implemented |
| **BigQuery** | Managed cloud infrastructure, GCP teams | 📋 Phase 4 |
| **PostgreSQL** | General purpose, medium scale | 📋 Phase 4 |

### SQLite Schema
For local dev, SQLite is used with a JSONB-like approach to store span attributes, making it extremely flexible for evolving OTel standards.

---

## 4. OTel Collector Distro

Candela provides a custom OTel Collector distribution (`candela-collector`). It includes the **`genai` processor** which:
1. Detects LLM-specific spans.
2. Extracts model names and token counts.
3. Injects `candela.cost_usd` attribute using a built-in pricing table.
4. Forwards the enriched spans to the Candela backend.
