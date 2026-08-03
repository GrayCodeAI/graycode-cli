# hawk-eco Code Audit Report

**Branch:** `feat/code-audit-improvements`
**Base:** `bfd5654` (main)
**Date:** 2026-08-03
**Scope:** `internal/` (~387K lines Go, 1,820 files), `cmd/` (372 files), `external/*` submodules (reference-only)
**Method:** automated tooling (golangci-lint, go vet, staticcheck, govulncheck, go test -race) + manual deep review of all critical paths + cross-checks against research literature and the 2026 competitor landscape. Every finding was verified against source; claims that could not be verified are marked.

---

## 1. Executive summary

hawk-eco is in unusually good health for a codebase of this size:

- **0** golangci-lint issues, **0** go vet issues, **0** reachable vulnerabilities (govulncheck), **1** trivial staticcheck finding
- Full test suite **passes**; engine packages average **~87% coverage**; sandbox/auth 74–78%
- The security architecture is genuinely strong where it matters most: fail-closed Docker-only execution, cap-drop/no-new-privileges/read-only containers, keychain credential storage, constant-time daemon auth, atomic session persistence

However, the audit found **1 critical, 12 high, ~20 medium, and ~30 low** findings. The dominant themes:

1. **Built-but-unwired safety infrastructure** — panic recovery exists but is never installed; the self-improvement memory loop never persists; budget tracking exists but is never fed.
2. **Dead subsystems shipping in production** — `engine/async` (0% coverage, 2 confirmed bugs), `engine/docs` (~2,000 lines, zero importers), `MessageBus` (700 lines), approval gate, composio stub.
3. **Fail-open trust edges** — project-controlled `.agents/runtime.jsonc` executes arbitrary shell as root at image build time; HTTP decision hooks fail open silently; bash subprocesses inherit API-key env vars.
4. **Performance regressions in hot paths** — O(N) re-embedding per codegraph query, full-transcript deep clone per turn, full-prefix TUI re-render per chunk, per-call regexp compilation.

---

## 2. Baseline (Phase 1) results

| Tool | Result |
|---|---|
| `golangci-lint run ./internal/... ./cmd/...` | **0 issues** |
| `go vet ./internal/... ./cmd/...` | **clean** |
| `govulncheck ./...` | **0 called vulnerabilities** (1 in a required module, not reachable) |
| `staticcheck` | 1 finding: unused `getKeys` in `internal/engine/code/coverage_extra_test.go:131` |
| `go test ./internal/... ./cmd/...` | **all pass** |
| Coverage (critical pkgs) | engine 61–97%, sandbox 74.7%, auth 77.5%; **`engine/async` 0%** |
| Code smells | 14 files with TODO/FIXME, 9 `panic(`, 5 `os.Exit`, 71 bare `go func(` |

---

## 3. Findings

Severity scale: **CRITICAL** (crash/data loss/RCE), **HIGH** (security boundary or functional break), **MEDIUM** (correctness/reliability/race), **LOW** (hygiene/performance).

### 3.1 CRITICAL

**C1. No panic recovery anywhere in the production binary**
- `cmd/hawk/main.go` — `Execute()` has no `recover()`. `cmd/errors.go:33` (`panicRecovery`) and `internal/crash/crash.go` are **dead code** — zero production callers (verified by grep).
- `internal/crash/crash.go:17-18` states explicitly: *"Do NOT call this from cmd/hawk yet — wiring into the binary entry point is a future wave."*
- **Impact:** any panic in a background goroutine (TUI render, spinner at `cmd/chat_tools.go:234`, tool execution) kills the process mid-session with no session save, no crash report, no cleanup.
- **Fix:** wrap `Execute()` in `panicRecovery(saveFn)` and install `crash.Install()` at startup. *(fixed: `panicRecovery` wraps the TUI execute path; `crash.Install()` wired in `cmd/hawk/main.go`)*

### 3.2 HIGH

