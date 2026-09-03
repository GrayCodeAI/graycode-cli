# MiniMax-AI → graycode Adoption Plan

Status: Implemented in working tree; sibling-repository PRs and skills curation remain follow-ups
Date: 2026-08-21
Scope: Adopt the high-value, verified concepts from MiniMax-AI's open-source
repos into the Graycode ecosystem (Graycode plus independent repositories: Eyrie, Eagle,
Falcon, Kestrel, and Merlin).

## Findings summary

Deep code review of 6 MiniMax-AI repos against Graycode's existing sibling
repositories and
internals produced these adoptions, ordered by value:

| # | MiniMax repo | Adopt into | Action |
|---|---|---|---|
| 1 | MiniMax-Provider-Verifier | sibling `eyrie` | Add provider-conformance metrics + wire the orphaned `verify` harness into CI |
| 2 | MiniMax/skills | `starling` | Curate high-quality skill content (content, not mechanism) |
| 3 | MiniMax-Coding-Plan-MCP | graycode / `falcon` | MCP v2 no-network test pattern + image-source normalization (patterns only) |
| 4 | minimax_search | graycode tooling | Jina page→Markdown browse extractor + evidence-extraction prompt (pattern) |
| 5 | Mini-Agent | graycode engine | Cooperative-cancellation-with-cleanup + token-aware lossy summarization (patterns) |
| 6 | MiniMax-MCP / JS | — | Not adoptable (media generation, out of scope) |

## 1. Eyrie provider-conformance metrics + CI wiring (highest value)

### Verified current state (eyrie repository)

- `eyrie/verify/verify.go` — a complete behavioral conformance harness
  exists: `Case`/`CaseResult`/`Expectation`/`Report`, `Run()`, `scoreResponse()`,
  `ToolCallF1()`, `DiffBaseline()`, `Markdown()`.
- `eyrie/verify/cases.go` — `CanonicalCases()` with 3 cases:
  `basic-chat`, `deterministic-answer`, `tool-call`.
- `eyrie/verify/metrics.go` — `ToolCallF1()` already implemented.
- **Confirmed gap:** the `verify` package is referenced ONLY by its own tests.
  It is not wired into any CLI, Makefile target, CI job, or provider-registration
  gate. Only structural registry/parity tests are enforced.
- eyrie already ships MiniMax providers (`minimax_token_plan`, `minimax_payg`)
  as OpenAI-compatible adapters with `ThinkingFormat: "minimax"`.
- `client/structured.go` already has `ValidateStructuredOutput()` — a recursive
  JSON-schema validator (`validateValue`, `validateObject`, `validateArray`)
  reusable for tool-call argument schema checks.

### What to implement

**A. Add a `SchemaValidate` helper (argument-level) — new file
`eyrie/verify/schema.go`**

Add `SchemaValidate(args map[string]any, schema map[string]any) error` that
checks tool-call arguments against the tool's `Parameters` JSON schema:
- required-arg presence
- property type checks (reuse pattern from `client.ValidateStructuredOutput`)

This is the `ToolCalls-Schema-Accuracy` dimension from MiniMax-Provider-Verifier.

**B. Add `MatchRate` to `Report` — edit `eyrie/verify/verify.go`**

Compute the `ToolCalls-Match-Rate` (trigger-vs-stop correctness) alongside F1.
Extend `CaseResult` already has `ExpectedTool`/`CalledAnyTool`/`CorrectTool`, so
compute:
```
tool_calls_match_rate = (TP + TN) / expected_tool_call_total_count
```
mirroring MiniMax's confusion-matrix metric.

**C. Add a `verify` CLI entrypoint — new file
`eyrie/cmd/verify/main.go`**

A small `go run`-able binary (or a `verify` subcommand if eyrie has a cmd/) that
runs `verify.Run` against a provider endpoint (live or cassette) and exits
non-zero on unmet thresholds:
- `--model`, `--base-url`, `--api-key`, `--provider`
- `--threshold-score` (default e.g. 0.98)
- prints the `Report.Markdown()` and `DiffBaseline()` against a stored baseline.

This makes the orphaned harness operational and enables CI gating.

**D. Add a Makefile target + CI job — edit `eyrie/Makefile` and
`eyrie/.github/workflows/ci.yml`**

- Makefile: `verify` target that runs the harness in cassette mode (no tokens)
  and `verify-live` for manual live runs.
- CI: a `verify` job that runs the cassette-replay conformance test as a gate
  on PRs, ensuring the harness is exercised and providers don't regress.

### Files
- `eyrie/verify/schema.go` (new)
- `eyrie/verify/schema_test.go` (new)
- `eyrie/verify/verify.go` (add MatchRate)
- `eyrie/verify/verify_test.go` (add tests)
- `eyrie/cmd/verify/main.go` (new)
- `eyrie/Makefile` (verify target)
- `eyrie/.github/workflows/ci.yml` (verify job)

## 2. Curate MiniMax/skills content into starling

### Verified current state (graycode)
- graycode's skills system (`internal/plugin`) reads markdown+YAML-frontmatter skills
  from dirs (`~/.graycode/skills`, `.claude/skills`, `.zero/skills`, `skills`) with a
  registry (`starling` repo, `graycode skills search/install/list/remove`).
