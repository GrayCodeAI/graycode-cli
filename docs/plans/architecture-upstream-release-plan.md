# Plan: Hawk Architecture Upstream and Release Convergence

> Status: ready for execution
> Scope: push, merge, repin, verify, and release the Hawk ecosystem architecture work
> Goal: move the locally verified Hawk-centered architecture into upstream default branches and aligned published versions

## Purpose

The architecture cleanup is complete in the local `hawk-eco` workspace.

This plan covers the remaining operational work:

- push local branches upstream
- open and merge PRs in dependency order
- repin Hawk submodules to merged upstream SHAs
- rerun final integration verification
- publish tags/modules only after upstream convergence

## Principles

1. Merge shared contracts first.
2. Merge support-engine boundaries before Hawk.
3. Merge consumer guards after the engine direction is settled.
4. Merge Hawk last, because Hawk pins the support repos.
5. Do not redesign architecture during release convergence.

## Repo order

### Phase 1: shared contract base

1. `hawk-core-contracts`

### Phase 2: support engines

2. `sight`
3. `inspect`
4. `eyrie`
5. `yaad`
6. `trace`
7. `tok`

### Phase 3: consumers

8. `hawk-sdk-go`
9. `hawk-sdk-python`
10. `hawk-community-skills`

### Phase 4: product repo

11. `hawk`

## Repo board

| Repo | Branch | Commit | PR title | Merge gate |
|---|---|---|---|---|
| `hawk-core-contracts` | `main` | `f9989e5` | `docs: describe hawk-core-contracts as the live cross-repo API` | none |
| `sight` | `feat/contracts-migration` | `b990666` | `feat(contracts): migrate to hawk-core-contracts and enforce boundary` | `hawk-core-contracts` merged |
| `inspect` | `feat/contracts-migration` | `d6ca739` | `feat(contracts): migrate to hawk-core-contracts and enforce boundary` | `hawk-core-contracts` merged |
| `eyrie` | `feat/ecosystem-boundary-guard` | `c1a6a4d` | `docs: remove legacy shared types references` | `hawk-core-contracts` merged |
| `yaad` | `feat/ecosystem-boundary-guard` | `010178d` | `chore: strip Co-authored-by trailers in lefthook hooks` | `hawk-core-contracts` merged |
| `trace` | `feat/ecosystem-boundary-guard` | `735e3f4` | `chore: strip Co-authored-by trailers in lefthook hooks` | `hawk-core-contracts` merged |
| `tok` | `feat/contracts-types-realignment` | `83cfc551` | `refactor: remove tok types compatibility shim` | `hawk-core-contracts` merged |
| `hawk-sdk-go` | `ci/consumer-boundary-guard` | `97b523e` | `ci: guard sdk-go consumer boundaries` | support-engine direction settled |
| `hawk-sdk-python` | `ci/consumer-boundary-guard` | `c43ad43` | `ci: guard sdk-python consumer boundaries` | support-engine direction settled |
| `hawk-community-skills` | `ci/consumer-boundary-guard` | `350f4f2c6` | `ci: guard skills consumer boundaries` | support-engine direction settled |
| `hawk` | `docs/contracts-architecture-truth` | `a2a4583` | `chore: align hawk external architecture snapshot` | all upstream support repos merged |
| `hawk` | `docs/contracts-architecture-truth` | `46697b9` | `docs: retire tok types shim references` | `tok` merged |
| `hawk` | `docs/contracts-architecture-truth` | `c204597` | `docs: normalize architecture status` | final architecture state agreed |

## Execution checklist

### Phase 1: push branches

For each repo:

1. confirm working tree is clean
2. push the local branch
3. open PR with the planned title/summary
4. wait for CI

Suggested command pattern:

```bash
git -C <repo> push -u origin <branch>
gh -R GrayCodeAI/<repo> pr create --fill
```

### Phase 2: merge in dependency order

Merge order:

1. `hawk-core-contracts`
2. `sight`
3. `inspect`
4. `eyrie`
5. `yaad`
6. `trace`
7. `tok`
8. `hawk-sdk-go`
9. `hawk-sdk-python`
10. `hawk-community-skills`
11. `hawk`

Rules:

- do not merge `hawk` before the support repos
- do not publish module tags before merge convergence
- if upstream rebases or squashes PRs, treat the merged upstream SHA as the new source of truth

### Phase 3: repin Hawk

After the support-repo PRs merge:

1. fetch upstream default branches for all pinned repos
2. update `hawk/external/*` to the merged upstream SHAs
3. run `go work sync`
4. rerun Hawk verification
5. commit the repin if the upstream SHAs differ from current local pins

Checks:

- `external/eyrie`
- `external/hawk-core-contracts`
- `external/inspect`
- `external/sight`
- `external/tok`
- `external/trace`
- `external/yaad`

### Phase 4: final verification

From `hawk`:

```bash
/bin/sh ./scripts/check-shared-types-imports.sh
/bin/sh ./scripts/check-ecosystem-boundaries.sh
go work sync
go test ./internal/testaudit -count=1
```

From support repos:

```bash
/bin/sh ./scripts/check-ecosystem-boundaries.sh
go test ./... -count=1
```

From consumer repos:

```bash
/bin/sh ./scripts/check-consumer-boundaries.sh
```

## Release/tag guidance

Only after merge convergence:

1. decide which repos need tags immediately
2. publish `hawk-core-contracts` first if modules consume tagged versions
3. publish any support repos whose released versions are referenced by Hawk or external consumers
4. verify Hawk docs/examples do not claim unpublished versions

Minimum release check:

- merged commit exists on upstream default branch
- CI green on merged branch
- version/tag points at the merged contract-compatible state

## Risks and responses

### Upstream merge SHA differs from local SHA

Response:

- update Hawk submodule pins to the merged upstream SHA
- rerun `go work sync`
- rerun Hawk verification

### PR is squashed and commit messages change

Response:

- treat the merged branch state as canonical
- do not assume local commit SHAs remain valid for Hawk submodule pins

### A support repo fails CI after merge

Response:

- stop before merging Hawk
- fix the support repo first
- only repin Hawk after the repaired upstream state is green

### A published module version lags merged code

Response:

- do not claim release convergence yet
- tag/publish before calling the ecosystem release-ready

## Exit criteria

This plan is complete when:

- all listed PRs are merged upstream
- Hawk submodules point at merged upstream SHAs
- Hawk verification passes against those SHAs
- any required published module versions match the merged architecture state
- no repo needs the old architecture path or compatibility shims

