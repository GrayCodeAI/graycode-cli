# GrayCode AI Ecosystem: World-Class Solo Developer Platform

## Research Summary

20 parallel research agents analyzed 1000+ papers and OSS repos across: AI agent architectures, token compression, code review automation, graph memory systems, LLM provider optimization, web scanning, developer productivity, context management, MCP ecosystem, sandboxing, evaluation frameworks, planning/reasoning, self-improving agents, multi-modal capabilities, CI/CD automation, RAG for code, terminal UI/DX, testing/verification, observability/debugging, and skill/plugin ecosystems.

---

## HAWK (AI Coding Agent)

### Tier 1: Critical Path (implement first)

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 1 | **Test-first workflow** | AgentCoder (96.3% HumanEval), Reflexion (91%) | Highest correlation with top benchmark scores |
| 2 | **Context budget allocator** | All top agents; Claude Code's design philosophy | The architectural glue that makes everything else work |
| 3 | **Change-set aware context** | Cursor shadow workspace, SWE-agent ACI design | 70-90% context reduction for focused tasks |
| 4 | **Tree-sitter integration** | Aider repo-map, AutoCodeRover AST navigation | Replace regex parsers; enables call graph + import graph |
| 5 | **Model cascading router** | RouteLLM (85% cost savings at 95% quality) | Route simple->Haiku, debug->Sonnet, generation->Opus |
| 6 | **Prompt caching in eyrie** | Anthropic docs: 90% savings on cached tokens | 64% reduction on system prompt costs per session |
| 7 | **LLM-based reflection** | Reflexion: 91% HumanEval via verbal self-reflection | Replace mechanical trajectory summaries with LLM reflections |
| 8 | **Self-review before apply** | Self-Debugging: +12% on MBPP | "Rubber duck" step between generation and file write |

### Tier 2: Competitive Advantages

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 9 | **File relevance prediction** | Cursor indexing, SWE-agent localization (~60% precision) | Pre-load predicted files, save 2-3 roundtrips per task |
| 10 | **Import/dependency graph** | All top agents use import traversal | Cheapest cross-file signal; prevents "undefined" errors |
| 11 | **Call graph for Go** | golang.org/x/tools/go/callgraph; 15-20% improvement | Load callers/callees when editing a function |
| 12 | **Plan revision on failure** | Plan-and-Solve (ACL 2023), MetaGPT SOPs | Re-plan when subtask fails instead of blindly continuing |
| 13 | **Landlock + seccomp-bpf** | Linux 5.13+; zero-dependency isolation | Default Linux sandbox without Docker/nsjail/root |
| 14 | **Session-end reflection loop** | Reflexion + ExpeL (AAAI-24) | Wire EvolvingMemory.Learn() into session lifecycle |
| 15 | **Skill distillation pipeline** | Voyager: 15.3x faster with accumulated skills | Auto-distill completed tasks into reusable skills |
| 16 | **Affected test detection** | Meta: runs 1/3 of tests, catches 99.9% regressions | 60-80% test time reduction on every run |

### Tier 3: Frontier Capabilities

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 17 | **Multi-file transactional staging** | Plandex sandbox, Cursor shadow workspace | Validate full changeset before any disk write |
| 18 | **Adaptive retry with failure classification** | Olausson ICLR 2024: self-repair bottlenecked by feedback quality | Type error != logic error != test failure; each needs different strategy |
| 19 | **Project history mining** | No agent does this well; identified as Gap #6 | Extract patterns from git history to inform planning |
| 20 | **Cost-aware execution planning** | AlphaCodium: 15-20 calls vs AlphaCode's millions | Estimate difficulty, allocate model tier and retry budget |
| 21 | **Parallel DAG execution** | GoT aggregation, hawk planner already has Depends field | Execute independent subtasks in parallel via sub-agents |
| 22 | **Few-shot example curation** | DSPy: 25-65% improvement over baseline prompting | Collect successful completions, inject as examples |
| 23 | **Multi-modal content blocks** | Vision models understand screenshots/diagrams/errors | Foundation for /mockup, error diagnosis, PDF reading |
| 24 | **Automated scientific debugging** | AutoSD paper: matches program repair, 70% want explanations | Hypothesis-driven /debug command |
| 25 | **Error fingerprinting** | Sentry algorithm: normalize, hash, deduplicate | Stop re-investigating known issues across sessions |