- MiniMax/skills (13.4k★) has 18 high-quality skills in the same format
  (frontmatter `name`/`description`/`license`/`metadata`), especially
  `frontend-dev`, `fullstack-dev`, `shader-dev`, mobile guides, `vision-analysis`.

### What to implement
- Port the best MiniMax skills into `GrayCodeAI/starling` (separate
  repo), adapting frontmatter to graycode's convention (`globs`, `alwaysApply`).
- This is a content curation task in a separate repo; tracked here for
  completeness but implemented as a follow-up PR in `starling`.

### Files
- `starling/registry.json` (add entries)
- `starling/skills/<name>/SKILL.md` (port content)

## 3. MCP v2 no-network test pattern + image-source normalization

### Verified current state (graycode)
- `internal/mcp` implements its own JSON-RPC client + server and does NOT use the
  shared `falcon` scaffolding (architectural divergence).
- graycode has `internal/attachment/image.go` for image decode, and `ScreenshotTool`
  / `BrowserTool`. No image-source (URL/file/data-URL) normalization helper.
- MiniMax-Coding-Plan-MCP shows: MCP v2 tool registration + no-network test
  harness (in-process `Client` + stdio `ClientSession` asserting exact payloads)
  and image-source normalization.

### What to implement (patterns only — no vendor-locked Python)
- Add a Go helper `internal/attachment/normalize_image_source.go` that converts
  HTTP URL / local path / data-URL / base64 into a data URL (`@`-prefix strip),
  mirroring MiniMax's `process_image_url`. Add unit tests.
- Add an MCP no-network integration test pattern to `internal/mcp` or
  `falcon` demonstrating in-process payload assertion without network.

### Files
- `internal/attachment/normalize_image_source.go` (new)
- `internal/attachment/normalize_image_source_test.go` (new)

## 4. Jina page→Markdown browse extractor

### Verified current state (graycode)
- graycode has `WebSearchTool` (6-provider cascade: Brave/SearXNG/DeepSeek/Exa/
  Perplexity/DDG), `AgenticFetchTool`, `WebFetchTool`, `DownloadTool`,
  `engine/search/url_scraper.go`.
- No lightweight "URL → Markdown" extractor (Jina Reader pattern).

### What to implement (pattern only)
- Add `internal/search/jina.go`: `FetchAsMarkdown(ctx, url)` calling
  `POST https://r.jina.ai/` with `X-Return-Format: markdown`, `X-Timeout`,
  `X-Engine: direct`, returning clean Markdown.
- Optional: an evidence-extraction prompt helper (chunk-then-merge) for
  long-content answering.
- Guarded behind config so it's off unless enabled (Jina key optional).

### Files
- `internal/search/jina.go` (new)
- `internal/search/jina_test.go` (new, with mock HTTP server)

## 5. Agent-loop robustness patterns (reference)

### Verified current state (graycode)
- graycode's `Session.agentLoop` (`internal/engine/stream.go:133`) is a complete
  think→act→observe loop with memory, 70+ tools, sub-agents, multi-agent.
- No cooperative-cancellation-with-history-cleanup, and long-session compaction
  uses truncation rather than "summarize between user turns".

### What to implement (small, low-risk)
- Add `_cleanup_incomplete_messages` equivalent to graycode's loop: on cancellation
  at a safe checkpoint, trim the partial assistant message + orphaned tool
  results so message history stays valid.
- Add a token-limit-driven lossy summarization option (keep user intents,
  collapse interleaved tool/assistant turns into an LLM summary) as a session
  compaction strategy, with a consecutive-trigger guard.

### Files (tentative)
- `internal/engine/stream.go` (cancellation cleanup)
- `internal/engine/compact/` (lossy summarize strategy)

## Out of scope (not adopted)
- MiniMax-MCP / MiniMax-MCP-JS: media generation (TTS/image/video) — no
  relevance to code intelligence; graycode has no TTS and that is a product decision.
- minimax_search's search layer: redundant (graycode has more providers).
- Mini-Agent's core loop: outclassed by graycode.
- The `verify` harness's live-token path: optional, off by default.

## Execution order
1. Eyrie verify metrics + schema validation + tests (highest value)
2. Eyrie verify CLI + Makefile + CI wiring
3. graycode image-source normalization helper + tests
4. graycode Jina browse extractor + tests
5. Agent-loop cancellation/summarization patterns
6. starling content curation (follow-up PR in separate repo)

## Implementation status

- [x] Eyrie schema-accuracy validation and tool-call match-rate metrics.
- [x] Eyrie verification CLI, deterministic Makefile target, and CI job.
- [x] Graycode image-source normalization for data URIs, URLs, local files, and raw base64.
- [x] Graycode Jina Reader page-to-Markdown client with opt-in configuration and tests.
- [x] Graycode cancellation cleanup for incomplete assistant tool-use/tool-result turns.
- [x] Confirmed graycode's existing `internal/engine/compact` already provides token-triggered
  compaction; no duplicate summarizer was added.
- [ ] Publish the Eyrie repository changes through its own feature branch and PR.
- [ ] Curate MiniMax skills into `starling` through its own feature branch and PR.
