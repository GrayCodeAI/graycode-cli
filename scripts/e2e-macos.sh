#!/usr/bin/env bash
# macOS End-to-End Test Suite for hawk
# Run: bash scripts/e2e-macos.sh
set -euo pipefail

PASS=0
FAIL=0

pass() { PASS=$((PASS+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); echo "  FAIL: $1"; }

# Ensure binary exists
if [ ! -f ./hawk ]; then
    echo "Building hawk..."
    go build -o hawk ./cmd/hawk
fi

echo "=== macOS E2E Tests ==="
echo

# 1. Version check
echo "--- version ---"
./hawk version 2>&1 | grep -q "[0-9]\+\.[0-9]\+\.[0-9]\+" && pass "version output" || fail "version output"

# 2. Doctor / preflight
echo "--- doctor ---"
./hawk doctor 2>&1 | head -20 | grep -qi "hawk\|ok\|check" && pass "doctor runs" || fail "doctor failed"

# 3. Preflight
echo "--- preflight ---"
./hawk preflight 2>&1 | head -20 && pass "preflight runs" || fail "preflight failed"

# 4. No API keys in provider.json
echo "--- provider.json has no plaintext API keys ---"
if [ -f ~/.hawk/provider.json ]; then
    if grep -qi '"api_key"' ~/.hawk/provider.json 2>/dev/null; then
        fail "provider.json contains api_key field"
    else
        pass "provider.json has no plaintext API keys"
    fi
else
    pass "provider.json not present (no setup done)"
fi

# 5. Credential store (macOS Keychain)
echo "--- credentials list ---"
./hawk credentials list 2>&1 | head -10 && pass "credentials list runs" || fail "credentials list failed"

# 6. Sandbox status
echo "--- sandbox status ---"
./hawk sandbox status 2>&1 | head -10 && pass "sandbox status runs" || fail "sandbox status failed"

# 7. Shell completions generate
echo "--- shell completions ---"
./hawk completion bash 2>&1 | head -5 | grep -q "complete\|completion" && pass "bash completions generated" || fail "bash completions"
./hawk completion zsh 2>&1 | head -5 | grep -q "compdef\|completion" && pass "zsh completions generated" || fail "zsh completions"

# 8. Help output
echo "--- help ---"
./hawk --help 2>&1 | head -5 | grep -qi "hawk\|usage\|flag" && pass "help output" || fail "help output"

# 9. Config subcommands
echo "--- config commands ---"
./hawk config --help 2>&1 | head -5 | grep -qi "manage\|config\|edit" && pass "config help" || fail "config help"

# 10. Session list (no-op test)
echo "--- sessions list ---"
./hawk sessions list 2>&1 | head -5 && pass "sessions list runs" || fail "sessions list"

echo
echo "=== Results: $PASS passed, $FAIL failed ==="
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
