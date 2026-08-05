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
- `session/setMode` — switch work mode (plan|act|review)
- `session/setIsolation` — apply isolation profile
- `session/status` — control-plane snapshot (mode, isolation, autoCommit, message count)

### Deprecations
- `BackgroundAgentPool` / `NewBackgroundAgentPool*` / `FormatResults` **removed**
  in favor of `Session.SpawnController()` (same taskruntime.Registry). No
  production callers existed; the type and shims were deleted outright.

## Iteration 5

### ACP client docs
- `docs/acp/client.md` — wire protocol, worked lifecycle example
  (new → setMode → setIsolation → status → prompt), reference client.

### Benchmark scorecard
- `hawk eval smoke` — headless agent-loop smoke benchmark (stub provider,
  no API key, CI-safe): drives the real `Session.Stream` loop and scores
  steps / tool calls / token usage per task.
- Fixtures: `smoke-read-file` (must emit ≥1 Read tool call), `smoke-no-tools`
  (must terminate cleanly). Run with `hawk eval smoke`.

## Not done yet (next iterations)

- True 60s binary install path (packaging/CI) — Homebrew tap configured in
  goreleaser; requires `GrayCodeAI/homebrew-tap` repo + `HOMEBREW_TAP_TOKEN`
  secret before the next tagged release.
- Deeper ACP (session/load, client fs routing)
- Full Terminal-Bench scorecard against external agents

## Tests

- `internal/engine/control_plane_test.go`
- `internal/engine/project_trust_test.go` / git safety tests
- Existing sandbox bridge + permission display + diff tests
