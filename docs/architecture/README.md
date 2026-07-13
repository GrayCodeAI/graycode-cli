# Hawk Architecture Plan

This directory holds the implementation planning docs for Hawk as a model-agnostic AI coding agent CLI.

Documents:

- `hawk-current-vs-proposed.md` - current workspace shape vs target Hawk-centered repo architecture
- `hawk-ecosystem-summary.md` - one-page repo role, dependency, and future cloud summary
- `hawk-product-architecture.md` - target architecture and runtime flow
- `hawk-repo-roles.md` - role of each Hawk repo in the product ecosystem
- `hawk-dependency-rules.md` - import and ownership boundaries
- `hawk-core-contracts-spec.md` - shared contracts layer and current status
- `hawk-provider-abstraction.md` - provider/runtime abstraction design
- `hawk-eyrie-engine-migration.md` - implemented Hawk-face/Eyrie-engine boundary and submodule upgrade order
- `verification-status-2026-07-13.md` - dated verification evidence, current hardening, and release blockers
- `hawk-review-verify-lifecycle.md` - review and verification lifecycle
- `hawk-trace-event-model.md` - trace and audit event model
- `hawk-contract-migration-inventory.md` - current shared-type usage and migration order
- `hawk-architecture-v1-definition-of-done.md` - realistic shipping bar for architecture v1
- `tasks.md` - historical implementation checklist from the initial architecture pass (superseded by the definition-of-done doc; kept for record)
- `adr/` - accepted architecture decision records, e.g. exceptions to the dependency rules above

Core rule:

`hawk` is the product. The other Hawk repos are capabilities that power it.

Final target shape:

- `hawk` is the orchestrator and only primary product surface
- six peer support engines sit below Hawk:
  `eyrie`, `yaad`, `tok`, `trace`, `sight`, `inspect`
- `hawk-core-contracts` and `hawk-mcpkit` sit below those engines as shared foundations
- SDKs and community skills sit above Hawk as consumers of Hawk public surfaces
