# graycode-eco Long-Horizon Plan: Grok-Class Agent Control Plane (Go)

**Status:** Planning  
**Date:** 2026-07-16  
**Horizon:** ~12–18 months (phased; can compress with parallel teams)  
**Language:** Go only (no Rust ports; reimplement contracts and behavior)  
**Primary product:** `graycode`  
**Supporting repositories:** `eagle`, `eyrie`, `harrier` (Harrier), `shrike` (Shrike), `swift` (Swift), `kestrel` (Kestrel), `merlin` (Merlin), `falcon`, `starling`, `sparrow`, `robin`, `wren`, `owl`, and `graycode-platform` (web/BFF/Graycode Cloud; outside the Graycode Go runtime graph)

---

## 0. Executive summary

### Goal

Bring Graycode to **Grok-class agent control-plane quality** (typed subagents, sandbox profiles, folder trust, hooks, plugins/marketplace, ACP depth, monitor/scheduler, enterprise managed policy) **without** abandoning Graycode’s multi-repo Go platform advantages (eyrie multi-provider, harrier graph memory, shrike compression, mission mode, contracts, cloud ledger).

### Non-goals

- Rewrite Graycode in Rust or monorepo-collapse engines into graycode.
- Vendor lock-in to a single LLM host auth model.
- Replace eyrie/harrier/shrike/swift with Grok-shaped internal crates.
- Default opt-in product telemetry that violates privacy-first posture.

### Strategy

1. **Close wiring gaps first** — Graycode already has many *types* and *partial systems* that are not exposed on the Agent tool or unified.
2. **Stabilize contracts** in `eagle` before multi-repo consumers.
3. **Ship vertical slices** (end-to-end user value) every quarter.
4. **Keep engines peer-independent**; graycode remains the only product orchestrator.

---

## 1. Deep audit: current state (verified 2026-07-16)

### 1.1 What already exists (do not rebuild)

| Area | Evidence | Maturity |
|------|----------|----------|
| Subagent **modes as types** | `internal/engine/agent/agent_types.go`: `explore`, `general`, `plan`; thoroughness quick/medium/very-thorough; `ExploreTools` / `PlanTools` | **Types strong** |
| Mode tool allowlists + budgets | `internal/engine/agent/subagent_budget.go`: `ModeToolAllowlist`, `FilterToolsForMode`, turn budgets | **Library strong** |
| Subagent spawn implementation | `internal/engine/agent_session_tool.go`: `spawnSubAgent`, model cascade, tool filter, max depth, inherits spec stage | **Partial** |
| Agent tool surface | `internal/tool/agent.go`: only `prompt`, `run_in_background`, `agent_id`, `retry_of` | **Weak vs types** |
| Default wire path | `WireAgentTool()` **always** calls `spawnSubAgent(..., SubAgentExplore, 0)` | **Critical gap** |
| Background systems (3+) | `tool.BackgroundAgentManager`, `engine.BackgroundRunner`, `agent.BackgroundAgentPool` | **Duplicated** |
| Permission + spec gate | `engine/safety/permission_engine.go`: DryRun → SpecStage → autonomy → classifier → auto-mode → memory → prompt | **Strong core** |
| Sandbox backends | seatbelt, landlock, seccomp, docker, gvisor; modes strict/workspace/off | **Strong backends, weak profiles** |
| Personas | `multiagent/agents`: ReadOnly, tools, hooks, YAML | **Strong** |
| Mission worktrees | `multiagent/worker.go` creates git worktrees | **Strong for missions only** |
| Worktree tools | `EnterWorktree` / `ExitWorktree` | **User-driven, not spawn isolation** |
| Plugins V1/V2 | subprocess/daemon/WASM, hooks on manifest | **Partial packaging** |
| Skills install + community registry | git install + huge `starling` | **Content strong** |
| ACP first cut | `internal/acp` + `cmd/acp.go`: init/new/prompt/cancel/permission | **Thin** |
| Session checkpoints/fork | `internal/session/checkpoint.go`, fork, export | **Strong** |
| Cron | `internal/tool/cron.go` max 256 jobs | **Present, UX thin** |
| Cloud enterprise policy DTO | `graycode-platform/apps/worker` (deployed as `graycode-cloud`): model allow/deny, capability allow/deny | **Primitive exists** |
| Contracts | severity, tools, events, policy, review, verify, sessions phases | **Missing agent spawn types** |
| Compaction | multiple `engine/compact*.go` | **Present** |
| Memory | harrier graph engine | **Architecturally ahead of Grok markdown memory** |
| Token/cost | shrike library | **Architecturally ahead of Grok heuristic** |