### Tier 4: TUI/DX Polish

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 26 | **Fuzzy command palette (Ctrl+K)** | Textual, Warp, VS Code patterns | 80+ commands need discoverable access |
| 27 | **Progressive disclosure (3 levels)** | NNGroup research, clig.dev | Default/verbose/debug with one keystroke toggle |
| 28 | **Block-style conversation grouping** | Warp terminal block model | Each turn is a visual unit (collapsible, copyable) |
| 29 | **Replace markdown renderer with Glamour** | charmbracelet/glamour; GitHub CLI uses it | Proper syntax highlighting, stylesheet-based |
| 30 | **Word-level diff highlighting** | delta, diff-so-fancy patterns | Show exactly which characters changed |

---

## TOK (Token Compression)

### Tier 1: High Impact, Moderate Effort

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 1 | **LLMLingua-2 ONNX bridge** | Microsoft: 2-5x compression, 3-6x faster, <5% quality loss | Optional learned token classification (keep/drop) via ONNX |
| 2 | **Pyramid budget distribution** | PyramidKV: full performance at 12% retention | Early layers permissive, later layers aggressive |
| 3 | **Cache breakpoint markers** | Anthropic caching: 90% savings on cache hits | Output [CACHE_BREAK] annotations for API callers |
| 4 | **Cross-layer chunk index sharing** | ChunkKV: 26.5% faster, 8.7% precision gain | Compute boundaries once, share across all layers |

### Tier 2: High Impact, Higher Effort

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 5 | **Proportional budget controller** | LLMLingua: budget proportional to semantic segment importance | System prompt gets higher budget than boilerplate |
| 6 | **Query-aware document reordering** | LongLLMLingua: 21.4% RAG accuracy improvement | Place highest-relevance at positions 1 and N |
| 7 | **Diff-aware incremental compression** | Differential context engineering pattern | Only output delta between previous and current context |
| 8 | **Tree-sitter AST integration** | LongCodeZip: 5.6x reduction, 16% better accuracy | Replace regex code parsing with exact AST boundaries |

### Tier 3: Medium Impact, Specialized

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 9 | **Multi-file RAPTOR mode** | RAPTOR: +20% accuracy via hierarchical summaries | Cluster files, produce tree of summaries |
| 10 | **Adaptive entropy thresholds** | Selective Context: code vs prose have different distributions | Separate frequency tables for code and natural language |
| 11 | **Expanded gist patterns (50+)** | Current 7 patterns -> 50+ covering all major output types | Python/Java/Go tracebacks, CI logs, Docker output |
| 12 | **Atomic proposition extraction** | Dense X Retrieval: atomic facts for precise retrieval | Summarize as structured propositions, not free-form |

---

## YAAD (Graph Memory)

### Architecture (adopt from MAGMA + Graphiti)

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 1 | **Bi-temporal validity windows** | Graphiti: valid_from + invalid_at per fact | Enable git-aware staleness and temporal queries |
| 2 | **Fast path <100ms ingestion** | MAGMA dual-stream: sync event + async consolidation | Never block agent waiting for memory processing |
| 3 | **Intent-aware retrieval routing** | MAGMA: Why/When/What queries traverse different edges | Boost causal edges for "why" queries, temporal for "when" |
| 4 | **Adaptive beam search (2-3 hops)** | MAGMA: S(n_j|n_i,q) dynamic transition scoring | Multi-hop traversal from anchor nodes |
| 5 | **Proactive context preloading** | MemoRAG: 30x faster context loading | When file opened, preload related memories |

