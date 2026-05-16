# release-please

We use [release-please](https://github.com/googleapis/release-please) for all
release automation in the hawk-eco. Each repo has three files:

| File | What it does |
|------|--------------|
| `release-please-config.json` | Per-repo settings (release-type, changelog sections, extra files to bump). |
| `.release-please-manifest.json` | The current version (kept in sync with `VERSION`). |
| `.github/workflows/release-please.yml` | Opens release PRs on every push to `main`. |

How a release flows:

```
conventional commits → push to main → release-please opens "release PR"
                                       (bumps VERSION, updates CHANGELOG, drafts release notes)
                                       ↓
                                       merge release PR → release-please pushes a git tag
                                                          ↓
                                                          goreleaser workflow runs on tag
                                                          (builds binaries, publishes release)
```

The two important things this gives us:

1. **`VERSION` is bumped automatically** via `extra-files` in
   `release-please-config.json`. Conventional commits drive the bump
   (`feat:` → minor, `fix:` → patch, `feat!:` / `BREAKING CHANGE:` → major).
2. **`CHANGELOG.md` is generated**, not hand-edited. Don't edit it manually
   between releases — your change will be overwritten.

## Templates

Canonical templates live in `.shared-templates/`:

- `release-please-config.json.tmpl`
- `.release-please-manifest.json.tmpl`
- `release-please.yml.tmpl`

Per-repo placeholders:

- `${PROJECT}` — repo short name (e.g. `hawk`, `tok`).
- `${RELEASE_TYPE}` — `go`, `python`, or `node`.
- `${EXTRA_FILES}` — JSON array of additional files to bump (e.g. Homebrew
  Formula).

## graycode-core

Excluded from this scheme. As a TypeScript monorepo with multiple publishable
packages, it should use [changesets](https://github.com/changesets/changesets)
instead, which has first-class support for monorepo versioning. (Not in scope
for this sweep.)