### 1.2 Critical gap pattern

```
┌─────────────────────────────────────────────────────────────┐
│  Types & budgets exist (explore/plan/general)               │
│  spawnSubAgent(mode) exists                                 │
│  WireAgentTool hardcodes explore                            │
│  Agent tool schema cannot pass mode / isolation / resume    │
│  → Model cannot use plan mode or general-purpose by design  │
└─────────────────────────────────────────────────────────────┘
```

This is the single highest-ROI fix: **wire what already exists**, then extend.

### 1.3 Permission pipeline (actual vs target)

**Actual order** (`PermissionEngine.CheckTool`):

1. DryRun deny-all  
2. Spec stage gate (independent of autonomy)  
3. Autonomy preset / “safe tool” short-circuit  
4. Bypass killswitch  
5. Bash classifier “safe”  
6. AutoMode  
7. Permission memory  
8. User prompt  

**Missing relative to Grok-class:**

- PreToolUse **hooks before** all of the above (hooks exist as registry but not as first deny gate in this path)
- Explicit ordered **deny rules** package as distinct stage  
- Segment-aware **safe-bash allowlist** (classifier is different, weaker documentation)
- Named modes `dontAsk` / `acceptEdits` as first-class (partially in jsonc `defaultMode`)
- Folder trust gate for project automation

### 1.4 Duplication / debt that the plan must fix early

| Debt | Risk if ignored |
|------|-----------------|
| 3 background task managers | Inconsistent ids, race bugs, broken Wait/Kill |
| Agent tool signature `func(ctx, prompt string)` | Blocks mode/isolation/model forever |
| explore Bash allowlisted without hard read-only bash AST gate | Explore mode can still mutate via shell |
| Plugin = tools binary, not multi-component package | Marketplace cannot ship team bundles |
| No folder trust | Project hooks/MCP = RCE vector |

---

## 2. Target architecture (Go)

```text
                    ┌──────────────────────────────┐
                    │  graycode TUI / CLI / daemon/ACP │
                    └──────────────┬───────────────┘
                                   │
         ┌─────────────────────────┼─────────────────────────┐
         ▼                         ▼                         ▼
┌─────────────────┐     ┌──────────────────┐      ┌──────────────────┐
│ SpawnController │     │ Trust & Policy   │      │ Extension Host   │
│ typed subagents │     │ folder trust     │      │ plugins/skills   │
│ isolation       │     │ sandbox.toml     │      │ marketplace      │
│ resume          │     │ hooks pipeline   │      │ MCP/LSP load     │
└────────┬────────┘     └────────┬─────────┘      └────────┬─────────┘
         │                       │                         │
         ▼                       ▼                         ▼
┌─────────────────┐     ┌──────────────────┐      ┌──────────────────┐
│ engine.Session  │     │ PermissionEngine │      │ plugin.Manager   │
│ + TaskRuntime   │     │ + HookRunner     │      │ skill.Loader     │
└────────┬────────┘     └──────────────────┘      └──────────────────┘
         │
    ┌────┴────┬──────────┬──────────┬──────────┐
    ▼         ▼          ▼          ▼          ▼
  eyrie     harrier       shrike       swift     kestrel/merlin
```

**New core package (proposed):** `graycode/internal/spawn` (or expand `engine/agent`) owning:

- `SpawnRequest` / `SpawnResult` (Go structs aligned with contracts)
- capability filter, isolation worktree lifecycle
- transcript persistence for resume
- unified task registry (shell bg + subagents + monitors)

**Contracts package (proposed):** `eagle/agent` (or `spawn`):

- enums + DTOs only; no engine imports

---

## 3. Concept inventory → implementation status

Legend: **Done** | **Partial** | **Missing** | **N/A (keep engine)**

### Wave A — Agent control (P0)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| A1 | Typed spawn schema on Agent tool | Missing on tool; Partial in engine | Full |
| A2 | `subagent_type` explore/plan/general-purpose | Types Partial; wire Missing | Full |
| A3 | Thoroughness for explore | Types Done; wire Missing | Full |
| A4 | Capability modes read-only/read-write/execute/all | Partial (modes only) | Full enum + filter |
| A5 | Isolation none/worktree | Mission Partial; Agent Missing | Full |
| A6 | True `resume_from` transcript | Missing (agent_id status only) | Full |
| A7 | Structured SpawnResult | Partial envelope | Full |
| A8 | MultiAgent typed task objects | Missing | Full |
| A9 | Unify background task runtime | Partial (3 systems) | Full |
| A10 | Explore Bash hard read-only | Partial (comment only) | Full (AST allowlist) |

