# ──────────────────────────────────────────────────
# Candela Server — Go backend only (universal image)
# No UI, no Firebase, no deployment-specific config.
# Config injected at runtime via environment variables.
#
# Build:  docker build -f deploy/Dockerfile -t candela-server .
# Run:    docker run -p 8181:8181 -e CANDELA_STORAGE_BACKEND=duckdb candela-server
# ──────────────────────────────────────────────────

# ── Stage 1: Build Go backend ──
# DuckDB requires glibc (not musl/Alpine) for its static bindings.
# golang:1.26-bookworm
FROM golang:1.27-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends build-essential && rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags='-s -w' -o candela-server ./cmd/candela-server

# ── Stage 2: Runtime ──
# Debian slim for glibc compatibility with DuckDB Go binary.
# debian:bookworm-slim
FROM debian:bookworm-slim@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251

RUN apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/candela-server /usr/local/bin/candela-server
COPY deploy/entrypoint-server.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# SECURITY: Run as non-root to limit blast radius if the server is compromised.
# A root-level container escape can lead to host compromise on misconfigured
# runtimes (no user namespaces, privileged mode). CIS Docker Benchmark 4.1.
RUN groupadd -r candela && useradd -r -g candela -s /sbin/nologin candela \
    && mkdir -p /etc/candela && chown candela:candela /etc/candela
WORKDIR /etc/candela
USER candela

EXPOSE 8181

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
