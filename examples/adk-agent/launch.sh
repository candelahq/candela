#!/usr/bin/env bash
# Launch the full ADK + Candela observability stack.
#
# Prerequisites:
#   - candela-sidecar binary (go build ./cmd/candela-sidecar)
#   - candela-collector binary (or otelcol)
#   - ADK installed (pip install google-adk)
#   - GCP ADC configured (gcloud auth application-default login)
#
# Usage:
#   GCP_PROJECT=my-project ./launch.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# ── Configuration ──
SIDECAR_PORT="${SIDECAR_PORT:-8080}"
COLLECTOR_GRPC_PORT="${COLLECTOR_GRPC_PORT:-4317}"
COLLECTOR_HTTP_PORT="${COLLECTOR_HTTP_PORT:-4318}"

# ── 1. Start candela-sidecar ──
echo "🕯️  Starting candela-sidecar on :${SIDECAR_PORT}..."
PORT="${SIDECAR_PORT}" \
GCP_PROJECT="${GCP_PROJECT:?Set GCP_PROJECT}" \
CANDELA_PROJECT_ID="${CANDELA_PROJECT_ID:-adk-demo}" \
OTLP_ENDPOINT="http://localhost:${COLLECTOR_HTTP_PORT}/v1/traces" \
  "${REPO_ROOT}/candela-sidecar" &
SIDECAR_PID=$!

# ── 2. Start collector ──
COLLECTOR_BIN="${REPO_ROOT}/candela-collector"
if [ ! -f "${COLLECTOR_BIN}" ]; then
  echo "⚠️  candela-collector not found, trying otelcol..."
  COLLECTOR_BIN="otelcol"
fi
echo "📡 Starting collector (OTLP on :${COLLECTOR_GRPC_PORT}/:${COLLECTOR_HTTP_PORT})..."
"${COLLECTOR_BIN}" --config="${REPO_ROOT}/collector/collector-config.yaml" &
COLLECTOR_PID=$!

# ── Cleanup on exit ──
cleanup() {
  echo ""
  echo "🛑 Shutting down..."
  kill "${SIDECAR_PID}" "${COLLECTOR_PID}" 2>/dev/null || true
  wait "${SIDECAR_PID}" "${COLLECTOR_PID}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Wait for services to be ready.
sleep 2

# ── 3. Start ADK ──
echo "🤖 Starting ADK web server..."
export CANDELA_SIDECAR_URL="http://localhost:${SIDECAR_PORT}/proxy/google"
export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="http://localhost:${COLLECTOR_HTTP_PORT}/v1/traces"
export OTEL_SERVICE_NAME="adk-candela-demo"
export OTEL_SEMCONV_STABILITY_OPT_IN="gen_ai_latest_experimental"
export OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT="EVENT_ONLY"

cd "${SCRIPT_DIR}"
adk web .
