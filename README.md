# Candela 🕯️

**Open-source, OTel-native LLM observability platform.**

Candela measures the light in your LLM applications — traces, costs, latency, and quality — with deep integration into OpenTelemetry, Google ADK, and the wider AI ecosystem.

## Features

- **OTel-native**: OTLP is the only ingestion protocol. No proprietary SDKs needed.
- **Proto-first**: All service boundaries defined by Protobuf. ConnectRPC for frontend, gRPC for backend.
- **Pluggable storage**: ClickHouse (default), BigQuery, PostgreSQL.
- **Google-first**: Deep ADK, Cloud Trace, Vertex AI integration — but works anywhere.
- **Zero-code instrumentation**: Leverage existing OpenTelemetry GenAI instrumentation libraries.
- **LLM-first, traces-aware**: Optimized for GenAI spans, with full distributed trace context.

## Architecture

```
Your Apps (ADK, LangChain, OpenAI, CrewAI, ...)
    │ OTel SDK (auto-instrumented)
    ▼
Candela OTel Collector (custom distro)
    │ OTLP/gRPC
    ▼
Candela Backend (Go)
    │ ConnectRPC
    ▼
Candela Web UI (Next.js)
```

## Quick Start

```bash
# Prerequisites: Docker, Docker Compose

# Start all services
docker compose -f deploy/docker-compose.yml up

# Send sample traces
# (instrument your app with any openinference-instrumentation-* library)

# Query traces
grpcurl -plaintext localhost:8080 candela.v1.TraceService/ListTraces
```

## Development

```bash
# Enter the nix dev shell
nix develop

# Generate protobuf code
cd proto && buf generate

# Run the backend
go run ./cmd/candela-server

# Run the worker
go run ./cmd/candela-worker
```

## Project Structure

```
candela/
├── proto/           # Protobuf definitions (source of truth)
├── gen/             # Generated code (Go, TypeScript, Python)
├── cmd/             # Binary entry points
├── pkg/             # Go library packages
├── collector/       # Custom OTel Collector distro
├── web/             # Next.js UI
├── eval/            # Python eval engine
└── deploy/          # Docker Compose, Helm, Terraform
```

## License

TBD (Apache 2.0 planned)