### Wave B — Trust & sandbox (P1)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| B1 | Built-in profiles incl. read-only, devbox | Partial (3 modes) | Full |
| B2 | `sandbox.toml` extends/deny globs | Missing | Full |
| B3 | Project additive-only profile merge | Missing | Full |
| B4 | Folder trust store | Missing | Full |
| B5 | Gate hooks/MCP/LSP/plugins on trust | Missing | Full |
| B6 | Safe-bash allowlist (segment-aware) | Partial classifier | Full |
| B7 | Permission pipeline doc + hooks first | Partial | Full |
| B8 | Named modes acceptEdits/dontAsk | Partial jsonc | Full product |

### Wave C — Hooks (P2)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| C1 | Full event set (subagent, stop, failure) | Partial | Full |
| C2 | Vendor event aliases | Missing | Full |
| C3 | File-discovered hooks | Partial (in-process) | Full |
| C4 | HTTP hooks | Missing | Full |
| C5 | Plugin env GRAYCODE_PLUGIN_ROOT/DATA | Missing | Full |
| C6 | PreToolUse can deny in CheckTool path | Missing | Full |

### Wave D — Plugins & marketplace (P3)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| D1 | Multi-component plugin layout | Missing | Full |
| D2 | Discovery scopes + priority | Partial | Full |
| D3 | Marketplace multi-source | Design only | Full |
| D4 | Multi-harness skill scan | Missing | Full |
| D5 | Audit on install (keep) | Partial | Harden |

### Wave E — Tasks / monitor / scheduler (P4)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| E1 | Shell background task_id | Partial | Full |
| E2 | GetTaskOutput / WaitTasks / KillTask | Missing as tools | Full |
| E3 | Monitor line-stream tool | Missing | Full |
| E4 | /loop UX over cron | Partial cron | Full |

### Wave F — Plan UX & AskUser (P5)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| F1 | Align /spec with plan subagent | Partial | Full |
| F2 | Structured AskUserQuestion | Partial free-text | Full |

### Wave G — ACP & SDKs (P6)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| G1 | session/load + resume | Missing | Full |
| G2 | Richer session/update | Partial | Full |
| G3 | Optional agent serve/WS | Missing | Optional |
| G4 | SDK spawn/plugin fields | Missing | Full |
| G5 | OpenAPI sync | Partial | Full |

### Wave H — Sessions / import / attribution (P7)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| H1 | Foreign session import | Missing | Full (swift) |
| H2 | Hunk agent vs external | Missing | Full |
| H3 | Checkpoint already | Done | Keep |

### Wave I — Memory UX (P8)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| I1 | Toggle priority order | Partial | Full |
| I2 | Dream/consolidate product UX | Partial harrier | Full |
| I3 | Graph memory core | Done (harrier) | Keep |

### Wave J — Enterprise (P9)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| J1 | Config layer merge order | Partial | Full |
| J2 | Managed signed policy pull | Partial cloud DTO | Full |
| J3 | Fail-open default / fail-closed org | Missing | Full |

### Wave K — Docs & polish (P10)

| # | Concept | Status | Target |
|---|---------|--------|--------|
| K1 | User-guide tiered docs | Missing | Full |
| K2 | Mermaid render optional | Partial text | Optional |
| K3 | Keep engines independent | Done | Keep |

---

## 4. Multi-quarter roadmap (long horizon)

Assumes ~1–2 senior Go engineers on graycode core + fractional engine/cloud; scale parallelizes quarters.

```text
Q1  Foundation: contracts + spawn wiring + background unify
Q2  Trust: sandbox.toml + folder trust + safe-bash + hooks gate
Q3  Extensions: plugins multi-component + marketplace MVP + skill multi-harness
Q4  Runtime: monitor/wait/loop + AskUser structured + plan alignment
Q5  Integration: ACP phase-2 + OpenAPI + SDKs
Q6  Enterprise: managed policy + cloud apply + IT tier
Q7  Ecosystem: foreign import (swift) + hunk attribution + memory UX
Q8  Hardening: perf, fuzz, security audit, docs completion, GA polish
```

Calendar can slip; **order of waves should not reverse** (contracts before marketplace consumers, trust before project plugins).

---

## 5. Detailed phase plans

### Phase 0 — Prep (1–2 weeks)

**Objectives**