**H1. `.agents/runtime.jsonc` → arbitrary root code execution at image build time** — `internal/sandbox/runtime_deps.go:14-67`, `container.go:154,236-238`
`runtime_extra_deps[]` becomes raw `RUN <shell>` layers in the sandbox image; `runtime_startup_env_vars` becomes `docker run -e KEY=VALUE`. The file is project-controlled and agent-writable. A malicious repo executes attacker shell as root during `docker build` (build network unrestricted, `--cap-drop` does not apply), and the result is baked into the session image — a persistent session backdoor.
**Fix:** allowlist validation (reject `curl|wget|nc|sh|bash|python` in deps; fixed key set for env; no `PATH`/`HOME`/`LD_PRELOAD`). *(fixed: blocklisted dep terms rejected with `slog.Warn`; env validated against a fixed key set)*

**H2. Project secrets readable + exfiltratable by default** — `container.go:143` (project rw mount), `mode.go:160-170` (`ModeAllowsNetwork`: workspace → network on), `bash.go:604-673`
Default mode mounts the whole project (incl. `.env`, credentials) rw into a container with **outbound network**. A compromised agent can exfiltrate project secrets. Strict mode denies network but is not the default. `NetworkProxy`/`BlockPrivateNetworks` exist but are never wired into production (only tests reference them).
**Fix:** make strict mode's network policy the default for workspace, or wire the blocklist; document the tradeoff.

**H3. HTTP decision hooks fail open silently** — `internal/hooks/http_hooks.go:45-88`, `decision.go:118-142`
Every failure path (marshal, request build, client error/3s timeout, non-2xx, decode, unknown action) returns `nil`, and `ExecuteDecisionHooks` treats nil as "no opinion, proceed". No logging on HTTP errors. A downed compliance/guardrail hook → every guarded tool call silently allowed.
**Fix:** return a deny decision + `slog.Warn` on error; make fail-open an explicit config option. *(fixed: deny + `slog.Warn` by default; `FailOpen` explicit opt-in)*

**H4. SSE generation >5 min permanently wedges the daemon** — `internal/daemon/daemon.go:215` (`WriteTimeout: 300s`), `streamSSE` `:721-777`
`WriteTimeout` is an absolute deadline; `streamSSE` ignores `fmt.Fprintf` errors (`_, _ =`) and only exits on `r.Context().Done()` or channel close — neither fires when the write deadline lapses. The handler never returns; the session stripe lock (`:580-582`) and global `concurrencySem` (`:551-557`) are held forever; with the default cap of 4, all subsequent `/v1/chat` requests 503 permanently. Agentic tasks routinely exceed 5 min.
**Fix:** exit the SSE loop on write error; use `http.ResponseController` for a per-write deadline that resets per flush. *(fixed: `writeSSE` reports failures, handler exits the loop; per-write deadline via `ResponseController`)*

**H5. External SIGINT/SIGTERM/SIGHUP bypass session save** — `cmd/chat_update.go` (no `tea.InterruptMsg`/`tea.QuitMsg` cases — verified absent), Bubble Tea v2 handles both and exits without `saveSession()`; SIGHUP unhandled (default kill).
`kill -TERM`, terminal close, or ssh drop mid-run → transcript lost, temp files left.
**Fix:** handle `tea.InterruptMsg`/`tea.QuitMsg` → run the same save path as the two-stage ctrl+c; install SIGHUP handler. *(fixed: `InterruptMsg`/`QuitMsg`/SIGHUP all route through the shared quit-save path)*

**H6. Self-improvement memory never persists (default CLI path)** — `internal/intelligence/memory/evolving.go:36-40` (`NewEvolvingMemory` never calls `Load`), `internal/engine/lifecycle/lifecycle_adapters.go:14-39` (adapter only calls `Learn`/`Retrieve`/`Format`, never `Save`)
Everything learned at session end is lost at process exit; `OnSessionStart` always returns empty guidelines. The Reflexion-style loop is a **no-op** in the shipped CLI.
**Fix:** `Load()` in constructor, `Save()` after `Learn` (debounced), test the round-trip. *(fixed: `Load` in constructor, atomic `Save` after `Learn`, round-trip test)*

