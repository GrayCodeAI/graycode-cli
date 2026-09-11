# Toolbench Comparison: hawk vs the Top-20 OSS Coding Agents (2026)

> Status: analysis complete
> Scope: built-in tool inventory of hawk and the leading open-source coding agents
> Related issue: browser/screenshot automation gap

## Executive summary

hawk ships **69 built-in agent tools** — more than any top-20 OSS rival — and covers every capability category the leader board demands (file I/O, shell, web, search, memory, planning, MCP, sub-agents). The only genuine gaps versus the field are:

1. **Browser / computer-use automation** — now closed (see this plan's outcome).
2. **IDE surfaces** — Cline/Continue/Zed are editor-native; hawk remains terminal-first by design.

hawk's tool count exceeds the field (OpenCode ~10, Claude Code 18, Qwen Code ~26, Codex ~9+), but count alone is not the point: hawk matches **Qwen Code** tool-for-tool *and* adds a spec/planning suite, persistent core memory, impact/codegraph/git tools, and an in-tree linter toolchain (`nilaway`, `revive`) that the others leave to MCP.

## Top-20 OSS coding agents by tool surface (2026 star snapshots)

Star counts are approximate (GitHub, ~July 2026): OpenCode ~172k, Gemini CLI ~99k, Claude Code ~81k, Zed ~83k, OpenHands ~70-81k, Codex CLI ~67k, Pi ~62k, Cline ~60-65k, OpenManus ~54k, Aider ~42-48k, Goose ~33-51k, Tabby ~34k, Continue ~32-35k, Kilo Code ~26k, Roo Code ~24k, bolt.diy ~20k, Qwen Code ~18k, Plandex ~11k. (Claude Code is proprietary but kept here as the benchmark for tool richness.)

| Agent | Built-in tools | Categories covered |
|---|---|---|
| hawk | **69** (+Power, +MCP) | all |
| Qwen Code | 26 + computer_use | all; computer use, cron, worktree |
| Claude Code | 18 | all; notebook_edit, skills, hooks |
| Kilo Code | ~15 + Playwright MCP | all + browser via MCP |
| OpenCode | ~10 + MCP + LSP | all |
| Codex CLI | ~9 + browser + screenshot | all + browser |
| Cline | ~10 + Puppeteer, MCP | all + browser |
| OpenManus | ~15 | all + browser/crawl |
| Zed agent | ~12 | all (editor) |
| OpenHands | ~10 | all |
| Aider | ~5 | files + shell + git |
| Goose | ~10 + MCP | all + MCP |
| Kilo Code | ~15 | all + browser (MCP) |
| Pi | ~8 | minimal |
| Continue | ~10 | editor |
| Plandex | ~5 | planning + git |

## hawk's current tool set

See `cmd/chat_tools.go` for the registry construction.

### Essential tools (22) — loaded at startup
`bash`, `read`, `write`, `edit`, `structured_edit`, `ls`, `glob`, `grep`, `web_fetch`, `web_search`, `tool_search`, `skill`, `task` (agent), `ask`, `todo_write`, `task_output`, `task_stop`, `wait_tasks`, `kill_task`, `monitor`, `lsp`, `multi_edit`, + the newly added `browser`/`screenshot` pair. On Windows PowerShell is added conditionally (`cmd/chat_tools.go`).

### Optional tools (47) — lazy-loaded
Spec/planning suite (`plan`, `spec_*`, `approve_implementation`, `clarify`, `converge`), task/cron management (`task_create/get/list/update`, `cron_*`, `sleep`), persistent memory (`core_memory_append/replace/rethink`), code intelligence (`code_search`, `impact`, `code_graph`, `git_history`, `diagnostics`, `nilaway`, `revive`), MCP management (`mcp_auth`, `list_mcp_resources`, `read_mcp_resource`, `mcp_language_server`), workflow (`workflow`, `brief`, `verify_plan_execution`), cloud (`download`, `agentic_fetch`), and `sql`.

## The browser gap and how it was closed

Every browser-native competitor drives a real browser: Cline bundles Puppeteer, Kilo bundles the Playwright MCP server, Codex CLI ships a Chrome extension + in-app browser, Qwen Code exposes `computer_use_*`, OpenManus embeds browser/crawl4ai. hawk previously had only file/shell/web tools.

**Outcome:** added `internal/tool/browser.go` and `internal/tool/screenshot.go`, exposed through hawk's existing `tool.Tool` interface:

- `BrowserTool` — headless-Chrome CDP driver with actions `navigate`, `content` (text/HTML, selector-scoped), `screenshot`, `click`, `type` (with clear), `title`, `location`, `close`. Risk level `high`; URL scheme validation restricts to `http(s)` (localhost/LAN included).
- `ScreenshotTool` — single-shot full-page PNG capture to a path (defaults to a temp file).

Implementation uses `github.com/chromedp/chromedp` v0.16.0, the canonical pure-Go Chrome DevTools client, and reuses hawk's existing `validatePathAllowed` guard for the output path. A lazily-allocated, mutex-guarded browser process is shared across calls (closed via `Browser … action:"close"` or `releaseBrowser()` in tests).

A live end-to-end test (`TestBrowserLive`, opt-in via `HAWK_LIVE_BROWSER=1`) navigates `example.com`, captures a screenshot, and reads the page title against a real Chrome install — it passes.

## Category parity vs the field

| Category | hawk | Field leaders |
|---|---|---|
| File ops | read/write/edit/structured/multi/notebook (Go) | Claude Code/Qwen have notebook_edit; hawk has it |
| Search | grep/glob/ls + LSP + code_search/code_graph | OpenCode LSP; hawk code_graph exceeds |
| Shell | bash (+PowerShell on Windows) + monitor/kill/wait | all |
| Web | web_fetch/web_search/agentic_fetch/download | all |
| Memory | **persistent core_memory_\* (Harrier)** | only Qwen/Claude have persistence |
| Planning | plan + spec_* suite + todo_write | Claude Code skills, Qwen plan_mode |
| Sub-agents | `task` tool + multiagent workers | all leaders |
| MCP | full client + mcp_auth/resources/server tools | all leaders |
| Browser/computer use | **new: Browser/Screenshot** (chromedp) | Cline/Kilo/Codex/Qwen/OpenManus |
| IDE/editor surface | none (terminal-first) | Cline/Continue/Zed |

## Remaining deltas (out of scope here)

- VS Code / JetBrains extension (planned in `docs/IMPLEMENTATION-ROADMAP.md`).
- Auto-commit-per-edit discipline à la Aider (hawk uses AGENTS.md branch rules instead).

## Verification

- `go build ./internal/tool/... ./cmd/...` — clean after adding the tools.
- `go test ./internal/tool/` — unit tests pass; live browser test passes with `HAWK_LIVE_BROWSER=1`.
- `/tools` REPL command now reports essential/optional breakdown (enhanced in this change; see `cmd/diagnostics.go`).
