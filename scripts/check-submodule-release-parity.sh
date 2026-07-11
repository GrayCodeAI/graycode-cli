#!/usr/bin/env bash
set -euo pipefail

repos=(hawk-core-contracts eyrie inspect sight tok trace yaad)
failed=0

printf '%-24s %-14s %-14s %s\n' MODULE GITLINK MODULE_COMMIT STATUS
for repo in "${repos[@]}"; do
  module="github.com/GrayCodeAI/${repo}"
  gitlink=$(git -C "external/${repo}" rev-parse HEAD 2>/dev/null || true)
  if [[ -z "$gitlink" ]]; then
    printf '%-24s %-14s %-14s %s\n' "$repo" missing - MISSING_GITLINK
    failed=1
    continue
  fi

  version=$(GOWORK=off go list -m -f '{{.Version}}' "$module")
  metadata=$(GOWORK=off go mod download -json "${module}@${version}")
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
