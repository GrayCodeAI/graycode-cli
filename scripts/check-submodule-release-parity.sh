#!/usr/bin/env bash
set -euo pipefail

# Workspace release parity: for each Go module hawk requires, verify that the
# version pinned in hawk's go.mod resolves to a commit reachable from the
# sibling repo's HEAD. This catches hawk pinning a version that the sibling
# repo has not published / is behind on. Run from the hawk repo root with the
# ecosystem cloned as siblings (../<repo>) in the graycode-eco workspace.

repos=(hawk-core-contracts eyrie inspect sight tok trace yaad hawk-mcpkit)
failed=0

printf '%-24s %-14s %-14s %s\n' MODULE MODULE_COMMIT SIBLING_HEAD STATUS
for repo in "${repos[@]}"; do
  module="github.com/GrayCodeAI/${repo}"
  sibling="../${repo}"

  version=$(GOWORK=off go list -m -f '{{.Version}}' "$module" 2>/dev/null || true)
  if [[ -z "$version" ]]; then
    printf '%-24s %-14s %-14s %s\n' "$repo" unknown - NOT_REQUIRED
    continue
  fi

  metadata=$(GOWORK=off go mod download -json "${module}@${version}" 2>/dev/null || true)
  module_commit=$(printf '%s\n' "$metadata" | sed -n 's/.*"Hash": "\([0-9a-f]*\)".*/\1/p' | head -1)

  if [[ ! -d "$sibling/.git" ]]; then
    printf '%-24s %-14s %-14s %s\n' "$repo" "${module_commit:0:12}" - NO_SIBLING
    failed=1
    continue
  fi
  sibling_head=$(git -C "$sibling" rev-parse HEAD 2>/dev/null || echo "")

  if [[ -z "$module_commit" ]]; then
    printf '%-24s %-14s %-14s %s\n' "$repo" unknown "${sibling_head:0:12}" UNRESOLVED
    failed=1
  elif git -C "$sibling" merge-base --is-ancestor "$module_commit" "$sibling_head" 2>/dev/null; then
    printf '%-24s %-14s %-14s %s\n' "$repo" "${module_commit:0:12}" "${sibling_head:0:12}" OK
  else
    printf '%-24s %-14s %-14s %s\n' "$repo" "${module_commit:0:12}" "${sibling_head:0:12}" AHEAD_OF_SIBLING
    failed=1
  fi
done

if ((failed)); then
  echo "workspace/module release parity failed" >&2
  exit 1
fi
