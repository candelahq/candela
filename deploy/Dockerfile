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
FROM golang:1.26-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends build-essential && rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o candela-server ./cmd/candela-server

# ── Stage 2: Runtime ──
# Debian slim for glibc compatibility with DuckDB Go binary.
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/candela-server /usr/local/bin/candela-server
COPY deploy/entrypoint-server.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

EXPOSE 8181

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
