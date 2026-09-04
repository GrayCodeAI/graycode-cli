#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

pattern='github\.com/GrayCodeAI/graycode-cli/(internal/|shared/types)'
violations=""

for repo in ../kestrel ../merlin ../shrike ../swift ../harrier ../eyrie; do
  if [[ -d "${repo}" ]]; then
    repo_hits="$(
      grep -RInE --include='*.go' "${pattern}" "${repo}" || true
    )"
    if [[ -n "${repo_hits}" ]]; then
      violations+="${repo_hits}"$'\n'
    fi
  fi
done

if [[ -n "${violations}" ]]; then
      echo "forbidden Graycode imports found in sibling ecosystem repos:"
  echo "${violations}"
  echo
  echo "support repos must use their own contracts, not graycode/internal or removed graycode/shared/types"
  exit 1
fi

echo "ecosystem boundary guard passed"
