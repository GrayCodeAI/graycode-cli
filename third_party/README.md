# third_party — committed Go module proxy for deleted ecosystem repos

## What

`modproxy/` is a file-based Go module proxy
(`GOPROXY=file://<repo>/third_party/modproxy,...`) holding the exact pinned
versions of GrayCodeAI modules whose GitHub repositories no longer exist and
which the public proxy never cached (their original fetches went direct-VCS):

| Module | Pinned version | Why kept |
|---|---|---|
| `github.com/GrayCodeAI/harrier` | v0.0.0-20260902154449-d52fa214feb7 | memory engine imported by `internal/intelligence/memory` |
| `github.com/GrayCodeAI/kestrel` | v0.0.0-20260902154440-1b4c8cf7ea62 | review engine imported by `cmd`, `internal/bridge/kestrel` |
| `github.com/GrayCodeAI/merlin` | v0.0.0-20260902154444-0f4b9f7326cb | audit engine imported by `cmd`, `internal/bridge/merlin` |
| `github.com/GrayCodeAI/shrike` | v0.0.0-20260902154002-4465cf58fe59 | tokenizer/compress imported by `internal/token`, `internal/lsp`, engine |
| `github.com/GrayCodeAI/swift` | v0.0.0-20260902154454-07d895ebce4d | session CLI mounted by `cmd/swift.go` |

Each entry is the standard proxy layout (`<module>/@v/<version>.{info,mod,zip}`),
byte-identical to what the module cache fetched before the repos were removed.
`go.sum` already pins these hashes; `go mod verify` confirms integrity.

`eagle` is intentionally absent: its contracts were vendored into
`internal/contracts/` instead (small, stdlib-only DTOs).

## How it is wired

- `.github/workflows/ci.yml` sets
  `GOPROXY=file://${{ github.workspace }}/third_party/modproxy,https://proxy.golang.org,direct`
  (committed proxy first, public proxy unchanged, direct fallback retained).
- `Dockerfile` / `Dockerfile.daemon` `COPY` this directory and set the same
  `GOPROXY` against the in-image path.

## When to remove

If (any of) these repositories are restored under `GrayCodeAI/` with the same
module paths, or the integrations are dropped: delete the corresponding
`@v` files, drop this directory once empty, and revert the `GOPROXY`
overrides to the default. Do NOT add new modules here for convenience —
this directory exists only for unresolvable pins.

## Regenerating

From a machine whose module cache holds the version (proxy paths escape
capitals as `!` + lowercase, e.g. `!gray!code!a!i`):

```bash
m=harrier v=v0.0.0-20260902154449-d52fa214feb7   # example pin
src=~/go/pkg/mod/cache/download/github.com/'!gray!code!a!i'/$m/@v
d="third_party/modproxy/github.com/!gray!code!a!i/$m/@v"
mkdir -p "$d"
cp "$src/$v".{info,mod,zip} "$d/"
GOPROXY="file://$PWD/third_party/modproxy,off" GOSUMDB=off \
  go mod download "github.com/GrayCodeAI/$m@$v"
```
