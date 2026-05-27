# Hawk Ecosystem - Complete Feature Reference

> 100 features implemented across the GrayCodeAI ecosystem, organized by category.

---

## Summary

| # | Category | Features | Description |
|---|----------|----------|-------------|
| 1 | Core Agent Intelligence | 14 | Reasoning, context management, optimization |
| 2 | Safety & Permissions | 9 | Sandboxing, permission enforcement, secrets |
| 3 | Memory & Knowledge | 9 | Cross-session learning, graph search, knowledge extraction |
| 4 | Code Understanding | 12 | AST analysis, navigation, dependency graphs |
| 5 | Developer Workflow | 13 | Commits, transactions, branching, automation |
| 6 | Tools & Generation | 8 | Code generation, plugins, MCP, completions |
| 7 | Observability & Analytics | 8 | Telemetry, profiling, cost analysis |
| 8 | Review & Security | 7 | Static analysis, CVE scanning, review bot, CI output |
| 9 | Session & Persistence | 7 | SQLite storage, search, export, replay |
| 10 | Agent Personas & Multi-Agent | 5 | Personas, messaging, task decomposition |
| 11 | Evaluation & Benchmarking | 4 | SWE-bench, model comparison, reporting |
| 12 | Platform & Config | 4 | Health checks, env management, scaffolding |
| | **Total** | **100** | |

---

## 1. Core Agent Intelligence (14 features)

Features that power hawk's reasoning, context management, and execution optimization.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 1 | Fuzzy edit matching | `internal/tool/file_edit.go` | Levenshtein + whitespace fallback when exact match fails | `FuzzyMatch()`, `LevenshteinDistance()`, `WhitespaceFallback()` | Aider |
| 2 | Auto-lint-fix loop | `internal/engine/validation/lint_loop.go` | Feed lint errors back for auto-correction | `LintLoop`, `RunLintFix()`, `ParseLintErrors()` | Aider |
| 3 | Auto-test loop | `internal/engine/validation/test_loop.go` | Run tests after edits, feed failures back | `TestLoop`, `RunTestFix()`, `ParseTestFailures()` | Aider |
| 4 | Architect/editor pipeline | `internal/engine/architect.go` | Cheap model plans, expensive model executes | `Architect`, `Plan()`, `Execute()`, `PipelineConfig` | Aider |
| 5 | Prompt optimizer | `internal/engine/prompt_optimizer.go` | DSPy-style few-shot selection with A/B testing | `PromptOptimizer`, `SelectExamples()`, `ABTest()` | DSPy |
| 6 | Thinking protocol | `internal/engine/thinking.go` | Structured reasoning phases before execution | `ThinkingProtocol`, `Reason()`, `Phase` | Claude |
| 7 | Context providers | `internal/engine/ctxmgr/context_providers.go` | Pluggable context sources (git, files, env) | `ContextProvider`, `GitProvider`, `FileProvider`, `EnvProvider` | Continue |
| 8 | Smart context packing | `internal/engine/ctxmgr/context_packer.go` | Priority-based message selection within token limits | `ContextPacker`, `Pack()`, `PriorityQueue` | Custom |
| 9 | Context window visualizer | `internal/engine/ctxmgr/context_viz.go` | ASCII visualization of token allocation | `ContextViz`, `Render()`, `AllocationChart` | Custom |
| 10 | Token budget allocator | `internal/engine/token/budget_allocator.go` | Dynamic priority-based token distribution | `BudgetAllocator`, `Allocate()`, `Budget` | Custom |
| 11 | Token usage predictor | `internal/engine/token/token_predictor.go` | Estimate cost before executing a request | `TokenPredictor`, `Predict()`, `CostEstimate` | Custom |
| 12 | Response cache | `internal/engine/streaming/response_cache.go` | Cache similar prompts to save tokens | `ResponseCache`, `Get()`, `Put()`, `SimilarityHash()` | Custom |
| 13 | Session compression | `internal/engine/session/session_compressor.go` | Tiered and semantic summarization of history | `SessionCompressor`, `Compress()`, `TieredSummary` | Custom |
| 14 | Stream optimizer | `internal/engine/streaming/stream_optimizer.go` | Buffer, deduplicate, progressive render | `StreamOptimizer`, `Buffer()`, `Dedupe()`, `Render()` | Custom |

