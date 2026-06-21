#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

violations="$(
  git grep -n 'github\.com/GrayCodeAI/hawk/shared/types' -- '*.go' \
    ':(exclude)external/**' \
    ':(exclude)shared/types/**' \
    ':(exclude)**/*_test.go' || true
)"

if [[ -n "${violations}" ]]; then
  echo "forbidden imports of github.com/GrayCodeAI/hawk/shared/types found:"
  echo "${violations}"
  echo
  echo "use github.com/GrayCodeAI/hawk-core-contracts/types instead"
  exit 1
fi

echo "shared/types import guard passed"
