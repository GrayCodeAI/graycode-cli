# Capability seams

Deepseek-harness keeps every side effect behind a named, replaceable seam with a
strict owner. Hawk adopts the same discipline without the Cordis dependency
container: seams are plain Go interfaces or registries wired at the composition
root, and every registration that can outlive a request returns a disposer.

## Roles

- **Owner**: the component that defines the seam's contract and is the only
  component allowed to call it.
- **Impl**: the concrete, production implementation wired at startup.
- **Consumer**: code that depends on the behavior. It calls the owner, never the
  impl type directly.

These roles are enforced socially (reviews + `docs/`), not mechanically.

## Registry seams

| Seam | Owner | Production impl | Register returns disposer? |
| --- | --- | --- | --- |
| Tool pipeline (`StagePreExecute`, `StagePostExecute`) | `internal/tool` `Pipeline` | `engine.DefaultToolPipeline()` | Yes, `Pipeline.Register` |
| Approval waterfall (`approval.request`) | `internal/engine` `ApprovalWaterfall` | wired per session via `ApprovalGate.Waterfall` | Yes, `ApprovalWaterfall.Add` |
| Event bus waterfall (`RunWaterfall`) | `internal/engine` `EventBus` | per component at composition root | Yes, `EventBus.Waterfall` |
| Event bus pub/sub (`Publish`) | `internal/engine` `EventBus` | per component | No; `Unsubscribe` is the explicit remove |
| Hooks (`pre_tool`, etc.) | `internal/hooks` `Registry` | command hooks / built-ins | No; `Unregister` permits explicit remove |
| Tool registry | `internal/tool` `Registry` | runtime registry | No; `Unregister` is added for dynamic surfaces |
| Skills | `internal/engine/scaffold` `SkillRegistry` | ownership by skill owner | No (curated set) |
| MCP clients | `internal/mcp` `MCPServer` | provider registration | No (client lifecycle) |

## Principles

- **Every append-only interception seam returns a disposer.** The disposer removes
  exactly that registration; calling it is irreversible and safe to repeat via a
  once wrapper.
- **Empty waterfall is fail-closed where the answerer is safe/approval.** For
  tool pre-execute, no interceptor means pass through; for approval, no decider
  means deny.
- **Do not import impl packages across capability boundaries.** Import owners, not
  concrete registries. `internal/engine` is a composition root, not a library.