---

## 2. Safety & Permissions (9 features)

Security enforcement, sandboxing, and permission management for safe agent operation.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 15 | Guardian auto-reviewer | `internal/permissions/guardian.go` | LLM-based permission decisions for risky operations | `Guardian`, `Review()`, `Decision`, `RiskLevel` | Codex |
| 16 | Declarative permission rules | `internal/permissions/rules.go` | .hawk/rules DSL file format for permissions | `RuleSet`, `ParseRules()`, `Rule`, `Match()` | Codex/OpenCode |
| 17 | Command canonicalization | `internal/permissions/canonicalize.go` | Normalize commands for stable rule matching | `Canonicalize()`, `NormalizeArgs()`, `CommandSignature` | Codex |
| 18 | Safety boundary checker | `internal/permissions/boundary.go` | Path traversal and scope enforcement | `BoundaryChecker`, `CheckPath()`, `EnforceScope()` | Custom |
| 19 | macOS seatbelt sandbox | `internal/sandbox/seatbelt.go` | SBPL profile generation for process isolation | `SeatbeltProfile`, `Generate()`, `Apply()`, `SBPLRule` | Codex |
| 20 | Network proxy sandbox | `internal/sandbox/netproxy.go` | Domain-level HTTP CONNECT proxy filtering | `NetProxy`, `AllowDomain()`, `BlockDomain()`, `Intercept()` | Codex |
| 21 | Secret detection | `tok/secrets.go` | 27 patterns + Shannon entropy scanning | `SecretScanner`, `Scan()`, `Pattern`, `ShannonEntropy()` | trufflehog |
| 22 | Rate limiter | `tok/ratelimit.go` | Daily/hourly/session token budgets with enforcement | `RateLimiter`, `Check()`, `TokenBudget`, `Refill()` | Custom |
| 23 | Read-only context files | `internal/engine/readonly_context.go` | Pin files as context, block edits to them | `ReadOnlyContext`, `Pin()`, `IsReadOnly()`, `BlockEdit()` | Custom |

---

## 3. Memory & Knowledge (9 features)

Persistent knowledge, cross-session learning, and graph-based retrieval.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 24 | Knowledge distillation | `internal/engine/memory/knowledge.go` | Extract reusable patterns from sessions | `KnowledgeDistiller`, `Distill()`, `Pattern`, `Store()` | Custom |
| 25 | Cross-session learning | `internal/engine/session/cross_session.go` | Transfer insights between sessions | `CrossSessionLearner`, `Transfer()`, `Insight`, `Apply()` | Custom |
| 26 | Error pattern learning | `internal/engine/errs/error_learning.go` | Remember fixes for recurring errors | `ErrorLearner`, `Learn()`, `RecallFix()`, `ErrorPattern` | Custom |
| 27 | Community detection (yaad) | `yaad/graph/community.go` | Louvain modularity optimization for code clusters | `CommunityDetector`, `Detect()`, `Louvain()`, `Community` | GraphRAG |
| 28 | Drift search (yaad) | `yaad/graph/drift_search.go` | Hierarchical beam search over knowledge graph | `DriftSearch`, `Search()`, `BeamState`, `Hierarchy` | GraphRAG |
| 29 | Community search (yaad) | `yaad/graph/community_search.go` | BM25 scoring on community summaries | `CommunitySearch`, `Query()`, `BM25Score()`, `Summary` | GraphRAG |
| 30 | Semantic code chunker (yaad) | `yaad/ingest/chunker.go` | AST-aware file splitting for indexing | `SemanticChunker`, `Chunk()`, `ASTBoundary`, `CodeChunk` | Custom |
| 31 | File mention detection | `internal/engine/history/file_mentions.go` | Auto-detect file paths in responses | `FileMentionDetector`, `Detect()`, `ResolvePath()` | Aider |
| 32 | Goal/objective tracking | `internal/engine/planning/goals.go` | Persistent objectives with token budgets | `GoalTracker`, `AddGoal()`, `Track()`, `Goal`, `Budget` | Codex |

---

## 4. Code Understanding (12 features)

