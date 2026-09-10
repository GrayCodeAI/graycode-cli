# Benchmarks

Published, reproducible CPU benchmarks for graycode-cli. These are TUI-independent
measurements (no latency theater) run with existing `go test -bench` targets; they
exist so performance is documented and regressions are visible, not to make
marketing claims.

## Method

- Runner: `go test -bench=. -benchmem -count=3` on the specific package.
- Every table reports the median of 3 runs.
- Repo map benchmark uses a synthetic 100-file Go tree (see
  `internal/intelligence/repomap/benchmark_test.go`).
- No benchmark gates releases; this is a reference snapshot.

## Hardware & commit

| Field | Value |
|---|---|
| Commit | `b157aaa4` (branch `feat/competitive-analysis-top20`, working tree) |
| Go | `go1.26.6 linux/amd64` |
| CPU | AMD EPYC 7543P 32-Core Processor |
| OS | Linux (cloud amd64) |

## Session save / load

Package: `internal/session` — `BenchmarkSessionSave_*` / `BenchmarkSessionLoad_*`.

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| Save 100 messages | ~183 µs | 32,488 | 256 |
| Save 1000 messages | ~790 µs | 255,818 | 2,056 |
| Load 100 messages | ~2.0 ms | 16,883,848 | 1,067 |
| Load 1000 messages | ~5.3 ms | 17,721,376 | 10,070 |

Saving is O(n) and cheap; loading allocates heavily (full JSON decode into the
session graph), which is the expected cost of materializing a full session.

## Repo map generation (size / tokens)

Package: `internal/intelligence/repomap` — `BenchmarkRepoMapGenerate` over a
synthetic 100-file tree with 2,500 functions/types.

| Metric | Value |
|---|---|
| ns/op | ~2.2 ms |
| est_tokens | 21,600 |
| format_bytes | 12,451 |
| B/op | ~364 KB |
| allocs/op | ~2,425 |

The token estimate is the model-facing budget (`RepoMap.TokenEst`); the format
bytes are the rendered map text truncated to a 2,000-token budget.

## Reproducing

```bash
# Session save/load
go test ./internal/session/ -bench 'BenchmarkSessionSave|BenchmarkSessionLoad' -benchmem -count=3

# Repo map size/tokens
go test ./internal/intelligence/repomap/ -bench 'BenchmarkRepoMap' -benchmem -count=3

# Everything (slow; runs the whole repo)
make bench
```

Record the machine + commit alongside any new numbers before comparing.
