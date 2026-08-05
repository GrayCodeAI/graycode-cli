# graycode-eco coverage thresholds

Each repo's CI enforces its own minimum test-coverage percentage, hardcoded
into that repo's `.github/workflows/ci.yml` (`THRESHOLD=` for the `go-ci.yml`
template, `--cov-fail-under=` for `python-ci.yml`). There is no automation
that keeps this table and each repo's CI in sync — **when you change a
repo's threshold, update both the CI file and this table in the same PR.**

| Repo | Threshold | Mechanism |
|---|---|---|
| `hawk` | 60% | inline `bc` check in `ci.yml` |
| `eyrie` | 60% | inline `bc` check in `ci.yml` |
| `yaad` | 49% | `THRESHOLD=` in `ci.yml` (go-ci.yml.tmpl) |
| `tok` | 38% | `THRESHOLD=` in `ci.yml` (go-ci.yml.tmpl) |
| `trace` | 58% | `THRESHOLD=` in `ci.yml` (go-ci.yml.tmpl) |
| `sight` | 74% | `THRESHOLD=` in `ci.yml` (go-ci.yml.tmpl) |
| `inspect` | 76% | `THRESHOLD=` in `ci.yml` (go-ci.yml.tmpl) |
| `hawk-sdk-go` | 80% | `THRESHOLD=` in `ci.yml` (go-ci.yml.tmpl) |
| `hawk-sdk-python` | 78% | `--cov-fail-under=` in `ci.yml` (python-ci.yml.tmpl) |
| `hawk-core-contracts` | none enforced | leaf library; add one before it grows past a handful of files |
| `hawk-mcpkit` | none enforced | leaf library; add one before it grows past a handful of files |
| `hawk-community-skills` | n/a | no Go/Python test suite (skill/content registry) |

## Why thresholds differ per repo

These are *floors*, not targets — set near each repo's actual coverage at
the time CI was set up, then only ever raised (never silently lowered) as
the repo's test suite grows. A wide range (38% in `tok` vs 80% in
`hawk-sdk-go`) reflects real differences in how much of each repo is
exercised by unit tests vs. requiring live provider credentials
(`test-live` targets) or manual verification — it is not an oversight.

## Adding a threshold to a repo that has none

1. Run `make cover` locally, note the current `total:` percentage.
2. Set the CI threshold at or slightly below that number — never above it,
   or the very next PR fails CI without having regressed anything.
3. Add the row to this table in the same PR.
