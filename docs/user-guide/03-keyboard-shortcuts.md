# Keyboard Shortcuts

Reference for key bindings in the Hawk TUI. Bindings are built-in and cannot currently be remapped.

---

## Input Modes

Hawk has two input modes that control how you navigate the scrollback:

- **Simple mode** (default): Arrow keys for navigation, `Shift+Arrow` for turn navigation, `Space` to focus the prompt
- **Vim mode** (opt-in): `j`/`k` for navigation, `H`/`L` for turn navigation, `h`/`l` for fold, `Tab` to focus the prompt

Simple mode is active by default. To switch to Vim mode:

```
/vim-mode
```

Or set `vim_mode = true` under `[ui]` in `~/.hawk/settings.json`:

```json
{
  "ui": {
    "vim_mode": true
  }
}
```

---

## Navigation (Scrollback Focused)

Move through conversation entries in the scrollback pane.

| Key | Alt Key | Action |
|-----|---------|--------|
| `j` | `Down` | Select next entry |
| `k` | `Up` | Select previous entry |
| `⇧L` | `Shift+Right` | Jump to next turn |
| `⇧H` | `Shift+Left` | Jump to previous turn |
| `g` | | Go to top of scrollback |
| `⇧G` | | Go to bottom of scrollback |
| `PageUp` | | Scroll up one page |
| `PageDown` | | Scroll down one page |

---

## View (Scrollback Focused)

Control how entries are displayed.

| Key | Alt Key | Action |
|-----|---------|--------|
| `h` | `Left` | Collapse selected entry |
| `l` | `Right` | Expand selected entry |
| `e` | | Toggle fold on selected entry |
| `⇧E` | | Expand/collapse all entries |
| `Enter` | | Open entry in fullscreen viewer |
| `r` | | Toggle raw markdown on selected entry |

---

## Focus

Switch between the prompt input and scrollback pane.

| Key | Alt Key | Action |
|-----|---------|--------|
| `Tab` | `Space` | Focus the prompt input |

---

## Escape

| State | Gesture | Effect |
|-------|---------|--------|
| Idle + non-empty prompt | `Esc` | Clear prompt (saved to history) |
| Idle + empty prompt + messages | `Esc` | Open session picker (same as `/resume`) |
| Turn running | `Ctrl+C` | Cancel the current turn |

---

## Agent-Level Actions

| Key | Action |
|-----|--------|
| `Ctrl+P` | Open command palette |
| `Ctrl+M` | Open model picker |
| `Ctrl+C` | Cancel current turn |
| `Ctrl+N` | Create new session (double-press to confirm) |
| `Ctrl+Q` | Quit the application |

---

## Mouse Support

The TUI supports mouse interaction:

- **Click** on a scrollback entry to select it
- **Scroll wheel** to scroll through scrollback
- **Click** on the prompt area to focus it
- **Middle click** on Linux to paste PRIMARY selection

---

## Quick Reference Card

### When scrollback is focused (Simple mode)

```
Navigation:     Up/Down (prev/next entry)
Turn nav:       Shift+Left/Right (prev/next turn)
Scrolling:      PageUp/PageDown
Focus prompt:   Space or Tab
```

### When scrollback is focused (Vim mode)

```
Navigation:     j/k (up/down)
Turn nav:       H/L (prev/next turn)
Scrolling:      Ctrl+J/K (line)  PageUp/PageDown
Folding:        h/l (collapse/expand)  e (toggle)
Focus prompt:   Tab or Space
```

### When prompt is focused

```
Send:           Enter
Newline:        Shift+Enter or Alt+Enter
Paste:          Ctrl+V
Leave:          Tab (back to scrollback)
Cancel (running): Ctrl+C
Clear (idle):   Esc (non-empty prompt)
```

---

## Command Palette

Press `Ctrl+P` or `?` to open the command palette — a searchable list of actions including all keyboard shortcuts, slash commands, and skills.

---

Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Slash Commands](04-slash-commands.md) | All available `/` commands |
| [Configuration](05-configuration.md) | Settings and sandbox profiles |

---

© 2026 GrayCode AI. All rights reserved.