- Freeze concept list (this document).
- Inventory all `AgentSpawnFn` call sites and background managers.
- Add ADR: “Typed spawn is the only subagent entrypoint.”

**Deliverables**

- [ ] ADR in `docs/architecture/adr/`
- [ ] Call-site map in this plan appendix
- [ ] Test plan template for spawn matrix
- [ ] Dependency freeze: no new third background system

**Exit criteria**

- Team agrees Wave A is first PR stack; no marketplace before folder trust.

---

### Phase 1 — Contracts foundation (2–3 weeks)

**Repo:** `eagle`  
**Module:** new package `agent` (or `spawn`) — stdlib only

#### 1.1 Types to add

```go
// CapabilityMode
const (
    CapReadOnly  = "read-only"
    CapReadWrite = "read-write"
    CapExecute   = "execute"
    CapAll       = "all"
)

// IsolationMode
const (
    IsoNone     = "none"
    IsoWorktree = "worktree"
)

// SubagentType
const (
    TypeGeneralPurpose = "general-purpose" // maps to general
    TypeExplore        = "explore"
    TypePlan           = "plan"
)

type SpawnRequest struct {
    Prompt         string
    Description    string
    SubagentType   string
    CapabilityMode string // optional; derived from type if empty
    Isolation      string
    ResumeFrom     string
    CWD            string
    Model          string
    Background     bool
    Thoroughness   string // explore only
    ParentSession  string
}

type SpawnResult struct {
    SubagentID   string
    SubagentType string
    Status       string // running|completed|failed
    Output       string
    Summary      string
    ToolCalls    int
    Turns        int
    DurationMs   int64
    WorktreePath string
    Persona      string
    Error        string
}
```

#### 1.2 Work items

| ID | Task | Effort | Tests |
|----|------|--------|-------|
| C-1 | Package + Parse helpers (aliases: general↔general-purpose, ReadOnly) | S | table tests |
| C-2 | Validate mutual exclusion cwd vs worktree | S | unit |
| C-3 | Version bump + CHANGELOG | S | CI module |
| C-4 | Proto optional later — **not required Q1** | — | — |

**Exit criteria**

- `go test ./...` green; graycode can depend on new pseudo-version without engine imports.

---

### Phase 2 — Spawn controller & Agent tool (4–6 weeks)  ★ highest ROI

**Repo:** `graycode`  
**Packages:** `internal/engine/agent`, `internal/engine`, `internal/tool`

#### 2.1 Change `AgentSpawnFn` signature

**Today:**

```go
AgentSpawnFn func(ctx context.Context, prompt string) (string, error)
```

**Target:**

```go
AgentSpawnFn func(ctx context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error)
```

Migration: temporary adapter for one release if needed; prefer single breaking change in 0.x.

#### 2.2 Wire `WireAgentTool` correctly

| Step | Change |
|------|--------|
| 1 | Parse Agent tool JSON → `SpawnRequest` |
| 2 | Map `subagent_type` → `SubAgentMode` (+ thoroughness) |
| 3 | Default capability from mode; allow override |
| 4 | Apply `FilterToolsForMode` **and** capability filter |
| 5 | Isolation worktree: create under `.graycode/worktrees/<id>` using shared helper from mission worker |
| 6 | Persist child session transcript under `~/.graycode/subagents/<id>/` |
| 7 | `resume_from`: load transcript, append prompt, re-spawn with same type |
| 8 | Return structured `SpawnResult` JSON to model |

#### 2.3 Explore Bash hard gate

- Reuse `internal/tool` bash AST if present (`bash_ast.go`).
- For `IsReadOnlyMode`: only allowlist commands (Grok-class list).
- Deny `rm`, redirects, `git commit`, network tools, etc.

#### 2.4 MultiAgent

```json
{
  "tasks": [
    {
      "prompt": "...",
      "description": "scan auth",
      "subagent_type": "explore",
      "capability_mode": "read-only",
      "thoroughness": "very-thorough"
    }
  ],
  "run_in_background": true
}
```

#### 2.5 Unify background runtime

**Target single package:** `internal/taskruntime`

| Capability | API |
|------------|-----|
| Register | shell / subagent / monitor |
| Get | status + output |
| Wait | any/all + timeout |
| Kill | cancel context + process group |
| List | active |

Deprecate:

- Prefer migrating call sites off `BackgroundRunner` and `BackgroundAgentPool` into one manager; keep thin wrappers one release.

#### 2.6 PR stack (Graphite-friendly)

