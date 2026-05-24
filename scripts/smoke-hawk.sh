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

echo "== hawk yaad =="
"$BIN" yaad --limit 2 >/dev/null || true

echo "== ecosystem tests =="
go test ./internal/config/ -run TestFormatEcosystemPanel -count=1
go test ./cmd/ -run 'TestDoctor|TestYaad|TestEcosystem' -count=1
go test ./internal/intelligence/memory/ -run 'FormatYaad' -count=1

echo "== smoke ok =="
