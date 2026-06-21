#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

violations=""

check_repo() {
  local dir="$1"
  if [[ ! -d "$dir" ]]; then
    return
  fi
  local hits
  hits="$(
    grep -RInE --include='*.go' '"github\.com/GrayCodeAI/hawk/shared/types"' "$dir" | grep -v 'internal/testaudit/' || true
  )"
  if [[ -n "$hits" ]]; then
    violations+="${hits}"$'\n'
  fi
}

# Compatibility package self-tests are allowed until the package is removed.
for repo in \
  cmd \
  internal \
  external/eyrie \
  external/yaad \
  external/tok \
  external/trace \
  external/sight \
  external/inspect \
  ../eyrie \
  ../yaad \
  ../tok \
  ../trace \
  ../sight \
  ../inspect \
  ../hawk-sdk-go
do
  check_repo "$repo"
done

if [[ -n "${violations}" ]]; then
  echo "hawk/shared/types is not ready for retirement; active imports remain:"
  echo "${violations}"
  echo
  echo "migrate remaining imports to github.com/GrayCodeAI/hawk-core-contracts/types before deleting hawk/shared/types"
  exit 1
fi

echo "shared/types retirement readiness passed for the local ecosystem"
