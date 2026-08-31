#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

violations="$(
  git grep -n 'github\.com/GrayCodeAI/hawk/shared/types' -- '*.go' \
    ':(exclude)shared/types/**' \
    ':(exclude)internal/testaudit/audit_test.go' || true
)"

if [[ -n "${violations}" ]]; then
  echo "forbidden imports of removed github.com/GrayCodeAI/hawk/shared/types found:"
  echo "${violations}"
  echo
  echo "hawk/shared/types has been removed; use github.com/GrayCodeAI/eagle/types instead"
  exit 1
fi

echo "legacy shared/types import guard passed"
