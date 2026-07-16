# Theming Enhancement Plan

**Goal:** Complete theming parity with Grok, adding auto-detection, customization options, and polish.

## Current State

Hawk already has:
- ✅ 17 built-in themes (dark, dracula, nord, gruvbox, tokyo-night, catppuccin, one-dark, solarized-dark, rose-pine, everforest, monokai, kanagawa, ayu, palenight, github-dark, light, solarized-light)
- ✅ Theme picker UI
- ✅ `/theme` slash command
- ✅ Settings persistence

Missing features:
- ✅ Auto-detect OS dark/light appearance (implemented)
- ✅ Color quantization for 256-color/16-color terminals (implemented)
- ✅ Scroll customization (speed, invert) (implemented)
- ✅ Compact mode toggle (implemented)
- ✅ Theme preview in picker (implemented)

---

## Implementation Plan

### Phase 1: Auto Theme Detection (Done)

**Add `/hawk/internal/theme/auto_detect.go`:**
- ✅ Detect macOS AppleInterfaceStyle
- ✅ Detect Windows AppsUseLightTheme
- ✅ Linux XDG fallback via GTK_THEME

**Add to theme picker (`theme_picker.go`):**
- ✅ Add "auto" option to registry

**Settings:**
```json
{
  "theme": "auto"
}
```

### Phase 2: Color Quantization (Done)

**Add `/hawk/internal/theme/quantize.go`:**
- ✅ DetectColorLevel (basic/256/truecolor)
- ✅ HexToRGB conversion
- ✅ RGBTo256 mapping
- ✅ RGBToANSI mapping
- ✅ QuantizePalette function

### Phase 3: Scroll Customization (Done)

**Implemented in `internal/config/settings.go` and `cmd/chat_subcommand_simple.go`:**
- ✅ `/scroll-speed <1-100>` command
- ✅ `/scroll-invert` toggle command
- ✅ Settings fields: `scroll_speed`, `invert_scroll`

### Phase 4: Compact Mode (Done)

**Implemented in `internal/config/settings.go` and `cmd/chat_subcommand_simple.go`:**
- ✅ `/compact-mode` toggle command
- ✅ Settings field: `compact_mode`

### Phase 5: Theme Preview (Done)

**Implemented in `cmd/theme_picker.go`:**
- ✅ Live preview shown when navigating themes
- ✅ Visual swatches for panel, prompt, accent, text, green, red colors

---

### Phase 6: Additional Commands (Done)

**Implemented in `cmd/chat_subcommand_simple.go` and `internal/tool/prompt_queue.go`:**
- ✅ `/prompt-queue` command with add/list/clear/remove subcommands
- ✅ `/scroll-mode` command with auto/wheel/trackpad options
- ✅ `/terminal-setup` command showing configuration recommendations
- ✅ `/pager-config` command for scrollback buffer and line numbers

**Added to `internal/theme/theme.go`:**
- ✅ PagerConfig struct with BufferLines, Margins, ShowLineNumbers fields
- ✅ PageMargins struct for layout spacing

**Added to `internal/config/settings.go`:**
- ✅ PaginatorLines setting
- ✅ PaginatorShowLineNums setting
- ✅ PaginatorMarginTop/Bottom settings

---

## Detailed Tasks

| Task | File | Effort |
|------|------|--------|
| Auto theme detection | `internal/theme/auto_detect.go` | 1 day |
| XDG portal support | `internal/theme/xdg.go` | 1 day |
| Color quantization | `internal/theme/quantize.go` | 2 days |
| Scroll config struct | `internal/theme/theme.go` | 0.5 day |
| Scroll slash commands | `cmd/chat_subcommand_simple.go` | 0.5 day |
| Compact mode toggle | `cmd/chat_subcommand_simple.go`, `internal/theme/theme.go` | 0.5 day |
| Theme preview in picker | `cmd/theme_picker.go` | 1 day |
| Terminal capability detection | `internal/theme/capabilities.go` | 1 day |
| Update user-guide docs | `docs/user-guide/06-theming.md` | 0.5 day |

---

## Settings Schema

```json
{
  "theme": "auto",
  "ui": {
    "scroll_speed": 50,
    "scroll_mode": "auto",
    "invert_scroll": false,
    "compact_mode": false,
    "vim_mode": false
  }
}
```

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `HAWK_THEME` | Override theme selection |
| `HAWK_SCROLL_SPEED` | Override scroll speed |
| `HAWK_INVERT_SCROLL` | Enable natural scrolling |

---

## Testing Matrix

| Terminal | Auto Theme | Quantization | Notes |
|----------|------------|--------------|-------|
| iTerm2 | ✅ | ✅ | Full support |
| Terminal.app | ✅ | ✅ | 256-color max |
| VS Code terminal | ✅ | ✅ | xterm.js |
| tmux | ✅ | ✅ | Requires config |
| SSH | ⚠️ | ✅ | Auto theme needs OSC 11 fallback |

---

## Milestones

1. **Week 1:** Auto theme detection working on all platforms
2. **Week 2:** Color quantization and terminal detection
3. **Week 3:** Scroll customization and compact mode
4. **Week 4:** Polish and documentation

---

© 2026 GrayCode AI. All rights reserved.