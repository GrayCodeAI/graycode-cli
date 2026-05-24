#!/usr/bin/env bash
# Quick smoke test before releases or after ecosystem wiring changes.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BIN="${SMOKE_HAWK_BIN:-/tmp/hawk-smoke}"
echo "== build =="
go build -mod=readonly -o "$BIN" .

echo "== hawk doctor =="
"$BIN" doctor | head -5

echo "== hawk yaad =="
"$BIN" yaad --limit 2 >/dev/null || true

echo "== ecosystem panel =="
go test ./internal/config/ -run TestFormatEcosystemPanel -count=1

echo "== doctor + yaad tests =="
go test -race ./cmd/ -run 'TestDoctor|TestYaad' -count=1
go test -race ./internal/intelligence/memory/ -run 'FormatYaad' -count=1

echo "== smoke ok =="
