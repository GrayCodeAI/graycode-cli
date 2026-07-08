#!/usr/bin/env bash
# Hard-reset hawk and all external/ submodules to origin/main.
# Use after a history rewrite or when your clone has stale SHAs.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# Guard: this script hard-resets hawk AND all submodules, destroying any
# uncommitted work. Refuse to run on a dirty tree unless explicitly forced.
if [ -n "$(git status --porcelain)" ] && [ "${SYNC_CLONE_FORCE:-}" != "1" ]; then
  echo "sync-clone: working tree has uncommitted changes." >&2
  echo "This will HARD-RESET hawk and all external/ submodules to origin/main." >&2
  echo "Commit/stash your work, or re-run with SYNC_CLONE_FORCE=1 to proceed." >&2
  exit 1
fi

echo "==> Fetching origin"
git fetch origin

echo "==> Resetting hawk to origin/main"
git checkout main
git reset --hard origin/main

echo "==> Updating submodules"
git submodule update --init --recursive
git submodule foreach 'git fetch origin && git checkout origin/main 2>/dev/null || git checkout origin/HEAD'

echo "==> Done. hawk: $(git rev-parse --short HEAD)"
git submodule status