1. Contracts bump in graycode  
2. `SpawnRequest` internal + adapter  
3. Agent tool schema + parse  
4. Wire modes plan/general/explore  
5. Worktree isolation  
6. Transcript resume  
7. Background unify  
8. Explore bash hard gate  
9. MultiAgent objects  
10. Docs + e2e tests  

**Exit criteria**

- Model can spawn plan agent that cannot Write.  
- Explore cannot `rm` via Bash.  
- Resume continues conversation with prior tool results present.  
- Single task id namespace for bg subagents.  
- Race tests pass under `-race`.

**Effort:** ~6–8 eng-weeks.

---

### Phase 3 — Sandbox profiles + folder trust (4–5 weeks)

**Repo:** `graycode`  
**Packages:** `internal/sandbox`, `internal/trust` (new), `internal/mcp`, `internal/hooks`, `internal/plugin`

#### 3.1 sandbox.toml

```toml
# ~/.graycode/sandbox.toml
[profiles.ci]
extends = "strict"
restrict_network = true
deny = ["**/.env", "**/*.pem", "**/*credentials*"]
```

**Loader rules (security-critical):**

1. Load user global.  
2. Load project `.graycode/sandbox.toml` **additive only** (new names only).  
3. Warn on conflicting redefinition; ignore project redefinition.  
4. Deny globs applied on top of backend (seatbelt/landlock/bwrap-equivalent).  
5. Fail closed when deny requested but OS cannot enforce.

#### 3.2 Profile matrix

| Name | FS read | FS write | Network | Maps from today |
|------|---------|----------|---------|-----------------|
| off | all | all | all | ModeOff / TierOff |
| workspace | all | cwd+state+tmp | allow | ModeWorkspace |
| read-only | all | state+tmp only | deny where possible | **new name** |
| strict | cwd+sys | cwd+state+tmp | deny | ModeStrict |
| devbox | all | broad except protected | allow | **new** |
| custom | extends | extends | flag | **new** |

#### 3.3 Folder trust

**Store:** `~/.graycode/trusted_folders.toml`

```toml
[[folders]]
path = "/Users/me/proj"
trusted_at = "2026-07-16T00:00:00Z"
```

**Commands:** `/trust`, `/trust status`, `/trust revoke`, CLI `--trust`

**Gates when untrusted:**

- Project hooks  
- Project MCP  
- Project LSP  
- Project plugins  
- Optional: project skills that execute scripts  

**Exit criteria**

- Malicious project cannot redefine global `strict` profile.  
- Untrusted project hooks never run (tested).  
- Custom deny `**/.env` blocks Read and Bash cat (integration tests per OS).

**Effort:** ~5–7 eng-weeks (OS matrix heavy).

---

### Phase 4 — Hooks pipeline completion (3–4 weeks)

**Repo:** `graycode`  
**Package:** `internal/hooks`

#### 4.1 Event model expansion

Add: `stop`, `stop_failure`, `post_tool_failure`, `permission_denied`, `user_prompt_submit`, `notification`, `subagent_start`, `subagent_stop`  
Keep existing; alias map for Claude/Cursor names.

#### 4.2 Runners

| Type | Behavior |
|------|----------|
| command | shell with timeout, env GRAYCODE_* |
| http | POST JSON, timeout, optional deny on non-2xx for pre_tool |

#### 4.3 Integration into PermissionEngine

```text
CheckTool:
  1. DryRun
  2. PreTool hooks (deny wins)
  3. Spec stage
  4. Explicit deny rules
  5. ... rest
```

#### 4.4 File discovery

- `~/.graycode/hooks/*.json`  
- `.graycode/hooks/*.json` (trust required)  
- Compat: `.claude/settings.json` hooks, `.cursor/hooks.json` behind flags  

**Exit criteria**

- Pre-tool hook can block `Bash(rm -rf)` even in Autonomous.  
- Project hook skipped until trust.  
- HTTP hook timeouts do not hang session (fail open or closed by config).

**Effort:** ~4 eng-weeks.

---

### Phase 5 — Plugins multi-component + marketplace MVP (6–8 weeks)

**Repos:** `graycode`, `starling`

#### 5.1 Plugin layout (convention)

```text
my-plugin/
  plugin.json          # optional
  skills/**/SKILL.md
  commands/*.md
  agents/*.yaml|*.md
  hooks/hooks.json
  .mcp.json
  .lsp.json
```

Loader merges components; tools optional.

#### 5.2 Discovery priority

