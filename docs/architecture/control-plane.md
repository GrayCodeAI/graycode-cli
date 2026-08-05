# Hawk Control Plane (proposed → implemented core)

Status: **core wired** on branch work (isolation · spawn · lazy tools · plan/act/review).

## Goal

World-class CLI experience: **one face (Hawk)**, deep engines, progressive power.

```text
Faces (TUI / headless / ACP / daemon)
        │
        ▼
┌───────────────────────────────────────┐
│  Control plane                        │
│  WorkMode  Isolation  SpawnController │
│  Lazy model surface                   │
└───────────────────┬───────────────────┘
                    ▼
              Session kernel
           (agentLoop · tools)
                    │
     eyrie · yaad · tok · trace · sight · inspect
```

## Pieces

### 1. IsolationProfile (`engine.IsolationProfile`)

Single story for OS sandbox + container requirement:

| Preset | OS mode | Container required |
|--------|---------|-------------------|
| `dev` | off | no |
| `workspace` | workspace | no |
| `strict` | strict | no |
| `container` | workspace | yes |

- API: `Session.ApplyIsolationProfile`, `Session.Isolation()`
- CLI TUI: `/isolation [preset]`
- Startup: `configureSessionStartup` applies profile from settings sandbox string

OS wrap for Bash still uses `ContextWithMode` when mode is workspace/strict (see tool_service).

### 2. SpawnController (`engine.SpawnController`)

Single entry for subagents + background tasks:

- `Spawn(ctx, SpawnRequest)` — sync
- `SpawnBackground(ctx, id, req)` — async via `taskruntime`
- `Tasks()` — unified registry
- Agent tool continues through `WireAgentTool` → same `spawnSubAgentRequest`

### 3. Lazy model surface (`tool.Registry`)

- Essential tools registered and **model-visible**
- Optional tools registered for **execution + ToolSearch**, hidden from `EyrieTools`
- `ToolSearch` `select:Name` **promotes** tool onto the model surface
- APIs: `EnableLazyModelSurface`, `SetModelVisibility`, `PromoteModelTool`

### 4. WorkMode plan / act / review

| Mode | Tools | Bash |
|------|-------|------|
| `act` | Essential model set | full |
| `plan` | Plan set (read + plan suite) | read-only allowlist |
| `review` | Review set | read-only allowlist |

- API: `Session.SetWorkMode`, `Session.WorkMode()`
- TUI: `/mode plan|act|review` (shell modes remain `auto|shell|agent`)
- Ephemeral system prompt addon injected in `agentLoop`

## User commands

```text
/mode                 # show work + shell + isolation
/mode plan|act|review
/mode auto|shell|agent
/isolation            # show profile
/isolation workspace
/isolation container
```

## Iteration 2 (additional surface)

### Folder trust UX
- Already enforced in hooks/plugins (PACK-03); now surfaced in product:
  - `engine.ProjectTrust` / `TrustProject` / `UntrustProject`
  - TUI `/trust [status|add|remove]`
  - Status bar shows `trusted` / `untrusted` / `trust:off`

### Onboarding
- `/start` — trust, work mode, isolation, git advice, first tasks
- `/start trust` — trust cwd
- `/start branch` — create agent branch from main

### Git safety
- `engine.InspectGitBranch` / `EnsureAgentBranch`
- `/branch-agent` when on main/master
- Status bar warns `default-branch` when on main

### HUD
- Wide status secondary row: `mode:act · iso:workspace · trusted`
- `/status` includes work mode, isolation, trust, visible tool counts, git advice

## Iteration 3

### Auto-commit productized
- `ToolService.SetAutoCommit` → `ToolContext.AutoCommit` (was never wired from CLI)
- `--auto-commit` flag + `settings.auto_commit` + `/auto-commit on|off|status`
- Status bar shows `auto-commit` when enabled
- Write/Edit/StructuredEdit already call `tool.AutoCommit` when flag is set

### Background tasks
- Production path: `BackgroundAgentManager` → `taskruntime` (SpawnController uses same)
- `BackgroundAgentPool` remains test/legacy reexport only (not chat session path)

## Iteration 4

### Container ↔ IsolationProfile
- `attachRequiredContainer` and TUI container-ready path call `ApplyIsolationProfile(IsolationContainer)`

### Onboarding / CI
- CLI `Welcome` surfaces control-plane commands + `hawk exec`
- Example workflow: `examples/github/hawk-ci-exec.yml`

### ACP
- `initialize` advertises `hawkCapabilities` (work modes, isolation, lazy tools, …)
- `session/new` returns `hawk` snapshot (`workMode`, `isolation`, `autoCommit`) and defaults act mode

## Not done yet (next iterations)

- True 60s binary install path (packaging/CI)
- Deeper ACP (session/setMode, client fs routing)

- Public Terminal-Bench scorecard
- Optional: deprecate BackgroundAgentPool reexports

## Tests

- `internal/engine/control_plane_test.go`
- `internal/engine/project_trust_test.go` / git safety tests
- Existing sandbox bridge + permission display + diff tests
