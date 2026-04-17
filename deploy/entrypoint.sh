#!/bin/sh
# Candela container entrypoint — runs Go backend + Next.js UI.
# Go handles API (ConnectRPC, proxy, healthz) on port 8181 (internal).
# Next.js handles UI routes on port 3000 (exposed to Cloud Run).
# Next.js rewrites proxy API calls to Go.

set -e

# ── Generate config from environment variables ──
# This avoids baking secrets into the container image.
CONFIG_PATH="/etc/candela/config.yaml"
mkdir -p /etc/candela

cat > "$CONFIG_PATH" <<EOF
server:
  host: "0.0.0.0"
  port: 8181

storage:
  backend: "${CANDELA_STORAGE_BACKEND:-duckdb}"
  bigquery:
    project_id: "${CANDELA_BQ_PROJECT:-}"
    dataset: "${CANDELA_BQ_DATASET:-candela}"
    table: "spans"
    location: "${CANDELA_BQ_LOCATION:-US}"

proxy:
  enabled: ${CANDELA_PROXY_ENABLED:-false}
  project_id: "default"
  vertex_ai:
    project_id: "${CANDELA_VERTEX_PROJECT:-}"
    region: "${CANDELA_VERTEX_REGION:-us-east5}"
  providers:
    - openai
    - google
    - anthropic
    - gemini-oai
  lmstudio:
    enabled: ${CANDELA_LMSTUDIO_ENABLED:-true}
    port: ${CANDELA_LMSTUDIO_PORT:-1234}
    models:
      - id: gpt-4o
        provider: openai
      - id: gpt-4o-mini
        provider: openai
      - id: o3-mini
        provider: openai
      - id: claude-sonnet-4-20250514
        provider: anthropic
      - id: claude-opus-4-20250514
        provider: anthropic
      - id: gemini-2.5-pro
        provider: gemini-oai
      - id: gemini-2.5-flash
        provider: gemini-oai

cors:
  allowed_origins: []

auth:
  dev_mode: ${CANDELA_DEV_MODE:-false}

firestore:
  enabled: true
  project_id: "${CANDELA_FIRESTORE_PROJECT:-}"
  database_id: "${CANDELA_FIRESTORE_DATABASE:-candela}"

worker:
  batch_size: 100
  flush_interval: "2s"
EOF

export CANDELA_CONFIG="$CONFIG_PATH"

# ── Start Go backend ──
echo "Starting Candela backend on :8181..."
/usr/local/bin/candela-server &
GO_PID=$!

# Monitor: if Go backend exits, kill the container.
(while kill -0 "$GO_PID" 2>/dev/null; do sleep 5; done; echo "Backend exited, shutting down..."; kill 1) &

# Wait for backend to be ready (up to 10s).
echo "Waiting for backend..."
for i in $(seq 1 20); do
  if wget -q -O /dev/null http://localhost:8181/healthz 2>/dev/null; then
    echo "Backend ready."
    break
  fi
  sleep 0.5
done

# ── Start Next.js UI ──
echo "Starting Candela UI on :3000..."
cd /app/ui
HOSTNAME="0.0.0.0" PORT=3000 BACKEND_URL="http://localhost:8181" exec node server.js
