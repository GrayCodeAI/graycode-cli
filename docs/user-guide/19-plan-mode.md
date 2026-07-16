# Plan Mode

Plan mode is a structured planning phase where Hawk explores the codebase and designs an implementation before writing code. Use it when tasks have genuine ambiguity about the right approach.

---

## What Plan Mode Does

When plan mode is active, Hawk:

1. Reads and searches the codebase to understand patterns
2. Designs an implementation approach
3. Writes the plan to a file under `.hawk/specs/`
4. May ask clarifying questions via `/ask`
5. Calls `exit_plan_mode` to request approval

During plan mode, only the plan file is editable. All other file edits are rejected.

---

## How to Enter Plan Mode

### Agent-Initiated

The agent enters plan mode when it determines a task has ambiguity. It calls `enter_plan_mode`, which requires your approval.

**Good triggers:**
- "Add user authentication" — architectural choices
- "Redesign the data pipeline" — major restructuring
- "Add caching" — multiple approaches possible

### User-Initiated

```bash
# Enter plan mode
/plan

# Enter and start planning
/plan Add authentication to the API
```

Or press **Ctrl+Shift+P** to cycle modes.

---

## The Plan File

Plans are written to `.hawk/specs/<slug>/plan.md`:

- **Context** — Why the change is needed
- **Approach** — Recommended implementation strategy
- **Files to modify** — Critical file paths
- **Verification** — How to test the changes

---

## Plan Approval

When planning completes, Hawk opens a preview with action bar:

| Key | Action |
|-----|--------|
| `a` | Approve and start implementation |
| `s` | Request changes (focus to prompt) |
| `c` | Comment on selected line |
| `q` | Quit and abandon plan |

Tab switches between plan and prompt.

---

## Plan Mode Lifecycle

| State | Description |
|-------|-------------|
| `Inactive` | Normal mode |
| `Pending` | Plan toggled on, no prompt sent |
| `Active` | Plan mode active (plan file editable) |
| `ExitPending` | User exited while turn in-flight |

The state persists to disk and survives restarts.

---

## Edits During Plan Mode

- **Plan file** — Auto-approved for edits
- **Other files** — Rejected with error message
- **Subagents** — Not restricted (they start fresh)

This is independent of autonomy mode.

---

## When to Use Plan Mode

**Use for:**
- Architectural ambiguity
- Unclear requirements
- High-impact changes

**Skip for:**
- Clear implementation paths
- Bug fixes
- Simple modifications

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Background Tasks](20-background-tasks.md) | Task management |
| [Terminal Support](21-terminal-support.md) | Terminal compatibility |

---

© 2026 GrayCode AI. All rights reserved.