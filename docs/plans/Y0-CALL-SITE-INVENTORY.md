# Year 0 Call-Site Inventory

**Date:** 2026-07-16  
**Purpose:** Freeze entry points before PACK-02 spawn and taskruntime work.  
**Rule:** Do not add a fourth background agent system.

## 1. Agent spawn

| Location | Role | Today |
|----------|------|--------|
| `internal/tool/tool.go` | `ToolContext.AgentSpawnFn` | **Updated:** `func(ctx, SpawnRequest) (SpawnResult, error)` |
| `internal/engine/agent_session_tool.go` | `WireAgentTool` | **Updated:** typed spawn; maps explore/plan/general |
| `internal/engine/agent_session_tool.go` | `spawnSubAgent` | Uses Normalized + mode; plan tools filter |
| `internal/tool/agent.go` | `Agent` tool | Schema: type, capability, isolation, thoroughness, cwd, model, resume, bg |
| `internal/tool/agent.go` | `MultiAgent` | String tasks + typed object tasks |
| `internal/tool/agentic_fetch.go` | Research spawn | Uses `AgentSpawnFn(prompt)` |
| `internal/tool/agent*_test.go` | Unit tests | Mock prompt-only spawn |

**Target (PACK-02):** `AgentSpawnFn(ctx, agent.SpawnRequest) (agent.SpawnResult, error)` from
`hawk-core-contracts/agent`, with adapter only if dual-path flag requires it.

## 2. Background / task systems (unify → one)

| System | Location | Role |
|--------|----------|------|
| `BackgroundAgentManager` | `internal/tool/background.go` | Sub-agent bg spawn + collect by id |
| `BackgroundRunner` | search under `internal/engine/` | Engine-level bg runs |
| `BackgroundAgentPool` | search under `internal/engine/agent/` | Pool for multi-agent |

**Target (PACK-02):** single `internal/taskruntime` (or equivalent) registry;
Wait/Kill/Monitor tools (PACK-06) bind only to that registry.

## 3. Mode / budget libraries (keep, wire)

| Location | Role |
|----------|------|
| `internal/engine/agent/agent_types.go` | explore / general / plan modes |
| `internal/engine/agent/subagent_budget.go` | tool allowlists + turn budgets |
| `internal/tool/bash_ast.go` | bash AST helpers for explore hard gate |

## 4. Permission / hooks / plugins (later packs)

| Location | Role | Y0 pack |
|----------|------|---------|
| `internal/engine/safety/permission_engine.go` (or permissions package) | CheckTool pipeline | PACK-03/04 |
| `internal/hooks/` | Hook registry/events | PACK-04 |
| `internal/plugin/` | Plugin manager V1/V2 | PACK-05 |
| `internal/sandbox/` | OS backends; modes | PACK-03 |

## 5. Feature flags

| Flag env | Package | Pack |
|----------|---------|------|
| `HAWK_Y0_SPAWN_V2` | `internal/flags` | PACK-02 |
| `HAWK_Y0_FOLDER_TRUST` | `internal/flags` | PACK-03 |
| `HAWK_Y0_MARKETPLACE` | `internal/flags` | PACK-05 |

## 6. Spawn test matrix template (PACK-02)

| subagent_type | capability | isolation | background | Expected |
|---------------|------------|-----------|------------|----------|
| explore | read-only (default) | none | false | No Write/Edit; bash AST gate |
| explore | read-only | worktree | false | Worktree cwd; read-only tools |
| plan | read-only | none | false | Plan tools only; no Write |
| general-purpose | all | none | false | Full tools |
| general-purpose | execute | worktree | true | Task id; killable; worktree |
| explore | — | none | false + resume_from | Continues transcript |

Cases must run under `go test` (+ race on taskruntime).

## 7. Dependency freeze

Until PACK-02 taskruntime cutover:

- [x] Document three bg systems
- [ ] No new background manager type without replacing an existing one
- [ ] All new spawn call sites take `SpawnRequest`
