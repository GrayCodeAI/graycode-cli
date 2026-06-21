# Hawk Architecture Plan

This directory holds the implementation planning docs for Hawk as a model-agnostic AI coding agent CLI.

Documents:

- `hawk-product-architecture.md` - target architecture and runtime flow
- `hawk-repo-roles.md` - role of each Hawk repo in the product ecosystem
- `hawk-dependency-rules.md` - import and ownership boundaries
- `hawk-core-contracts-spec.md` - shared contracts layer and current status
- `hawk-provider-abstraction.md` - provider/runtime abstraction design
- `hawk-review-verify-lifecycle.md` - review and verification lifecycle
- `hawk-trace-event-model.md` - trace and audit event model
- `hawk-contract-migration-inventory.md` - current shared-type usage and migration order

Core rule:

`hawk` is the product. The other Hawk repos are capabilities that power it.
