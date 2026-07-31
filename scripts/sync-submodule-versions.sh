#!/usr/bin/env bash
# Bump every external/ submodule's requirement in hawk's go.mod to the exact
# commit the submodule pointer (gitlink) is pinned to.
#
# Hawk tracks its engine dependencies two ways:
#   1. as git submodules  external/<repo>
#   2. as Go module requires  github.com/GrayCodeAI/<repo> <version>
# in go.mod. These must agree, otherwise `make submodule-release-parity` fails.
#
# `make sync-submodules` advances the submodule working trees, but does NOT
# update go.mod. This script is the companion "sync-versions" step: for every
# submodule that maps to a go.mod require, it runs `go get <module>@<gitlink>`
# so the pseudo-version in go.mod resolves to the same commit as the gitlink.
#
# Run from the repo root:
#     make sync-submodules              # advance + checkout external/ trees
#     make sync-submodule-versions      # THEN bump go.mod to match
#
# The script is read-only until it writes — it prints each update and exits
# non-zero on the first `go get` that can't resolve a published commit.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ ! -f .gitmodules ]]; then
  echo "sync-submodule-versions: no .gitmodules — nothing to do"
  exit 0
fi

# Submodule paths that are also Go modules hawk requires.
# (Derived from WORKSPACE_REPOS in Makefile plus hawk-mcpkit.) Parallel arrays
# keep this portable across bash 3.2 (macOS /bin/bash) and bash 5.x (CI).
repos=(
  eyrie
  inspect
  sight
  tok
  trace
  yaad
  hawk-core-contracts
  hawk-mcpkit
)
mods=(
  github.com/GrayCodeAI/eyrie
  github.com/GrayCodeAI/inspect
  github.com/GrayCodeAI/sight
  github.com/GrayCodeAI/tok
  github.com/GrayCodeAI/trace
  github.com/GrayCodeAI/yaad
  github.com/GrayCodeAI/hawk-core-contracts
  github.com/GrayCodeAI/hawk-mcpkit
)

echo "Syncing go.mod require versions to external/ submodule gitlinks:"
exit_code=0
for i in "${!repos[@]}"; do
  repo="external/${repos[$i]}"
  mod="${mods[$i]}"
  gitlink="$(git ls-tree HEAD "$repo" 2>/dev/null | awk '{print $3}')"
  if [[ -z "$gitlink" ]]; then
    echo "  $repo: MISSING_GITLINK (submodule never committed) — skipping"
    continue
  fi
  echo "  $repo ($mod): $gitlink -> go get $mod@$gitlink"
  if ! go get "$mod@$gitlink"; then
    echo "  ERROR: could not resolve $mod@$gitlink (commit must be pushed)" >&2
    exit_code=1
  fi
done

if [[ $exit_code -ne 0 ]]; then
  echo "sync-submodule-versions: one or more go get calls failed" >&2
  exit $exit_code
fi

echo "Tidying go.mod / go.sum..."
go mod tidy

echo "Verifying submodule/module release parity..."
bash ./scripts/check-submodule-release-parity.sh

echo
echo "Sync complete. go.mod now tracks each external/ submodule's gitlink."
echo "Review with:  git diff go.mod go.sum"
echo "Then commit:  git add go.mod go.sum external/ && git commit -m 'chore: sync external/ module versions'"