1. Session meta / SDK  
2. `--plugin-dir`  
3. Project `.graycode/plugins` (trust)  
4. User `~/.graycode/plugins`  
5. Config extra paths  

#### 5.3 Marketplace

| Component | Owner |
|-----------|--------|
| Source list config | graycode |
| Index schema | community-skills + graycode |
| Install resolve (git) | graycode |
| Audit | graycode plugin malware_check (extend) |
| CLI `graycode plugins` / TUI tab | graycode |
| Optional web gallery | graycode-platform later |

#### 5.4 Multi-harness skills

Scan `.claude`, `.cursor`, `.agents` with toggles in settings; dedupe by name.

**Exit criteria**

- Install plugin with skill+hook+MCP in one command.  
- Marketplace install from official + one third-party source.  
- Audit fails install of skill with shell exfil pattern.

**Effort:** ~8–10 eng-weeks (product + content).

---

### Phase 6 — Task runtime: monitor, wait, loop (3–4 weeks)

Depends on Phase 2 unified `taskruntime`.

| Tool | Spec |
|------|------|
| `GetTaskOutput` | id, optional timeout_ms |
| `WaitTasks` | ids[], mode any\|all, timeout |
| `KillTask` | id |
| `Monitor` | command, persistent?, timeout; line events → stream |

**Loop UX**

- `/loop 5m <prompt>` → cron wrapper with caps (max jobs, 7-day expiry)  
- Fire immediate + interval  

**Exit criteria**

- Dev server monitor produces line events without flooding (rate limit).  
- Wait any returns when first subagent completes.

**Effort:** ~4 eng-weeks.

---

### Phase 7 — Plan alignment + structured AskUser (2–3 weeks)

| Work | Detail |
|------|--------|
| Plan subagent | Default tool for “design only”; writes plan file under `.graycode/plans/` or specs |
| Spec workflow | Document mapping: plan agent → `/spec` stages; avoid double systems |
| AskUserQuestion | questions[], options, multi_select, other, cancel message |
| TUI | Reuse autonomy/spec pickers |

**Exit criteria**

- Structured multi-question UI in chat.  
- Plan agent never writes production code tools.

**Effort:** ~3 eng-weeks.

---

### Phase 8 — ACP phase-2 + SDKs + OpenAPI (5–7 weeks)

**Repos:** `graycode`, `sparrow`, `robin`

| Milestone | Scope |
|-----------|--------|
| ACP-1 | session/load, list sessions |
| ACP-2 | richer updates (tool start/end, thoughts if available) |
| ACP-3 | permission round-trip hardened |
| API | OpenAPI fields: spawn options, sandbox, autonomy, plugins meta |
| SDK | AgentConfig + ChatRequest fields; plugin dirs |
| Optional | `graycode agent serve --bind` WS |

**Exit criteria**

- Zed/VS Code can drive graycode ACP for multi-turn with permissions.  
- SDK e2e against daemon contract snapshot tests.

**Effort:** ~7 eng-weeks (+ extension work separate).

---

### Phase 9 — Enterprise managed policy (4–6 weeks)

**Repos:** `graycode-platform/apps/worker` (deployed as `graycode-cloud`), `graycode`, optional `eyrie`

| Piece | Detail |
|-------|--------|
| Policy document | models allow/deny, capabilities, sandbox deny, max budget, tool denylist |
| Signing | Ed25519 envelope; graycode verifies before apply |
| Apply path | `~/.graycode/managed_policy.json` layers under user config |
| Default | fail-open for individual; org can require fail-closed |
| IT tier | non-excludable rules (already sketched in product — finish) |
| Cloud UI | graycode-platform admin later |

Build on existing enterprise policyInput (model/capability lists).

**Exit criteria**

- Org policy denies model X on device within TTL.  
- Tampered signature rejected.  
- Offline individual still works without cloud.

**Effort:** ~6 eng-weeks.

---

### Phase 10 — Ecosystem polish (swift import, hunks, harrier UX) (4–6 weeks)

| Item | Repo | Detail |
|------|------|--------|
| Foreign import | swift + graycode CLI | Claude/Codex session metadata → index |
| Hunk attribution | graycode | agent vs external edits via fsnotify |
| Memory UX | harrier + graycode | toggle priority, `/dream` consolidate |
| Mermaid optional | graycode | sandbox render path |

**Exit criteria**

- Import at least Claude sessions list.  
- “Agent changed files this turn” accurate in TUI.

**Effort:** ~5 eng-weeks.

---

### Phase 11 — Hardening & GA (ongoing last quarter)

