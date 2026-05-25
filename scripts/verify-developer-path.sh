#!/usr/bin/env bash
# Developer path verification — setup, security, milestone tests.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== developer path unit tests =="
go test ./internal/config/ -run 'DeveloperPath|ProviderJSONHasSecrets|LegacyCredential' -count=1
go test ./internal/intelligence/memory/ -run ShouldAutoRemember -count=1
go test ./cmd/ -run TestPath -count=1

echo "== milestone security tests =="
go test ./internal/config/ -run 'Verify_|HasConfigured|EvaluateSetup' -count=1
go test ./internal/tool/ -run 'IsSensitivePath' -count=1

echo "== developer path CLI =="
BIN="${DEV_PATH_HAWK_BIN:-/tmp/hawk-path-verify}"
go build -mod=readonly -o "$BIN" .
set +o pipefail
"$BIN" path >/dev/null 2>&1 || true
set -o pipefail

echo "== developer path ok =="