**H7. Budget enforcement is split-brain** — `internal/engine/lifecycle/limits.go:14` (`MaxCostUSD` "default: from MaxBudgetUSD" — never implemented), `:86` (`IsExceeded` checks `MaxCostUSD` only), `RecordCost`/`RecordTokens` have **zero production callers** (verified by grep); production budget flows through `Session.SetMaxBudgetUSD` and enforcement at `stream.go:515`. `VibeLimits` sets `MaxCostUSD: 5.0` with `MaxBudgetUSD: 0` (limits.go:147-156) — inconsistent.
**Fix:** fallback `MaxCostUSD = MaxBudgetUSD` when unset; wire `RecordCost` into the stream cost accounting; make `VibeLimits` consistent. *(fixed: `MaxCostUSD` falls back to `MaxBudgetUSD`; accessors mutex-protected (M1); cost synced from the session cost accumulator)*

**H8. Codegraph semantic search is O(N) full re-embedding per query** — `internal/codegraph/embeddings_cgo.go:13-57`, `tool/codegraph.go:482`
Every `SemanticSearch`/`HybridSearch` `SELECT`s all nodes then recomputes `GenerateEmbedding(n)` per node (hash-based, uncached), then cosine-compares. On 100k-node repos this is seconds per tool call. The precomputed `CodeVectorStore` (`vector_store.go:122-239`) exists but is unused by `SemanticSearch` (dead duplication; itself brute-force O(N²) sort, no locks).
**Fix:** `CodeGraph.embeddingFor` memoizes embeddings in a bounded cache (200k entries, content-hash key covering every field `extractFeatures` reads; full reset when full — far cheaper than recomputing per query). `SemanticSearch` now goes through the cache; repeated queries and unchanged nodes skip recomputation. 3 new tests (memoization, content invalidation, bound). *(fixed)*

**H9. Mission retry loop is structurally broken; failures report success** — `internal/multiagent/mission.go:158,238-258`, `worker.go:206`, `graph.go:80-90`, `cmd/mission.go:134-136`
`feature.Branch` is deterministic (`hawk-mission/<id>/<feature>`); `git worktree add -b` fails on retry 2+ because attempt 1's branch survives worktree removal — every retry fails, branch leaks. `runFeatureSet`/`RunWaves` return `nil` unconditionally → `hawk mission` **exits 0 when all features fail** (CI sees green).
**Fix:** the retry loop now rewrites `feat.Branch` to `<base>/attempt-N` before every worker call (unique per attempt); `createWorktree` falls back to checking out an existing-but-unchecked-out branch (leaked branch or validation reuse); `removeWorktreeDetached` deletes the branch after removing the worktree (best-effort); `cmd/mission.go` returns an error — non-zero exit — when any feature failed, so CI no longer sees green on failure. *(fixed)*