### Memory Management

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 6 | **Access-weighted Ebbinghaus decay** | Generative Agents + MemoryBank | Spaced repetition: access resets decay |
| 7 | **Hierarchical summarization** | RAPTOR tree: individual -> cluster -> domain -> project | Retrieval at multiple abstraction levels |
| 8 | **Content-hash + semantic near-dedup** | nano-graphrag: MD5 dedup; Mem0: entity linking | Multi-layer deduplication at ingest and consolidation |
| 9 | **Code-structural edges** | Import graphs, call graphs, type hierarchies | First-class edge types alongside MAGMA's four types |
| 10 | **Procedural memory category** | CoALA framework: episodic/semantic/procedural | Store learned patterns separately from facts |

### Yaad MCP Gaps

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 11 | **Register documented resources** | MCP spec: yaad://context, yaad://graph/stats, yaad://stale | Architecture docs promise them, code doesn't register them |
| 12 | **Register documented prompts** | MCP spec: recall_context, session_handoff | Same gap as above |
| 13 | **Streamable HTTP transport** | MCP 2025-03-26 spec | Enable remote access beyond stdio |
| 14 | **Progress notifications** | MCP spec: notifications/progress | Long operations (compact, hybrid_recall) emit progress |

---

## EYRIE (LLM Provider Runtime)

### Tier 1: Highest ROI

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 1 | **Prompt caching headers** | Anthropic: 90% input cost reduction on cached tokens | Add cache_control to system + tool definitions |
| 2 | **Two-tier model cascade** | RouteLLM: 85% savings; FrugalGPT: 98% savings | Heuristic complexity classification -> model selection |
| 3 | **Adaptive rate limiting** | Parse anthropic-ratelimit-* headers | Predict capacity, throttle proactively before 429s |
| 4 | **Dynamic max_tokens** | Output tokens 3-5x more expensive than input | Context-aware: 2048 for tool calls, 8192 for generation |

### Tier 2: Strong Value

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 5 | **Ollama as first-class provider** | Zero marginal cost for simple tasks | Route commit msgs, summaries to local models |
| 6 | **Batch API client** | Anthropic: 50% discount on batch requests | Non-interactive operations (research, analysis, review) |
| 7 | **Provider health scoring** | Circuit breaker + latency tracking + error rate | Route away from degraded providers automatically |
| 8 | **TTFT tracking** | Time-to-first-token as provider quality signal | Surface slow providers, switch proactively |

### Tier 3: Protocol Compliance

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 9 | **MCP protocol version 2025-03-26** | Current hawk sends 2024-11-05 | Blocks interop with modern MCP servers |
| 10 | **Streamable HTTP transport** | New in 2025-03-26, replaces HTTP+SSE | Session management, SSE streaming from POST |
| 11 | **Sampling support** | Server-initiated LLM calls through client | Enables yaad to auto-summarize via hawk's model |
| 12 | **Notifications handling** | list_changed, progress, logging, resource updates | Currently readLoop ignores all notifications |

---

## INSPECT (Website Auditor)

### Architecture: Pipeline Model (crawl -> collect -> analyze -> report)

| # | Check Category | Replaces | Priority |
|---|---------------|----------|----------|
| 1 | **Broken links** (internal + external) | muffet, lychee | P0 |
| 2 | **Security headers** (10+ headers) | securityheaders.com | P0 |
| 3 | **Cookie security** (Secure, HttpOnly, SameSite) | Manual | P0 |
| 4 | **Mixed content detection** | Browser console | P0 |
| 5 | **TLS/certificate validation** | testssl.sh basics | P0 |
| 6 | **CSP validation and grading** | Manual | P0 |
| 7 | **Meta tags** (title, description, viewport) | Screaming Frog | P0 |
| 8 | **Image alt text** | axe-core basic | P0 |
| 9 | **Redirect chains/loops** | Screaming Frog | P1 |
| 10 | **CORS misconfiguration** | Manual | P1 |
| 11 | **Exposed sensitive files** (.env, .git) | Nikto | P1 |
| 12 | **Technology fingerprinting** | Wappalyzer | P1 |
| 13 | **Core Web Vitals** (LCP, CLS, INP) | Lighthouse | P2 |
| 14 | **WCAG 2.2 automated checks** | axe-core, Pa11y | P2 |
| 15 | **DOM-based XSS pattern detection** | ZAP passive | P2 |

