# Project Rules (AGENTS.md)

Project rules let you configure Hawk per project or directory. By placing an AGENTS.md file in your repository, you can set coding conventions, build instructions, and style guides that Hawk follows automatically.

---

## Supported File Names

Hawk checks for these filenames in each directory (from repo root to CWD):

- `AGENTS.md`
- `Agents.md`
- `CLAUDE.md` (Claude compatibility)
- `Claude.md`
- `AGENT.md`

Hawk loads all matching files. Deeper files take precedence when instructions conflict.

---

## Rules Directories

Hawk also scans for `*.md` files in rules directories:

| Location | Purpose |
|----------|---------|
| `<dir>/.hawk/rules/` | Always scanned |
| `<dir>/.claude/rules/` | Claude compatibility |

---

## How Discovery Works

Hawk scans rules in this order:

1. **Global rules**: `~/.hawk/AGENTS.md` (applies to all projects)
2. **Repo rules**: Every directory from git root to CWD (inclusive)
3. **CWD-only**: If not in a git repo, only current directory

### Example

```
my-monorepo/
  AGENTS.md                 # Monorepo-wide rules
  packages/
    frontend/
      AGENTS.md             # "Use React. Prefer CSS modules."
    backend/
      AGENTS.md             # "Use Express. Follow REST conventions."
```

When Hawk runs in `packages/frontend/`, it loads all instructions. The frontend-specific rules appear later and take precedence.

---

## What to Put in AGENTS.md

### Coding Conventions

```markdown
# Coding Standards

- Use TypeScript for all new code
- Prefer functional components with hooks
- Use `const` by default; only `let` when needed
- Maximum line length: 100 characters
```

### Build and Test

```markdown
# Build & Test

- Run `go test ./...` before committing
- Use `golangci-lint run` for linting
- Build with `go build ./...`
```

### Architecture

```markdown
# Architecture

- API routes go in `api/` with one file per resource
- Business logic goes in `services/`
- Database queries go in `repository/`
```

---

## Scoping Rules

AGENTS.md files scope to their entire directory tree. Use this for monorepos:

```
my-monorepo/
  AGENTS.md                    # Monorepo-wide
  services/
    AGENTS.md                # "Use Go conventions"
  web/
    AGENTS.md                # "Use React + TypeScript"
```

---

## Session Flags

For one-off rules without editing files:

```bash
hawk --rules "Always use TypeScript" -p "Implement feature"
```

---

## Best Practices

1. **Start with the root** — Most important rules at repo root
2. **Be specific** — "Use TypeScript" beats "Use modern JS"
3. **Keep it short** — Concise rules are easier to follow
4. **Use subdirectory scoping** — Different conventions for monorepo packages
5. **Version control your rules** — Commit to share with team
6. **Review periodically** — Update as project evolves

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Memory](13-memory.md) | Cross-session memory via harrier |
| [Subagents](16-subagents.md) | Parallel agent sessions |

---

© 2026 GrayCode AI. All rights reserved.