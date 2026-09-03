# Subagents and Personas

Subagents are independent child sessions that handle tasks in parallel. Each subagent has its own context window, enabling the main agent to delegate work without consuming its own context.

Subagents are enabled by default.

---

## How Subagents Work

When Graycode needs to delegate work, it spawns a child session using the `Agent` tool. The child runs with:

- Its own context window (isolated from parent)
- A toolset determined by its type and capability mode
- Optional persona instructions

The parent receives the child's summary when it finishes.

---

## Built-in Agent Types

The `Agent` tool accepts a `subagent_type` parameter:

| Type | Description |
|------|-------------|
| `general-purpose` | Full-capability agent for any task |
| `explore` | Research agent — searches and reads, no edits |
| `plan` | Planning agent — investigates and plans, no edits |

---

## Capability Modes

Restrict a subagent's tools:

| Mode | Read | Write | Execute | Description |
|------|------|-------|---------|-------------|
| `read-only` | Yes | No | No | Read, search, merlin only |
| `read-write` | Yes | Yes | No | Read/write files, no shell |
| `execute` | Yes | No | Yes | Read + shell commands |
| `all` | Yes | Yes | Yes | Full tool access |

---

## Spawning Subagents

Parameters for the `Agent` tool:

| Parameter | Description |
|-----------|-------------|
| `prompt` | Task description for the subagent |
| `description` | Short label (3-5 words) |
| `subagent_type` | `general-purpose`, `explore`, or `plan` |
| `background` | Run in background (returns subagent ID) |
| `capability_mode` | Tool restrictions |
| `isolation` | `none` or `worktree` |
| `resume_from` | Continue previous subagent's conversation |
| `cwd` | Working directory |

---

## Worktree Isolation

For tasks that modify files, use `isolation: worktree`:

```
Agent tool with isolation=worktree
```

This creates an isolated git worktree, preventing conflicts with the parent session. The subagent's changes can be merged back when complete.

---

## Personas

Personas are behavioral overlays applied to subagents. Define them in `~/.graycode/settings.json`:

```json
{
  "personas": {
    "researcher": {
      "instructions": "Be thorough. Always cite file paths.",
      "model": "gpt-4o"
    }
  }
}
```

Personas are applied during subagent resolution and shape tone and focus.

---

## Viewing Subagents

In the TUI:

- **Scrollback** — Subagent lifecycle blocks appear in parent history
- **Tasks pane** — Press `Ctrl+B` to view active/completed subagents
- **Fullscreen view** — Press Enter on a subagent block to see its full transcript

Press `q` or `Esc` to return to the parent view.

---

## Depth Limits

Subagents cannot spawn their own subagents. Maximum nesting depth is one. This prevents runaway spawning.

---

## When to Use Subagents

**Good use cases:**
- Research while parent works on other tasks
- Testing in parallel with implementation
- Code review before committing
- Independent tasks requiring no interaction

**Avoid for:**
- Simple tasks the parent can handle
- Tasks requiring back-and-forth interaction
- Context-light operations

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Sessions](17-sessions.md) | Session management |
| [Sandbox](18-sandbox.md) | Security isolation |

---

© 2026 GrayCode AI. All rights reserved.