# 📡 OTel Collector — Agent-Native Ingestion

Candela ships a **custom OpenTelemetry Collector distribution** that enriches LLM trace spans with cost data and forwards them to the Candela backend. This is the recommended path for deep observability into agent frameworks like **Google ADK**, **LangChain**, **CrewAI**, and any OTel-instrumented application.

## When to Use the Collector

| Ingestion Mode | Best For | Complexity |
|---------------|----------|------------|
| **Proxy Mode** (`/proxy/*`) | Quick start, single LLM call observability | Zero-code |
| **Collector Mode** (OTLP) | Agent frameworks, multi-span traces, DAG visualization | Requires OTel SDK |

Use the Collector when you need:
- **Multi-span traces** — see the full agent DAG (planner → tool calls → LLM → retrieval)
- **Framework integration** — ADK, LangChain, and CrewAI emit OTel spans natively
- **Custom attributes** — attach business context to spans (user, session, experiment)
- **Dual-write** — export to Candela AND Google Cloud Trace simultaneously

> **💡 Candela can also _export_ OTLP**: In addition to _receiving_ spans via the Collector, Candela can _forward_ its traces as standard OTLP to any OTel-compatible backend (Datadog, Grafana Tempo, Jaeger, etc.) using the built-in OTLP export sink. See [architecture.md](architecture.md#otlp-export-sink-only) for configuration.

---

## Architecture

```
┌──────────────────┐     OTLP/gRPC or HTTP
│  Your App/Agent  │────────────────────────┐
│  (OTel SDK)      │                        │
└──────────────────┘                        ▼
                                  ┌──────────────────┐
                                  │ Candela Collector │
                                  │  ┌──────────────┐ │
                                  │  │ OTLP Receiver │ │  ← :4317 (gRPC) / :4318 (HTTP)
                                  │  └──────┬───────┘ │
                                  │         ▼         │
                                  │  ┌──────────────┐ │
                                  │  │ GenAI        │ │  ← cost enrichment
                                  │  │ Processor    │ │
                                  │  └──────┬───────┘ │
                                  │         ▼         │
                                  │  ┌──────────────┐ │
                                  │  │ Batch        │ │  ← 256 spans / 5s
                                  │  │ Processor    │ │
                                  │  └──────┬───────┘ │
                                  │     ┌───┴───┐     │
                                  │     ▼       ▼     │
                                  │  ┌─────┐ ┌─────┐  │
                                  │  │OTLP │ │Debug│  │  ← export to Candela + stdout
                                  │  └─────┘ └─────┘  │
                                  └──────────────────┘
                                         │
                                         ▼ OTLP/gRPC
                                  ┌──────────────────┐
                                  │ Candela Backend   │
                                  │ (IngestionService)│
                                  └──────────────────┘
```

---

## Building the Collector

The custom collector is built using the [OTel Collector Builder](https://opentelemetry.io/docs/collector/custom-collector/) (`ocb`).

### Prerequisites
- Go 1.26+ (available via `nix develop`)

### Build

```bash
# Install the builder
go install go.opentelemetry.io/collector/cmd/builder@latest

# Build the custom collector binary
cd collector
builder --config=builder.yaml
```

This produces `../cmd/candela-collector/candela-collector` — a single binary with the GenAI processor baked in.

### What's Included

The builder config (`collector/builder.yaml`) bundles:

| Component | Type | Purpose |
|-----------|------|---------|
| `otlpreceiver` | Receiver | Accepts OTLP spans via gRPC (`:4317`) and HTTP (`:4318`) |
| `batchprocessor` | Processor | Batches spans (256 per batch, 5s timeout) |
| `genaiprocessor` | Processor | **Custom** — enriches GenAI spans with USD cost |
| `otlpexporter` | Exporter | Forwards to Candela backend (gRPC) |
| `otlphttpexporter` | Exporter | Forwards to Candela backend (HTTP) |
| `debugexporter` | Exporter | Logs spans to stdout for dev |
| `zpagesextension` | Extension | Collector diagnostics UI |

---

## GenAI Processor

The custom `genaiprocessor` (`collector/processors/genaiprocessor/processor.go`) enriches spans in the OTel pipeline:

### What It Does

1. **Identifies GenAI spans** by checking for `gen_ai.system` or `gen_ai.request.model` attributes
2. **Extracts token counts** from `gen_ai.usage.input_tokens` and `gen_ai.usage.output_tokens`
3. **Calculates cost** using the shared `pkg/costcalc` engine
4. **Injects** `gen_ai.usage.cost_usd` attribute into the span

### OTel Semantic Conventions Used

| Attribute | Description | Example |
|-----------|-------------|---------|
| `gen_ai.system` | LLM provider identifier | `openai`, `google`, `anthropic` |
| `gen_ai.request.model` | Model name | `gpt-4o`, `gemini-2.5-pro` |
| `gen_ai.usage.input_tokens` | Prompt tokens | `150` |
| `gen_ai.usage.output_tokens` | Completion tokens | `42` |
| `gen_ai.usage.cost_usd` | **Enriched** — calculated cost | `0.0023` |

The processor is idempotent — if `gen_ai.usage.cost_usd` is already present, it skips the span.

---

## Running the Collector

### Local Development

```bash
# Start the collector with the dev config
./candela-collector --config=collector/collector-config.yaml
```

The default config (`collector/collector-config.yaml`) exports to:
- **Candela backend** at `localhost:8181` (OTLP/gRPC, insecure)
- **Debug output** (stdout, basic verbosity)

### Production

For production, update the export endpoint and optionally add Cloud Trace:

```yaml
exporters:
  otlp/candela:
    endpoint: "your-candela-server:8181"
    tls:
      insecure: false
      # Or use mTLS / GCP auth

  # Optional: dual-write to Google Cloud Trace
  otlp/cloudtrace:
    endpoint: "cloudtrace.googleapis.com:443"

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [genai, batch]
      exporters: [otlp/candela, otlp/cloudtrace]
```

---

## Instrumenting Your Application

### Google ADK (Agent Development Kit)

ADK emits OTel spans natively. For the complete integration guide — including proxy-mode trace correlation that nests proxy spans under ADK agent spans — see [ADK Integration](adk-integration.md).

Quick setup using environment variables:

```bash
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="http://localhost:4318/v1/traces"
export OTEL_SERVICE_NAME="my-adk-agent"
export OTEL_SEMCONV_STABILITY_OPT_IN="gen_ai_latest_experimental"
adk web path/to/agent/
```

Or configure programmatically:

```python
import opentelemetry
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

# Point at Candela Collector
exporter = OTLPSpanExporter(endpoint="http://localhost:4317", insecure=True)
provider = TracerProvider()
provider.add_span_processor(BatchSpanProcessor(exporter))
opentelemetry.trace.set_tracer_provider(provider)

# ADK will now emit spans to Candela
from google.adk.agents import Agent
from google.adk.models import Gemini

agent = Agent(model=Gemini(model="gemini-2.5-pro"), ...)
```

### LangChain

```python
from langchain.callbacks.tracers import OpenTelemetryTracer

tracer = OpenTelemetryTracer()  # Uses the global TracerProvider
chain.invoke({"input": "..."}, config={"callbacks": [tracer]})
```

### Generic OTel SDK (Go)

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

exporter, _ := otlptracegrpc.New(ctx,
    otlptracegrpc.WithEndpoint("localhost:4317"),
    otlptracegrpc.WithInsecure(),
)
tp := sdktrace.NewTracerProvider(
    sdktrace.WithBatcher(exporter),
)
otel.SetTracerProvider(tp)
```

### Manual Span Attributes

For custom applications, ensure your spans include GenAI attributes:

```python
from opentelemetry import trace

tracer = trace.get_tracer("my-app")
with tracer.start_as_current_span("llm.completion") as span:
    span.set_attribute("gen_ai.system", "openai")
    span.set_attribute("gen_ai.request.model", "gpt-4o")
    # ... make LLM call ...
    span.set_attribute("gen_ai.usage.input_tokens", 150)
    span.set_attribute("gen_ai.usage.output_tokens", 42)
    # gen_ai.usage.cost_usd will be added by the GenAI Processor
```

---

## Collector vs. Standard `otelcol`

You can use the standard OpenTelemetry Collector (`otelcol`) without the custom GenAI processor. Spans will still be ingested, but **cost enrichment will not happen at the collector level** — the Candela backend will calculate costs instead.

```bash
# Using the standard collector (no cost enrichment in pipeline)
otelcol --config=collector/collector-config.yaml

# Using the Candela custom collector (with cost enrichment)
./candela-collector --config=collector/collector-config.yaml
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| No spans appearing in Candela | Collector not connected | Check `otlp/candela` endpoint and TLS settings |
| Cost is $0.00 on all spans | Unknown model in pricing table | Add model to `pricing:` config or `loadDefaults()` — check server logs for `⚠️ missing pricing` warnings |
| `gen_ai.usage.cost_usd` missing | Span lacks `gen_ai.system` attribute | Ensure your SDK sets GenAI semantic conventions |
| High memory usage | Batch too large | Reduce `send_batch_size` in batch processor config |
| Connection refused on :4317 | Collector not running | Start with `./candela-collector --config=...` |
