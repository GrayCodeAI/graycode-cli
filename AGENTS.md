---
description: Extending hawk — how to write AGENTS.md files, custom specialists, skills, hooks, MCP servers, and plugins.
globs: "*.go, *.js, *.md, *.json, *.toml, *.yaml, *.yml"
alwaysApply: false
---

# Extending hawk

hawk is an open-source code intelligence platform. It lives in the `graycode-eco`
workspace alongside the ecosystem repos that power it (`eyrie`, `hawk-core-contracts`,
`tok`, `yaad`, `trace`, `sight`, `inspect`). This document describes how to extend
hawk with custom tools, skills, hooks, and integrations.

## Development workflow

When starting any new work (feature, fix, refactor, chore), always create a feature branch from `main` first. Never commit directly to `main`. Use branch naming conventions like `feat/<description>`, `fix/<description>`, or `chore/<description>`. Open a PR, ensure CI is green, then merge.

## 1. Drop a project `AGENTS.md`

When hawk starts in a directory, it looks for project-level instructions and injects them into the system prompt. The lookup walks from your current working directory **up to the nearest git root** and reads the first matching file at each level — general rules at the repo root, more specific rules in sub-trees. Files are labeled with their directory in the prompt (e.g. `## Project guidelines (services/api/AGENTS.md)`).

Accepted file names, in priority order at each level:

| Path | Notes |
| --- | --- |
| `./AGENTS.md` | The classic spot — committed to your repo, shared with the team. |
| `./ZERO.md` | Brand-specific alias. Same format, lower priority. |
| `./.zero/AGENTS.md` | Project-local, hidden, gitignored. Personal notes that stay out of git. |

Matching is **case-insensitive** on the basename, so `AGENTS.md`, `Agents.md`, and `agents.md` resolve to the same file on Windows and macOS. The git-tracked filename in this repo is `AGENTS.md` — keep that on case-sensitive filesystems (Linux, the WSL filesystem, or a CI runner) to match what the loader looks for.

Both files use the same format. YAML frontmatter is optional; the markdown body is loaded as instructions for the agent. hawk reads the file once at session start, so changes take effect on the next launch — not mid-session.

```markdown
# Project conventions for <your project>

- Build with `make`, not `go build` directly.
- Tests live next to the source file (`foo_test.go` next to `foo.go`).
- Run `make lint` before opening a PR.
- Never edit files under `third_party/` — those are vendored.
```

Tips:

- Keep each file under ~8 KiB. hawk caps the **total** across all matched files at 32 KiB; everything past the cap is dropped.
- Re-state rules in the imperative voice: "Run `make lint`", not "you should consider running the linter".
- Don't put secrets, model IDs, or environment-specific paths in `AGENTS.md`. Use config files for those.
- In a monorepo, drop a narrower `AGENTS.md` in each sub-tree (e.g. `services/api/AGENTS.md`). hawk picks those up automatically when you launch from inside the sub-tree.
- A YAML frontmatter block (`---\n...\n---`) at the top is preserved verbatim in the injected prompt but is not parsed for `globs:` or `alwaysApply:` scoping today — keep the body self-contained.

### Personal guidelines, across every project

For preferences that follow *you*, not a specific repo (tone, tooling habits, workflow), drop a `ZERO.md` in your user config directory: `~/.hawk/ZERO.md` on Linux/macOS, `%AppData%\hawk\ZERO.md` on Windows — the same directory as config files and your personal specialists. Same format and 8 KiB cap as the project files above, and the same case-insensitive basename match.

This file is injected as its own `## User guidelines` section, before the project's `AGENTS.md`/`ZERO.md`, and is labeled as personal preference in the prompt: project guidelines are the later, more specific instruction and take precedence over it when the two conflict.

## 2. Custom specialists

Specialists are hawk's sub-agents. Three scopes, in priority order:

