# Hawk Archive Parity Plan

Goal: bring the Go implementation to behavioral parity with `../hawk-archive` while preserving the Go distribution goal: one static binary where practical.

## Current Snapshot

- Go repo: small core agent with Bubble Tea REPL, eyrie streaming, persistence, permissions, MCP stdio, and a basic tool suite.
- Archive repo: full product surface with rich CLI modes, Ink UI, command/plugin/skill systems, hooks, advanced permissions/sandboxing, provider routing, IDE/remote integrations, updater/auth flows, and broad tests.
- Estimated parity after the current Go changes: about 75% of full product parity, about 95% of a lean core-agent MVP.

## Completed In Go

- Archive tool wire names are exposed to the model: `Bash`, `Read`, `Write`, `Edit`, `LS`, `Glob`, `Grep`, `WebFetch`, `WebSearch`, `ToolSearch`, `Skill`, `Agent`, `AskUserQuestion`, `TodoWrite`, `TaskOutput`, `TaskStop`, `LSP`, `EnterPlanMode`, `ExitPlanMode`, `NotebookEdit`, `ListMcpResourcesTool`, `ReadMcpResourceTool`, `Config`, `SendUserMessage`.
- Legacy Go tool names remain accepted as aliases, including `bash`, `file_read`, `file_write`, and `file_edit`.
- MCP tools use archive-compatible names: `mcp__<server>__<tool>`, with legacy `mcp_<server>_<tool>` aliases.
- `hawk -p "prompt"` and positional `hawk -p "prompt"` print-mode flows exist with `text`, `json`, and `stream-json` output formats.
- Print mode supports session save/resume and `--no-session-persistence`.
- CLI now accepts archive-compatible tool/permission flags: `--tools`, `--allowedTools`, `--allowed-tools`, `--disallowedTools`, `--disallowed-tools`, `--permission-mode`, `--dangerously-skip-permissions`, `--max-turns`, `--max-budget-usd`, `--system-prompt`, `--system-prompt-file`, `--append-system-prompt`, and `--append-system-prompt-file`.
- CLI now accepts archive-compatible session/context flags: `--input-format`, `--settings`, `--add-dir`, `--continue`, `--fork-session`, and `--session-id`.
- CLI now includes local operational subcommands: `doctor`, `config`, `mcp`, `sessions`/`history`, and `tools`.
- Settings loading accepts archive-style camelCase/snake_case aliases for API key, auto-allow rules, budget, custom headers, MCP servers, and tool allow/deny lists, and project settings now merge all supported fields over global settings.
- `hawk config get/set` and the model-facing `Config` tool now read/write supported global settings instead of returning placeholders.
- Permission rules understand archive syntax such as `Bash(git:*)`, `Write(*.env)`, and bare tool names.
- Permission modes have initial Go semantics for `default`, `acceptEdits`, `bypassPermissions`, `dontAsk`, and `plan`.
- Interactive startup now shows a fuller welcome/status screen with provider, model, session, permission mode, working directory, project instruction status, MCP status, and available tools.
- REPL discovery now includes `/tools`, `/welcome`, `/permissions deny`, `/permissions mode`, `/add-dir`, `/skills`, `/mcp`, `/files`, `/branch`, `/env`, `/version`, and several archive prompt-style commands (`/review`, `/security-review`, `/bughunter`, `/summary`, `/release-notes`, `/pr-comments`).
- `stream-json` output now includes `session_id`, `uuid`, cost, and archive-style final `result` records.
- Bash can start background tasks with `run_in_background`, with `TaskOutput` and `TaskStop` support.
- Local `Skill` discovery works for `.hawk/skills`, `~/.hawk/skills`, and `~/.codex/skills`.
- MCP resource listing/reading tools are present for connected MCP servers.
- File tools accept archive-compatible arguments: `file_path` for `Read`/`Write`/`Edit`, `offset`/`limit` for `Read`, and `old_string`/`new_string` for `Edit`.
- `LS` is present as an archive-style directory listing tool.
- File discovery/read/write/edit tools enforce a working-directory plus `--add-dir`/`/add-dir` boundary when executed by the agent.
- `TodoWrite` accepts archive-style full `todos` arrays as well as the earlier Go action API.
- **Hook system** with 8 event types (pre_query, post_query, pre_tool, post_tool, session_start, session_end, permission_ask, error) and priority-based execution.
- **Plugin system** with manifest validation, install/list/uninstall commands, hook registration, and command execution.
- **Advanced permissions** with auto-mode learning, command classifier (safe/unsafe/unknown), bypass killswitch, and shadowed rule detection.
- **Model catalog** with 25+ models across 7 providers, pricing, context sizes, and recommendations.
- **Session memory** with extraction, search, and consolidation.
- **Analytics** with event logging, session traces, and cost tracking.
- **Auth system** with secure token storage (OS keychain integration) and OAuth flow support.
- **Auto-update** with GitHub release checking and semver comparison.
- **LSP integration** with JSON-RPC client and server manager.
- **Voice mode** with Whisper.cpp integration and keyterms.
- **Magic docs** with Go AST parsing and automatic markdown generation.
- **Worktree tools** with EnterWorktree/ExitWorktree validation.
- **NotebookEdit** with cell insert/delete/list operations.
- **Bash security** with zsh bypass protection, process substitution blocking, IFS injection detection, carriage return prevention, ANSI-C quoting detection, and git commit safety.

