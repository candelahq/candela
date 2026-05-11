# Candela Functional Test Suite

Language-agnostic HTTP tests using [Hurl](https://hurl.dev). Runs against any
binary that speaks the Candela HTTP API — Go or Rust.

## Quick Start

```bash
# 1. Start the binary under test (example: Go candela-local)
./candela-local --config config.yaml &

# 2. Start a mock upstream LLM (see below)
# The mock server listens on :9999 by default.

# 3. Run all tests
hurl --test \
  --variable CANDELA_URL=http://localhost:8080 \
  --variable MOCK_UPSTREAM_URL=http://localhost:9999 \
  test/functional/**/*.hurl
```

## Structure

```
test/functional/
├── README.md               ← you are here
├── run.sh                  ← convenience runner script
├── mock/
│   └── upstream.go         ← tiny Go mock LLM server (stateless echo)
├── proxy/
│   ├── proxy_openai.hurl   ← PROXY-01: OpenAI round-trip
│   ├── proxy_anthropic.hurl← PROXY-02: Anthropic/Vertex AI path + translation
│   ├── proxy_streaming.hurl← PROXY-03: SSE streaming, [DONE] terminator
│   └── proxy_errors.hurl   ← PROXY-04/05: 413, unknown provider, 4xx shapes
├── billing/
│   ├── budget_gate.hurl    ← BILLING-01/02: budget exhausted → 402
│   ├── pricing_gate.hurl   ← BILLING-03/04: unknown model, local bypass
│   └── rate_limit.hurl     ← BILLING-05: 429 + Retry-After
├── compat/
│   └── compat_routes.hurl  ← COMPAT-01..05: /v1/models, alias routing
└── security/
    └── header_security.hurl← SEC-01..05: ID injection, X-User-Id bypass
```

## Variables

| Variable | Default | Description |
|---|---|---|
| `CANDELA_URL` | `http://localhost:8080` | Base URL of the binary under test |
| `MOCK_UPSTREAM_URL` | `http://localhost:9999` | Mock LLM upstream URL |

Pass via `--variable KEY=VALUE` or export as env vars (Hurl picks them up automatically when prefixed with `HURL_`):

```bash
export HURL_CANDELA_URL=http://localhost:8080
export HURL_MOCK_UPSTREAM_URL=http://localhost:9999
hurl --test test/functional/**/*.hurl
```

## Mock Upstream

The `mock/upstream.go` is a minimal echo server that mimics LLM API responses:

```bash
# In a separate terminal
cd test/functional/mock
go run upstream.go          # listens on :9999
```

It responds to:
- `POST /v1/chat/completions` — OpenAI-shaped response (gpt-4o)
- `POST /v1/messages` — Anthropic-shaped response (claude)
- `POST /v1/projects/*/locations/*/publishers/anthropic/models/*:rawPredict` — Vertex AI path
- `POST /v1/projects/*/locations/*/publishers/anthropic/models/*:streamRawPredict` — Vertex AI streaming

## Running Against Go vs Rust

```bash
# Go (candela-local)
HURL_CANDELA_URL=http://localhost:8080 hurl --test test/functional/**/*.hurl

# Rust (candela-sidecar)
HURL_CANDELA_URL=http://localhost:8181 hurl --test test/functional/**/*.hurl
```

## CI Integration

GitHub Actions uses `--report-junit` to surface results in the PR check:

```yaml
- name: Run functional tests
  run: |
    hurl --test \
      --report-junit test-results/functional.xml \
      --variable CANDELA_URL=http://localhost:8080 \
      test/functional/**/*.hurl
```
