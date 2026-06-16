#!/usr/bin/env python3
r"""
Update test files that assert on literal emoji to use icons.*() helpers
instead.

For a string literal like "✓ foo" used in a test assertion, we want the
test to assert on icons.CheckBold() + " foo" so the test is mode-aware
(Nerd Font vs ASCII). The script:

  1. Finds every string literal that contains a tracked emoji.
  2. Splits the body on each emoji and emits a Go concatenation
     expression: "text1" + icons.X() + "text2" + icons.Y() + "text3".

This works for `strings.Contains(s, "✓ foo")` because the inner
concatenation evaluates to the same string at runtime.

For bare-emoji strings ("✓"), the whole expression becomes the
function call: `icons.CheckBold()`. This is used in equality tests
like `assertEquals(got, "✓")` — the result is `assertEquals(got, icons.CheckBold())`.
"""

import re
import sys

# Status emoji -> icons.*() call.
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


def transform_string_literal(s):
    """Take a Go string literal `s` (with surrounding quotes) and return
    a Go expression that evaluates to the same string. Emoji in the
    body are replaced with the corresponding icons.*() call.
    """
    if len(s) < 2:
        return s
    body = s[1:-1]
    present = [ch for ch in STATUS if ch in body]
    if not present:
        return s
    # Special case: body is just one emoji. Replace with the call.
    if len(present) == 1 and body == present[0]:
        return STATUS[present[0]]
    # Otherwise: split body on each emoji and emit concatenation.
    # After splitting, segs is a list of (kind, val) tuples:
    #   ('text', '...')  — body text segment
    #   ('sub', '...')   — icons.*() call (starts with ' + ' when joined)
    segs = [('text', body)]
    for ch in present:
        new_segs = []
        for kind, val in segs:
            if kind == 'text' and ch in val:
                parts = val.split(ch)
                for i, p in enumerate(parts):
                    if i > 0:
                        new_segs.append(('sub', STATUS[ch]))
                    if p:
                        new_segs.append(('text', p))
            else:
                new_segs.append((kind, val))
        segs = new_segs
    out = []
    for kind, val in segs:
        if kind == 'text':
            out.append('"' + val + '"')
        else:
            out.append(val)
    return ' + '.join(out)


def replace_in_test_strings(content: str) -> str:
    pattern = re.compile(r'"(?:[^"\\]|\\.)*"')

    def sub(m):
        return transform_string_literal(m.group(0))

    return pattern.sub(sub, content)


def migrate(path: str) -> bool:
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()
    new_content = replace_in_test_strings(content)
    if new_content != content:
        if 'icons.' in new_content:
            new_content = ensure_icons_import(new_content)
        with open(path, 'w', encoding='utf-8') as f:
            f.write(new_content)
        return True
    return False


if __name__ == '__main__':
    if len(sys.argv) < 2:
        print('usage: migrate_emoji_tests.py FILE...')
        sys.exit(1)
    changed = 0
    for p in sys.argv[1:]:
        if migrate(p):
            print(f'changed: {p}')
            changed += 1
    print(f'total changed: {changed}')
