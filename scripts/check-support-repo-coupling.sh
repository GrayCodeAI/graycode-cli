#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

support_repos=(eyrie inspect sight tok trace yaad)
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

if [[ -d "../hawk-sdk-go" ]]; then
  sdk_hits="$(
    grep -RInE --include='*.go' 'github\.com/GrayCodeAI/(eyrie|inspect|sight|tok|trace|yaad)(/|")' ../hawk-sdk-go || true
  )"
  if [[ -n "${sdk_hits}" ]]; then
    violations+="${sdk_hits}"$'\n'
  fi
fi

if [[ -n "${violations}" ]]; then
  echo "forbidden cross-repo peer imports found:"
  echo "${violations}"
  echo
  echo "support engines must not import each other; Hawk is the orchestrator and shared contracts belong in hawk-core-contracts"
  exit 1
fi

echo "support repo coupling guard passed"
