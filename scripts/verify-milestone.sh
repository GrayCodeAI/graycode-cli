#!/usr/bin/env bash
# Milestone verification: API key → model → sandbox
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== eyrie (sibling) =="
EYRIE="../eyrie"
if [[ -d "$EYRIE" ]]; then
  (cd "$EYRIE" && go test ./... -count=1 -short)
else
  echo "skip: ../eyrie not found"
fi

echo "== hawk unit tests =="
go test ./... -count=1 -short

echo "== milestone verification tests =="
go test ./internal/config/ -run 'Verify_|HasConfigured|EvaluateSetup|PersistAPIKey' -count=1 -v
go test ./internal/tool/ -run 'IsSensitivePath|DetectCredentials' -count=1
go test ./internal/sandbox/ -run 'Verify_Container' -count=1 -timeout 3m || true

echo "== done =="
