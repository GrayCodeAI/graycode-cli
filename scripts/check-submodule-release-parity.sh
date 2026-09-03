#!/usr/bin/env bash
set -euo pipefail

# Deprecated compatibility entry point. The ecosystem is multi-repository;
# use check-module-release-parity.sh for the canonical name.
#
# Workspace release parity: verify every ecosystem module graycode requires
# resolves to a published, reachable commit. This catches go.mod pins to
# versions that do not exist. Run from the graycode repo root.
#
# Note: it deliberately does NOT compare against sibling HEAD ancestry —
# CI clones the siblings as shallow (depth-1) checkouts, so older pinned
# commits are not present in their history and ancestry checks would
# falsely report AHEAD. Resolution (go mod download) is the robust signal.

exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check-module-release-parity.sh" "$@"
