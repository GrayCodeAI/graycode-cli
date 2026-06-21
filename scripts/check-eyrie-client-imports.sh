#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

violations="$(
  git grep -n 'github\.com/GrayCodeAI/eyrie/client' -- '*.go' \
    ':(exclude)external/**' \
    ':(exclude)internal/types/client.go' \
    ':(exclude)**/*_test.go' || true
)"

if [[ -n "${violations}" ]]; then
  echo "forbidden direct imports of github.com/GrayCodeAI/eyrie/client found:"
  echo "${violations}"
  echo
  echo "hawk production code must go through internal/types transport adapters instead of importing eyrie/client directly"
  exit 1
fi

echo "eyrie/client boundary guard passed"
