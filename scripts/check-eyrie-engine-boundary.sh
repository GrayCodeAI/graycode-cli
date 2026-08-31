#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if command -v rg >/dev/null 2>&1; then
  eyrie_imports="$(
    rg -n '"github\.com/GrayCodeAI/eyrie/[^\"]+"' \
      --glob '*.go' --glob '!*_test.go' . || true
  )"
else
  eyrie_imports="$(
    grep -RInE --include='*.go' --exclude='*_test.go' \
      '"github\.com/GrayCodeAI/eyrie/[^\"]+"' . || true
  )"
fi
violations="$(printf '%s\n' "$eyrie_imports" | grep -vE '"github\.com/GrayCodeAI/eyrie/engine(/|\")' || true)"

if [[ -n "$violations" ]]; then
  echo "direct production imports below the eyrie/engine facade found:"
  echo "$violations"
  echo
  echo "route every Hawk production integration through github.com/GrayCodeAI/eyrie/engine"
  exit 1
fi

if command -v rg >/dev/null 2>&1; then
  credential_symbols="$(rg -n '\b(apiKeys|SetAPIKey|SetAPIKeys)\b' internal/engine --glob '*.go' --glob '!*_test.go' || true)"
else
  credential_symbols="$(grep -RInE --include='*.go' --exclude='*_test.go' '(^|[^[:alnum:]_])(apiKeys|SetAPIKey|SetAPIKeys)([^[:alnum:]_]|$)' internal/engine || true)"
fi
if [[ -n "$credential_symbols" ]]; then
	printf '%s\n' "$credential_symbols"
  echo
  echo "provider credentials must not enter Hawk's agent/session layer"
  exit 1
fi

echo "eyrie engine boundary passed (zero lower-level production imports)"
