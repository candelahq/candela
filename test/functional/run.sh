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
echo ""

mkdir -p "$REPORT_DIR"

hurl --test \
  --variable CANDELA_URL="$CANDELA_URL" \
  --variable MOCK_UPSTREAM_URL="$MOCK_URL" \
  --report-junit "$REPORT_DIR/functional-$TARGET.xml" \
  "${EXTRA_ARGS[@]}" \
  "$SCRIPT_DIR"/proxy/*.hurl \
  "$SCRIPT_DIR"/billing/*.hurl \
  "$SCRIPT_DIR"/compat/*.hurl \
  "$SCRIPT_DIR"/security/*.hurl
