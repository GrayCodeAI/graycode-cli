#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

support_repos=()
while IFS= read -r repo; do
  [[ -n "${repo}" ]] && support_repos+=("${repo}")
done < <("${ROOT_DIR}/scripts/ecosystem-manifest.sh" list engines)
violations=""

scan_dir() {
  local owner="$1"
  local dir="$2"
  local peers=()
  local pattern=""
  local hits=""

  if [[ ! -d "${dir}" ]]; then
    return
  fi

  for repo in "${support_repos[@]}"; do
    if [[ "${repo}" != "${owner}" ]]; then
      peers+=("${repo}")
    fi
  done

  pattern="$(IFS='|'; echo "${peers[*]}")"
  hits="$(
    grep -RInE --include='*.go' "github\\.com/GrayCodeAI/(${pattern})(/|\")" "${dir}" || true
  )"
  if [[ -n "${hits}" ]]; then
    violations+="${hits}"$'\n'
  fi
}

for repo in "${support_repos[@]}"; do
  scan_dir "${repo}" "../${repo}"
done

if [[ -d "../sparrow" ]]; then
  peer_pattern="$(IFS='|'; echo "${support_repos[*]}")"
  sdk_hits="$(
    grep -RInE --include='*.go' "github\\.com/GrayCodeAI/(${peer_pattern})(/|\")" ../sparrow || true
  )"
  if [[ -n "${sdk_hits}" ]]; then
    violations+="${sdk_hits}"$'\n'
  fi
fi

if [[ -n "${violations}" ]]; then
  echo "forbidden cross-repo peer imports found:"
  echo "${violations}"
  echo
  echo "support engines must not import each other; Hawk is the orchestrator and shared contracts belong in eagle"
  exit 1
fi

echo "support repo coupling guard passed"