- Fuzz bash allowlist, sandbox deny globs, spawn JSON  
- Race + stress multiagent  
- Security review: trust, hooks, marketplace install  
- User-guide `docs/user-guide/01–22`  
- Performance: spawn latency, worktree creation  
- Compatibility matrix: macOS seatbelt, Linux landlock, Windows best-effort  
- Release: feature flags default-on after bake  

---

## 6. Cross-repo ownership matrix

| Concept family | Primary | Secondary | Contracts? |
|----------------|---------|-----------|------------|
| Spawn / capability / isolation | graycode | — | yes |
| Sandbox profiles | graycode | — | optional DTO |
| Folder trust | graycode | — | no |
| Hooks | graycode | — | event names yes |
| Plugins / marketplace | graycode | community-skills | manifest schema |
| Multi-harness skills | graycode | community-skills | no |
| Monitor/tasks | graycode | — | optional |
| ACP | graycode | sdk-go/python | OpenAPI |
| Managed policy | graycode-platform/apps/worker (deployed as `graycode-cloud`) | graycode, graycode-platform/apps/bff | yes DTO |
| Foreign import | swift | graycode | optional |
| Memory dream UX | harrier | graycode | no |
| Token | shrike | — | no change |
| Providers | eyrie | graycode | no change |
| Review engines | kestrel/merlin | graycode | findings already |

---

## 7. Testing strategy (all phases)

### 7.1 Unit

- Parse/validate SpawnRequest (aliases, mutual exclusion)  
- FilterToolsForMode × capability matrix  
- sandbox.toml merge security  
- hook alias map  
- safe-bash segments  

### 7.2 Integration

- Spawn explore/plan/general end-to-end with fake LLM  
- Worktree isolation: parent tree unchanged  
- Resume transcript  
- Untrusted project hooks never execute  
- Deny glob blocks file read  

### 7.3 Race / stress

- MultiAgent max concurrency  
- Background collect  
- Monitor rate limit  

### 7.4 OS matrix

| Feature | macOS | Linux | Windows |
|---------|-------|-------|---------|
| Seatbelt profiles | required | n/a | n/a |
| Landlock/seccomp | n/a | required | n/a |
| Worktree isolation | yes | yes | best-effort |
| Folder trust | yes | yes | yes |

### 7.5 Compatibility / contract CI

- `eagle` version pin  
- OpenAPI snapshot for SDKs  
- Engine import boundary scripts (existing ecosystem boundary checks)

---

## 8. Feature flags & rollout

| Flag | Default early | GA |
|------|---------------|-----|
| `spawn.v2` | on for contributors | on |
| `sandbox.profiles` | off → on | on |
| `trust.folder` | on (secure default) | on |
| `hooks.file_discovery` | off → on | on |
| `plugins.marketplace` | off | on |
| `monitor.tool` | off | on |
| `acp.v2` | off | on |
| `policy.managed` | off | org-only |

Prefer **secure defaults** (folder trust on) even if noisy.

---

## 9. Risk register

| Risk | Impact | Mitigation |
|------|--------|------------|
| Signature change of AgentSpawnFn breaks plugins | High | Adapter release; version min_graycode |
| Worktree disk bloat | Med | GC old worktrees; symlink shared dirs (already pattern) |
| Explore still escapes via Bash | High | AST allowlist + sandbox |
| Marketplace supply chain | High | audit, pin commit SHAs, signatures later |
| Hook RCE via project | Critical | folder trust + fail closed |
| Three bg systems mid-migration | Med | single package + deprecation window |
| Over-scoping vs release readiness | High | Phases 1–2 only until contributor GA |
| Cloud policy vs local-first conflict | Med | fail-open default; explicit org enroll |

---

## 10. Success metrics

| Metric | Baseline (now) | Target (post Phase 2) | Target (GA) |
|--------|----------------|------------------------|-------------|
| Agent tool can select plan/explore/general | No (hardcoded explore) | Yes | Yes |
| Explore shell mutation blocked | Partial | 100% allowlist tests | Fuzz green |
| Untrusted project hooks run | Possible | 0 | 0 |
| Time to implement team plugin (skill+mcp+hook) | High friction | <30 min | <10 min marketplace |
| ACP multi-turn with permissions | First-cut | Full load/resume | Editor-certified |
| Subagent resume fidelity | Status only | Full transcript | Full + worktree |
| Doc user-guide chapters | ~0 numbered | 8 | 20+ |

---

## 11. Suggested PR / milestone naming

