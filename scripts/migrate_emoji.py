#!/usr/bin/env python3
r"""
Migrate emoji in internal/ files to icons.*() calls or text tokens.

The script edits a Go source file in place, adds the icons import if
missing, and is safe to re-run (idempotent). It also handles Go string
literals that contain \uXXXX or \UXXXXXXXX escape sequences encoding
emoji.

We do not use a regex to find string boundaries because Python's re
engine uses leftmost-first matching, which can split a string like
`"\U0001f534"` at the wrong quote. Instead we walk char-by-char.
"""

import re
import sys

# Emoji -> icons.*() call (for status glyphs that need a real icon).
STATUS = {
    '\u2713': 'icons.CheckBold()',  # ✓
    '\u2714': 'icons.CheckBold()',  # ✔
    '\u2717': 'icons.CloseThick()',  # ✗
    '\u2715': 'icons.CloseThick()',  # ✕
    '\u2705': 'icons.CheckBold()',  # ✅
    '\u274c': 'icons.CloseThick()',  # ❌
    '\u26a0': 'icons.Alert()',  # ⚠
    '\u26a1': 'icons.Bolt()',  # ⚡
    '\U0001f4a1': 'icons.Brain()',  # 💡
    '\U0001f50d': 'icons.Magnify()',  # 🔍
    '\u2753': 'icons.Question()',  # ❓
    '\u23f8': 'icons.Pause()',  # ⏸
    '\u23f3': 'icons.Hourglass()',  # ⏳
    '\u2699': 'icons.Cog()',  # ⚙
}

# Emoji -> text token (for risk colors and decorative icons where the
# shape/color is the only meaning; the label carries the semantics).
TOKEN = {
    '\U0001f7e2': 'LOW',
    '\U0001f7e1': 'MED',
    '\U0001f7e0': 'HIGH',
    '\U0001f534': 'CRIT',
    '\u26aa': '-',
    '\U0001f535': 'INFO',
    '\U0001f527': 'FIX:',
    '\U0001f4ad': 'NOTE:',
    '\U0001f6a8': 'ALERT:',
    '\U0001f512': 'LOCK:',
    '\U0001f4e6': 'PKG:',
    '\U0001f4dd': 'EDIT:',
    '\U0001f4cc': 'PIN:',
    '\U0001f4cb': 'LIST:',
    '\U0001f4bb': 'DEV:',
    '\U0001f441': 'VIEW:',
    '\U0001f41b': 'BUG:',
    '\U0001f3d7': 'BUILD:',
    '\U0001f3a8': 'STYLE:',
    '\U0001f30d': 'GLOBAL:',
    '\U0001f6e1': 'SHIELD:',
    '\U0001f6d1': 'STOP:',
    '\u2606': 'o',
    '\u2605': '*',
    '\u23f0': '[time]',
    '\u270f': '[edit]',
    '\u26d4': '[deny]',
}

ALL = {**STATUS, **TOKEN}

# Go escape sequences for emoji — needed because the audit test reports
# the rune codepoint regardless of how it was written in source.
GO_ESCAPE_FOR = {
    '\u2713': '\\u2713', '\u2714': '\\u2714',
    '\u2717': '\\u2717', '\u2715': '\\u2715',
    '\u2705': '\\u2705', '\u274c': '\\u274c',
    '\u26a0': '\\u26a0', '\u26a1': '\\u26a1',
    '\U0001f4a1': '\\U0001f4a1', '\U0001f50d': '\\U0001f50d',
    '\u2753': '\\u2753', '\u23f8': '\\u23f8',
    '\u23f3': '\\u23f3', '\u2699': '\\u2699',
    '\U0001f7e2': '\\U0001f7e2', '\U0001f7e1': '\\U0001f7e1',
    '\U0001f7e0': '\\U0001f7e0', '\U0001f534': '\\U0001f534',
    '\u26aa': '\\u26aa', '\U0001f535': '\\U0001f535',
    '\U0001f527': '\\U0001f527', '\U0001f4ad': '\\U0001f4ad',
    '\U0001f6a8': '\\U0001f6a8', '\U0001f512': '\\U0001f512',
    '\U0001f4e6': '\\U0001f4e6', '\U0001f4dd': '\\U0001f4dd',
    '\U0001f4cc': '\\U0001f4cc', '\U0001f4cb': '\\U0001f4cb',
    '\U0001f4bb': '\\U0001f4bb', '\U0001f441': '\\U0001f441',
    '\U0001f41b': '\\U0001f41b', '\U0001f3d7': '\\U0001f3d7',
    '\U0001f3a8': '\\U0001f3a8', '\U0001f30d': '\\U0001f30d',
    '\U0001f6e1': '\\U0001f6e1', '\U0001f6d1': '\\U0001f6d1',
    '\u2606': '\\u2606', '\u2605': '\\u2605',
    '\u23f0': '\\u23f0', '\u270f': '\\u270f',
    '\u26d4': '\\u26d4',
}

