#!/usr/bin/env bash
# Quick smoke test before releases or after ecosystem wiring changes.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="${SMOKE_GRAYCODE_BIN:-/tmp/graycode-smoke}"
echo "== build =="
go build -mod=readonly -o "$BIN" ./cmd/graycode

echo "== graycode doctor =="
set +o pipefail
"$BIN" doctor >/dev/null 2>&1 || true
set -o pipefail

echo "== graycode ecosystem =="
"$BIN" ecosystem >/dev/null

echo "== graycode path =="
set +o pipefail
"$BIN" path >/dev/null 2>&1 || true
set -o pipefail

echo "== ecosystem tests =="
go test ./internal/config/ -run TestFormatEcosystemPanel -count=1
go test ./cmd/ -run 'TestDoctor|TestHarrier|TestEcosystem|TestPath' -count=1
go test ./internal/config/ -run 'DeveloperPath|FormatEcosystemPanel' -count=1
go test ./internal/intelligence/memory/ -run 'FormatHarrier|ShouldAutoRemember' -count=1

echo "== verify developer path =="
./scripts/verify-developer-path.sh

echo "== smoke ok =="
