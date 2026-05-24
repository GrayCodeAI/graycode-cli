#!/usr/bin/env bash
# Solo developer path verification — setup, security, milestone tests.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== solo path unit tests =="
go test ./internal/config/ -run 'Solo|ProviderJSONHasSecrets|LegacyCredential' -count=1
go test ./internal/intelligence/memory/ -run ShouldAutoRemember -count=1
go test ./cmd/ -run TestSolo -count=1

echo "== milestone security tests =="
go test ./internal/config/ -run 'Verify_|HasConfigured|EvaluateSetup' -count=1
go test ./internal/tool/ -run 'IsSensitivePath' -count=1

echo "== solo path CLI =="
BIN="${SOLO_HAWK_BIN:-/tmp/hawk-solo-verify}"
go build -mod=readonly -o "$BIN" .
set +o pipefail
"$BIN" solo >/dev/null 2>&1 || true
set -o pipefail

echo "== solo path ok =="
