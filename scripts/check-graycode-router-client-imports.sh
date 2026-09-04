#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if command -v rg >/dev/null 2>&1; then
  violations="$(
    rg -n '"github\.com/GrayCodeAI/graycode-router/client(?:/[^\"]*)?"' \
      --glob '*.go' --glob '!*_test.go' . || true
  )"
else
  violations="$(
    grep -RInE --include='*.go' --exclude='*_test.go' \
      '"github\.com/GrayCodeAI/graycode-router/client(/[^\"]*)?"' . || true
  )"
fi

if [[ -n "${violations}" ]]; then
  echo "forbidden direct imports of github.com/GrayCodeAI/graycode-router/client found:"
  echo "${violations}"
  echo
  echo "Graycode production code must go through github.com/GrayCodeAI/graycode-router/engine"
  exit 1
fi

echo "graycode-router/client boundary guard passed (zero production imports)"
