# Permissions and Safety Controls

Hawk can read files, edit code, and run shell commands. The permission system controls what the agent is allowed to do.

---

## Permission Pipeline

When the model requests a tool:

1. **PreToolUse hooks** — Can deny before other checks
2. **Permission rules** — `deny` > `ask` > `allow`
3. **Remembered grants** — Per-project approvals
4. **Auto-approvals** — Read-only tools
5. **Prompt policy** — Based on autonomy tier

---

## Autonomy Tiers

| Tier | Behavior |
|------|----------|
| `always_ask` | Prompt for every tool |
| `scout` | Classify and approve safe tools |
| `builder` | Broader tool access |
| `operator` | Full tool access (trusted) |
| `autonomous` | No prompts, hooks and rules still apply |

Set with:

```
/autonomy tier builder
```

---

## Rule Matching

### Bash Rules

```bash
# Prefix matching
Bash(git *) — matches `git status`, `git commit`, etc.

# Glob matching
Bash(git * main) — matches `git checkout main`
```

### Path Rules

```toml
# Edit rules
Edit(src/**/*.go) — matches Go files under src/
Read(/Users/**/secrets/*) — matches secrets anywhere
```

### MCP Rules

```toml
MCPTool(linear__*) — matches all tools from linear server
```

---

## Sandbox Integration

Permissions control what the model can request. The sandbox controls what actually happens:

| Layer | Controls |
|-------|----------|
| Rules | Tool access |
| Hooks | Pre-execution blocking |
| Sandbox | OS-level enforcement |

Recommended combination:
- `restrictive` rules
- PreToolUse hooks
- `--sandbox strict`

---

## Hook-based Security

Create a `PreToolUse` hook to enforce allow lists:

```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command", "command": "bin/safe-shell.sh"}]
    }]
  }
}
```

See [Hooks](10-hooks.md) for hook authoring.

---

## Best Practices

1. **Use narrow patterns** — More specific rules are safer
2. **Combine layers** — Rules + hooks + sandbox
3. **Review project config** — Unknown repos may have allow rules
4. **Test policies** — Verify with `dontAsk` mode
5. **Trust model** — Require trust for project automation

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Dashboard](23-dashboard.md) | HUD and monitoring |
| [Monitoring Usage](24-monitoring-usage.md) | Telemetry |

---

© 2026 GrayCode AI. All rights reserved.