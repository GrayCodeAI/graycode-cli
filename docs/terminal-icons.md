# Terminal icons

Graycode uses current Nerd Font Codicon glyphs for interactive terminal output.
The application does not try to infer the installed font from `TERM`: terminal
names do not report the active font, and guessing can produce tiny fallback
boxes or missing glyphs.

Interactive TTYs use Nerd Font icons by default. Captured output, CI, and
`NO_COLOR` use ASCII automatically. Select the tier explicitly when needed:

```bash
# Real icons (requires a Nerd Font configured in the terminal profile)
GRAYCODE_ICONS=nerd ./bin/graycode

# Portable text-only output
GRAYCODE_ICONS=ascii ./bin/graycode
```

For the real icons, configure the terminal profile—not Graycode's Go code—with a
patched font such as `JetBrainsMono Nerd Font` or `Symbols Nerd Font Mono`.
Font size and glyph scale are controlled by that profile. Graycode applies bold
weight to status icons for contrast, but there is no portable ANSI escape that
can resize one glyph independently of the surrounding text.
