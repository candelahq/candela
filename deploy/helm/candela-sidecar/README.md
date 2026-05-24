# Transparent Proxy & eBPF Enforcement

> Kernel-level LLM governance enforcement for Kubernetes workloads.

## Quick Start

### Prerequisites

- Kubernetes cluster with [Cilium](https://cilium.io/) CNI (for FQDN policies)
- [Tetragon](https://tetragon.io/) installed (for kprobe enforcement — optional)
- Helm 3.x

### Deploy the Sidecar

```bash
# Basic: transparent proxy only (recommended starting point).
helm install candela-sidecar deploy/helm/candela-sidecar/ \
  --set transparent.enabled=true \
  --set iptables.enabled=true \
  --set gcpProject=my-project

# With Cilium FQDN enforcement (soft egress filtering).
helm install candela-sidecar deploy/helm/candela-sidecar/ \
  --set transparent.enabled=true \
  --set iptables.enabled=true \
  --set enforcement.fqdn.enabled=true \
  --set gcpProject=my-project

# Hard enforcement only (no transparent proxy — Tetragon SIGKILL).
# WARNING: Cannot be used with transparent.enabled=true.
helm install candela-sidecar deploy/helm/candela-sidecar/ \
  --set enforcement.tetragon.enabled=true \
  --set enforcement.tetragon.action=Sigkill \
  --set gcpProject=my-project
```

### Configuration

All enforcement is derived from the `providers[]` array in `values.yaml`:

```yaml
providers:
  - name: openai
    host: api.openai.com
    intercept: true
  - name: anthropic
    host: api.anthropic.com
    intercept: true
  - name: vertex-ai
    hostPattern: "*-aiplatform.googleapis.com"
    intercept: true
  - name: google
    host: generativelanguage.googleapis.com
    intercept: true
```

This single config generates:
1. **SNI map** — transparent proxy routing table
2. **FQDNNetworkPolicy** — Cilium L7 egress rules
3. **TracingPolicy** — Tetragon kprobe enforcement
4. **iptables rules** — port 443 → 15001 redirect

## Architecture

```
┌─ Pod ────────────────────────────────────────────┐
│                                                  │
│  App Container          Sidecar Container        │
│  ┌──────────┐          ┌──────────────────┐      │
│  │ curl     │──:443──► │ Transparent      │      │
│  │ python   │          │ Listener (:15001)│      │
│  │ node     │          │   ↓ peek SNI     │      │
│  └──────────┘          │   ↓ lookup map   │      │
│       │                │   ↓ resolve dst  │      │
│       │                │   ↓ tunnel       │──────┼──► upstream
│  ┌────┴─────┐          └──────────────────┘      │
│  │ iptables │                                    │
│  │ init     │  Redirects :443 → :15001           │
│  └──────────┘  (exempt sidecar UID 1337)         │
└──────────────────────────────────────────────────┘
```

## Enforcement Modes

| Mode | Mechanism | Effect | Use When |
|------|-----------|--------|----------|
| **Transparent proxy** | iptables REDIRECT + SNI peek | Intercept & audit LLM traffic | Default — recommended |
| **FQDN policy** | Cilium L7 egress filter | Block non-whitelisted domains | Add-on to transparent proxy |
| **Tetragon kprobe** | Kernel SIGKILL | Kill process on unauthorized :443 | Strict environments, no proxy |

> ⚠️ **Tetragon and transparent proxy are mutually exclusive.** Tetragon kprobes fire at the syscall level *before* iptables NAT, so they would kill the app process before the proxy can intercept. The Helm chart will fail if both are enabled.

## MITM TLS Termination

When `mitm.enabled=true`, the sidecar generates an ephemeral CA at startup and performs TLS termination on intercepted LLM traffic. This enables **request-level observability** (model, tokens, cost) without requiring SDK modifications.

### How It Works

```
App Container                Sidecar Container
┌──────────┐   TLS (ephemeral cert)   ┌──────────────────┐   plaintext   ┌────────┐   TLS   ┌──────────┐
│ curl/SDK │ ────────────────────────► │ Transparent      │ ────────────► │ HTTP   │ ──────► │ Upstream │
│          │                           │ Listener :15001  │               │ Proxy  │         │ LLM API  │
└──────────┘                           └──────────────────┘               │ :8080  │         └──────────┘
     ▲                                        │                           └────────┘
     │ trusts CA via SSL_CERT_FILE            │ writes CA PEM
     └────────────────────────────────────────┘
              /var/run/candela/ca.pem (emptyDir volume)
```

### Enable MITM

```bash
helm install candela-sidecar deploy/helm/candela-sidecar/ \
  --set transparent.enabled=true \
  --set iptables.enabled=true \
  --set mitm.enabled=true \
  --set gcpProject=my-project
```

The Helm chart generates a `ConfigMap` with sidecar container spec fragments
and trust injection env vars. Merge these into your Deployment:

1. **Add the sidecar container** (from `sidecar.yaml` in the ConfigMap)
2. **Add the `candela-ca` emptyDir volume** to the pod spec
3. **Mount the volume** in the application container (read-only)
4. **Set trust env vars** (`SSL_CERT_FILE`, `REQUESTS_CA_BUNDLE`, `NODE_EXTRA_CA_CERTS`)

### Provider Opt-Out

Providers with certificate pinning can opt out of MITM while still being
intercepted for connection-level metrics:

```yaml
providers:
  - name: openai
    host: api.openai.com
    intercept: true
    mitm: true    # default
  - name: pinned-service
    host: pinned.example.com
    intercept: true
    mitm: false   # tunnel passthrough, no TLS termination
```

### W3C Trace Context

The MITM layer is protocol-transparent — it performs byte-level
`copy_bidirectional` between the decrypted TLS stream and the proxy.
W3C `traceparent` headers in the client's HTTP request propagate
naturally to the proxy handler, which already parses them for span
correlation. No header injection is needed.

## Monitoring

The transparent proxy exposes stats via:

```go
// HTTP endpoint (register in your mux):
http.Handle("/debug/transparent/stats", listener.Stats())
// Returns: {"intercepted":42,"passthrough":3,"errors":0}

// Structured logging (call periodically):
listener.Stats().LogStats()
// INFO transparent proxy stats intercepted=42 passthrough=3 errors=0 total=45
```

## Wildcard Patterns

Two wildcard styles are supported:

| Pattern | Matches | Example |
|---------|---------|---------|
| `*.example.com` | Subdomain wildcards | `sub.example.com`, `deep.sub.example.com` |
| `*-aiplatform.googleapis.com` | Suffix wildcards | `us-central1-aiplatform.googleapis.com` |

> GCP Vertex AI uses `{region}-aiplatform.googleapis.com` (suffix pattern), not subdomains.

## E2E Testing

```bash
# Generate self-signed certs for mock upstream.
chmod +x test/e2e/gen-certs.sh && ./test/e2e/gen-certs.sh

# Run E2E (requires Docker with NET_ADMIN capability).
docker compose -f test/e2e/docker-compose.yml up --build --abort-on-container-exit
```

## Development

```bash
# Run all tests with race detector.
nix develop -c go test -race ./pkg/transparent/ ./pkg/proxy/ ./pkg/tetragonaudit/

# Lint Helm chart.
nix develop -c helm lint deploy/helm/candela-sidecar/

# Rust tests.
nix develop -c bash -c 'cd rust && cargo test --workspace'
```