Static analysis, navigation, and codebase comprehension features.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 33 | Tree-sitter repomap | `internal/intelligence/repomap/treesitter.go` | Go AST + enhanced multi-language parsers | `RepoMap`, `BuildMap()`, `TreeSitterParser`, `Symbol` | Aider |
| 34 | Code complexity analyzer | `internal/intelligence/repomap/complexity.go` | Cyclomatic + cognitive complexity scoring | `ComplexityAnalyzer`, `Analyze()`, `CyclomaticScore`, `CognitiveScore` | Custom |
| 35 | Dependency graph | `internal/intelligence/repomap/depgraph.go` | DOT/ASCII/Mermaid visualization of dependencies | `DepGraph`, `Build()`, `RenderDOT()`, `RenderMermaid()` | Custom |
| 36 | Code smell detector | `internal/intelligence/repomap/smells.go` | God objects, feature envy, data clumps detection | `SmellDetector`, `Detect()`, `Smell`, `GodObject`, `FeatureEnvy` | Custom |
| 37 | Codebase summary generator | `internal/intelligence/repomap/summary.go` | Architecture inference and prompt rendering | `SummaryGenerator`, `Generate()`, `InferArchitecture()` | Custom |
| 38 | Code navigation index | `internal/intelligence/repomap/navigation.go` | Go-to-definition, find-references without LSP | `NavigationIndex`, `GoToDef()`, `FindRefs()`, `Location` | Custom |
| 39 | Semantic diff analyzer | `internal/engine/semantic_diff.go` | Breaking change detection via AST comparison | `SemanticDiff`, `Analyze()`, `BreakingChange`, `ASTDiff` | Custom |
| 40 | Change impact analyzer | `internal/engine/project/impact_analyzer.go` | Blast radius prediction for code changes | `ImpactAnalyzer`, `Predict()`, `BlastRadius`, `AffectedFiles` | Custom |
| 41 | Diff-aware test selection | `internal/engine/diff/diff_test_selector.go` | Only run tests related to changed code | `DiffTestSelector`, `Select()`, `TestMapping`, `Coverage` | Custom |
| 42 | API compatibility checker | `internal/tool/api_compat.go` | Detect breaking API changes across versions | `APICompatChecker`, `Check()`, `BreakingChange`, `Semver` | Custom |
| 43 | Code action suggestions | `internal/engine/code/code_actions.go` | IDE-like quick fixes from pattern matching | `CodeActions`, `Suggest()`, `Action`, `QuickFix` | Custom |
| 44 | Project fingerprinting | `internal/feature/fingerprint/project.go` | Auto-detect stack, conventions, CI configuration | `Fingerprint`, `Detect()`, `Stack`, `Convention`, `CI` | Custom |

---

## 5. Developer Workflow (13 features)

Tools that enhance the day-to-day development experience.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 45 | Smart commit messages | `internal/tool/smart_commit.go` | LLM + rule-based conventional commits | `SmartCommit`, `Generate()`, `ConventionalFormat`, `CommitMsg` | Aider |
| 46 | Undo/restore system | `internal/engine/undo.go` | Granular file rollback with history tracking | `UndoManager`, `Undo()`, `Restore()`, `Snapshot`, `History` | Custom |
| 47 | Multi-file transactions | `internal/tool/transaction.go` | Atomic all-or-nothing edits across files | `Transaction`, `Begin()`, `Commit()`, `Rollback()`, `FileOp` | Custom |
| 48 | Streaming patch format | `internal/tool/patch.go` | Context-anchored hunk application | `PatchStream`, `Apply()`, `Hunk`, `ContextAnchor` | Codex |
| 49 | AI-comment watch mode | `internal/engine/directive_scanner.go` | `// ai:` comment triggers agent work | `AIWatch`, `Watch()`, `ParseDirective()`, `Trigger` | Aider |
| 50 | Conversation branching | `internal/engine/branching/branching.go` | Fork/merge like git for conversation threads | `Branch`, `Fork()`, `Merge()`, `ConversationTree` | OpenCode |
| 51 | Workspace snapshots | `internal/snapshot/workspace.go` | Gzip-compressed state capture and restore | `WorkspaceSnapshot`, `Capture()`, `Restore()`, `GzipState` | Custom |
| 52 | Config migration | `internal/config/migrate.go` | Version upgrades with backup and rollback | `ConfigMigrator`, `Migrate()`, `Backup()`, `Rollback()`, `Version` | Custom |
| 53 | Release automation | `internal/engine/project/release.go` | Changelog generation, version bump, release prep | `ReleaseManager`, `Prepare()`, `BumpVersion()`, `Changelog` | Custom |
| 54 | Migration planner | `internal/engine/project/migration_planner.go` | Plan and execute large-scale codebase changes | `MigrationPlanner`, `Plan()`, `Execute()`, `Step`, `Rollback` | Custom |
| 55 | PR description generator | `internal/tool/pr_generator.go` | Auto-generate PR descriptions from commits | `PRGenerator`, `Generate()`, `PRDescription`, `CommitSummary` | Custom |
| 56 | Import organizer | `internal/tool/import_organizer.go` | Go AST + TS regex import grouping and sorting | `ImportOrganizer`, `Organize()`, `Group`, `SortImports()` | Custom |
| 57 | Dependency updater | `internal/engine/project/dep_updater.go` | Detect outdated dependencies, plan safe updates | `DepUpdater`, `Check()`, `PlanUpdate()`, `SafetyCheck` | Custom |

