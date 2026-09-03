# Terminal Support and Troubleshooting

Graycode runs as a full-screen TUI powered by Bubble Tea. This covers terminal compatibility and common fixes.

---

## Quick Fixes

### Truecolor (24-bit colors)

```bash
# Add to ~/.zshrc or ~/.bashrc
export COLORTERM=truecolor
```

Inside tmux, add to `~/.tmux.conf`:

```tmux
set -g default-terminal "tmux-256color"
set -as terminal-features ",*:RGB"
```

---

## Terminal Detection

Graycode detects these terminals:

- Apple Terminal
- iTerm2
- Ghostty
- Kitty
- WezTerm
- VS Code / Cursor / Windsurf / Zed terminals
- Alacritty
- GNOME Terminal (VTE)
- Windows Terminal

Run `/terminal-setup` for diagnostics.

---

## Common Problems

### Colors Look Wrong

**Cause**: Terminal not configured for truecolor.

**Fix**: Set `COLORTERM=truecolor` and configure tmux for RGB.

### Clipboard Not Working

**Cause**: Terminal doesn't support OSC 52.

**Fix**:
- iTerm2: Enable "Applications in terminal may access clipboard"
- Ghostty: Add `clipboard-provider = "work-area"` to config
- tmux: Add `set -g set-clipboard on`

### Fullscreen Not Activating

**Cause**: Zellij, tmux control mode, or config.

**Fix**: Set in `~/.graycode/settings.json`:

```json
{ "terminal": { "alt_screen": "always" } }
```

### Shift+Enter Not Working in VS Code

**Cause**: xterm.js partial Kitty protocol support.

**Fix**: Use **Alt+Enter** for newlines.

---

## tmux Configuration

Recommended settings in `~/.tmux.conf`:

```tmux
set -g default-terminal "tmux-256color"
set -as terminal-features ",*:RGB"
set -g set-clipboard on
set -g allow-passthrough on
```

---

## WezTerm Configuration

Add to `~/.config/wezterm/wezterm.lua`:

```lua
config.enable_kitty_keyboard = true
```

---

## SSH Considerations

Over SSH:
- OSC 52 clipboard may not work (Apple Terminal limitation)
- Use terminal-native paste (`Shift+Insert` or `Shift+Middle Click`)
- Run `/terminal-setup` to verify detection

---

## Multiplexers

### Zellij

Zellij intercepts many Ctrl/Alt keys. Use:
- **Unlock-First mode** (recommended)
- Press `Ctrl+O` then `C` to enter configuration
- Select "Unlock-First (non-colliding)"

### tmux

Add to config:
```tmux
set -g allow-passthrough on
set -g set-clipboard on
```

---

## Diagnostics

Run in Graycode:

```
/terminal-setup
```

Shows:
- Terminal detection
- Color level
- Available themes
- Clipboard routes
- Fix recommendations

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Permissions](22-permissions-and-safety.md) | Safety controls |
| [Dashboard](23-dashboard.md) | HUD and monitoring |

---

© 2026 GrayCode AI. All rights reserved.