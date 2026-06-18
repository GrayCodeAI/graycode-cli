#!/usr/bin/env bash
# Commit without IDE-injected Co-authored-by trailers (Cursor, etc.).
# Usage: ./scripts/commit-clean.sh [-m "message"] [other git commit args...]
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
exec git -c core.hooksPath=/dev/null commit "$@"
