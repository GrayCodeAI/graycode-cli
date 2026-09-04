# Sessions

Graycode saves every conversation to disk automatically. Whether you work in the TUI, in headless mode, or over ACP, Graycode records the exchange as a session.

---

## What Sessions Are

A session is a persistent conversation with full history:

- All user prompts and agent responses
- Tool calls and results
- TODO/task list state
- Token usage and turn counts
- Subagent sessions

Sessions are identified by a unique session ID and stored under `~/.graycode/sessions/`.

---

## Starting Sessions

### New Session

```
/new
```

This clears the current context and starts fresh.

### Exit

```
/quit
```

Alias: `/exit`

To leave the session but stay in Graycode:

```
/home
```

---

## Resuming Sessions

### From TUI

```
/resume
```

Opens a session picker. Select a session to resume.

### From Command Line

```bash
# Resume specific session
graycode --resume <session-id>

# Continue most recent
graycode --continue

# New session with specific ID
graycode --session-id <uuid> -p "prompt"
```

---

## Forking and Rewinding

### Fork

Branch into a peer agent with copied history:

```
/fork [--worktree] [directive]
```

### Rewind

Undo changes by restoring files to an earlier state:

```
/rewind
```

Select a rewind point to restore files and truncate history.

---

## Compaction

Compress conversation history to save context:

```
/compact
/compact preserve important context
```

Auto-compact triggers when context window approaches limit.

---

## Session Info

View session details:

```
/session-info
```

Shows model, working directory, context usage, and session ID.

---

## Headless Sessions

Maintain context across headless calls:

```bash
# Start and capture ID
ID=$(graycode -p "First" --output-format json | jq -r '.sessionId')

# Continue
graycode -p "Second" --resume "$ID"
```

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Sandbox](18-sandbox.md) | Security isolation |
| [Plan Mode](19-plan-mode.md) | Spec workflow |

---

© 2026 GrayCode AI. All rights reserved.