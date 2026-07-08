# hawk-eco shared templates

Every hawk-ecosystem repo's `Makefile`, `lefthook.yml`, and
`.github/workflows/*.yml` carries a header comment like:

```
# Source of truth: .shared-templates/Makefile.library.tmpl at the eco root.
```

This directory is that source of truth. It lives here, in `hawk`, because
`hawk` is the one repo every consumer already depends on or references —
there is no separate monorepo at the workspace root to hold it.

**This directory is not built or run by hawk itself.** It is a template
library that other hawk-ecosystem repos copy from and diff against.

## Layout

- `Makefile.library.tmpl` — Go library repos (engines, SDKs, foundation repos)
- `Makefile.binary.tmpl` — Go binary repos (currently only `hawk`)
- `Makefile.python.tmpl` — Python repos (`hawk-sdk-python`)
- `lefthook.yml.tmpl` — git hooks config, identical across all Go repos
- `.goreleaser.yml.tmpl` — goreleaser config for Go binary repos
- `workflows/go-ci.yml.tmpl` — CI pipeline for Go repos
- `workflows/go-release.yml.tmpl` — release workflow for Go **binary** repos (goreleaser)
- `workflows/go-lib-release.yml.tmpl` — release workflow for Go **library** repos (GitHub Release only, no binaries)
- `workflows/python-ci.yml.tmpl` — CI pipeline for Python repos
- `workflows/python-release.yml.tmpl` — PyPI publish workflow (Trusted Publishing)
- `workflows/compatibility-test.yml.tmpl` — cross-repo compatibility matrix check (see `docs/compatibility.md`)
- `scripts/check-ecosystem-boundaries.sh.tmpl` — the import-boundary guard, parameterized per repo role
- `scripts/sync-external.sh` — read-only drift report for `hawk`'s `external/` submodule pins (hawk-specific, not templated elsewhere)
- `docs/coverage-matrix.md` — per-repo test coverage thresholds enforced in CI, kept in one place so they don't silently drift out of sync with each repo's `go-ci.yml`

## How repos use this

There is currently no rendering tool — repos copy a template, replace the
placeholders marked `{{LIKE_THIS}}`, and keep the "Source of truth" header
comment pointing back here. When you change a template, the repos that
copied it are now stale; update them in the same PR or file a follow-up
per repo. `hawk`'s own `Makefile`/`lefthook.yml`/CI predate this directory
and intentionally diverge in a few binary-specific ways (see
`Makefile.binary.tmpl`, which documents the real deltas against `hawk`'s
Makefile).

## Boundary rule

Templates here must stay generic. If a change only applies to one repo, it
does not belong in the shared template — put it in that repo's own file and
leave the header comment noting the local deviation.
