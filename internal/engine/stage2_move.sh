#!/usr/bin/env bash
# Stage 2 engine sub-package move script.
# Usage: ./stage2_move.sh <subpackage> <file_glob_pattern>
# Example: ./stage2_move.sh git "git_*"

set -euo pipefail

SUBPKG="$1"
PATTERN="$2"

cd "$(dirname "$0")"  # cd to internal/engine/

echo "=== Stage 2: moving $SUBPKG/ ==="

# Gather matching non-test source files
FILES=$(ls $PATTERN.go 2>/dev/null || true)
TFILES=$(ls $PATTERN\_test.go 2>/dev/null || true)

if [ -z "$FILES" ]; then
  echo "No files matching $PATTERN.go found"
  exit 0
fi

echo "Source files: $FILES"
echo "Test files: $TFILES"

# 1. Read aliases.go to find all re-exported symbols
SYMBOLS=$(grep -E "^type\s+\w+\s+=\s+engine\.\w+" "$SUBPKG/aliases.go" 2>/dev/null || true)
FUNCS=$(grep -E "^func\s+\w+" "$SUBPKG/aliases.go" 2>/dev/null || true)
echo "Symbols to re-export: $(echo "$SYMBOLS" | wc -l) types, $(echo "$FUNCS" | wc -l) funcs"

# 2. Update package declaration in source files
for f in $FILES; do
  sed -i '' 's/^package engine/package '"$SUBPKG"'/' "$f"
  echo "  Updated $f -> package $SUBPKG"
done

# 3. Update package declaration in test files
for f in $TFILES; do
  sed -i '' 's/^package engine/package '"$SUBPKG"'/' "$f"
  echo "  Updated $f -> package $SUBPKG"
done

# 4. Copy source files to sub-package
for f in $FILES; do
  cp "$f" "$SUBPKG/$f"
  rm "$f"
  echo "  Moved $f -> $SUBPKG/"
done

# 5. Copy test files to sub-package  
for f in $TFILES; do
  cp "$f" "$SUBPKG/$f"
  rm "$f"
  echo "  Moved $f -> $SUBPKG/"
done

# 6. Update aliases.go: remove engine import, make aliases local
if [ -f "$SUBPKG/aliases.go" ]; then
  # Remove the import of engine
  sed -i '' '/^import/d' "$SUBPKG/aliases.go"
  sed -i '' '/^[[:space:]]*"github.com\/GrayCodeAI\/graycode\/internal\/engine"/d' "$SUBPKG/aliases.go"
  sed -i '' '/^[[:space:]]*)/d' "$SUBPKG/aliases.go"
  # Update type aliases: engine.Xxx -> Xxx
  sed -i '' 's/= engine\./=/g' "$SUBPKG/aliases.go"
  # Update func wrappers: engine.Func -> Func
  sed -i '' 's/return engine\./return /g' "$SUBPKG/aliases.go"
  # Update doc comment
  sed -i '' 's/Stage-1 namespace for/retry-queue and smart-retry types for/' "$SUBPKG/aliases.go"
  echo "  Updated $SUBPKG/aliases.go"
fi

# 7. Create re-exports file in engine root
REEXPORT="engine/${SUBPKG}_reexports.go"
{
  echo "// This file re-exports symbols from the $SUBPKG sub-package so that existing"
  echo "// callers of engine.* keep compiling during Stage 2 migration."
  echo "// See docs/plans/engine-refactor-plan.md."
  echo "package engine"
  echo ""
  echo "import \"github.com/GrayCodeAI/graycode-cli/internal/engine/$SUBPKG\""
  echo ""
  # Extract type aliases from the original aliases.go - but with names that match the engine-package naming
  # For each type alias "type Foo = engine.BarFoo", create "type BarFoo = subpkg.Foo"
  # Actually, the types are already named correctly in the source files (RetryQueue, GitContext, etc.)
  # So we need to figure out the original names from the source files
} > "$REEXPORT"

echo "=== $SUBPKG/ done ==="