## Parity Definition

Full parity means these archive behaviors work in Go with compatible user-facing names, flags, config files, session data, and tool semantics:

- CLI flags and subcommands from `hawk --help`, including `-p/--print`, JSON streaming, resume/continue, settings, tools allow/deny, MCP config, worktree, plugin, doctor, update, and install paths.
- Slash command registry from `src/commands.ts`, including built-ins, skills, plugins, and dynamic command loading.
- Tool registry from `src/tools.ts`, including archive tool names (`Bash`, `Read`, `Edit`, `Write`, `TodoWrite`, etc.) and MCP names (`mcp__server__tool`).
- Permission behavior, including `default`, `acceptEdits`, `bypassPermissions`, `dontAsk`, and `plan` modes.
- File/tool behavior parity for reads, edits, notebooks, PDFs/images where supported, shell safety, output truncation, and background tasks.
- Session persistence compatibility or migration from archive JSONL/project session layout.
- Provider routing, model catalog, fallback, and profile/config behavior.
- UI parity for common REPL flows: command palette/help, approvals, diffs, tasks, status, history, and cancellation.
- Test parity for core unit/e2e scenarios.

## Implementation Phases

1. Compatibility foundation
- [x] Archive-compatible tool names with Go aliases.
- [x] Archive-compatible `-p/--print` prompt mode.
- [x] MCP `mcp__server__tool` names with old Go aliases.
- [x] Parity tests for names, flags, and session save/resume basics.

2. CLI and non-interactive mode
- [x] Implement archive CLI flags safely: `--output-format`, `--tools`, `--allowed-tools`, `--disallowed-tools`, `--permission-mode`, `--system-prompt`, `--append-system-prompt`, `--max-turns`, `--max-budget-usd`.
- [x] Add `text`, `json`, and `stream-json` output formats.
- [x] Add `--no-session-persistence`.
- [x] Add `--input-format`, `--settings`, `--add-dir`, `--continue`, `--fork-session`, and `--session-id`.
- [x] Add initial archive-style local subcommands for diagnostics, config display, MCP listing, session history, and tool listing.
- [x] Match archive stream-json schemas exactly, including all SDK message subtypes, usage objects, and hook events.
- [x] Match archive session storage exactly, not just local JSON session behavior.

3. Tool behavior parity
- Bring `Read`, `Write`, `Edit`, `LS`, `Glob`, `Grep`, `Bash`, `NotebookEdit`, `WebFetch`, `WebSearch`, and `LSP` behavior up to archive semantics.
- [x] Add missing lightweight tools enabled in external archive builds: `Skill`, `TaskOutput`, `TaskStop`, `ListMcpResourcesTool`, `ReadMcpResourceTool`, and `ToolSearch`.
- [x] Add archive-compatible file argument aliases and `LS`.
- [x] Add gated worktree tools and richer task/sandbox tools.
- [x] Add initial permission-aware filesystem path validation for file tools.
- [x] Add richer shell parsing with zsh dangerous command detection, process substitution blocking, IFS injection detection, carriage return prevention, ANSI-C quoting detection, locale quoting detection, empty quote pair obfuscation detection, heredoc validation, and git commit safety.

4. Commands, skills, plugins, and hooks
- Port the command registry structure from `src/commands.ts`.
- [x] Implement more built-in slash commands before plugin/dynamic commands: `/add-dir`, `/skills`, `/mcp`, `/files`, `/branch`, `/env`, `/version`, and prompt-style review/summary commands.
- [x] Add skill discovery/loading.
- [x] Add plugin manifest validation, install/list/enable/disable/update, and plugin-provided commands/skills.
- [x] Add hook lifecycle execution with 8 event types and priority-based execution.

5. Sessions, config, and provider parity
- [x] Improve settings source precedence and config schema aliases for the currently supported fields.
- [x] Add provider profiles, model catalog, smart routing, fallback, and `/provider-status` style output.
- [x] Add session JSONL storage compatibility/migration and cross-project resume behavior.

6. UI, remote, and release systems
- [x] Expand Bubble Tea UI to match common archive REPL flows (command palette, diff colors, file tree, history search).
- [x] Add updater/install/native release behavior (auto-update checking, version command).
- Add IDE integration, remote/direct-connect/SSH/server paths where feasible.

7. Test hardening
- [x] Port archive unit/e2e tests in batches (hooks, permissions, memory, model, plugin, update, auth, lsp, voice, magicdocs, integration).
- [x] Add golden tests for CLI outputs and tool schemas.
- [x] Maintain `go test ./...` as the minimum merge gate, then add integration/e2e gates.

## Rules

- Prefer compatibility shims over breaking session/config names.
- Do not silently claim parity for stubbed features; mark them as partial until behavior and tests match.
- Preserve Go ergonomics and static binary constraints unless parity requires external helpers.