### Design Principles (from research)
- Zero false positives (axe-core philosophy)
- Template-based checks (Nuclei extensibility)
- Passive by default, active by opt-in (ZAP philosophy)
- Technology-aware (Wapiti: detect stack, run relevant checks)
- Every finding includes: severity + element + copy-pasteable fix

---

## SIGHT (AI Code Review)

### Architecture Improvements

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 1 | **Two-pass false positive elimination** | Tencent: 94-98% FP reduction at $0.001-0.12/alarm | Generate findings, then LLM-filter them |
| 2 | **Multi-concern routing** | iCodeReviewer: 84% production acceptance | Specialized sub-prompts per concern, not one mega-prompt |
| 3 | **Context enrichment priority** | ContextCRBench: textual context > code context | Commit messages, issue descriptions boost quality more |
| 4 | **Detect -> explain -> fix pipeline** | arXiv:2312.17485: 72.97% repair rate | Each stage feeds the next; explanation becomes fix spec |
| 5 | **Metadata redaction for security** | arXiv:2603.18740: 100% attack success via adversarial commits | Strip metadata during security analysis pass |
| 6 | **Restraint: 63% PRs need zero comments** | Greptile production data | Speak only when it matters; trusted senior engineer model |

### New Capabilities

| # | Feature | Research Basis | Impact |
|---|---------|---------------|--------|
| 7 | **Incremental review** (only changed + context) | All commercial tools | Don't re-review unchanged code |
| 8 | **Cross-file impact analysis** | Change impact research; gap in all current agents | "If you change schema.sql, what else breaks?" |
| 9 | **Historical pattern learning** | Gap #6 identified in agent research | Learn what gets rejected in this project's PRs |
| 10 | **Auto-fix with confidence scores** | CodeRabbit, Qodo patterns | Not just "bug here" but "fix: change X to Y" |

---

## CROSS-ECOSYSTEM INTEGRATION

### The Closed Loop (what makes it world-class)

```
SESSION START
  |-- yaad: inject hot-tier context (conventions, active tasks, stale warnings)
  |-- hawk: load EvolvingMemory guidelines
  |-- hawk: inject few-shot examples from prior successes
  |-- eyrie: pre-warm prompt cache (system + tools)

DURING SESSION
  |-- tok: compress all tool outputs (80% savings)
  |-- eyrie: route by task complexity (Haiku/Sonnet/Opus)
  |-- hawk: parallel tool execution for read-only ops
  |-- sight: review every edit before apply (optional)
  |-- inspect: audit generated web code (optional)

SESSION END
  |-- hawk: LLM reflection ("what worked, what failed, why")
  |-- yaad: store reflection as episodic memory
  |-- hawk: distill successful approaches into skills
  |-- yaad: consolidate, decay, deduplicate memories
  |-- eyrie: log cost per task for analytics
```

### The Solo Developer Multiplier Stack

```
WRITE      hawk (AI agent) + tok (compression) ............ 55% speed boost
REVIEW     sight (code review) + hawk /review ............. Replaces human reviewer
TEST       hawk /test --generate + mutation testing ....... Enterprise-level coverage
SCAN       inspect (website audit) ........................ One command, full audit
REMEMBER   yaad (graph memory) ............................ Zero context re-explaining
SHIP       hawk /commit + goreleaser + CI ................. One command to production
MONITOR    hawk /debug + error fingerprinting ............. Debug like a team with SRE
LEARN      hawk self-improvement loop ..................... Gets better every session
```

---

## COST MODEL: From $150/month to $20/month

