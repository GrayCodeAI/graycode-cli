# hawk/engine sub-package split — analysis and migration plan

> Status: **analysis only**. No code is moved by this document. The actual
> split is multi-PR work that should land incrementally to keep hawk's
> build green at every step.

## The problem

```
hawk/engine/
├── *.go         161 source files,    66,682 lines
└── *_test.go    141 test files,      65,907 lines
                 ───── total: ~133K lines, 302 files, ONE package
```

For comparison: the entire `kubernetes/kubectl` (a non-trivial CLI) is
~110K lines split across **dozens** of internal packages. hawk's engine
is bigger than that and lives in a single `package engine`.

Concrete pain that creates today:

- **Slow IDE/LSP indexing.** `gopls` re-parses the whole package on any
  edit; round-trip latency for autocomplete is measurable.
- **Test runtime.** `go test ./engine/...` is one invocation; a single
  flaky test in a far-away concern slows the whole package.
- **Implicit coupling.** Any function in any of 161 files can call any
  unexported helper in any other. There's no compiler enforcement of
  module boundaries — only convention.
- **Cognitive load.** A new contributor has 161 files in one directory
  to file-find through, named in inconsistent conventions
  (`adaptive_prompt.go` vs `prompt_optimizer.go` vs `efficient_prompt.go`).
- **Test discoverability.** `engine/error_context_test.go` tests
  `error_context.go`, but is it relevant to `error_grouper.go`? No way
  to know without grep.

## Proposed sub-package layout

After clustering filenames + spot-reading representative files, I propose
splitting into ~15 sub-packages. The line counts are file-name-based
estimates; the real numbers will shift during the split.

```
hawk/engine/
├── engine.go                  # top-level Engine type, public API only
├── lifecycle.go               # session start/stop, hooks, graceful shutdown
├── stream.go                  # the main response stream loop (large; consider further split)
│
├── prompt/                    # prompt construction & optimisation  (~5 files)
│   ├── adaptive.go
│   ├── compact.go
│   ├── efficient.go
│   ├── tuner.go
│   └── optimizer.go
│
├── compact/                   # context compaction strategies       (~8 files)
│   ├── files.go
│   ├── micro.go
│   ├── auto.go
│   ├── api.go
│   ├── prompt.go
│   ├── session_memory.go
│   ├── split.go
│   └── strategy.go
│
├── context/                   # context budgeting & assembly        (~6 files)
│   ├── budget.go
│   ├── decay.go
│   ├── packer.go
│   ├── providers.go
│   ├── viz.go
│   └── readonly.go
│
├── token/                     # token counting & budget allocation  (~3 files)
│   ├── budget.go
│   ├── predictor.go
│   └── reporter.go
│
├── cost/                      # cost tracking & optimisation        (~5 files)
│   ├── tracker.go
│   ├── optimizer.go
│   ├── display.go
│   ├── table.go
│   └── budget.go
│
├── diff/                      # diff handling, sandbox, summary     (~6 files)
│   ├── sandbox.go
│   ├── staging.go
│   ├── preview.go
│   ├── summarizer.go
│   ├── test_selector.go
│   └── diff3.go
│
├── docs/                      # docgen, external docs, magic docs   (~3 files)
│   ├── docgen.go
│   ├── external.go
│   └── updater.go
│
├── error/                     # error handling, recovery, learning  (~5 files)
│   ├── context.go
│   ├── grouper.go
│   ├── learning.go
│   ├── patterns.go
│   └── recovery.go
│
├── retry/                     # smart retry + queue                 (~2 files)
│   ├── smart.go
│   └── queue.go
│
├── session/                   # session services, timeline, compress (~4 files)
│   ├── services.go
│   ├── timeline.go
│   ├── compressor.go
│   └── cross.go
│
├── workflow/                  # workflow + workspace + trajectory   (~5 files)
│   ├── workflow.go
│   ├── workspace_state.go
│   ├── workspace_diff_report.go
│   ├── trajectory.go
│   └── trajectory_inspector.go
│
├── review/                    # critic, self-assess, consensus, etc (~7 files)
│   ├── bot.go
│   ├── critic.go
│   ├── self_assessment.go
│   ├── self_review.go
│   ├── consensus.go
│   ├── quality_scorer.go
│   └── solution_reviewer.go
│
├── scaffold/                  # scaffolding, recipes, patterns      (~5 files)
│   ├── scaffold.go
│   ├── recipe.go
│   ├── patterns.go
│   ├── skills.go
│   └── fewshot.go
│
├── code/                      # code-aware features                 (~4 files)
│   ├── context.go
│   ├── lens.go
│   ├── actions.go
│   └── explainer.go
│
├── git/                       # git provider + context              (~2 files)
│   ├── provider.go
│   └── context.go
│
├── memory/                    # knowledge + experience consolidation (~3 files)
│   ├── knowledge.go
│   ├── experience.go
│   └── consolidator.go
│
├── search/                    # url_scraper, web search, issue search (~3 files)
│   ├── scraper.go
│   ├── issues.go
│   └── research.go
│
├── validation/                # generated-code validation           (~4 files)
│   ├── gen.go
│   ├── schema.go
│   ├── test_loop.go
│   └── lint_loop.go
│
├── streaming/                 # response cache, formatter, stream optimiser (~5 files)
│   ├── cache.go
│   ├── formatter.go
│   ├── optimizer.go
│   ├── thinking.go
│   └── steering.go
│
├── agent/                     # agent / persona / subagent          (~4 files)
│   ├── agent.go
│   ├── background.go
│   ├── subagent_budget.go
│   └── subagent_synthesis.go
│
├── control/                   # loop detection, stall, backtrack    (~3 files)
│   ├── loop_detect.go
│   ├── stall_detector.go
│   └── backtrack.go
│
└── io/                        # clipboard, notify, watch, cron      (~4 files)
    ├── clipboard.go
    ├── ai_watch.go
    ├── filewatcher.go
    └── cron_scheduler.go
```

