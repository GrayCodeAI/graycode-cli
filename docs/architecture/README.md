# Graycode Architecture Plan

This directory holds the implementation and design docs for Graycode, the main
model-agnostic AI coding-agent CLI. `graycode-eco` is only the local parent
folder for independent repositories.

Documents:

- `graycode-current-vs-proposed.md` - current workspace shape vs target Graycode-centered repository architecture
- `graycode-ecosystem-summary.md` - one-page repo role, dependency, and future cloud summary
- `graycode-product-architecture.md` - target architecture and runtime flow
- `graycode-repo-roles.md` - role of each Graycode repo in the product ecosystem
- `graycode-dependency-rules.md` - import and ownership boundaries
- `eagle-spec.md` - shared contracts layer and current status
- `graycode-provider-abstraction.md` - provider/runtime abstraction design
- `graycode-review-verify-lifecycle.md` - review and verification lifecycle
- `graycode-swift-event-model.md` - swift and audit event model
- `graycode-architecture-v1-definition-of-done.md` - realistic shipping bar for architecture v1
- `adr/ADR-0004-file-first-session-history.md` - canonical session history and SQLite projection boundary
- `adr/ADR-0001-graycode-platform-telemetry-edge.md` - constrained optional
  runtime edge from Graycode to GrayCode Platform
- `tasks.md` - historical implementation checklist from the initial architecture pass (superseded by the definition-of-done doc; kept for record)
- `adr/` - accepted architecture decision records, e.g. exceptions to the dependency rules above
  - `ADR-0003-grok-behavioral-port-go-multirepo.md` - Year 0 Grok behavioral port keeps Go multi-repo
- Related active execution track: `docs/plans/YEAR-0-ACTIVE.md`

Core rule:

`graycode` is the main CLI/product. The other repositories are capabilities,
contracts, integrations, tooling, or optional platform services that connect
to it through the boundaries documented here.

Final target shape:

- `graycode` is the orchestrator and only primary product surface
- six peer support engines sit below Graycode:
  `graycode-router`, `harrier`, `shrike`, `swift`, `kestrel`, `merlin`
- `eagle` and `falcon` sit below those engines as shared foundations
- SDKs and community skills sit above Graycode as consumers of Graycode public surfaces
- `graycode-platform` stays outside the Graycode runtime module graph and exposes
  the optional web/BFF/Graycode Cloud plane
