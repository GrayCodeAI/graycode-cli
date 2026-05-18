# Versioning Standard — GrayCodeAI repositories

This document describes the versioning convention used across every GrayCodeAI
repository that follows this layout. Adopted 2026-05-14.

## TL;DR

- Every repo has a `VERSION` file at the root (plain text, e.g. `0.2.0`).
- `VERSION` is the single source of truth — everything else (code, build
  tooling, release tooling, CI, package metadata) reads from it.
- Versions are independent per repo, following [SemVer](https://semver.org).

## Pattern by repo type

### Go binaries (`hawk`, `yaad`, `trace`)

- `VERSION` at the repo root.
- A version package (`internal/version` for binaries, or `main` itself) declares
  three settable variables defaulting to `"dev"` / `"none"` / `"unknown"`:
  ```go
  var (
      Version = "dev"   // overridden by ldflags at release time
      Commit  = "none"
      Date    = "unknown"
  )
  ```
- The `Makefile` reads `VERSION` and injects all three via ldflags:
  ```make
  VERSION := $(shell cat VERSION 2>/dev/null | tr -d '[:space:]' || echo "dev")
  COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
  DATE    := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
  LDFLAGS := -s -w \
      -X main.Version=$(VERSION) \
      -X main.Commit=$(COMMIT) \
      -X main.BuildDate=$(DATE)
  ```
- `goreleaser` injects from the git tag at release time (which always matches
  the `VERSION` file because release-please bumps both atomically).
- This is the Kubernetes / Helm / gh-cli pattern.

### Go libraries (`hawk-sdk-go`, `eyrie`, `sight`, `inspect`, `tok`)

- `VERSION` at the repo root.
- A `version.go` file co-located with `VERSION` uses `//go:embed` to read it at
  compile time:
  ```go
  package mylib

  import (
      _ "embed"
      "strings"
  )

  //go:embed VERSION
  var versionFile string

  var Version = strings.TrimSpace(versionFile)
  ```
- This is the AWS SDK / restic / hashicorp pattern. It works without ldflags,
  so consumers who `go install` or `go get` get the correct version
  automatically.

#### Sub-packages that need the version

When an internal sub-package (e.g. `internal/output`, `internal/report`) needs
the version too, it declares its own `var ToolVersion = "dev"` plus a
`SetToolVersion(v string)` setter. The parent package's `init()` propagates
the canonical `Version` into the sub-package, avoiding an import cycle:

```go
// in <root>/version.go
import "github.com/<org>/<repo>/internal/output"

func init() {
    output.SetToolVersion(Version)
}
```

### Python (`hawk-sdk-python`)

- `VERSION` at the repo root.
- `pyproject.toml` uses `dynamic = ["version"]` with hatch reading from `VERSION`:
  ```toml
  [project]
  dynamic = ["version"]

  [tool.hatch.version]
  source = "regex"
  path = "VERSION"
  pattern = "^(?P<version>[^\\s]+)"

  [tool.hatch.build.targets.wheel]
  force-include = { "VERSION" = "hawk/VERSION" }
  ```
- `_version.py` reads the same `VERSION` file at runtime via `pathlib`, so
  `__version__` matches the package metadata both in source checkouts and in
  installed wheels (where `VERSION` ships as package data).

## What's banned

- Hardcoding a version in source code (e.g. `const Version = "0.2.0"`). Always
  read from `VERSION` or a build-time injection.
- Maintaining a version string in two places (e.g. `pyproject.toml` AND
  `_version.py`) — they drift, always.
- Embedding hardcoded version literals in tests. Compare against the package
  variable instead: `if got != mypkg.Version { ... }`.

## Bumping a version

Use [release-please](https://github.com/googleapis/release-please) (per repo).
It reads conventional commits, bumps `VERSION`, updates `CHANGELOG.md`, and
creates a git tag — all atomically. Goreleaser then runs on the tag.

For manual bumps, edit the `VERSION` file. Don't edit anything else.

## Cross-repo compatibility

Versions are independent per repo. A future `compatibility-matrix.json` (at
the eco root) will record which combinations are tested together (similar to
the AWS SDK / Hashicorp release matrices).
