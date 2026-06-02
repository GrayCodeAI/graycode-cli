# Compatibility Matrix

This eco uses **independent SemVer per repo** (see [VERSIONING.md](./versioning.md)).
That gives each component its own release cadence, but raises an obvious
question: *which combinations of versions are actually tested together?*

The answer lives in [`testdata/compatibility-matrix.json`](./testdata/compatibility-matrix.json).

Platform/provider capability metadata is separate: [`platform-capabilities.json`](./docs/platform-capabilities.json).

## What it records

```jsonc
{
  "components":   ["hawk", "eyrie", ...],   // canonical eco roster
  "dependencies": { "hawk": ["eyrie", ...] }, // who depends on who
  "matrices": [
    { "name": "stable", "components": { "hawk": "0.2.0", "eyrie": "0.2.0", ... } },
    { "name": "next",   "components": { "hawk": "main",  "eyrie": "main",  ... } }
  ]
}
```

Two named matrices:

- **`stable`** — the most recent combination of released versions that has
  been tested end-to-end. New users getting "the eco" install these versions.
- **`next`** — the bleeding edge: the latest commit on each repo's `main`
  branch. CI runs the integration tests against this matrix on every PR to
  any eco repo.

Additional matrices (e.g. `lts`, `enterprise`) can be added without breaking
existing consumers.

## When to update

- **`next` updates automatically** — release-please bumps `main` references
  as components advance. No manual edit needed.
- **`stable` is updated by hand** — when you cut a coordinated release of
  the eco. The release engineer:
  1. Verifies CI green on `next` matrix.
  2. Tags each component repo at the version that's about to be in `stable`.
  3. Updates `stable` in this file with those versions, in a single PR.
  4. Releases.

## CI integration

`.shared-templates/workflows/compatibility-test.yml.tmpl` is a reusable
workflow that:

1. Reads `testdata/compatibility-matrix.json` from the hawk repo.
2. Checks out each component at the version listed in the named matrix.
3. Builds + tests the cross-repo integration scenarios.

It runs on:
- Push to `main` of any eco repo.
- A nightly schedule.
- Manual dispatch (e.g. before cutting a `stable` release).

## Why bother

- **Bug reports become triageable.** "I ran hawk 0.4 with eyrie 0.2" — you
  immediately know whether that combination was ever tested.
- **Consumers can pin reliably.** Downstream projects that vendor multiple
  eco packages can pin to a known-good stable matrix instead of guessing.
- **Coordinated releases are a real thing.** When you want to ship "the
  eco v1.0", you have a definition of what that means.
- **Drift is visible.** A component that's never updated in `next` but is
  fine in `stable` shows up as a drift candidate in CI reports.

## Validating the file

The file is validated against [`testdata/compatibility-matrix.schema.json`](./testdata/compatibility-matrix.schema.json)
in CI. To validate locally:

```bash
make compat-check   # structural validation + version pins

# Or with ajv (Node)
npx ajv-cli validate \
  -s testdata/compatibility-matrix.schema.json \
  -d testdata/compatibility-matrix.json
```