```text
feat(contracts): agent spawn DTOs
feat(spawn): wire explore/plan/general through Agent tool
feat(spawn): capability modes + tool filter
feat(spawn): worktree isolation
feat(spawn): resume_from transcripts
refactor(taskruntime): unify background managers
feat(sandbox): sandbox.toml profiles
feat(trust): folder trust store
feat(hooks): pre_tool deny + file/http runners
feat(plugin): multi-component layout
feat(marketplace): multi-source install MVP
feat(tools): monitor + wait + kill
feat(acp): session load/resume
feat(cloud): managed policy apply
docs(user-guide): 01-getting-started … 
```

---

## 12. Resource plan (long horizon)

| Role | Q1–Q2 | Q3–Q4 | Q5–Q8 |
|------|-------|-------|-------|
| Graycode core (Go) | 1.5 FTE | 1.5 FTE | 1 FTE |
| Security-minded sandbox | 0.5 FTE | 0.5 FTE | 0.25 FTE |
| Community-skills / marketplace content | 0.25 | 0.75 | 0.5 |
| Cloud (TS) | 0 | 0.25 | 0.75 |
| SDK / ACP | 0.25 | 0.25 | 0.75 |
| Docs / DX | 0.25 | 0.5 | 0.5 |

**Minimum viable long-horizon track** (one engineer): Phases 1→2→3→4 only (~4–5 months) yields most product quality jump.

---

## 13. Appendix A — Call sites to migrate (AgentSpawnFn)

Verified entry points (non-exhaustive; re-grep at start of Phase 2):

| File | Usage |
|------|--------|
| `internal/engine/session.go` | field declaration |
| `internal/engine/agent_session_tool.go` | WireAgentTool / spawnSubAgent |
| `internal/engine/stream_tool_exec.go` | ToolContext injection |
| `internal/tool/agent.go` | Agent / MultiAgent |
| `internal/tool/agentic_fetch.go` | research spawn |
| `internal/tool/tool.go` | ToolContext |

Background:

| File | System |
|------|--------|
| `internal/tool/background.go` | BackgroundAgentManager |
| `internal/engine/background_runner.go` | BackgroundRunner |
| `internal/engine/agent/background_agent.go` | BackgroundAgentPool |

---

## 14. Appendix B — Mapping Grok names → Graycode names

| Grok | Graycode |
|------|------|
| `task` tool | `Agent` / `Task` alias |
| `general-purpose` | `general` / `general-purpose` |
| `capability_mode` | same |
| `isolation: worktree` | same |
| `resume_from` | same (replace weak `agent_id` semantics) |
| `sandbox.toml` | `~/.graycode/sandbox.toml` |
| folder trust | `~/.graycode/trusted_folders.toml` |
| `/loop` | `/loop` over CronScheduler |
| plugin marketplace | `graycode plugins` + community registry |
| managed_config | managed policy via graycode-cloud |
| ACP | `graycode acp` |

---

## 15. Appendix C — First 30-day execution checklist

Week 1

- [ ] Land contracts package + tests  
- [ ] Document ADR  
- [ ] Inventory + delete path for bg managers (design only)

Week 2

- [ ] SpawnRequest internal  
- [ ] Agent tool schema expanded (flags behind feature if needed)  
- [ ] Wire plan + general (stop hardcoding explore)

Week 3

- [ ] Explore bash allowlist enforcement  
- [ ] SpawnResult structured output  
- [ ] Integration tests fake LLM

Week 4

- [ ] Worktree isolation MVP  
- [ ] Transcript save (resume stub OK)  
- [ ] CHANGELOG + user-facing `/help` snippet  

---

## 16. Decision log (planning)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go only | graycode-eco ecosystem is Go |
| First work | Wire existing modes | Types already exist; highest ROI |
| Contracts location | eagle | Engines/SDKs may later honor modes |
| Trust default | On | Security |
| Marketplace before trust? | No | Supply chain |
| Replace harrier? | No | Graph memory superior |
| Replace eyrie? | No | Multi-provider is differentiator |
| Three bg systems | Unify early | Blocks monitor/wait |

---

## 17. Document maintenance

- Update this plan at each phase exit.  
- Mark concept table statuses.  
- Link PRs under each phase.  
- Supersedes marketing claims in `docs/IMPLEMENTATION-ROADMAP.md` for agent-control scope (that doc remains for marketplace/IDE product ideas).

---

**End of plan.**  
Next action if approved: execute **Phase 1 (contracts)** then **Phase 2 PR stack** without waiting for marketplace or cloud.
