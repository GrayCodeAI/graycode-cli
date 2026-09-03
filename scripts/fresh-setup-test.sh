#!/usr/bin/env bash
# Fresh first-run /config test — build graycode and optional isolated ~/.graycode.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> Ensuring ecosystem repos are present (make setup)..."
make setup

echo ""
echo "==> Building graycode..."
go build -o graycode ./cmd/graycode/
echo "    $(./graycode --version 2>/dev/null || echo built)"

echo ""
echo "==> Credential status (macOS Keychain / secret service):"
./graycode credentials status || true

echo ""
echo "==> Optional: isolated config dir (sessions/settings separate from ~/.graycode)"
ISOLATED="${GRAYCODE_FRESH_CONFIG_DIR:-$(mktemp -d)/graycode-fresh}"
mkdir -p "$ISOLATED"
rm -f "$ISOLATED/learned_credential_prefixes.json"
rm -f "$ISOLATED/settings.json"
echo "    GRAYCODE_CONFIG_DIR=$ISOLATED"

echo ""
echo "To match first-run Setup (no API key in Keys tab):"
echo "  - Remove stored keys: ./graycode credentials remove <provider>   (e.g. anthropic, openai)"
echo "  - This script already clears the isolated settings.json so model selection starts fresh"
echo ""
echo "Run TUI (from $ROOT):"
echo "  export GRAYCODE_CONFIG_DIR=\"$ISOLATED\""
echo "  ./graycode"
echo ""
echo "In Setup: Keys → Add key · <Gateway> → paste → one probe → Models."
echo "After setup: ./graycode path  and  ./graycode preflight"
