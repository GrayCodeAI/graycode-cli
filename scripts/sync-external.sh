#!/usr/bin/env bash
# Read-only drift report: compares each external/<repo> submodule's pinned
# commit against the HEAD of the sibling dev clone at ../<repo> (relative to
# the graycode-eco workspace root). Unlike `make sync-submodules` (which mutates
# the submodule checkout), this makes no changes — it only reports.
#
# Typical drift: you commit changes in ../tok, but forget `make
# sync-submodules` + a commit in hawk to bump the external/tok pin. This
# script catches that before it becomes a stale-dependency surprise in CI.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ ! -f .gitmodules ]]; then
  echo "no .gitmodules file found — nothing to check"
  exit 0
fi

exit_code=0
printf '%-28s %-10s %-10s %s\n' "SUBMODULE" "PINNED" "SIBLING" "STATUS"

while IFS= read -r path; do
  name="$(basename "$path")"
  sibling="../$name"

  pinned="$(git ls-tree HEAD "$path" 2>/dev/null | awk '{print $3}')"
  if [[ -z "$pinned" ]]; then
    printf '%-28s %-10s %-10s %s\n' "$path" "none" "-" "NOT-PINNED (submodule never committed)"
    exit_code=1
    continue
  fi
  pinned_short="${pinned:0:10}"

  if [[ ! -d "$sibling/.git" ]]; then
    printf '%-28s %-10s %-10s %s\n' "$path" "$pinned_short" "-" "NO-SIBLING (expected clone at $sibling)"
    exit_code=1
    continue
  fi

  sibling_head="$(git -C "$sibling" rev-parse HEAD 2>/dev/null || echo "")"
  if [[ -z "$sibling_head" ]]; then
    printf '%-28s %-10s %-10s %s\n' "$path" "$pinned_short" "-" "SIBLING-UNREADABLE"
    exit_code=1
    continue
  fi
  sibling_short="${sibling_head:0:10}"

  if [[ "$pinned" == "$sibling_head" ]]; then
    printf '%-28s %-10s %-10s %s\n' "$path" "$pinned_short" "$sibling_short" "OK"
  elif git -C "$sibling" merge-base --is-ancestor "$pinned" "$sibling_head" 2>/dev/null; then
    printf '%-28s %-10s %-10s %s\n' "$path" "$pinned_short" "$sibling_short" "BEHIND (sibling has newer commits)"
    exit_code=1
  elif git -C "$sibling" merge-base --is-ancestor "$sibling_head" "$pinned" 2>/dev/null; then
    printf '%-28s %-10s %-10s %s\n' "$path" "$pinned_short" "$sibling_short" "AHEAD (pin is newer than local sibling checkout)"
    exit_code=1
  else
    printf '%-28s %-10s %-10s %s\n' "$path" "$pinned_short" "$sibling_short" "DIVERGED (different history)"
    exit_code=1
  fi
done < <(git config -f .gitmodules --get-regexp path | awk '{print $2}')

if [[ $exit_code -ne 0 ]]; then
  echo
  echo "drift detected — run 'make sync-submodules' then 'make sync-submodule-versions'" 
  echo "(advances the submodule working trees, then bumps hawk's go.mod requires to match the gitlinks), then commit the updated external/ pins"
fi

exit $exit_code
