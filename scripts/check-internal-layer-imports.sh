#!/usr/bin/env bash
set -euo pipefail

failed=0

check_layer() {
  local directory=$1
  local forbidden=$2
  local matches
  matches=$(git grep -n -E "github.com/GrayCodeAI/graycode-cli/(${forbidden})([\"/])" -- "${directory}/**/*.go" \
    ':(exclude)**/*_test.go' 2>/dev/null || true)
  if [[ -n "$matches" ]]; then
    echo "forbidden upward/sideways imports from ${directory}:" >&2
    echo "$matches" >&2
    failed=1
  fi
}

# These are existing stable package boundaries. Delivery packages may compose
# them, but domain/runtime packages must not depend back on delivery or concrete
# platform/bridge adapters.
check_layer internal/engine 'cmd|internal/daemon|internal/platform|internal/bridge'
check_layer internal/permissions 'cmd|internal/daemon|internal/engine|internal/platform|internal/bridge'
check_layer internal/session 'cmd|internal/daemon|internal/engine|internal/platform|internal/bridge'
check_layer internal/platform 'cmd|internal/daemon|internal/engine|internal/bridge'
check_layer internal/bridge 'cmd|internal/daemon|internal/engine|internal/platform'

if ((failed)); then
  echo "Graycode internal layer boundary check failed" >&2
  exit 1
fi
echo "Graycode internal layer boundary check passed"
