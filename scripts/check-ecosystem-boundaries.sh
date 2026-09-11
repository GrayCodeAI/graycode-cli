#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

pattern='github\.com/GrayCodeAI/hawk/internal/'
violations=""

while IFS= read -r repo; do
  [[ "${repo}" == "hawk" ]] && continue
  dir="../${repo}"
  if [[ -d "${dir}" ]]; then
    repo_hits="$(
      grep -RInE --include='*.go' "${pattern}" "${dir}" || true
    )"
    if [[ -n "${repo_hits}" ]]; then
      violations+="${repo_hits}"$'\n'
    fi
  fi
done < <(./scripts/ecosystem-manifest.sh list workspace)

if [[ -n "${violations}" ]]; then
      echo "forbidden Hawk imports found in sibling ecosystem repos:"
  echo "${violations}"
  echo
  echo "support repos must use their own contracts, not hawk/internal"
  exit 1
fi

echo "ecosystem boundary guard passed"