| Scope | Path | Shared? |
| --- | --- | --- |
| Built-in | compiled into hawk | yes |
| User | `~/.hawk/specialists/*.md` | no — your machine only |
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
hawk specialist list
hawk specialist show api-reviewer
hawk specialist create api-reviewer \
    --project \
    --description "Reviews API changes" \
    --tools read-only,plan \
    --prompt "$(cat api-reviewer.md)"
hawk specialist edit api-reviewer --project
hawk specialist delete api-reviewer --project
hawk specialist path                       # prints the resolved specialists directory
```

## 3. Skills

hawk ships **no bundled skills** by default. Skills are markdown instruction
files that extend agent capabilities, sourced from the separate
`GrayCodeAI/hawk-community-skills` repo and installed on demand:

```bash
hawk skills search <query>          # find skills in hawk-community-skills
hawk skills install <owner/repo> [skill-name]   # install after user approval
hawk skills list                    # list installed skills
hawk skills remove <name>
```

Installed skills live in user or project scope:
- User-scoped: `~/.hawk/skills/`
- Project-scoped: `./.zero/skills/` or `./skills/`

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
hawk hook add beforeReview --command "lint-check"
hawk hook remove beforeReview
hawk hook list
```

## 5. MCP integration

MCP (Model Context Protocol) servers can expose tools to hawk:

```bash
hawk mcp add --name server --url http://localhost:8080
hawk mcp remove server
hawk mcp list
```

## 6. Plugins

Plugins extend hawk with custom tools and capabilities:

```bash
hawk plugin add --name my-plugin --path ./my-plugin
hawk plugin remove my-plugin
hawk plugin list
```

## 7. Verification

hawk includes a self-verification system to validate local changes before contributing:

```bash
hawk verify
hawk verify --fix
```

## Development

```bash
make lint
hawk verify
```

### Architecture note: cross-repo contracts

Legacy `hawk/shared/types` has been removed. Cross-repo severity and finding contracts now live in `github.com/GrayCodeAI/hawk-core-contracts` (`hawk-core-contracts/types`) — extensions and support repos must import that module instead of Hawk internals.

### Architecture note: provider ownership

Implement provider protocols, adapters, catalog metadata, credential mappings, and
provider contract tests in `external/eyrie` first. Hawk consumes providers only
through Eyrie's stable engine facade; Hawk changes should be limited to host UX
and facade integration. Concentrate AI is a pay-as-you-go gateway implemented
with its native Responses API (`/v1/responses`) under the
`concentrate-payg` deployment.

<!-- gitnexus:start -->
## GitNexus — Code Intelligence

This project is indexed by GitNexus as **hawk** (86470 symbols, 279855 relationships, 300 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> Index stale? Run `node .gitnexus/run.cjs analyze` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? `npx gitnexus analyze` (npm 11 crash → `npm i -g gitnexus`; #1939).

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows. For regression review, compare against the default branch: `detect_changes({scope: "compare", base_ref: "main"})`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `query({search_query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method without first running `impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit changes without running `detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/hawk/context` | Codebase overview, check index freshness |
| `gitnexus://repo/hawk/clusters` | All functional areas |
| `gitnexus://repo/hawk/processes` | All execution flows |
| `gitnexus://repo/hawk/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

### Submodule workflow (external/ repos)

hawk depends on ecosystem repos (`eyrie`, `hawk-core-contracts`, etc.) via git submodules under `external/`. Hawk's `go.work` points to `./external/<repo>`, so changes in the submodule are automatically picked up by hawk.

1. Edit + test in `hawk/external/<repo>` — run its tests, run `make test` in hawk
2. Push from the submodule: `git push origin <branch>`
3. Sync to the independent repo: `cd ../<repo> && git fetch origin <branch> && git checkout <branch>`
4. PR → merge from the independent repo
5. Pull main in the submodule: `cd ../hawk/external/<repo> && git checkout main && git pull origin main`
6. Commit the pointer in hawk: `cd .. && git add external/<repo> && git commit -m "chore: update <repo>"`
