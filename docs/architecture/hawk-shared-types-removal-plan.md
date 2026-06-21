# Hawk Shared Types Removal Plan

## Purpose

This document defines the only safe path to remove:

- `github.com/GrayCodeAI/hawk/shared/types`

The key constraint is simple:

- local migration is complete
- external compatibility is not automatically knowable from this workspace

That means removal is a release decision, not just a code cleanup.

## Current state

Local reality today:

- Hawk production code does not import `hawk/shared/types`
- support repos in this workspace do not import `hawk/shared/types`
- consumer repos in this workspace do not import `hawk/shared/types`
- the package now exists only as a compatibility shim

What is still unknown:

- whether downstream users outside this workspace still import it

## Decision rule

Do not delete `hawk/shared/types` until both are true:

1. local retirement readiness passes
2. external compatibility risk is explicitly accepted

## Local retirement readiness

The local readiness check is now executable:

```bash
cd hawk
bash ./scripts/check-shared-types-retirement-readiness.sh
```

Passing this check means:

- no active local ecosystem repo still imports `github.com/GrayCodeAI/hawk/shared/types`

It does **not** mean deletion is automatically safe for external users.

## External compatibility gate

Before deletion, complete this checklist:

1. Search GitHub code search, internal consumers, and release notes history for `github.com/GrayCodeAI/hawk/shared/types`.
2. Confirm whether any public or internal downstream repos still import it.
3. If yes, migrate them first to `github.com/GrayCodeAI/hawk-core-contracts/types`.
4. Publish a release note that `hawk/shared/types` is deprecated and scheduled for removal.
5. Leave at least one visible release window for downstream migration.
6. Delete the package only in the next intentional breaking-change release.

## Recommended removal sequence

### Phase A: present state

- keep the package
- keep the deprecation comments
- keep blocking new imports
- keep the retirement readiness script green

### Phase B: release warning

- publish changelog note
- mark the removal target version/date
- notify downstream maintainers if known

### Phase C: breaking removal

Delete:

- `shared/types/doc.go`
- `shared/types/severity.go`
- `shared/types/finding.go`
- `shared/types/severity_test.go`
- `shared/types/finding_test.go`

Then update:

- docs that still describe it as a compatibility shim
- migration backlog status
- any CI/tests that assume the shim still exists

## What “done” means

This migration is fully done only when:

- `hawk-core-contracts/types` is the sole shared-type source of truth
- `hawk/shared/types` no longer exists
- no local or known external consumer depends on the shim

## Honest status

Today:

- local migration: complete
- local enforcement: complete
- removal safety for external consumers: not yet proven

So the correct current status is:

- ready for retirement
- not yet ready for blind deletion
