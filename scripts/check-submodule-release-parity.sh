#!/usr/bin/env bash
set -euo pipefail

# Compare each engine Gitlink (from the superproject index) to the commit
# resolved by the go.mod module version. Uses `git ls-tree` so the check
# works without `git submodule update` / checkout-eyrie having populated
# external/* working trees.

repos=(hawk-core-contracts eyrie inspect sight tok trace yaad)
failed=0

printf '%-24s %-14s %-14s %s\n' MODULE GITLINK MODULE_COMMIT STATUS
for repo in "${repos[@]}"; do
  module="github.com/GrayCodeAI/${repo}"
  gitlink=$(git ls-tree HEAD "external/${repo}" | awk '{print $3}')
  if [[ -z "$gitlink" ]]; then
    printf '%-24s %-14s %-14s %s\n' "$repo" missing - MISSING_GITLINK
    failed=1
    continue
  fi

  version=$(GOWORK=off go list -m -f '{{.Version}}' "$module")
  metadata=$(GOWORK=off go mod download -json "${module}@${version}" 2>/dev/null || true)
  module_commit=$(printf '%s\n' "$metadata" | sed -n 's/.*"Hash": "\([0-9a-f]*\)".*/\1/p' | head -1)
  if [[ -z "$module_commit" ]]; then
    printf '%-24s %-14s %-14s %s\n' "$repo" "${gitlink:0:12}" unknown UNRESOLVED
    failed=1
  elif [[ "$module_commit" == "$gitlink" ]]; then
    printf '%-24s %-14s %-14s %s\n' "$repo" "${gitlink:0:12}" "${module_commit:0:12}" OK
  else
    printf '%-24s %-14s %-14s %s\n' "$repo" "${gitlink:0:12}" "${module_commit:0:12}" MISMATCH
    failed=1
  fi
done

if ((failed)); then
  echo "submodule/module release parity failed" >&2
  exit 1
fi
