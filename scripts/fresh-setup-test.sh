#!/usr/bin/env bash
# Fresh first-run /config test — build hawk and optional isolated ~/.hawk.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> Ensuring ecosystem repos are present (make setup)..."
make setup

echo ""
echo "==> Building hawk..."
go build -o hawk ./cmd/hawk/
echo "    $(./hawk --version 2>/dev/null || echo built)"

echo ""
echo "==> Credential status (macOS Keychain / secret service):"
./hawk credentials status || true

echo ""
echo "==> Optional: isolated config dir (sessions/settings separate from ~/.hawk)"
ISOLATED="${HAWK_FRESH_CONFIG_DIR:-$(mktemp -d)/hawk-fresh}"
mkdir -p "$ISOLATED"
rm -f "$ISOLATED/learned_credential_prefixes.json"
rm -f "$ISOLATED/settings.json"
echo "    HAWK_CONFIG_DIR=$ISOLATED"

echo ""
echo "To match first-run Setup (no API key in Keys tab):"
echo "  - Remove stored keys: ./hawk credentials remove <provider>   (e.g. anthropic, openai)"
echo "  - This script already clears the isolated settings.json so model selection starts fresh"
echo ""
echo "Run TUI (from $ROOT):"
echo "  export HAWK_CONFIG_DIR=\"$ISOLATED\""
echo "  ./hawk"
echo ""
echo "In Setup: Keys → Add key · <Gateway> → paste → one probe → Models."
echo "After setup: ./hawk path  and  ./hawk preflight"