---

## 6. Tools & Generation (8 features)

Code generation, plugin infrastructure, and developer tooling.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 58 | Code generation templates | `internal/tool/codegen.go` | 13 templates (CRUD, handlers, tests, etc.) | `CodeGen`, `Generate()`, `Template`, `CRUDTemplate`, `HandlerTemplate` | Custom |
| 59 | Smart file creation | `internal/tool/smart_create.go` | Convention-aware boilerplate generation | `SmartCreate`, `Create()`, `DetectConvention()`, `Boilerplate` | Custom |
| 60 | Plugin system | `internal/plugin/manager.go` | Subprocess plugins with security scanning | `PluginManager`, `Load()`, `Register()`, `SecurityScan()`, `Plugin` | hashicorp/go-plugin |
| 61 | MCP server mode | `internal/mcp/server.go` | Expose hawk as a tool provider via MCP protocol | `MCPServer`, `Serve()`, `RegisterTool()`, `HandleRequest()` | mcp-go |
| 62 | Shell completions | `cmd/completions.go` | bash/zsh/fish completion generators | `Completions`, `GenerateBash()`, `GenerateZsh()`, `GenerateFish()` | cobra |
| 63 | Markdown renderer | `cmd/markdown.go` | Syntax highlighting with streaming support | `MarkdownRenderer`, `Render()`, `Highlight()`, `Stream()` | glamour |
| 64 | Context-aware autocomplete | `cmd/autocomplete.go` | Fuzzy matching suggestions in REPL | `Autocomplete`, `Suggest()`, `FuzzyMatch()`, `Candidate` | Custom |
| 65 | Workflow engine | `internal/engine/workflow/workflow.go` | JSON-defined multi-step automation pipelines | `WorkflowEngine`, `Run()`, `Step`, `Pipeline`, `Condition` | Custom |

---

## 7. Observability & Analytics (8 features)

Monitoring, profiling, and cost analysis for agent operations.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 66 | OTel observability (eyrie) | `eyrie/internal/observability/observability.go` | Spans, metrics, Prometheus export | `Observability`, `StartSpan()`, `RecordMetric()`, `PrometheusExporter` | OpenTelemetry |
| 67 | Provider health checks (eyrie) | `eyrie/internal/health/healthcheck.go` | Periodic provider pinging and status tracking | `HealthChecker`, `Ping()`, `Status`, `ProviderHealth` | Custom |
| 68 | Cache warming (eyrie) | `eyrie/internal/cache/cache_warmer.go` | Keep Anthropic prompt cache hot | `CacheWarmer`, `Warm()`, `Schedule()`, `CacheEntry` | Aider |
| 69 | Telemetry dashboard | `internal/engine/observability/profiler.go` | Box-drawing ASCII analytics display | `Dashboard`, `Render()`, `Chart`, `BoxDraw()`, `Metric` | Custom |
| 70 | Performance profiler | `internal/engine/observability/profiler.go` | P50/P95/P99 latencies, hot path detection | `Profiler`, `Profile()`, `Percentile`, `HotPath`, `Report` | Custom |
| 71 | Quality scorer | `internal/engine/review/quality_scorer.go` | Multi-dimension response quality scoring | `QualityScorer`, `Score()`, `Dimension`, `QualityReport` | Custom |
| 72 | Cost optimizer | `internal/engine/cost/cost_optimizer.go` | Analyze usage patterns, recommend savings | `CostOptimizer`, `Analyze()`, `Recommend()`, `Savings`, `UsagePattern` | Custom |
| 73 | Token compression advisor (tok) | `tok/advisor.go` | Per-content-type strategy recommendations | `CompressionAdvisor`, `Advise()`, `Strategy`, `ContentType` | Custom |

