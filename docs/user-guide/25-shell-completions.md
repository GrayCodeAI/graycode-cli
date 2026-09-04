# Shell Completions

graycode ships completion scripts for **bash**, **zsh**, **fish**, and **PowerShell**,
plus a machine-readable **JSON** spec for IDE integration.

## Quick Install

```bash
# Auto-install to the standard location for your shell and OS:
graycode completion install bash
graycode completion install zsh
graycode completion install fish
```

## Manual Setup

### Bash

```bash
# Load for current session:
source <(graycode completion bash)

# Persist (Linux):
graycode completion bash > ~/.local/share/bash-completion/completions/graycode

# Persist (macOS with Homebrew):
graycode completion bash > /opt/homebrew/etc/bash_completion.d/graycode
```

### Zsh

```bash
# Load for current session:
source <(graycode completion zsh)

# Persist:
graycode completion zsh > "${fpath[1]}/_graycode"
```

### Fish

```bash
# Load for current session:
graycode completion fish | source

# Persist:
graycode completion fish > ~/.config/fish/completions/graycode.fish
```

### PowerShell

```powershell
# Load for current session:
graycode completion powershell | Out-String | Invoke-Expression

# Persist: add to your $PROFILE
graycode completion powershell > graycode.ps1
. ./graycode.ps1
```

## JSON Spec (IDE Integration)

```bash
graycode completion json
```

Prints a machine-readable command/flag spec that IDEs and editor plugins can
consume for inline completions without shell integration.

## Install Paths

`graycode completion install` resolves the correct path automatically:

| Shell | Linux | macOS (Homebrew) |
|-------|-------|-------------------|
| bash | `~/.local/share/bash-completion/completions/graycode` | `/opt/homebrew/etc/bash_completion.d/graycode` |
| zsh | First `$fpath` entry (e.g. `/usr/local/share/zsh/site-functions/_graycode`) | Same |
| fish | `~/.config/fish/completions/graycode.fish` | Same |
