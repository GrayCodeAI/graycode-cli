# Hooks

Hooks let you run code at key moments in a Hawk session. Use them to automate tasks, enforce safety checks, log activity, and integrate custom tools.

---

## What Are Hooks?

A hook is a shell command or HTTP endpoint that Hawk calls when a specific lifecycle event occurs. Hooks can:

- **Block actions** — A `PreToolUse` hook can deny a dangerous command before it runs
- **React to events** — A `PostToolUse` hook can log every tool execution
- **Set up context** — A `SessionStart` hook can run setup scripts

---

## Hook Events

| Event | When It Fires | Blocking |
|-------|---------------|----------|
| `SessionStart` | Session starts | No |
| `UserPromptSubmit` | You submit a prompt | No |
| `PreToolUse` | Tool is about to run | Yes (can deny) |
| `PostToolUse` | Tool completes | No |
| `PostToolUseFailure` | Tool fails | No |
| `PermissionDenied` | Permission denied | No |
| `Stop` | Agent turn ends | No |
| `Notification` | Agent sends notification | No |
| `SubagentStart` | Subagent starts | No |
| `SubagentStop` | Subagent finishes | No |
| `SessionEnd` | Session ends | No |

Only `PreToolUse` can block a tool call. All other events are passive.

---

## Hook Configuration

Create hooks in `.hawk/hooks/hooks.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          { "type": "command", "command": "bin/safety-check.sh" }
        ]
      }
    ],
    "SessionStart": [
      {
        "hooks": [
          { "type": "command", "command": "echo 'Session started'" }
        ]
      }
    ]
  }
}
```

### Key Fields

- **matcher** (optional): Regex matching the tool name
- **type**: `"command"` (shell script) or `"http"` (HTTP endpoint)
- **command**: Script path or inline command
- **timeout**: Seconds before killing the hook

---

## PreToolUse Hooks

PreToolUse hooks receive JSON on stdin and must output a decision:

```json
{
  "hookEventName": "PreToolUse",
  "sessionId": "abc-123",
  "toolName": "Bash",
  "toolInput": { "command": "rm -rf /" }
}
```

### Output Format

**Allow:**
```json
{"decision": "allow"}
```

**Deny:**
```json
{"decision": "deny", "reason": "Unsafe command blocked"}
```

Exit code `2` also signals denial. Any other exit code is fail-open (tool is allowed).

---

## Trust Model

- **User hooks** (`~/.hawk/hooks/`) — Always trusted
- **Project hooks** (`.hawk/hooks/`) — Require folder trust

Project hooks are blocked until you trust the folder:

```
/trust           # Trust the current folder
/trust status    # Check trust status
```

---

## Environment Variables

Hawk sets these variables for every hook:

| Variable | Description |
|----------|-------------|
| `HAWK_HOOK_EVENT` | Event name (e.g., `PreToolUse`) |
| `HAWK_SESSION_ID` | Current session ID |
| `HAWK_WORKSPACE_ROOT` | Project root path |
| `HAWK_PLUGIN_ROOT` | Plugin directory (for plugin hooks) |
| `HAWK_PLUGIN_DATA` | Plugin data directory |

---

## CLI Management

```bash
# List hooks
hawk hooks list

# Trust project hooks
hawk hooks trust

# Untrust project hooks
hawk hooks untrust
```

---

## Example: Safe Shell Guard

Create `.hawk/hooks/safety.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          { "type": "command", "command": "bin/safe-shell.sh", "timeout": 5 }
        ]
      }
    ]
  }
}
```

Create `bin/safe-shell.sh`:

```bash
#!/bin/bash
INPUT=$(cat)
CMD=$(echo "$INPUT" | jq -r '.toolInput.command // empty')

# Block destructive patterns
if echo "$CMD" | grep -qE '(rm -rf /|mkfs|dd if=)'; then
  echo '{"decision": "deny", "reason": "Blocked destructive command"}'
  exit 2
fi

echo '{"decision": "allow"}'
```

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Custom Models](11-custom-models.md) | Provider configuration |
| [Project Rules](12-project-rules.md) | AGENTS.md configuration |

---

© 2026 GrayCode AI. All rights reserved.