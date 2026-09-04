#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if command -v rg >/dev/null 2>&1; then
  graycoderouter_imports="$(
    rg -n '"github\.com/GrayCodeAI/graycode-router/[^\"]+"' \
      --glob '*.go' --glob '!*_test.go' . || true
  )"
else
  graycoderouter_imports="$(
    grep -RInE --include='*.go' --exclude='*_test.go' \
      '"github\.com/GrayCodeAI/graycode-router/[^\"]+"' . || true
  )"
fi
# Graycode uses the full vendored GraycodeRouter API surface for provider, graph, and
# tooling contracts that the engine facade does not re-export.
violations="$(printf '%s\n' "$graycoderouter_imports" | grep -vE '"github\.com/GrayCodeAI/graycode-router/(engine|llm|graph|tools)(/|\")' || true)"

if [[ -n "$violations" ]]; then
  echo "direct production imports below the graycode-router/engine facade found:"
  echo "$violations"
  echo
  echo "route every Graycode production integration through github.com/GrayCodeAI/graycode-router/engine"
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
  echo "provider credentials must not enter Graycode's agent/session layer"
  exit 1
fi

echo "graycode-router engine boundary passed (zero lower-level production imports)"
