# OpenAI Codex CLI Adoption Plan

Status: Audited. Core ideas already implemented natively in hawk; remaining
deltas recorded as future RFCs.

Source: `https://github.com/openai/codex` (Apache-2.0, Rust workspace
`codex-rs`, ~100 crates)

## Executive Decision

The audit found that every codex-rs capability relevant to hawk's security and
runtime model already has a native Go implementation, several of them deeper
than codex's equivalents because they build on hawk's ecosystem submodules.
No second runtime, sandbox layer, or policy engine was created.

One small transparency improvement was adopted: the resolved native sandbox
backend (seatbelt / landlock / docker / …) is now reported in the unified
status snapshot alongside the requested policy label.

Three codex ideas are deliberately deferred as future RFCs; see
[Deliberately Deferred](#deliberately-deferred).

## Capability Audit

| codex-rs crate/concept | hawk implementation | Decision |
|---|---|---|
| `core` agent loop | `internal/engine` | Keep hawk |
| `tui`, `ansi-escape`, `terminal-detection` | Bubble Tea/Lipgloss TUI | Keep hawk |
| `rollout`, `thread-store`, `history` JSONL sessions with resume/fork | `internal/session` JSONL + WAL + named checkpoints + fork + recovery + handover | Keep hawk (richer) |
| `app-server-daemon`, `app-server-protocol` (JSON-RPC for IDE/desktop) | `internal/daemon` HTTP/SSE on 4590 + `internal/acp` | Keep hawk |
| `mcp-server`, `codex-mcp`, `rmcp-client`, `connectors` | `internal/mcp` client+server, `external/hawk-mcpkit` scaffolding | Keep hawk |
| `skills`, `plugin`, `hooks` | community skill registry + structural validator, plugins, expanded lifecycle hook events | Keep hawk |
| `login`, `keyring-store`, `aws-auth` | eyrie credential store in OS keychain across 28 providers | Keep hawk (broader) |
| `model-provider(-info)`, `models-manager`, `ollama`, `lmstudio` | `external/eyrie` adapters, catalog, cascade routing | Keep hawk (much broader) |
| `memories`, `agent-graph-store`, `context-fragments` | `external/yaad` graph memory; eventlog/graphjournal projections | Keep hawk |
| `apply-patch`, `file-search`, `file-watcher`, `git-utils` | edit tools, codegraph, git tooling, watcher hooks | Keep hawk |
| `external-agent-migration` | trace reads Claude Code / Codex / Gemini CLI / OpenCode / Cursor sessions | Parity |
| **`linux-sandbox`** (Landlock + seccomp-bpf) | `internal/sandbox/landlock.go`, `seccomp.go` — raw syscalls and BPF filter, no external tools | Already implemented |
| **macOS Seatbelt** | `internal/sandbox/seatbelt.go` — SBPL profile generator with per-policy read/write/process/network rules | Already implemented |
| Windows confinement | `internal/sandbox/windows_acl.go` | Already implemented |
| **`bwrap`**, nsjail, container fallbacks | `selector.go` orders landlock > nsjail > bwrap > docker per platform | Already implemented |
| **`network-proxy`** (egress through inspectable proxy) | `internal/sandbox/netproxy.go` + egress tests | Already implemented |
| **`execpolicy`** (structured pre-exec command analysis) | `internal/sandbox/code_verifier.go` static analysis of generated code (blocked modules/functions/patterns incl. privilege escalation) plus permission-engine destructive-command hard block and user `NeverAllow` ceiling | Covered by equivalent layers |
| **`shell-escalation`** (exact re-validated widening approval) | sandbox policy statements direct denial/escalation flow; `PermissionService.EscalatePermission` binds single-use opaque tokens to exact calls | Covered by equivalent layers |
| `sdk` (TS), `thread-manager-sample` | daemon REST/SSE API is the programmatic surface; Go SDK deferred until consumers require it | Deferred (matches fx plan) |

### Adopted in this change

- Status transparency: `hawk status` (text and `--json`) now resolves the
  effective sandbox backend via `sandbox.SelectSandbox` and reports it as
  `permission.sandbox_backend`, so operators can confirm real kernel-level
  isolation (seatbelt on macOS, landlock/seccomp on Linux, ACL on Windows,
  docker fallbacks) instead of only the strict/workspace/off label.

## Deliberately Deferred

- **Code Mode** (`code-mode`, `code-mode-runtime`, `v8-poc`): letting the model
  author a short script that batches many tool calls into one sandboxed
  execution. Promising token-cost lever, but it introduces an embedded JS
  runtime and a new execution authority boundary. Requires its own threat
  model (script capabilities, network/file scope, output trust) before any
  implementation. Track as a standalone RFC.
- **Agent identity signing** (`agent-identity`): cryptographic identity for
  agents and subagents woven into audit records. hawk's tamper-evident
  security log covers integrity today; signed delegation chains are worth a
  focused design once multi-org delegation exists.
- **Cloud tasks client** (`cloud-tasks*`): remote task queue integration.
  Hawk Cloud already provides sync/review surfaces; a queue protocol would
  duplicate that until a concrete consumer exists.

## Verification

- `go test ./...` full suite green.
- `make vet`, `make lint`, `hawk verify` green.
- Repo-owned markdown passes `markdownlint-cli2 '**/*.md'` (CI scope);
  findings under `external/*` belong to the submodule repos and follow their
  own contribution flow.
