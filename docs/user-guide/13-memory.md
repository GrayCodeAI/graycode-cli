# Memory

Memory lets Hawk recall facts, decisions, and patterns from earlier sessions. Hawk indexes saved information and searches it automatically, so new sessions can reuse relevant context.

---

## What Is Memory?

Without memory, each Hawk session starts fresh. When you enable memory, Hawk can:

- Recall project conventions you explained before
- Reuse debugging steps that worked
- Carry architectural decisions forward across sessions
- Avoid re-asking questions it already has answers to

Memory is powered by **harrier**, the graph-based persistent memory engine.

---

## Enabling Memory

### Slash Command

Toggle memory in the TUI:

```
/harrier    # Opens memory management
```

### CLI Flag

```bash
hawk --memory
```

### Settings

```json
// ~/.hawk/settings.json
{
  "memory": {
    "enabled": true
  }
}
```

---

## How Memory Is Stored

Memory is stored in harrier's graph database under `~/.hawk/harrier/`.

| Location | Scope | Description |
|----------|-------|-------------|
| `~/.hawk/harrier/global/` | Global | Cross-project memory |
| `~/.hawk/harrier/workspaces/<repo>/` | Workspace | Project-specific memory |
| `~/.hawk/harrier/sessions/` | Sessions | Session logs and summaries |

The graph structure enables semantic search and relationship mapping between memories.

---

## Working with Memory

### Remember

Ask Hawk to remember something, or use the slash command:

```
/remember always open PR links after pushing
```

Hawk records entries as durable statements organized by topic.

### Forget

Ask what Hawk should forget:

```
/forget the snake_case convention
```

Forget is best-effort. For guaranteed removal, use harrier's query tools directly.

### Recall

Ask what Hawk remembers:

```
/what do you remember about auth?
```

Hawk searches across all memory sources and summarizes.

---

## Browsing Memory

Open the harrier browser in the TUI:

```
/harrier
```

Or search directly:

```
/harrier search <query>
```

### CLI Commands

```bash
hawk harrier                    # Open harrier UI
hawk harrier search <query>     # Search memory
hawk harrier stats              # Show memory statistics
```

---

## First-Turn Injection

On the first turn of each session, Hawk automatically searches memory for content relevant to the current project and injects it as context.

Configure injection:

```json
{
  "memory": {
    "first_turn_injection": true
  }
}
```

---

## Memory Search

Hawk searches memory automatically. Manual search:

```
Search harrier memory for "auth patterns"
```

The harrier engine provides hybrid search:
- **Graph traversal** for semantic relationships
- **Full-text search** for keyword matching

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Headless Mode](14-headless-mode.md) | Non-interactive usage |
| [Agent Mode](15-agent-mode.md) | Agent configuration |

---

© 2026 GrayCode AI. All rights reserved.