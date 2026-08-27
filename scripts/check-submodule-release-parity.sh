#!/usr/bin/env bash
set -euo pipefail

# Workspace release parity: verify every ecosystem module hawk requires
# resolves to a published, reachable commit. This catches go.mod pins to
# versions that do not exist. Run from the hawk repo root.
#
# Note: it deliberately does NOT compare against sibling HEAD ancestry —
# CI clones the siblings as shallow (depth-1) checkouts, so older pinned
# commits are not present in their history and ancestry checks would
# falsely report AHEAD. Resolution (go mod download) is the robust signal.

repos=(hawk-core-contracts eyrie inspect sight tok trace yaad hawk-mcpkit)
failed=0

printf '%-24s %-14s %s\n' MODULE MODULE_COMMIT STATUS
for repo in "${repos[@]}"; do
  module="github.com/GrayCodeAI/${repo}"

  version=$(GOWORK=off go list -m -f '{{.Version}}' "$module" 2>/dev/null || true)
  if [[ -z "$version" ]]; then
    printf '%-24s %-14s %s\n' "$repo" - NOT_REQUIRED
    continue
  fi

  metadata=$(GOWORK=off go mod download -json "${module}@${version}" 2>/dev/null || true)
  module_commit=$(printf '%s\n' "$metadata" | sed -n 's/.*"Hash": "\([0-9a-f]*\)".*/\1/p' | head -1)

  if [[ -z "$module_commit" ]]; then
    printf '%-24s %-14s %s\n' "$repo" unknown UNRESOLVED
    failed=1
  else
    printf '%-24s %-14s %s\n' "$repo" "${module_commit:0:12}" OK
  fi
done

if ((failed)); then
  echo "workspace/module release parity failed (unresolvable module version)" >&2
  exit 1
fi
