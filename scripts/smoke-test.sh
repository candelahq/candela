#!/usr/bin/env bash
# scripts/smoke-test.sh — Post-deploy smoke tests for Candela
#
# Verifies a deployed Candela instance is healthy and serving traffic.
# Portable: works locally, in CI, against Cloud Run, GKE, or any URL.
#
# Usage:
#   ./scripts/smoke-test.sh https://candela.example.com
#   ./scripts/smoke-test.sh https://candela.example.com --token "$(gcloud auth print-identity-token)"
#   ./scripts/smoke-test.sh http://localhost:8181 --timeout 30
#
# Exit codes:
#   0 — all checks passed
#   1 — one or more checks failed

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────
TIMEOUT=60
TOKEN=""
RETRY_INTERVAL=5

# ── Parse arguments ───────────────────────────────────────────────────
usage() {
  echo "Usage: $0 <url> [--token TOKEN] [--timeout SECONDS]"
  echo ""
  echo "Arguments:"
  echo "  url          Base URL of the Candela instance"
  echo "  --token      Auth token (Bearer) for authenticated probes"
  echo "  --timeout    Max seconds to wait for health (default: 90)"
  exit 1
}

if [[ $# -lt 1 ]]; then
  usage
fi

BASE_URL="${1%/}"  # Strip trailing slash
shift

while [[ $# -gt 0 ]]; do
  case "$1" in
    --token)
      TOKEN="$2"
      shift 2
      ;;
    --timeout)
      TIMEOUT="$2"
      shift 2
      ;;
    --help|-h)
      usage
      ;;
    *)
      echo "Unknown argument: $1"
      usage
      ;;
  esac
done

# ── Helpers ───────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

pass() { echo -e "${GREEN}✓${NC} $1"; }
fail() { echo -e "${RED}✕${NC} $1"; }
info() { echo -e "${YELLOW}…${NC} $1"; }

FAILED=0

# check_endpoint URL EXPECTED_STATUS DESCRIPTION [BODY_CONTAINS]
check_endpoint() {
  local url="$1"
  local expected_status="$2"
  local description="$3"
  local body_contains="${4:-}"

  local elapsed=0
  local status=""
  local body=""

  while [[ $elapsed -lt $TIMEOUT ]]; do
    # Build curl args
    local curl_args=(-s -o /dev/null -w "%{http_code}" --connect-timeout 10 --max-time 15)
    if [[ -n "$TOKEN" ]]; then
      curl_args+=(-H "Authorization: Bearer $TOKEN")
    fi

    status=$(curl "${curl_args[@]}" "$url" 2>/dev/null || echo "000")

    if [[ "$status" == "$expected_status" ]]; then
      # If we need to check body content, fetch the body
      if [[ -n "$body_contains" ]]; then
        local body_args=(-s --connect-timeout 10 --max-time 15)
        if [[ -n "$TOKEN" ]]; then
          body_args+=(-H "Authorization: Bearer $TOKEN")
        fi
        body=$(curl "${body_args[@]}" "$url" 2>/dev/null || echo "")

        if echo "$body" | grep -q "$body_contains"; then
          pass "$description (${status}, ${elapsed}s)"
          return 0
        else
          info "$description — status OK but body missing '$body_contains', retrying... (${elapsed}s)"
        fi
      else
        pass "$description (${status}, ${elapsed}s)"
        return 0
      fi
    else
      info "$description — got ${status}, want ${expected_status}, retrying... (${elapsed}s)"
    fi

    sleep "$RETRY_INTERVAL"
    elapsed=$((elapsed + RETRY_INTERVAL))
  done

  fail "$description — timed out after ${TIMEOUT}s (last status: ${status})"
  FAILED=1
}

# ── Run checks ────────────────────────────────────────────────────────
echo ""
echo "🕯️  Candela Smoke Test"
echo "   Target:  $BASE_URL"
echo "   Timeout: ${TIMEOUT}s"
echo "   Auth:    $(if [[ -n "$TOKEN" ]]; then echo "token provided"; else echo "none"; fi)"
echo ""

# 1. Liveness — process is alive
check_endpoint "$BASE_URL/healthz" "200" "Liveness  (/healthz)" '"status": "ok"'

# 2. Readiness — storage backend reachable
check_endpoint "$BASE_URL/readyz" "200" "Readiness (/readyz)" '"status": "ready"'

# 3. UI serving — HTML page loads
check_endpoint "$BASE_URL/" "200" "UI served (/)"

# 4. Authenticated probe (if token provided)
if [[ -n "$TOKEN" ]]; then
  check_endpoint \
    "$BASE_URL/candela.v1.UserService/GetCurrentUser" \
    "200" \
    "Auth probe (GetCurrentUser)" \
    '"email"'
else
  info "Auth probe — skipped (no --token provided)"
fi

# ── Summary ───────────────────────────────────────────────────────────
echo ""
if [[ $FAILED -eq 0 ]]; then
  echo -e "${GREEN}All smoke tests passed.${NC}"
  exit 0
else
  echo -e "${RED}Smoke tests FAILED.${NC}"
  exit 1
fi
