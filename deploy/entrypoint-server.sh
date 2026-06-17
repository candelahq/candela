#!/bin/sh
# Candela server entrypoint — Go backend only.
# Generates config.yaml from environment variables at startup.
# This avoids baking secrets into the container image.

set -e

# ── Generate config from environment variables ──
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
    caching_mode: "${CANDELA_CACHING_MODE:-auto}"
    cache_ttl: "${CANDELA_CACHE_TTL:-}"
    provider_overrides:
      mistral:
        region: "${CANDELA_MISTRAL_REGION:-us-central1}"
      deepseek-v3:
        region: "${CANDELA_DEEPSEEK_V3_REGION:-global}"
      qwen:
        region: "${CANDELA_QWEN_REGION:-us-south1}"
      zai:
        region: "${CANDELA_ZAI_REGION:-global}"
  providers:
    - openai
    - google
    - anthropic
    - anthropic-vertex
    - anthropic-direct
    - anthropic-bedrock
    - gemini-oai
    - gemini-vertex
    - mistral
    - deepseek
    - deepseek-v3
    - qwen
    - zai

cors:
  allowed_origins: []

auth:
  dev_mode: ${CANDELA_DEV_MODE:-false}
  allowed_service_accounts:
    - "${CANDELA_SERVICE_ACCOUNT:-}"

firestore:
  enabled: true
  project_id: "${CANDELA_FIRESTORE_PROJECT:-}"
  database_id: "${CANDELA_FIRESTORE_DATABASE:-candela}"

worker:
  batch_size: 100
  flush_interval: "2s"

users:
  default_daily_budget_usd: ${CANDELA_DEFAULT_DAILY_BUDGET:-5.00}
EOF

export CANDELA_CONFIG="$CONFIG_PATH"

echo "Starting Candela server on :8181..."
exec /usr/local/bin/candela-server
