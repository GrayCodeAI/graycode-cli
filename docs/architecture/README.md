# Hawk Architecture Plan

This directory holds the implementation and design docs for Hawk, the main
model-agnostic AI coding-agent CLI. `graycode-eco` is only the local parent
folder for independent repositories.

Documents:

- `hawk-current-vs-proposed.md` - current workspace shape vs target Hawk-centered repository architecture
- `graycode-ecosystem-summary.md` - one-page repo role, dependency, and future cloud summary
- `hawk-product-architecture.md` - target architecture and runtime flow
- `hawk-repo-roles.md` - role of each Hawk repo in the product ecosystem
- `hawk-dependency-rules.md` - import and ownership boundaries
- `eagle-spec.md` - shared contracts layer and current status
- `hawk-provider-abstraction.md` - provider/runtime abstraction design
- `hawk-review-verify-lifecycle.md` - review and verification lifecycle
- `hawk-swift-event-model.md` - swift and audit event model
- `hawk-architecture-v1-definition-of-done.md` - realistic shipping bar for architecture v1
- `adr/ADR-0004-file-first-session-history.md` - canonical session history and SQLite projection boundary
- `adr/ADR-0001-graycode-platform-telemetry-edge.md` - constrained optional
  runtime edge from Hawk to GrayCode Platform
- `tasks.md` - historical implementation checklist from the initial architecture pass (superseded by the definition-of-done doc; kept for record)
- `adr/` - accepted architecture decision records, e.g. exceptions to the dependency rules above
  - `ADR-0003-grok-behavioral-port-go-multirepo.md` - Year 0 Grok behavioral port keeps Go multi-repo
- Related active execution track: `docs/plans/YEAR-0-ACTIVE.md`

Core rule:

`hawk` is the main CLI/product. The other repositories are capabilities,
contracts, integrations, tooling, or optional platform services that connect
to it through the boundaries documented here.

Final target shape:

- `hawk` is the orchestrator and only primary product surface
- six peer support engines sit below Hawk:
  `eyrie`, `harrier`, `shrike`, `swift`, `kestrel`, `merlin`
- `eagle` and `falcon` sit below those engines as shared foundations
- SDKs and community skills sit above Hawk as consumers of Hawk public surfaces
- `graycode-platform` stays outside the Hawk runtime module graph and exposes
  the optional web/BFF/Hawk Cloud plane
