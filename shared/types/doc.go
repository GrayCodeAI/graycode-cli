// Package types is a deprecated compatibility layer for pre-contracts Hawk
// ecosystem consumers.
//
// Migration target:
//
//	github.com/GrayCodeAI/hawk-core-contracts/types
//
// Status:
//
// - The local Hawk ecosystem has already migrated off this package.
// - The package remains only to avoid a blind breaking change for downstream
//   consumers outside this workspace.
// - New code must not import this package.
//
// Removal policy:
//
// Delete this package only after the retirement checklist in
// docs/architecture/hawk-shared-types-removal-plan.md is satisfied.
package types
