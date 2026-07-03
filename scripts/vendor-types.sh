#!/usr/bin/env bash
# vendor-types.sh — Vendor proto2type-generated Rust types from candela-protos.
#
# Vendors domain types and SQLite converters from candela-protos into
# candela-types. Buffa converters are NOT vendored — they are generated
# locally in candela-harness-connect via `cd rust/crates/candela-harness-connect
# && buf generate` because they use a different domain_module path.
#
# Usage:
#   ./scripts/vendor-types.sh [PROTOS_DIR]
#
# PROTOS_DIR defaults to ../candela-protos (sibling checkout).
# Run `buf generate --template buf.gen.rust.yaml` in candela-protos first.
set -euo pipefail
unset CDPATH

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

PROTOS_DIR="${1:-${REPO_DIR}/../candela-protos}"
GEN_DIR="${PROTOS_DIR}/gen/rust/github.com/candelahq/candela/gen/go/candela/types"

if [ ! -d "${GEN_DIR}" ]; then
  echo "Error: generated types not found at ${GEN_DIR}" >&2
  echo "Run 'buf generate --template buf.gen.rust.yaml' in candela-protos first." >&2
  exit 1
fi

TYPES_DIR="${REPO_DIR}/rust/crates/candela-types/src"

if [ ! -d "${TYPES_DIR}" ]; then
  echo "Error: destination directory not found: ${TYPES_DIR}" >&2
  exit 1
fi

echo "Vendoring proto2type domain types from: ${GEN_DIR}"
echo ""

copy() {
  local src="${GEN_DIR}/$1"
  local dst="$2"
  if [ ! -f "${src}" ]; then
    echo "Error: missing generated file: $1" >&2
    exit 1
  fi
  cp "${src}" "${dst}"
  echo "  COPY  $1 → ${dst#"${REPO_DIR}"/}"
}

# Domain types + SQLite converters → candela-types
copy "chat.type.rs"                  "${TYPES_DIR}/chat.rs"
copy "chat_sqlite.type.rs"           "${TYPES_DIR}/chat_sqlite.rs"
copy "session.type.rs"               "${TYPES_DIR}/session.rs"
copy "session_sqlite.type.rs"        "${TYPES_DIR}/session_sqlite.rs"
copy "model_catalog.type.rs"         "${TYPES_DIR}/model_catalog.rs"
copy "model_catalog_sqlite.type.rs"  "${TYPES_DIR}/model_catalog_sqlite.rs"

echo ""
echo "Note: buffa converters are generated locally — run:"
echo "  cd rust/crates/candela-harness-connect && buf generate"
echo ""
echo "Done. Run 'cargo test --workspace' to verify."
