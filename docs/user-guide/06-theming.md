# Theming and Appearance Customization

Hawk draws all TUI colors from a central theme. You can switch themes while running, follow your operating system's light or dark appearance, and adjust scroll speed and compact mode through slash commands or settings.

---

## Available Themes

Hawk includes 17 built-in themes, plus an `auto` option that follows your system appearance:

| Theme | Description |
|-------|-------------|
| **dark** | Neutral dark base with Hawk's Talon Gold accent. Default theme. |
| **dracula** | The Dracula color scheme with muted violet surfaces. |
| **nord** | Arctic cold blue palette. |
| **gruvbox** | Warm retro browns with olive-green accent. |
| **tokyo-night** | Deep indigo surface with soft blue accent. |
| **catppuccin** | Soft lavender surface with mauve accent. |
| **one-dark** | Slate-gray surface with a blue accent. |
| **solarized-dark** | Signature teal base with cyan accent. |
| **rose-pine** | Muted rose-quartz base with soft-rose accent. |
| **everforest** | Warm forest-gray surface with sage-green accent. |
| **monokai** | Classic high-contrast Monokai scheme. |
| **kanagawa** | Inspired by Hokusai's "The Great Wave", cool deep navy. |
| **ayu** | Ayu Mirage variant: warm dark with vivid orange accent. |
| **palenight** | Material Palenight: deep blue-purple with cyan accent. |
| **github-dark** | GitHub's official dark theme palette. |
| **light** | Light theme for bright terminal backgrounds. |
| **solarized-light** | Base3/base2 cream surface with fixed accent wheel. |
| **auto** | Follow system appearance (dark mode → dark theme, light mode → light theme). |

Theme names are case-insensitive.

---

## Switching Themes

### In the TUI

Run the `/theme` slash command to open the theme picker. As you move through the list with the arrow keys, Hawk previews each theme in real time:

```
/theme
```

To switch without the picker:

```
/theme tokyo-night
/theme rose-pine
```

Submitting `/theme` on its own opens the picker.

### Via Settings

Set the theme in `~/.hawk/settings.json`:

```json
{
  "theme": "tokyo-night"
}
```

---

## Auto Theme (System Appearance)

Set `theme: "auto"` to have Hawk follow your operating system's light/dark appearance and switch themes automatically:

```json
{
  "theme": "auto"
}
```

By default, dark mode maps to the **dark** theme and light mode maps to the **light** theme. Override either mapping:

```json
{
  "theme": "auto",
  "auto_dark_theme": "tokyo-night",
  "auto_light_theme": "light"
}
```

### How Detection Works

| Platform | Method |
|----------|--------|
| **macOS** | Reads `AppleInterfaceStyle` system preference via `defaults read`. |
| **Linux** | Checks `GTK_THEME` environment variable, with fallback to portal detection. |
| **Windows** | Reads the system personalization registry via PowerShell. |
| **SSH / headless** | Defaults to dark theme when detection fails. |

Once running, Hawk checks for appearance changes when you explicitly switch themes (no continuous polling to avoid resource drain).

---

## Color Support Detection

On startup, Hawk detects your terminal's color capability:

| Level | Description |
|-------|-------------|
| **Truecolor** (24-bit) | Full RGB color. All themes render as designed. |
| **256-color** | Indexed palette. Colors are mapped to the nearest index. |
| **16-color** | ANSI names only. Colors map to the closest ANSI color. |

When `COLORTERM=truecolor` is set, Hawk uses truecolor mode. When `TERM` contains `256color`, it uses 256-color mode. When `NO_COLOR` is set, Hawk renders in monochrome.

---

## Scroll Customization

### Scroll Speed

Adjust scroll speed (1-100, default 50):

```
/scroll-speed 75
```

### Scroll Mode

Choose scroll behavior based on input device:

```
/scroll-mode auto       # automatic detection
/scroll-mode wheel      # mouse wheel
/scroll-mode trackpad   # smooth trackpad scrolling
```

### Natural Scrolling

Toggle inverted scroll direction (natural scrolling):

```
/scroll-invert
```

### Pager Configuration

Configure the scrollback buffer and line numbers:

```
/pager-config lines 5000      # set buffer to 5000 lines (0 = unlimited)
/pager-config linenumbers true # show line numbers in scrollback
```

---

## Compact Mode

Toggle compact mode to maximize content on small screens:

```
/compact-mode
```

Compact mode reduces outer padding and margins. Settings:

```json
{
  "compact_mode": true
}
```

---

## Prompt Queue

Queue prompts for later processing without manual intervention:

```
/prompt-queue add Refactor the codebase
/prompt-queue add Write unit tests
/prompt-queue list              # view queued prompts
/prompt-queue clear             # clear the queue
/prompt-queue remove 1          # remove item by index
```

Queued prompts persist across sessions and can be processed when you're ready.

---

## Terminal Setup

View terminal configuration recommendations and current capabilities:

```
/terminal-setup
```

This shows detected color support, current settings, and tips for optimal Hawk experience.

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [MCP Servers](07-mcp-servers.md) | External tool integrations |
| [Skills](08-skills.md) | Installing and using skills |
| [Plugins](09-plugins.md) | Multi-component plugins and marketplace |

---

© 2026 GrayCode AI. All rights reserved.
