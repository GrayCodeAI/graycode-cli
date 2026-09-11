#!/usr/bin/env python3
"""Analyze which engine re-exported symbols are used by external packages."""

import os
import re
import subprocess

# Get all re-exported symbols
reexport_files = []
engine_dir = "internal/engine"
for f in os.listdir(engine_dir):
    if f.endswith("_reexports.go"):
        reexport_files.append(os.path.join(engine_dir, f))

symbols = {}
for f in reexport_files:
    subpkg = os.path.basename(f).replace("_reexports.go", "")
    symbols[subpkg] = []
    with open(f) as fh:
        for line in fh:
            m = re.search(r'type\s+(\w+)\s*=|var\s+(\w+)\s*=|const\s+(\w+)\s*=', line)
            if m:
                sym = m.group(1) or m.group(2) or m.group(3)
                symbols[subpkg].append(sym)

# Find external files importing engine
result = subprocess.run(
    ['rg', '-l', '"github.com/GrayCodeAI/hawk/internal/engine"', '-g', '*.go'],
    capture_output=True, text=True
)
external_files = [f for f in result.stdout.strip().split('\n') if f and 'internal/engine/' not in f]

print(f"Re-exported symbols by sub-package:")
for subpkg, syms in symbols.items():
    print(f"  {subpkg}: {len(syms)} symbols")

print(f"\nExternal files importing engine: {len(external_files)}")

# Check which re-exported symbols each external file uses
for ext_file in sorted(external_files):
    used = {}
    try:
        with open(ext_file) as fh:
            content = fh.read()
        for subpkg, syms in symbols.items():
            for sym in syms:
                if re.search(rf'\bengine\.{sym}\b', content):
                    used.setdefault(subpkg, []).append(sym)
    except Exception as e:
        print(f"  ERROR reading {ext_file}: {e}")
        continue
    if used:
        print(f"\n{ext_file}:")
        for subpkg, syms in used.items():
            print(f"  {subpkg}: {', '.join(syms)}")
