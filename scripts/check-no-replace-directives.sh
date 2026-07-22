#!/usr/bin/env bash
# CI guard: fail if any go.mod in this workspace has a local replace directive.
# Local replace directives must not be present when tagging a release.
set -euo pipefail

errors=0
while IFS= read -r modfile; do
  if grep -qE '^replace .+ => \.\./' "$modfile"; then
    echo "ERROR: local replace directive found in $modfile"
    grep -nE '^replace .+ => \.\./' "$modfile"
    errors=$((errors + 1))
  fi
done < <(find . -name 'go.mod' -not -path '*/vendor/*' -not -path '*/.gocache/*' -not -path '*/external/*')

if [ $errors -gt 0 ]; then
  echo "\nFail: $errors go.mod file(s) have local replace directives that must be removed before tagging a release."
  exit 1
fi
echo "OK: no local replace directives found."