| Layer | Savings | How |
|-------|---------|-----|
| tok compression | 60-80% fewer input tokens | Command output filtering + prompt compression |
| Prompt caching | 90% on system/tools (repeating tokens) | cache_control headers in eyrie |
| Model cascading | 50-70% on tasks routable to cheap models | Task classifier + Haiku for simple, Sonnet for medium |
| Local models | 100% on commit msgs, summaries | Ollama for trivial tasks |
| Output right-sizing | 15-25% fewer output tokens | Dynamic max_tokens per turn type |
| Batch API | 50% on background work | Research, analysis, consolidation |
| Tool call reduction | 30-50% fewer round trips | Better planning, parallel calls |
| **Combined** | **~85% total reduction** | **$150 -> $20-30/month** |

---

## IMPLEMENTATION SEQUENCE

### Phase 1 (Weeks 1-2): Cost Foundations
1. eyrie: Prompt caching headers
2. hawk: Wire task classifier as pre-request model selector
3. hawk: Dynamic max_tokens
4. hawk: Use cheap model for compaction summaries
5. tok: Cache breakpoint markers

### Phase 2 (Weeks 3-4): Core Intelligence
6. hawk: Tree-sitter integration (replace regex parsers)
7. hawk: Context budget allocator
8. hawk: Import/dependency graph for Go
9. hawk: Change-set aware context loading
10. yaad: Bi-temporal validity windows + fast path ingestion

### Phase 3 (Weeks 5-6): Quality Loop
11. hawk: Test-first workflow (test designer sub-agent)
12. hawk: LLM-based reflection at session end
13. hawk: Self-review before file write
14. sight: Two-pass false positive elimination
15. hawk: Affected test detection

### Phase 4 (Weeks 7-8): Self-Improvement
16. hawk: Session-end reflection -> EvolvingMemory
17. hawk: Skill distillation pipeline
18. hawk: Few-shot example curation from successes
19. yaad: Intent-aware retrieval routing
20. yaad: Proactive context preloading

### Phase 5 (Weeks 9-10): Advanced
21. hawk: Landlock + seccomp-bpf (Linux default sandbox)
22. hawk: Multi-file transactional staging
23. hawk: Parallel DAG execution for independent tasks
24. eyrie: MCP protocol 2025-03-26 + Streamable HTTP
25. inspect: Full pipeline (crawl->collect->analyze->report)

### Phase 6 (Weeks 11-12): Polish
26. hawk: Fuzzy command palette (Ctrl+K)
27. hawk: Progressive disclosure (3 levels)
28. hawk: Block-style conversation grouping
29. tok: LLMLingua-2 ONNX bridge (optional learned compression)
30. tok: Pyramid budget distribution

---

## KEY RESEARCH CITATIONS

| Finding | Source | Implication |
|---------|--------|-------------|
| Minimal tools beat complex scaffolding | Anthropic SWE-bench (49%) | Invest in tool quality, not agent complexity |
| External validation essential | Olausson ICLR 2024 | Run tests/lints in the loop, not just self-reflection |
| Architect/editor split works | Aider: 85% benchmark | Separate reasoning from formatting |
| AST-aware context beats naive RAG | Aider, AutoCodeRover, Plandex | Use tree-sitter, not embeddings |
| 72% of Devin successes took >10 min | Devin paper | Iteration in safe environment is how complex problems get solved |
| Time-to-automate doubles every 7 months | METR temporal research | Focus on extending reliable task duration |
| LLM test gen: 80% on simple, <2% on real code | EASE 2024 study | Generate-filter-validate pipeline mandatory |
| Tuned BM25 beats naive embeddings for code | Sourcegraph (dropped embeddings) | Invest in tokenization before adding vectors |
| Agent self-improvement needs no weight updates | Reflexion, ExpeL, Voyager | Prompt optimization + memory accumulation suffices |
| Solo dev productivity = automation density | Laravel/Rails ecosystem study | % of non-creative work handled by machines |
