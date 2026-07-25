# Shell Completions

hawk ships completion scripts for **bash**, **zsh**, **fish**, and **PowerShell**,
plus a machine-readable **JSON** spec for IDE integration.

## Quick Install

```bash
# Auto-install to the standard location for your shell and OS:
hawk completion install bash
hawk completion install zsh
hawk completion install fish
```

## Manual Setup

### Bash

```bash
# Load for current session:
source <(hawk completion bash)

# Persist (Linux):
hawk completion bash > ~/.local/share/bash-completion/completions/hawk

# Persist (macOS with Homebrew):
hawk completion bash > /opt/homebrew/etc/bash_completion.d/hawk
```

### Zsh

```bash
# Load for current session:
source <(hawk completion zsh)

# Persist:
hawk completion zsh > "${fpath[1]}/_hawk"
```

### Fish

```bash
# Load for current session:
hawk completion fish | source

# Persist:
hawk completion fish > ~/.config/fish/completions/hawk.fish
```

### PowerShell

```powershell
# Load for current session:
hawk completion powershell | Out-String | Invoke-Expression

# Persist: add to your $PROFILE
hawk completion powershell > hawk.ps1
. ./hawk.ps1
```

## JSON Spec (IDE Integration)

```bash
hawk completion json
```

Prints a machine-readable command/flag spec that IDEs and editor plugins can
consume for inline completions without shell integration.

## Install Paths

`hawk completion install` resolves the correct path automatically:

| Shell | Linux | macOS (Homebrew) |
|-------|-------|-------------------|
| bash | `~/.local/share/bash-completion/completions/hawk` | `/opt/homebrew/etc/bash_completion.d/hawk` |
| zsh | First `$fpath` entry (e.g. `/usr/local/share/zsh/site-functions/_hawk`) | Same |
| fish | `~/.config/fish/completions/hawk.fish` | Same |
