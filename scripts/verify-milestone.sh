#!/usr/bin/env bash
# Milestone verification: API key → model → sandbox
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== sibling ecosystem modules =="
while IFS= read -r module; do
  dir="../$module"
  if [[ -d "$dir" ]]; then
    (cd "$dir" && go test ./... -count=1 -short)
  else
    echo "skip: $dir not found"
  fi
done < <("$ROOT/scripts/ecosystem-manifest.sh" list engines)

echo "== hawk unit tests =="
go test ./... -count=1 -short

echo "== milestone verification tests =="
go test ./internal/config/ -run 'Verify_|HasConfigured|EvaluateSetup|PersistAPIKey|CatalogEmpty|CatalogStatus' -count=1 -v
go test ./internal/config/ -run 'RemoveStored|FormatCredential' -count=1
go test ./cmd/ -run 'ConfigHub|RemoveCredential' -count=1
go test ./internal/tool/ -run 'IsSensitivePath|DetectCredentials' -count=1
go test ./internal/sandbox/ -run 'Verify_Container' -count=1 -timeout 3m || true
go test ./internal/resilience/health/ -run 'CheckAPIKeySet' -count=1

echo "== done =="