Plus ~63 files currently in **`misc`** that need file-by-file triage —
some will fit existing buckets after reading, some may justify a new
sub-package, a few are likely candidates for outright deletion (dead
code from earlier experiments).

## Migration strategy

The split is high-risk because it touches every other package in hawk
that imports `engine.Foo`. To keep hawk green at every commit:

1. **Stage 1 — alias-only.** For each proposed sub-package, create
   `engine/<subpkg>/<file>.go` containing only re-exports:
   ```go
   package compact
   import "github.com/GrayCodeAI/hawk/engine"
   type Strategy = engine.CompactStrategy
   var Default = engine.DefaultCompactStrategy
   ```
   Hawk's external callers can start migrating to the new import paths.
   Old code keeps working unchanged. **Land this first.**

2. **Stage 2 — move bodies.** For one sub-package at a time:
   - Move the implementation files into the sub-directory.
   - Update the package declaration in each moved file.
   - Replace the alias re-exports with real definitions.
   - Add `internal` types as needed within the sub-package.
   - Move the matching `_test.go` files alongside.
   - Update `engine.go` to re-export the public surface as type aliases
     so external `engine.Foo` callers keep compiling.
   - Run full test suite. Land as a single PR per sub-package.

3. **Stage 3 — purge re-exports.** After all sub-packages are moved
   and external callers are updated to use the new paths, remove the
   re-export aliases from `engine.go`. Now `engine` is a thin
   coordinator package only.

This approach scales: each PR is small, reviewable, and individually
revertible. No "big bang" merge that paralyses hawk for a week.

## Estimated effort

- Stage 1 (alias scaffolding): **0.5 day** — mechanical, low-risk.
- Stage 2 (per-sub-package moves): **1 day per cluster × ~15 clusters
  = ~15 working days**. Several can land in parallel.
- Stage 3 (cleanup): **1 day**.

Total: **~3 weeks of focused work**, spread across as many engineers as
work in parallel without merge conflicts.

## What I did NOT do

- I did **not** move any files. The engine package is unchanged.
- I did **not** read every file's contents — clusters above are based on
  filenames and spot-reads. Real grouping will adjust slightly.
- I did **not** identify dead code. A separate pass with
  `unused-funcs` / `staticcheck SA1019` would find candidates for
  deletion before splitting (deleting dead code first reduces the
  size of the move).

## Suggested first PR (smallest valuable step)

The `compact/` sub-package is the cleanest extraction candidate:

- 8 files, all named `compact_*.go`
- Self-contained logic (compaction strategies for context window)
- Few external dependencies (mostly used by `engine.Stream`)
- Has its own tests already grouped together

Recommend doing `compact/` first end-to-end (Stage 1 + Stage 2 for that
cluster). If it goes smoothly, the same template applies to the other 14.
If it surfaces problems (e.g. unexpected coupling), the split plan can
be adjusted before committing to all of it.