# Build the set of strings to match in source. Each emoji character has
# both a literal form and (sometimes) an escape form.
ALL_FORMS = {}
for ch, replacement in ALL.items():
    ALL_FORMS[ch] = replacement
    if ch in GO_ESCAPE_FOR:
        ALL_FORMS[GO_ESCAPE_FOR[ch]] = replacement

# Pre-compute the status-form set for quick lookup.
STATUS_ESCAPES = {GO_ESCAPE_FOR[c] for c in STATUS}


def transform_string(s):
    """Replace emoji inside a single Go string literal `s` (with
    surrounding quotes) with a Go concatenation expression that uses
    icons.*() or text tokens."""
    if len(s) < 2:
        return s
    body = s[1:-1]
    present = [form for form in ALL_FORMS if form in body]
    if not present:
        return s
    segs = [('text', body)]
    for form in present:
        replacement = ALL_FORMS[form]
        is_status = form in STATUS or form in STATUS_ESCAPES
        new_segs = []
        for kind, val in segs:
            if kind == 'text' and form in val:
                parts = val.split(form)
                for i, p in enumerate(parts):
                    if i > 0:
                        if is_status:
                            new_segs.append(('sub', replacement))
                        else:
                            new_segs.append(('subq', '"' + replacement + '"'))
                    if p:
                        new_segs.append(('text', p))
            else:
                new_segs.append((kind, val))
        segs = new_segs
    out = []
    for kind, val in segs:
        if kind == 'text':
            if val:
                out.append('"' + val + '"')
        elif kind == 'sub':
            out.append(val)
        elif kind == 'subq':
            out.append(val)
    return ' + '.join(out)


def replace_in_strings(content: str) -> str:
    r"""Find each Go string literal, split on emoji, emit concatenation.

    We do not use a regex here. Python's re engine uses leftmost-first
    matching, which can match a string like `"\U0001f534"` as either
    `""` (empty body) or `"\U0001f534"` (full body), picking the
    shorter match first. We walk char-by-char instead.
    """
    pieces = []
    i = 0
    n = len(content)
    while i < n:
        if content[i] == '"':
            j = i + 1
            closed = False
            while j < n:
                c = content[j]
                if c == '\\':
                    if j + 1 >= n:
                        break
                    nxt = content[j + 1]
                    if nxt == 'u':
                        j += 6
                    elif nxt == 'U':
                        j += 10
                    elif nxt == 'x':
                        j += 4
                    elif nxt.isdigit():
                        k = j + 2
                        while k < n and k - (j + 1) < 3 and content[k].isdigit():
                            k += 1
                        j = k
                    else:
                        j += 2
                    continue
                if c == '"':
                    s = content[i:j + 1]
                    pieces.append(transform_string(s))
                    # Advance past the closing `"`. The next
                    # iteration starts AFTER this string.
                    i = j + 1
                    closed = True
                    break
                j += 1
            if not closed:
                pieces.append(content[i])
                i += 1
        else:
            pieces.append(content[i])
            i += 1
    return ''.join(pieces)


def ensure_icons_import(content: str) -> str:
    if 'github.com/GrayCodeAI/hawk/internal/ui/icons' in content:
        return content
    m2 = re.search(r'^import\s+"([^"]+)"\s*$', content, re.MULTILINE)
    if m2:
        old = m2.group(0)
        new = 'import (\n\t"' + m2.group(1) + '"\n\t"github.com/GrayCodeAI/hawk/internal/ui/icons"\n)'
        return content.replace(old, new, 1)
    m = re.search(r'^(import \([^)]+\))', content, re.MULTILINE)
    if not m:
        return content
    block = m.group(1)
    new_block = block.replace(
        '\n)',
        '\n\n\t"github.com/GrayCodeAI/hawk/internal/ui/icons"\n)',
        1,
    )
    return content.replace(block, new_block, 1)


def migrate(path: str) -> bool:
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()
    new_content = replace_in_strings(content)
    if new_content != content:
        if 'icons.' in new_content:
            new_content = ensure_icons_import(new_content)
        with open(path, 'w', encoding='utf-8') as f:
            f.write(new_content)
        return True
    return False


if __name__ == '__main__':
    if len(sys.argv) < 2:
        print('usage: migrate_emoji.py FILE...')
        sys.exit(1)
    changed = 0
    for p in sys.argv[1:]:
        if migrate(p):
            print(f'changed: {p}')
            changed += 1
    print(f'total changed: {changed}')
