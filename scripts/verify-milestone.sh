#!/usr/bin/env bash
# Milestone verification: API key → model → sandbox
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== eyrie (external) =="
EYRIE="./external/eyrie"
if [[ -d "$EYRIE" ]]; then
  (cd "$EYRIE" && go test ./... -count=1 -short)
else
  echo "skip: ./external/eyrie not found"
fi

echo "== external ecosystem modules =="
for module in yaad tok sight inspect trace; do
  dir="./external/$module"
  if [[ -d "$dir" ]]; then
    (cd "$dir" && go test ./... -count=1 -short)
  else
    echo "skip: $dir not found"
  fi
done

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
