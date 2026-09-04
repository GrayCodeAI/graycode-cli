#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ECO_DIR="$(cd "${ROOT_DIR}/.." && pwd)"

"${ROOT_DIR}/scripts/ecosystem-manifest.sh" validate

rm -f "${ECO_DIR}/go.work" "${ECO_DIR}/go.work.sum"
(
  cd "${ECO_DIR}"
  go work init ./graycode-cli
  go_version="$(awk '$1 == "go" { print $2; exit }' "${ROOT_DIR}/go.mod")"
  go work edit -go="${go_version}"
  while IFS= read -r repo; do
    [[ "${repo}" == "graycode-cli" ]] && continue
    if [[ ! -f "${repo}/go.mod" ]]; then
      echo "WARNING: ${repo} is not checked out; skipping workspace entry"
      continue
    fi
    go work use "./${repo}"
    module="$(awk '$1 == "module" { print $2; exit }' "${repo}/go.mod")"
    while IFS= read -r version; do
      [[ -n "${version}" ]] && go work edit -replace="${module}@${version}=./${repo}"
    done < <(
      {
        while IFS= read -r consumer; do
          [[ -f "${consumer}/go.mod" ]] || continue
          awk -v wanted="${module}" '
            /^(replace|exclude)[[:space:]]*\($/ { skip = 1; next }
            skip && /^\)/                       { skip = 0; next }
            skip                                { next }
            $1 == "replace" || $1 == "exclude"  { next }
            {
              for (i = 1; i < NF; i++) {
                if ($i == wanted && $(i + 1) ~ /^v[0-9]/) print $(i + 1)
              }
            }
          ' "${consumer}/go.mod"
        done < <("${ROOT_DIR}/scripts/ecosystem-manifest.sh" list workspace)
      } | sort -u
    )
  done < <("${ROOT_DIR}/scripts/ecosystem-manifest.sh" list workspace)
  go work sync
)

echo "workspace generated at ${ECO_DIR}/go.work"
