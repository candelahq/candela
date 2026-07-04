#!/usr/bin/env bash
# fix-proto2type-clone.sh — Fix proto2type v0.4.2 Clone() receiver shadowing.
#
# proto2type v0.4.2 generates Clone() methods like:
#
#   func (c *Foo) Clone() *Foo {
#       if c == nil { return nil }
#       c := &Foo{ ... }       // ← shadows receiver
#       return c
#   }
#
# This causes "c declared and not used" / infinite recursion because the inner
# `c` shadows the receiver. This script rewrites `c :=` → `clone :=` and
# `return c` → `return clone` in the Clone() function body.
#
# Upstream issue: https://github.com/protocgen/proto2type/issues/125
# Remove this script once proto2type is fixed and upgraded.
#
# Usage:
#   ./scripts/fix-proto2type-clone.sh [DIR]
#
# DIR defaults to gen/go/candela/types/domain/
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
TARGET_DIR="${1:-${REPO_DIR}/gen/go/candela/types/domain}"

TAB=$'\t'

if [ ! -d "${TARGET_DIR}" ]; then
  echo "Error: directory not found: ${TARGET_DIR}" >&2
  exit 1
fi

fixed=0
for f in "${TARGET_DIR}"/*.type.go; do
  [ -f "$f" ] || continue

  # Only process files that have the bug pattern
  if grep -q 'func (c \*.*) Clone()' "$f" && grep -q "${TAB}c := &" "$f"; then
    # Use sed to fix within Clone() method bodies:
    # 1. Replace `c := &` with `clone := &` (variable declaration)
    # 2. Replace `c.Field = ` with `clone.Field = ` (field assignments)
    # 3. Replace `return c` with `return clone` (return statement)
    # Write to temp file + mv for portability (BSD vs GNU sed -i differs).
    sed \
      -e "/func (c \\*.*) Clone()/,/^}/ {
        s/${TAB}c := \&/${TAB}clone := \&/g
        s/${TAB}c\.\([A-Z]\)/${TAB}clone.\1/g
        s/${TAB}return c$/${TAB}return clone/
      }" "$f" > "$f.tmp"
    # Only count as fixed if the content actually changed.
    if ! cmp -s "$f" "$f.tmp"; then
      mv "$f.tmp" "$f"
      fixed=$((fixed + 1))
      echo "  FIXED  ${f#"${REPO_DIR}/"}"
    else
      rm -f "$f.tmp"
    fi
  fi
done

if [ "$fixed" -eq 0 ]; then
  echo "No Clone() shadowing bugs found — proto2type may have been fixed upstream."
else
  echo ""
  echo "Fixed ${fixed} file(s). Run 'go vet ./...' to verify."
fi
