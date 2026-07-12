#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ALLOWLIST="scripts/eyrie-lower-import-allowlist.txt"
actual="$(mktemp)"
new_imports="$(mktemp)"
trap 'rm -f "$actual" "$new_imports"' EXIT

if ! sort -c "$ALLOWLIST"; then
  echo "eyrie lower-import allowlist must remain sorted"
  exit 1
fi

rg -l 'github\.com/GrayCodeAI/eyrie/(catalog|client|config|conversation|credentials|router|runtime|setup|storage)' \
  --glob '*.go' --glob '!*_test.go' --glob '!external/**' . \
  | sed 's#^\./##' | sort -u > "$actual"

comm -13 "$ALLOWLIST" "$actual" > "$new_imports"
if [[ -s "$new_imports" ]]; then
  echo "new direct imports below the eyrie/engine facade found:"
  cat "$new_imports"
  echo
  echo "route new production integrations through github.com/GrayCodeAI/eyrie/engine"
  exit 1
fi

echo "eyrie engine boundary ratchet passed"
