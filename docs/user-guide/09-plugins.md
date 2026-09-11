# Plugins

Plugins bundle skills, agents, hooks, MCP servers, and LSP configurations into one installable unit. They enable team sharing and marketplace distribution of automation.

---

## What a Plugin Contains

A plugin is a directory that can hold:

- **Skills** — in `skills/` directory
- **Agents** — in `agents/` directory
- **Hooks** — in `hooks/hooks.json`
- **MCP servers** — in `.mcp.json`
- **LSP servers** — in `.lsp.json`

For example, a `team-tools` plugin might include a deploy skill, a code-review agent, and pre-commit hooks. Install them together in one step.

---

## Plugin Locations

Hawk discovers plugins from these locations, in priority order:

| Location | Scope | Trust Required |
|----------|-------|----------------|
| `.hawk/plugins/` | Project | Yes |
| `~/.hawk/plugins/` | User | No |
| `--plugin-dir` | Process | No |

When two plugins share a name, the higher-priority location wins.

---

## CLI Management

Manage plugins from the command line:

```bash
# List installed plugins
hawk plugins list

# Install a plugin
hawk plugins install user/repo
hawk plugins install user/repo@v1.0
hawk plugins install /path/to/local/plugin

# Install with trust (required for project plugins)
hawk plugins install user/repo --trust

# Uninstall a plugin
hawk plugins uninstall my-plugin

# Update plugins
hawk plugins update
```

---

## Marketplace

Browse and install plugins from the community marketplace:

```bash
# Search marketplace
hawk plugins search go

# Install from marketplace
hawk plugins install go-review --trust
```

### Adding Marketplace Sources

Configure additional marketplaces in `~/.hawk/settings.json`:

```json
{
  "marketplaces": [
    {
      "name": "Team Plugins",
      "source": {
        "git": "https://github.com/my-org/plugins.git"
      }
    }
  ]
}
```

---

## Trust Model

- **User plugins** (`~/.hawk/plugins/`) — Trusted automatically
- **Project plugins** (`.hawk/plugins/`) — Require explicit trust before activation

Trust is required because plugins can execute code via hooks and MCP servers. To trust a plugin:

```bash
hawk plugins install <source> --trust
```

---

## Plugin Directory Layout

```
my-plugin/
  skills/
    SKILL.md
  agents/
    reviewer.yaml
    deployer.yaml
  hooks/
    hooks.json
  plugin.json        # Optional manifest
```

### plugin.json Manifest

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Team development tools",
  "components": {
    "skills": true,
    "agents": true,
    "hooks": true
  }
}
```

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Hooks](10-hooks.md) | Event-driven automation |
| [Custom Models](11-custom-models.md) | Provider configuration |

---

© 2026 GrayCode AI. All rights reserved.