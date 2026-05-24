#!/usr/bin/env bash
# run.sh — Convenience runner for the Candela functional test suite.
# Usage: ./test/functional/run.sh [--go | --rust] [hurl options...]
#
# Prerequisites:
#   - hurl in PATH (available in the Nix dev shell)
#   - mock upstream running: cd test/functional/mock && go run upstream.go
#   - binary under test running on the appropriate port

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── Defaults ────────────────────────────────────────────────────────────────
CANDELA_URL="${HURL_CANDELA_URL:-http://localhost:8080}"
MOCK_URL="${HURL_MOCK_UPSTREAM_URL:-http://localhost:9999}"
ANALYSIS_URL="${HURL_ANALYSIS_URL:-http://localhost:8090}"
REPORT_DIR="${SCRIPT_DIR}/../../test-results"

# ── Flag parsing ─────────────────────────────────────────────────────────────
TARGET="go"
EXTRA_ARGS=()
for arg in "$@"; do
  case "$arg" in
    --go)    TARGET="go";   CANDELA_URL="http://localhost:8080" ;;
    --rust)  TARGET="rust"; CANDELA_URL="http://localhost:8181" ;;
    *)       EXTRA_ARGS+=("$arg") ;;
  esac
done

echo "🕯️  Candela functional tests"
echo "   Target:   $TARGET ($CANDELA_URL)"
echo "   Upstream: $MOCK_URL"
echo "   Analysis: $ANALYSIS_URL"
echo ""

mkdir -p "$REPORT_DIR"

  # Compat routes (/v1/) are only registered by the CLI binary, not candela-server.
  # Include compat tests only when a CLI binary is under test.
  COMPAT_TESTS=()
  if [[ -n "${HURL_INCLUDE_COMPAT:-}" ]]; then
    COMPAT_TESTS=("$SCRIPT_DIR"/compat/*.hurl)
  fi

hurl --test \
  --variable CANDELA_URL="$CANDELA_URL" \
  --variable MOCK_UPSTREAM_URL="$MOCK_URL" \
  --variable ANALYSIS_URL="$ANALYSIS_URL" \
  --report-junit "$REPORT_DIR/functional-$TARGET.xml" \
  "${EXTRA_ARGS[@]}" \
  "$SCRIPT_DIR"/proxy/*.hurl \
  "$SCRIPT_DIR"/billing/*.hurl \
  "${COMPAT_TESTS[@]}" \
  "$SCRIPT_DIR"/security/*.hurl \
  "$SCRIPT_DIR"/dashboard/dashboard_auth.hurl

# Dashboard contract tests require AUTH_TOKEN and are run separately.
if [[ -n "${HURL_AUTH_TOKEN:-}" ]]; then
  echo ""
  echo "🔐 Running authenticated dashboard contract tests..."
  hurl --test \
    --variable ANALYSIS_URL="$ANALYSIS_URL" \
    --variable AUTH_TOKEN="$HURL_AUTH_TOKEN" \
    --report-junit "$REPORT_DIR/functional-dashboard-$TARGET.xml" \
    "${EXTRA_ARGS[@]}" \
    "$SCRIPT_DIR"/dashboard/dashboard_contract.hurl
fi

