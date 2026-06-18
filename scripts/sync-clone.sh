#!/usr/bin/env bash
# Hard-reset hawk and all external/ submodules to origin/main.
# Use after a history rewrite or when your clone has stale SHAs.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

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
