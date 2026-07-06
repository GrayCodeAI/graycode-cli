---
description: Extending hawk-eco — how to write AGENTS.md files, custom specialists, skills, hooks, MCP servers, and plugins.
globs: "*.go, *.js, *.md, *.json, *.toml, *.yaml, *.yml"
alwaysApply: false
---

# Extending hawk-eco

hawk-eco is an open-source code intelligence platform. This document describes how to extend it with custom tools, skills, hooks, and integrations.

## 1. Drop a project `AGENTS.md`

When hawk-eco starts in a directory, it looks for project-level instructions and injects them into the system prompt. The lookup walks from your current working directory **up to the nearest git root** and reads the first matching file at each level — general rules at the repo root, more specific rules in sub-trees. Files are labeled with their directory in the prompt (e.g. `## Project guidelines (services/api/AGENTS.md)`).

Accepted file names, in priority order at each level:

| Path | Notes |
| --- | --- |
| `./AGENTS.md` | The classic spot — committed to your repo, shared with the team. |
| `./ZERO.md` | Brand-specific alias. Same format, lower priority. |
| `./.zero/AGENTS.md` | Project-local, hidden, gitignored. Personal notes that stay out of git. |

Matching is **case-insensitive** on the basename, so `AGENTS.md`, `Agents.md`, and `agents.md` resolve to the same file on Windows and macOS. The git-tracked filename in this repo is `AGENTS.md` — keep that on case-sensitive filesystems (Linux, the WSL filesystem, or a CI runner) to match what the loader looks for.

Both files use the same format. YAML frontmatter is optional; the markdown body is loaded as instructions for the agent. hawk-eco reads the file once at session start, so changes take effect on the next launch — not mid-session.

```markdown
# Project conventions for <your project>

- Build with `make`, not `go build` directly.
- Tests live next to the source file (`foo_test.go` next to `foo.go`).
- Run `make lint` before opening a PR.
- Never edit files under `third_party/` — those are vendored.
```

Tips:

- Keep each file under ~8 KiB. hawk-eco caps the **total** across all matched files at 32 KiB; everything past the cap is dropped.
- Re-state rules in the imperative voice: "Run `make lint`", not "you should consider running the linter".
- Don't put secrets, model IDs, or environment-specific paths in `AGENTS.md`. Use config files for those.
- In a monorepo, drop a narrower `AGENTS.md` in each sub-tree (e.g. `services/api/AGENTS.md`). hawk-eco picks those up automatically when you launch from inside the sub-tree.
- A YAML frontmatter block (`---\n...\n---`) at the top is preserved verbatim in the injected prompt but is not parsed for `globs:` or `alwaysApply:` scoping today — keep the body self-contained.

### Personal guidelines, across every project

For preferences that follow *you*, not a specific repo (tone, tooling habits, workflow), drop a `ZERO.md` in your user config directory: `~/.config/hawk-eco/ZERO.md` on Linux/macOS, `%AppData%\Roaming\hawk-eco\ZERO.md` on Windows — the same directory as config files and your personal specialists. Same format and 8 KiB cap as the project files above, and the same case-insensitive basename match.

This file is injected as its own `## User guidelines` section, before the project's `AGENTS.md`/`ZERO.md`, and is labeled as personal preference in the prompt: project guidelines are the later, more specific instruction and take precedence over it when the two conflict.

## 2. Custom specialists

Specialists are hawk-eco's sub-agents. Three scopes, in priority order:

| Scope | Path | Shared? |
| --- | --- | --- |
| Built-in | compiled into hawk-eco | yes |
| User | `~/.config/hawk-eco/specialists/*.md` | no — your machine only |
| Project | `./.zero/specialists/*.md` | yes — the repo team |

Project overrides user overrides built-in when names collide.

A specialist is a markdown manifest with frontmatter and a system prompt:

```markdown
---
description: Reviews API changes for breaking-change risk and missing tests.
tools: read-only,plan
---

You review API changes. For every changed hunk in `internal/api/` or any file
that ends in `_api.go`:

1. Confirm the public signature is backward-compatible, or note the breaking
   change explicitly with the migration path.
2. Confirm a corresponding test exists in `internal/api/*_test.go` and that
   the new behaviour is exercised.
3. Flag any new exported symbol without a doc comment.

Reply with one JSON object per finding: `{"file", "line", "severity", "message", "fix"}`.
```

CLI management:

```bash
hawk-eco specialist list
hawk-eco specialist show api-reviewer
hawk-eco specialist create api-reviewer \
    --project \
    --description "Reviews API changes" \
    --tools read-only,plan \
    --prompt "$(cat api-reviewer.md)"
hawk-eco specialist edit api-reviewer --project
hawk-eco specialist delete api-reviewer --project
hawk-eco specialist path                       # prints the resolved specialists directory
```

## 3. Skills

Skills are markdown instruction files that extend agent capabilities. They can be:
- Project-scoped: dropped in `./.zero/skills/` or `./skills/`
- User-scoped: dropped in `~/.config/hawk-eco/skills/`

A skill manifest:

```markdown
---
description: How to review Go code for security issues
globs: "*.go"
alwaysApply: true
---

When reviewing Go code for security:

1. Check for SQL injection patterns
2. Verify error handling doesn't expose sensitive data
3. Confirm secrets are not hardcoded
4. Validate input sanitization
```

## 4. Hooks

Hooks allow custom commands to run at specific lifecycle points:
- `beforeReview` — runs before code review starts
- `afterReview` — runs after code review completes
- `sessionStart` — runs at session initialization
- `sessionEnd` — runs at session teardown

```bash
hawk-eco hook add beforeReview --command "lint-check"
hawk-eco hook remove beforeReview
hawk-eco hook list
```

## 5. MCP integration

MCP (Model Context Protocol) servers can expose tools to hawk-eco:

```bash
hawk-eco mcp add --name server --url http://localhost:8080
hawk-eco mcp remove server
hawk-eco mcp list
```

## 6. Plugins

Plugins extend hawk-eco with custom tools and capabilities:

```bash
hawk-eco plugin add --name my-plugin --path ./my-plugin
hawk-eco plugin remove my-plugin
hawk-eco plugin list
```

## 7. Verification

hawk-eco includes a self-verification system to validate local changes before contributing:

```bash
hawk-eco verify
hawk-eco verify --fix
```

## Development

```bash
make lint
hawk-eco verify
```
