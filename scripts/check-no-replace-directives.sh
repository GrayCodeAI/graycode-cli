#!/usr/bin/env bash
# CI guard: fail if any go.mod in this workspace has a local replace directive.
# Local replace directives must not be present when tagging a release.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ECO_DIR="$(cd "${ROOT_DIR}/.." && pwd)"
errors=0
while IFS= read -r modfile; do
  if grep -qE '=>[[:space:]]+(\./|\.\./|/)' "$modfile"; then
    echo "ERROR: local replace directive found in $modfile"
    grep -nE '=>[[:space:]]+(\./|\.\./|/)' "$modfile"
    errors=$((errors + 1))
  fi
done < <(
  while IFS= read -r repo; do
    [[ -d "${ECO_DIR}/${repo}" ]] || continue
    find "${ECO_DIR}/${repo}" -name vendor -prune -o -name go.mod -type f -print
  done < <("${ROOT_DIR}/scripts/ecosystem-manifest.sh" list workspace)
) | sort

if [ $errors -gt 0 ]; then
  printf '\nFail: %d go.mod file(s) have local replace directives that must be removed before tagging a release.\n' "$errors"
  exit 1
fi
echo "OK: no local replace directives found."
