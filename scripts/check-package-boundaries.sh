#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

go test ./internal/testaudit -run '^TestPackageDependencyGraph$' -count=1
echo "AST package boundary guard passed"