---

## 8. Review & Security (7 features)

Automated code review, static analysis, and vulnerability scanning.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 74 | Static analysis rules (sight) | `sight/static_rules.go` | 35 security and correctness rules | `StaticAnalyzer`, `Analyze()`, `Rule`, `Violation`, `Severity` | semgrep |
| 75 | API security checks (inspect) | `inspect/api_security.go` | CORS, JWT, rate limiting vulnerability checks | `APISecurityChecker`, `Check()`, `CORSCheck`, `JWTCheck` | OWASP |
| 76 | Dependency CVE scanning (inspect) | `inspect/dependency_check.go` | 30+ known vulnerability patterns | `CVEScanner`, `Scan()`, `Vulnerability`, `Advisory` | Custom |
| 77 | Multi-format reporting (inspect) | `inspect/report.go` | HTML/Markdown/JUnit report generation | `Reporter`, `Generate()`, `HTMLReport`, `JUnitReport` | Custom |
| 78 | Code review bot | `internal/engine/review/review_bot.go` | 22 rule-based automated review checks | `ReviewBot`, `Review()`, `ReviewRule`, `Comment`, `Suggestion` | Custom |
| 79 | Inline annotations | `internal/engine/history/annotations.go` | Temporary comments visible only to agent | `Annotator`, `Add()`, `Remove()`, `Annotation`, `Scope` | Custom |
| 80 | CI output formatting (inspect) | `inspect/ci_output.go` | GitHub Actions/GitLab/JSON CI annotations | `CIOutput`, `FormatFindings()`, `formatGitHub()`, `formatGitLab()` | Custom |

---

## 9. Session & Persistence (7 features)

Session storage, search, export, and debugging infrastructure.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 81 | SQLite session storage | `internal/session/sqlite_store.go` | WAL mode, FTS5 indexes, schema migrations | `SQLiteStore`, `Save()`, `Load()`, `Migrate()`, `WALConfig` | OpenCode |
| 82 | Session search | `internal/session/search.go` | BM25 full-text search across sessions | `SessionSearch`, `Search()`, `BM25Score()`, `Result` | Custom |
| 83 | Session export/sharing | `internal/session/export.go` | MD/HTML/JSON/Replay formats + import | `Exporter`, `Export()`, `Import()`, `Format`, `ShareLink` | Custom |
| 84 | Session replay | `internal/session/replay.go` | Re-execute sessions for debugging | `Replayer`, `Replay()`, `Step()`, `Breakpoint`, `ReplayState` | Custom |
| 85 | Conversation search | `internal/session/search.go` | BM25 scoring, regex support, highlights | `ConversationSearch`, `Query()`, `Highlight()`, `RegexMatch` | Custom |
| 86 | Diff renderer (trace) | `trace/cmd/trace/cli/diff_renderer.go` | Word-level diff highlighting | `DiffRenderer`, `Render()`, `WordDiff()`, `ColorScheme` | lazygit |
| 87 | Session timeline (trace) | `trace/cmd/trace/cli/session_timeline.go` | ASCII timeline visualization | `SessionTimeline`, `Render()`, `Event`, `TimelineBar` | Custom |

---

## 10. Agent Personas & Multi-Agent (5 features)

Multi-agent orchestration, personas, and collaborative features.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 88 | Agent personas | `internal/multiagent/agents/persona.go` | YAML frontmatter personas, auto-select by expertise | `Persona`, `Load()`, `Select()`, `Expertise`, `YAMLConfig` | Custom |
| 89 | Inter-agent messaging | `internal/multiagent/messaging.go` | Topic bus, conflict resolution, discovery sharing | `MessageBus`, `Publish()`, `Subscribe()`, `Topic`, `Conflict` | Custom |
| 90 | Task decomposer | `internal/engine/planning/task_decomposer.go` | Break complex tasks into executable sub-steps | `TaskDecomposer`, `Decompose()`, `SubTask`, `DAG`, `Execute()` | Custom |
| 91 | Session services refactor | `internal/engine/session_services.go` | Composed service architecture for sessions | `SessionServices`, `Compose()`, `Service`, `Lifecycle` | OpenCode |
| 92 | Sub-agent spawning | `internal/tool/agent.go` | Explore and general modes for sub-agents | `SubAgent`, `Spawn()`, `ExploreMode`, `GeneralMode`, `Result` | Custom |

