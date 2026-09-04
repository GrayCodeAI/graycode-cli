# Qwen Code Adoption Plan

Status: Implemented selectively in the current Graycode feature branch.

## Guardrails

Qwen Code is Apache-2.0 TypeScript software with a Gemini-shaped core. Graycode
will independently reimplement behavioral contracts in Go, preserve the Eyrie
provider boundary, and retain Graycode's event-sourced sessions and OS sandbox.
No Qwen source or dependency is vendored.

## Existing Graycode Capabilities

Graycode already has durable sessions, event logging, WAL/recovery, branching,
review contracts, provider routing in Eyrie, MCP integration, skills, memory,
context compaction, policy snapshots, subagents, daemon security, and
filesystem/process sandboxing. These systems will not be duplicated.

## Implemented

1. Added an explicit tool lifecycle vocabulary to the existing tool execution
   path: validating, permission pending, executing, completed, failed,
   cancelled, and timed out.
2. Added terminal reasons for permission denial, approval denial, unknown tool,
   pipeline failure, timeout, cancellation, execution failure, and success.
3. Added regression coverage for the lifecycle contract.
4. Preserved Graycode's existing cancellation transcript cleanup and compaction.
5. Preserved policy-snapshot inheritance and subagent cleanup already present.
6. Added explicit `StreamEvent` tool lifecycle state and terminal-reason fields,
   with execution-path transitions and regression coverage.

## Future Work

### P0: Safety and lifecycle

- Emit lifecycle state transitions as structured eventlog entries.
- Record whether cancellation occurred before or after a side effect.
- Add shell virtual-operation escalation for compound commands.
- Add resume repair records for synthetic tool results.

### P1: MCP and background work

- Add per-session MCP server/tool/process budgets.
- Add deterministic refusal and health/reconnect status events.
- Add background task records with bounded drain and cancellation.
- Add ACP replay windows, prompt ledgers, and pre-attach bounds.

### P2: Declarative extensibility

- Map Markdown subagent frontmatter to Graycode's typed SpawnRequest.
- Add path-conditional skill activation and parse-error diagnostics.
- Add hot reload with bounded activation listeners.
- Scope child hooks and MCP resources by session/agent ID.

### P3: Memory and orchestration

- Add fast deterministic memory recall followed by optional refinement.
- Persist task origin, delivery/discard decisions, and memory deduplication.
- Add workflow snapshots only after lifecycle and budget contracts are stable.

## Verification

- Focused lifecycle, skills, engine, and command tests.
- Full Graycode build, vet, and test suite.
- Independent second verification pass.
- Final diff and sibling-repository status inspection.
