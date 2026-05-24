#!/usr/bin/env bash
# Quick smoke test before releases or after ecosystem wiring changes.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="${SMOKE_HAWK_BIN:-/tmp/hawk-smoke}"
echo "== build =="
go build -mod=readonly -o "$BIN" .

echo "== hawk doctor =="
set +o pipefail
"$BIN" doctor >/dev/null 2>&1 || true
set -o pipefail

echo "== hawk ecosystem =="
"$BIN" ecosystem >/dev/null

echo "== hawk solo =="
set +o pipefail
"$BIN" solo >/dev/null 2>&1 || true
set -o pipefail

echo "== hawk yaad =="
"$BIN" yaad --limit 2 >/dev/null || true
"$BIN" yaad search decision --limit 2 >/dev/null || true

echo "== ecosystem tests =="
go test ./internal/config/ -run TestFormatEcosystemPanel -count=1
go test ./cmd/ -run 'TestDoctor|TestYaad|TestEcosystem|TestSolo' -count=1
go test ./internal/config/ -run 'Solo|FormatEcosystemPanel' -count=1
go test ./internal/intelligence/memory/ -run 'FormatYaad|ShouldAutoRemember' -count=1

echo "== verify solo path =="
./scripts/verify-solo-path.sh

echo "== smoke ok =="
