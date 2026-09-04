# ADR-0003: Grok behavioral port keeps Go multi-repo Graycode

- Status: Accepted
- Date: 2026-07-16
- Owners: Graycode maintainers
- Related: `docs/plans/FULL-GROK-ECO-TO-GRAYCODE-ECO-PORT-PLAN.md`,
  `docs/plans/GROK-CLASS-CAPABILITY-LONG-HORIZON-PLAN.md`,
  `docs/plans/YEAR-0-ACTIVE.md`

## Context

Grok Build (`grok-eco/grok-build`) is a large Rust monorepo for a terminal AI
coding agent. Graycode is a multi-repo Go platform: product (`graycode`) plus peer
engines (`graycode-router`, `harrier`, `shrike`, `swift`, `kestrel`, `merlin`), foundation
contracts (`eagle`, `falcon`), SDKs, cloud, and skills.

The product goal is Grok-class **agent control-plane quality** (typed
subagents, sandbox profiles, folder trust, hooks, plugins/marketplace, task
runtime, plan UX) without abandoning Graycode’s multi-provider, local-first, and
peer-engine advantages.

## Decision

1. **Port means behavioral reimplementation in Go**, not a Rust crate copy,
   not a monorepo collapse, and not a runtime dependency on Grok binaries.
2. **Graycode remains multi-repo.** Map Grok capabilities onto existing owners:
   - product surface → `graycode`
   - shared DTOs → `eagle`
   - LLM routing/stream → `graycode-router`
   - memory → `harrier`
   - tokens/secrets/compress → `shrike`
   - session capture/import → `swift`
   - review / live audit → `kestrel` / `merlin`
   - marketplace content → `starling`
   - managed policy/tenancy → `graycode-platform/apps/worker` (deployed as `graycode-cloud`)
3. **Wire-first.** Prefer completing types and paths that already exist
   (for example typed subagent modes) over greenfield rewrites.
4. **Privacy-first.** Do not port default Mixpanel/product analytics. Opt-in
   OTEL remains the telemetry model.
5. **Typed spawn is the only subagent entrypoint** once Year 0 spawn work
   lands. `WireAgentTool` must not hardcode a single mode forever.
6. **Order is non-negotiable:** contracts → spawn/taskruntime → folder trust
   - sandbox profiles → hooks-first permissions → plugins/marketplace →
   monitor/wait/loop and UX batch. No project marketplace auto-load before
   folder trust.
7. **Year 0 active track** (control plane, trust, hooks, plugins, tasks/UX,
   user-guide 01–12) is the current execution program. Full TUI parity,
   computer hub, and deep enterprise policy are Year 1+.

## Consequences

- New shared agent spawn types live in `eagle/agent` (stdlib
  only). Engines and SDKs may import them; contracts never import graycode.
- Feature work updates the matrices in the full/long-horizon plans rather
  than inventing parallel roadmaps.
- Skip lists stay explicit: Ratatui widgets, Mixpanel defaults, Grok-only
  auth as sole path, computer hub until product decision.
- Contributors measure progress against Year 0 exit criteria in
  `docs/plans/YEAR-0-ACTIVE.md`.