**H10. `engine/async`: goroutine leak + double-loop + missing terminal event (dead code today)** — `internal/engine/async/engine.go:93-106`, `:49-56`, `event.go:146`, `engine.go:128-146`
`Stop()` cancels ctx but the loop is parked in `subQ.Next()` (`<-sq.notify`) → **leak on every stop**; `Start()` after `Stop()` spawns a second loop draining the same queue (double processing); on stream error `EventDone` is never emitted → consumers hang; `toAsyncEvent` has no default for `compact_start`/`blast_radius` events → zero-value garbage events; `ReplyTo` contract is unfulfilled; subscribers can't unsubscribe.
**Fix:** rewritten engine: `Stop()` cancels a loop ctx and joins via WaitGroup (bounded wait); `Start` after `Stop` spawns one fresh loop; single-threaded loop drains the queue via non-recursive `pop()` after each notify (no stack-growth, no parked-goroutine leak); `Cancel()` aborts the in-flight turn directly (a queued cancel could never be popped while the loop is blocked inside the turn's stream); `EventDone` is always emitted (success, stream error, or canceled turn) and forwarded to `ReplyTo`; unmapped events map to `EventInfo` preserving the raw type; `EventQueue.Unsubscribe` added; full-UUID event/submission IDs. **9 tests, 88.2% coverage (was 0%), race-clean.** *(fixed)*

**H11. `engine/docs`: ~2,000 lines shipping with zero importers** — `internal/engine/docs/` (docgen.go, doc_updater.go, external_docs.go)
Verified: no file outside the package references it. Within it: multi-line doc comments truncated to last line (doc_updater.go:350-368), `OldDoc` populated from *new* content (`:56,:87`), false-positive machine for capitalized words (`:522-539`), parser chokes on nested parens (`:330`), `ExternalDocs.Cache` never written (`external_docs.go:77`), methods of generic types dropped (docgen.go:938-951).
**Fix:** either wire to a `hawk docs` command or delete; at minimum fix the top-3 bugs. *(fixed: deleted — dead since f0aa8fd, no importers, six known bugs; recoverable from git history if ever wanted)*

**H12. Bash tool subprocesses inherit API-key env vars** — `internal/tool/task_tools.go:80`, `bash.go` (`exec.CommandContext` with no `cmd.Env` → full `os.Environ()`)
Guard regexes (bash.go:102-105) block obvious dump patterns but are trivially bypassed (`python3 -c "import os;print(os.environ['ANTHROPIC_API_KEY'])"`). Keys are readable by anything the agent runs.
**Fix:** strip provider key env vars (or pass a scrubbed env) when spawning agent commands. *(fixed: agent subprocesses spawn with scrubbed env — `internal/env/scrub.go` builds the allowlist once from `ScrubSet`)*

### 3.3 MEDIUM

| ID | Finding | Location |
|---|---|---|
| M1 | Data race on `LimitTracker.limits` accessors (read/write without mutex) while daemon/multiagent goroutines call `SetMaxTurns` concurrently | `internal/engine/lifecycle/limits.go:129-132` *(fixed: mutex-protected accessors, covered by `limits_test.go`)* |
| M2 | `ParseAndApplyMemoryOps` swallows all errors (nil bridge, discarded `bridge.Remember`, malformed JSON); 0% coverage; runs in background goroutine | `sleeptime_ops.go:25-35`, `stream.go:626` *(fixed: returns `error` — `ErrNoMemoryOps`, wrapped parse/remember errors via `errors.Join`, nil-bridge error; call site logs `slog.Warn`; 7 new tests)* |
| M3 | `SkillDistillerAdapter` returns nil on error — "not configured" and "failed" indistinguishable | `lifecycle_adapters.go:50-52,77-80` *(fixed: `Retrieve` logs `slog.Warn` with the `Search` error)* |
| M4 | Cost metrics use fabricated session IDs (`"session_"+UnixNano`) instead of the real session ID | `lifecycle.go:158-160` *(fixed: `OnSessionEnd` reads the real ID via `Session.SessionID()` — added in `execution_graph_observations.go`; nil-getter falls back to empty)* |
| M5 | `MissionApprovalGate` is dead code (zero production callers); workers auto-approve everything incl. arbitrary bash; `sessionApproved` map would race when wired | `multiagent/approval.go:110-144`, `worker.go:61-66` *(fixed: wired into production — `Config.ApprovalGate` consulted by the worker permission fn; `Check(ctx, toolName, summary)` classifies bash/network/web actions as risky; `sessionApproved` mutex-protected)* |
| M6 | Validation-worker cleanup regressed (cancellable ctx kills `git worktree remove` → permanent leak) | `multiagent/worker.go:144` *(fixed: detached-context cleanup + branch deletion)* |
| M7 | Oversized MCP response (>1MB scanner cap) silently kills the client connection; server child stays alive; no recovery | `internal/mcp/mcp.go:95,167-179` *(fixed: dead-server flag marks the connection on readLoop end, kills the child process, and `callWithTimeout` fails fast)* |
| M8 | `Composio.ExecuteTool` returns fake success (`Success: true` echoing params); agents would report unexecuted actions | `internal/composio/composio.go:147-177` *(fixed: package deleted — unwired stub, zero importers; provider implementations belong in `external/eyrie` per the architecture note; recoverable from git history)* |
| M9 | In-memory `Tracer` accumulates spans unboundedly (daemon lifetime); `Disable()` doesn't stop recording | `internal/observability/oteltrace/trace.go:45-59,112-116` *(fixed: `StartSpan` checks `enable`, buffer capped at 10k spans; dropped spans stay functional)* |
| M10 | `diffsandbox.absPath` is lexical-only; symlinked intermediate components escape the sandbox root | `internal/diffsandbox/sandbox.go:419-435` *(fixed: component-wise walk with `Lstat` — symlinks resolved and containment-re-checked against the resolved root, dangling symlinks rejected; covers macOS `/var → /private/var`)* |
| M11 | `PolicyManager` defaults to `DecisionAllow` — stated deny-by-default posture not reflected | `internal/sandbox/manager.go:48` *(fixed: default is `DecisionDeny`; project policy takes precedence with global filling gaps; reload no longer resets to allow)* |
| M12 | userns remap conditional; without it container runs as root with rw project mount; no `--user` fallback | `container.go:33-40,149-153` *(fixed: `--user <uid>:<gid>` appended when userns remap is unavailable)* |
| M13 | Host-side file tools: check-then-open symlink TOCTOU; name-based sensitivity (`secrets.txt` allowed) | `internal/tool/file_read.go`, `file_write.go`, `safety.go:251+` *(fixed: resolve-then-revalidate + `os.SameFile` fd guard; writes land at the resolved parent; `blockedBasenames` covers secrets/credential files; symlink-escape tests added)* |
| M14 | Per-call `regexp.MustCompile` in hot paths (5 sites) | `internal/feature/eval/filters.go:13-32`, `tool/spec_checklist.go:119-149`, `tool/ticket_compliance.go:62`, `feature/fingerprint/project_conventions.go:180-181` *(fixed: all hoisted to package-level vars)* |
| M15 | Full-transcript deep clone per access in `RawMessages()` — quadratic over session length | `internal/engine/persistence_service.go`, callers `context_governor.go:120-148` *(fixed: hot per-turn reads use the read-only, non-retaining `RawMessagesView()` (no clone); `RawMessages` keeps its deep-copy snapshot contract)* |
| M16 | TUI viewport re-renders full prefix per streamed chunk — O(messages) per token | `cmd/chat_viewport_render.go` *(no change needed: render cache + incremental stream tail already make per-chunk rendering amortized O(tail); incremental-vs-full-rebuild equivalence asserted by `chat_viewport_render_test.go`)* |
| M17 | `hawk path` 1.83s wall; `MigrateProviderSecrets`→`newEyrieEngine()`+`gateway.New()` runs on **every** root command | `cmd/root.go:136`, `internal/config/eyrie_engine.go:15-17,127-133` *(fixed: migration moved off the root preamble into the chat/print/repl branches and before `runChat()`; cold commands like `hawk path` never build the engine)* |
| M18 | Unbounded TUI-side growth (history, messageQueue, messages, `toolResultExpanded`) | `cmd/chat_submit.go:51`, `chat_model.go:185` *(fixed: prompt history capped (200) via `pushHistory`, queue capped (100) via `enqueueMessage`, messages already trimmed at 500, expansion map reindexed+pruned on trim; unit tests added)* |
| M19 | Async hook goroutines never drained (`WaitAsync` has no callers) — unbounded under tool loops | `internal/hooks/hooks.go:134-156` *(fixed: session-end drains queued async hooks via `WaitAsync` with a 30s cap after `ExecuteAsync`)* |
| M20 | Legacy `Sandbox.Run` fails open when `Enabled=false` (host `bash -c`); no production callers — latent footgun | `internal/sandbox/sandbox.go:134-135` *(fixed: fails closed unless explicitly opted out via `Tier == TierOff`)* |

### 3.4 LOW (selected)

- Engine stream retry ignores `Retry-After`, fixed 1–3s delay (`stream.go:448`) *(fixed: `streamRetryDelay` parses a "retry in|after N[ms]" hint from the stream error (matching eyrie's retryDelayRe), honors it capped at `maxStreamRetryDelay` (60s), else falls back to the existing linear 1–N backoff; `isRetryableStreamError` broadened to surface rate-limited (429) and 503 streams for retry; added `stream_retry_test.go`)
- Deployment retry can re-select the same dead deployment (`deployment_router.go:149-150`) *(fixed in `external/eyrie` on branch `fix/deployment-retry-no-repeat`: `selectDeploymentChoice(choices, exclude)` skips the just-failed deployment when alternatives remain; Chat/StreamChat tracks a stage-scoped `recentlyFailed` id; single-deployment stages still retry once to trip the breaker; `TestDeploymentRouterRetriesPreferDifferentEndpoint` asserts dead is tried ≤1× and healthy is reached; pushed to eyrie for PR — Hawk consumes via the live submodule in go.work)*
- Substring-based retry/credit/overflow classification causes spurious retries and silent emergency-compact (`stream_helpers.go:32-40`, `retry.go:41-57`, `chat_service.go:258-264`) *(partially addressed: `isContextOverflow` tightened to match structured provider signals — `context_length_exceeded`/`context_length_error`/`exceeds the limit` — and to require a token/context qualifier alongside the legacy "too long"/"too many tokens" phrasing so ordinary "request timeout, too long" no longer spurious-compacts (reduces false positives, cannot storm); remaining substring heuristics in `retry.go` `IsRetryable` left as-is per the risk note — they are additive (more retry coverage) but traffic-driven tuning is still recommended)*
- Linux token-file write non-atomic; concurrent Set races (`auth.go:235-264`) *(fixed: token store now uses `internal/safewrite` — atomic temp-write + fsync + symlink guard)*
- Non-atomic `0o600` writes without fsync (`session/cross_session.go:376`, `memory/knowledge.go:519`) *(fixed: both now use `safewrite.WriteFile`)*
- Unbounded `EndSession` goroutine without context (`stream.go:681`) *(fixed: `IntegrationPipeline.EndSession` now takes `context.Context` and bails on a canceled context; caller passes the session ctx)*
- Sandbox image pulled by mutable tag, no digest pinning (`image.go:40-42`) *(fixed: `HAWK_SANDBOX_IMAGE_DIGEST` env pins `repo@sha256:<digest>` when set)*
- `ModeOff` disables path guard (`path_guard.go:21`) *(no change — intentional: `--sandbox off` is an explicit opt-out of all sandbox protections incl. the path guard; changing it risks breaking host-mode workflows)*
- Session load bricks on >1MB message line (`session.go:389`); fixed tmp name `id.jsonl.tmp` across processes (`session.go:97`); stale `.wal` after recovery *(fixed: `scanJSONLLines` reader with a 16 MB per-line cap drains+logs oversize/corrupt lines instead of bricking the load; corrupt meta line is a load error (500) while an empty file is still ErrNotFound (404); `RecoverFromWAL` reuses the same tolerant reader; Save's temp name is namespaced with getpid())*
- MCP stale `pendErrors` entries + zombie on failed connect (`mcp.go:118-155`) *(no change needed: all `callWithTimeout` terminal paths (success/timeout/ctx-cancel) and the EOF/readLoop-exit path already delete `pendErrors[id]` and `pending[id]`; the connection-lost zombie is resolved by M7's dead-flag + child-kill)*
- `trackSession`/`sessions` grow unboundedly in long-lived daemon (`daemon.go:75,924`) *(fixed: in-memory sessions index capped at `maxTrackedSessions` (1000), evicting oldest by LastUsed)*
- `MessageBus` (700 lines) dead in production; `hooks.EventBus` unused *(partial: `internal/hooks/events.go` + its test deleted (genuinely dead — no production callers); `multiagent.MessageBus` retained — it backs the agent file-lock feature (`AcquireLock`/`IsLocked`) and its lock tests exercise real behavior, so the "dead in production" claim is inaccurate for it)*
- Plugin security scanner advisory-only; `CheckExtensionMalware` has no callers *(fixed: `internal/plugin/malware_check.go` deleted)*
- `WithTimeout` no-op cancel footgun (`timeout.go:33-40`); fabricated session IDs; dead exports (`RemainingTime`, `Countdown`) *(fixed: `RemainingTime`/`Countdown` now wired into both `runPrint` and REPL print paths (one remaining-time notice per turn); fabricated `session_<nanos>` replaced with `genID()` in the memory manager startup; WithTimeout cancel is correctly deferred at both call sites)*
- Staticcheck: unused `getKeys` (`coverage_extra_test.go:131`) *(no change needed: verified clean — `getKeys` is no longer present/used)*

### 3.5 Verified-clean (defense-in-depth that holds)

- Docker-socket not mounted; host env not passed into container; `--read-only` + `noexec` tmpfs + `cap-drop ALL` + `no-new-privileges` + `pids-limit 256`
- Fail-closed verified at: container boot (CLI/headless/TUI), `WrapCommand`, tool service (container required → tools disabled), `ParseMode` (typo → Strict)
- ApprovalGate fails closed, consulted after permission check, never loosens a denial
- Bash hard-deny regexes layered; `safewrite` uses `O_NOFOLLOW` + temp+rename+0600
- API keys in OS keychain, never in config (`settings.go:485-491` rejects `apiKey.*` writes); macOS piped via stdin, never argv; constant-time daemon auth
- Exponential backoff with full jitter + `Retry-After` honored (eyrie); token-bucket rate limiter ctx-aware and leak-free; SSE bounded (128-buf/64KB); circuit breaker with half-open
- Atomic session persistence (temp+sync+rename, WAL, `busy_timeout`, FK on); migrations present
- Agent-loop background goroutines all timeout-bounded (10s–2min); async hooks WaitGroup-tracked
- Loop guards: SnowballDetector, LoopDetector, turn limit, budget limit, max_tokens recovery cap
- Telemetry strictly opt-in (`HAWK_CODE_ENABLE_TELEMETRY=1`), span content hygiene, redaction of 25+ patterns

---

## 4. Competitor comparison (June–July 2026 data)

Sources: official docs matrix (hidekazu-konishi.com), MorphLLM ranked table, codemyspec.com, sanj.dev, Starkslab control-surface notes. Verified June 28, 2026.

| Agent | License / Stars | Model freedom | MCP | Sandboxing | Headless/CI | Benchmarks (agent+model) |
|---|---|---|---|---|---|---|
| Claude Code | Proprietary / 134K | Claude only | Client (1,000+ servers) | Modes: plan→bypassPermissions; checkpoints, worktree isolation | `claude -p`, JSON | 88.6% SWE-bench V; 78.9% TB 2.1 |
| Codex CLI | Apache-2.0 / 94K | OpenAI only | Client + **server**; 9,000+ plugins | 3-tier permission + sandbox modes | `codex exec` JSONL | **83.4% TB 2.1 (#1)**; 82.1% SWE-bench V |
| Antigravity (ex-Gemini CLI) | Apache-2.0 / 105K | Gemini only | Client | plan mode, folder trust, checkpoints | `antigravity -p` JSON | 70.7% TB 2.1 |
| opencode | MIT / 180K | **75+ providers + local** | Client | permission rules, plan/build agents | `opencode run`, `serve` | varies (BYOK) |
| Aider | Apache-2.0 / 47K | any OpenAI-compatible | No | git-first (auto-commit/revert) | `aider --message` | 88% polyglot (GPT-5); dormant since Aug 2025 |
| Goose | Apache-2.0 / 38K | any LLM | Client (extensions) | optional macOS sandbox; recipes | `goose run` | n/a |
| Cline / Kilo Code / Qwen Code | OSS | BYOK | Yes | approval modes | headless | n/a |

**Where hawk-eco is already competitive:**
- **Only player with Docker-isolated, fail-closed command execution** (AgentForge paper validates this exact design; Codex sandbox is closest but host-process-based)
- Model-agnostic like opencode/Aider/Goose (23 first-class providers via eyrie)
- Zero-CGO single static binary; privacy-first
- Depth of in-repo instrumentation (codegraph, executiongraph, graphjournal, GitNexus-style impact analysis) exceeds every OSS competitor

**Where hawk-eco trails (actionable):**
1. **Benchmark presence** — no published SWE-bench/Terminal-Bench numbers; `internal/bench` exists but has no test files. Competitors publish; credibility demands a harness + public run (even on a subset).
2. **MCP server mode** — Codex ships `codex mcp-server`; hawk is client-only. Cheap to add via existing `internal/mcp`.
3. **JSONL event output for CI** — `codex exec --json` / `claude -p --output-format json` set the bar; hawk's headless path should emit machine-readable events (daemon already streams SSE — expose the same shape on stdout).
4. **Startup latency** — 1.83s `hawk path` vs Rust-based Codex "near-instant"; defer eyrie engine init until first use.
5. **Ecosystem** — opencode's TUI Mission Control, Claude Code's Agent Teams; hawk has multiagent + HUD already — needs a public story + docs polish.
6. **Aider's git discipline** — auto-commit-per-edit with clean revert is the OSS gold standard; hawk should consider opt-in auto-checkpoints.

---

## 5. Research papers mapped to concrete improvements

| Paper (year) | Core idea | Relevance to hawk | Action |
|---|---|---|---|
| **CAT — Context as a Tool** (ACL 2026 Findings) | Context management as a callable, plannable tool; proactive folding at milestones; SWE-Compressor 57.6% SWE-bench V | hawk's compaction is passive/heuristic (`context_governor.go`), exactly the criticized pattern | Expose a `context` tool the agent can call; fold at stage boundaries |
| **SWE-MeM** (arXiv 2606.28434, 2026) | Adaptive memory management; memory-aware GRPO; 60.2% @30B | hawk's `EvolvingMemory` is the right idea, unpersisted and untrained | Fix H6 (persistence); add evaluation harness to measure guideline quality |
| **Git-Context-Controller (GCC)** (arXiv 2508.00031, 2025) | Versioned memory hierarchy: COMMIT/BRANCH/MERGE/CONTEXT; 48% SWE-bench-Lite (SOTA) | hawk already has `graphjournal`, `branching`, `session` decomposition | Wire session milestones into a navigable, versioned memory (ties to H10/mission worktrees) |
| **SWE-Adept** (arXiv 2603.01327, 2026) | Agent-directed DFS localization + two-stage filtering; checkpointed git-based resolution (+4.7% end-to-end) | `codegraph` exists but semantic search is brute-force (H8) | Adopt dependency-aware traversal + deferred full-code loading; reuse `branching` for checkpoints |
| **ContextBench** (arXiv 2602.05892, 2026) | Process-level retrieval eval; "Bitter Lesson": complex scaffolding ≠ better retrieval; recall>precision; consolidation gap | Warning against over-engineering; hawk's breadth is high | Prioritize retrieval precision + consolidation; add context-eval metrics |
| **AgentForge** (arXiv 2604.13120, 2026) | Execution-grounded verification; mandatory Docker sandbox; 40% SWE-bench Lite | **Validates hawk's Docker-only design**; five-role decomposition beats single-agent by 26–28pts | Cite in README/architecture docs; consider Tester→Debugger loop wiring in mission mode |
| ReAct (2022) / Reflexion (2023) | Interleave reasoning+action; verbal self-reflection | hawk's lifecycle loop is Reflexion-style | Fix H6 so the loop actually persists |

---

## 6. Recommended roadmap (draft — in execution on this branch)

1. **Triage (C1, H1, H3–H6, H12):** wire panic recovery, runtime.jsonc allowlist, fail-closed HTTP hooks, SSE write-error exit, signal-safe session save, EvolvingMemory persistence, env scrubbing for bash
2. **Concurrency & budgets (H7, M1, M2, M9):** mutex'd limits accessors, wire RecordCost, bounded tracer, honest error propagation
3. **Dead code (H10, H11, M5, M8):** fix-and-test async; delete docs; wire or delete approval gate/composio stub/MessageBus (H11 docs deleted; M5 approval gate wired; M8 composio deleted; dead `hooks.EventBus` and `plugin.CheckExtensionMalware` deleted; `multiagent.MessageBus` retained — backs agent file-lock)
4. **Performance (H8, M14–M18):** embedding cache, hoisted regexes, no-clone context access, viewport incremental render, lazy eyrie init
5. **Multiagent correctness (H9, M6):** retryable branch names, exit-code propagation, detached worktree cleanup
6. **Competitor deltas:** MCP server mode, JSONL headless output, benchmark harness
7. **Paper-backed features:** context-as-tool, milestone-based memory folding

## 7. Method & verification notes

- All `file:line` references verified against HEAD `bfd5654`; dead-code claims verified via import-graph search
- `go test -race` passes on exercised paths; racy findings (M1) exist because the racy paths are untested
- Research (Phase 5/6) uses June–July 2026 sources only; star counts/benchmarks are point-in-time