---

## 11. Evaluation & Benchmarking (4 features)

Testing, benchmarking, and quality evaluation infrastructure.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 93 | SWE-bench eval framework | `internal/feature/eval/eval.go` | 15 Go coding tasks for agent evaluation | `EvalFramework`, `Run()`, `Task`, `GoTask`, `Score` | SWE-bench |
| 94 | Model benchmark runner | `internal/feature/eval/benchmark.go` | Side-by-side model comparison | `Benchmark`, `Run()`, `Compare()`, `ModelResult`, `Leaderboard` | Custom |
| 95 | Eval reporting | `internal/feature/eval/report.go` | Markdown leaderboard generation | `EvalReporter`, `Generate()`, `Leaderboard`, `Ranking` | Custom |
| 96 | Debug session recorder | `internal/engine/observability/debug_recorder.go` | Capture debugging sessions for replay analysis | `DebugRecorder`, `Record()`, `Capture`, `Playback()` | Custom |

---

## 12. Platform & Config (4 features)

System health, configuration management, and project setup.

| # | Feature | File Path(s) | Description | Key Functions/Types | Inspired By |
|---|---------|-------------|-------------|---------------------|-------------|
| 97 | Health diagnostics | `internal/resilience/health/diagnostics.go` | 18 checks across 5 categories | `Diagnostics`, `RunAll()`, `Check`, `Category`, `HealthReport` | Custom |
| 98 | Env var manager | `internal/config/envmanager.go` | .env parsing, profiles, value masking | `EnvManager`, `Load()`, `Profile`, `Mask()`, `GetEnv()` | Custom |
| 99 | Project scaffolder | `internal/engine/scaffold/scaffold.go` | 6 template types, variable substitution | `Scaffolder`, `Scaffold()`, `Template`, `Variable`, `ProjectType` | Custom |
| 100 | Hook event system | `internal/hooks/events.go` | 21 events, sync/async, subscriptions | `EventSystem`, `Emit()`, `Subscribe()`, `Event`, `Hook` | Custom |

---

## Inspiration Sources

| Source Project | Features Inspired | Category Focus |
|---------------|-------------------|----------------|
| Aider | 8 | Edit matching, lint/test loops, architect mode, repo maps, commits, watch mode, file mentions, cache warming |
| Codex | 5 | Guardian permissions, sandboxing, streaming patches, goal tracking |
| OpenCode | 3 | Permission rules, conversation branching, SQLite sessions |
| GraphRAG | 3 | Community detection, drift search, community search |
| DSPy | 1 | Prompt optimization |
| Claude | 1 | Thinking protocol |
| Continue | 1 | Context providers |
| trufflehog | 1 | Secret detection |
| semgrep | 1 | Static analysis rules |
| OWASP | 1 | API security checks |
| SWE-bench | 1 | Eval framework |
| hashicorp/go-plugin | 1 | Plugin system |
| mcp-go | 1 | MCP server |
| cobra | 1 | Shell completions |
| glamour | 1 | Markdown rendering |
| lazygit | 1 | Diff rendering |
| Custom | 69 | Original implementations |

---

## Architecture Notes

- All features are implemented in Go with zero CGo dependencies (except optional tree-sitter)
- The `yaad/` directory contains the knowledge graph subsystem (separate go module)
- The `eyrie/` directory contains the LLM gateway/router subsystem
- The `sight/` directory contains the static analysis subsystem
- The `inspect/` directory contains the security audit subsystem
- The `trace/` directory contains the session visualization subsystem
- The `tok/` directory contains tokenization utilities (secrets, rate limiting, compression)
- Features are designed to compose: e.g., the auto-test loop (#3) uses diff-aware test selection (#41) and error pattern learning (#26)

---

*Generated for hawk ecosystem reference v1.0 — 100 features across 12 categories*
