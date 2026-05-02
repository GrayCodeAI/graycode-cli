# Observability, Debugging, and Root Cause Analysis: State of the Art Research

Research compiled 2026-05-03. Sources: GitHub repos, API documentation, arXiv papers, and project wikis for 30+ systems.

Goal: determine what hawk should provide so a solo developer can debug issues as effectively as a team with a dedicated SRE.

---

## Table of Contents

1. [OpenTelemetry -- Distributed Tracing/Metrics/Logs](#1-opentelemetry----distributed-tracingmetricslogs)
2. [Jaeger and Tempo -- Distributed Tracing Backends](#2-jaeger-and-tempo----distributed-tracing-backends)
3. [Better Stack -- Log Management](#3-better-stack----log-management)
4. [Highlight.io -- Full-Stack Observability](#4-highlightio----full-stack-observability)
5. [Axiom -- Log Analytics](#5-axiom----log-analytics)
6. [Parca and Pyroscope -- Continuous Profiling](#6-parca-and-pyroscope----continuous-profiling)
7. [AI-Assisted Debugging -- Automated Root Cause Analysis](#7-ai-assisted-debugging----automated-root-cause-analysis)
8. [Time-Travel Debugging -- rr and Replay.io](#8-time-travel-debugging----rr-and-replayio)
9. [AI-Powered Log Analysis -- LogAI and Drain](#9-ai-powered-log-analysis----logai-and-drain)
10. [Error Grouping and Deduplication](#10-error-grouping-and-deduplication)
11. [Anomaly Detection in Metrics and Logs](#11-anomaly-detection-in-metrics-and-logs)
12. [Performance Profiling Integration](#12-performance-profiling-integration)
13. [Memory Leak Detection Automation](#13-memory-leak-detection-automation)
14. [Deadlock and Race Condition Detection](#14-deadlock-and-race-condition-detection)
15. [Git Bisect Automation for Regression Finding](#15-git-bisect-automation-for-regression-finding)
16. [Stack Trace Analysis and Deduplication](#16-stack-trace-analysis-and-deduplication)
17. [Crash Reporting -- Sentry and Bugsnag Patterns](#17-crash-reporting----sentry-and-bugsnag-patterns)
18. [Real User Monitoring Patterns](#18-real-user-monitoring-patterns)
19. [Synthetic Monitoring for Solo Developers](#19-synthetic-monitoring-for-solo-developers)
20. [Automated Performance Regression Detection](#20-automated-performance-regression-detection)
21. [Hawk Integration Strategy -- The Solo SRE](#21-hawk-integration-strategy----the-solo-sre)

---

## 1. OpenTelemetry -- Distributed Tracing/Metrics/Logs

### What it is

OpenTelemetry (OTel) is the CNCF standard observability framework providing vendor-agnostic instrumentation for traces, metrics, and logs. It defines the OTLP wire protocol, language SDKs, a Collector pipeline, and semantic conventions. It is not a backend -- it generates and exports telemetry data to backends like Jaeger, Prometheus, Grafana, or commercial platforms.

### Core signals

| Signal | Go SDK maturity | Purpose |
|--------|----------------|---------|
| Traces | Stable | Request flow across operations. Span trees with parent-child relationships, attributes, events. |
| Metrics | Stable | Quantitative measurements. Counters, histograms, gauges with label dimensions. |
| Logs | Beta | Structured event records. Correlated with traces via trace/span IDs. |

### Architecture

```
Application (SDK) --> OTLP --> Collector --> Backend(s)
                                 |
                          processors (batch, filter, sample)
```

The Collector is a proxy that receives, processes, and exports telemetry. It supports receivers (OTLP, Jaeger, Zipkin), processors (batching, filtering, tail sampling), and exporters (OTLP, Prometheus, Jaeger, file).

### Go SDK

- `go.opentelemetry.io/otel` -- core API
- `go.opentelemetry.io/otel/sdk/trace` -- trace SDK with span processors and exporters
- `go.opentelemetry.io/otel/sdk/metric` -- metric SDK with readers and exporters
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` -- OTLP HTTP exporter
- `go.opentelemetry.io/contrib` -- auto-instrumentation for net/http, gRPC, database/sql, etc.

No auto-instrumentation agent exists for Go (unlike Java). Instrumentation is manual or via contrib libraries wrapping standard packages.

### How it helps debugging

- **Distributed tracing** shows the exact path of a request across service boundaries, tool invocations, and LLM calls. Each span carries timing, status, and contextual attributes.
- **Correlated signals**: a log line carries the trace ID of the operation that produced it, so you can jump from an error log to the full trace.
- **Metrics** detect anomalies (latency spikes, error rate increases) that trigger investigation.

### How hawk should integrate

Hawk already has `trace/otel_sdk.go` with a build-tag-gated OTel SDK integration and `trace/otel.go` with env-var-based configuration (OTEL_EXPORTER_OTLP_ENDPOINT, HAWK_CODE_ENABLE_TELEMETRY). This is the right foundation. Extensions needed:

1. **Wire OTel spans into SessionTrace**: The built-in `SessionTrace` tree and the OTel span tree should be a single system. When the `otel` build tag is active, `SessionTrace.StartSpan` should create real OTel spans that propagate context. When inactive, the existing lightweight spans suffice.
2. **Instrument all agent operations**: Every LLM call (`api.chat`), tool execution (`tool.Bash`, `tool.Edit`), compaction, and permission check should be a span. The span helpers in `trace/spans.go` already cover the main ones -- extend to all 40 tools.
3. **Export to local file by default**: For a solo dev, running a Collector is overkill. Export OTLP JSON to `~/.hawk/traces/` so the agent can read its own traces for analysis. Add a `hawk traces` subcommand that renders them.
4. **Metrics for agent health**: Track counters (tool_calls_total, llm_errors_total, cache_hits), gauges (active_goroutines, pending_tool_calls), and histograms (llm_latency, tool_duration) using the existing `metrics/` package, with optional OTel export.
5. **Log correlation**: The existing `logger/` package should attach trace IDs and span IDs to every log line when tracing is active.

---

## 2. Jaeger and Tempo -- Distributed Tracing Backends

### Jaeger

- **Repo**: github.com/jaegertracing/jaeger (22.7k stars)
- **Latest**: v2.17.0 (March 2026). V2 is a complete rewrite built on OTel Collector.
- **Architecture**: OTel SDKs send traces via HTTP/gRPC to Jaeger Collector, which stores in Elasticsearch, Cassandra, Kafka, or Badger (embedded). Query service serves the UI.
- **Key features**: Service dependency graphs, trace comparison, adaptive sampling, deep search with tag filters.
- **V2 change**: Jaeger v2 IS the OTel Collector with Jaeger-specific extensions. No separate Jaeger client libraries needed -- use OTel SDK directly.

### Grafana Tempo

- **Repo**: github.com/grafana/tempo (5.2k stars)
- **Latest**: v2.10.5 (April 2026)
- **Architecture**: Object storage only (S3, GCS, Azure Blob, local disk). No indexing database required.
- **TraceQL**: A traces-first query language inspired by PromQL. Supports span-level queries, structural queries (parent-child), and aggregation.
- **Key features**: TraceQL metrics (ad-hoc aggregation over traces), Apache Parquet storage, Traces Drilldown UI, automatic RED metrics.
- **OTel integration**: Wire format and storage are OTel-native. Accepts Jaeger, Zipkin, and Kafka formats too.

### How they help debugging

- **Jaeger** excels at trace visualization and service dependency mapping. Best for understanding multi-service request flows.
- **Tempo** excels at cost-efficient storage (no index DB) and TraceQL for querying traces by arbitrary span attributes. Best for high-volume trace storage and ad-hoc analysis.

### How hawk should integrate

For a solo developer's coding agent, neither Jaeger nor Tempo should be a hard dependency. Instead:

1. **OTLP export compatibility**: Hawk's OTel integration already exports to any OTLP endpoint. If a developer runs Jaeger or Tempo locally (e.g., via Docker), hawk traces appear automatically.
2. **Built-in trace viewer**: For developers who do not run a backend, hawk should render trace trees in the terminal. The `SessionTrace.FormatTree()` method already does this -- extend it with color coding, filtering by duration/status, and span detail expansion.
3. **TraceQL-inspired local queries**: Implement a lightweight query syntax for filtering local traces: `hawk traces --where "tool.name=Bash AND duration>5s"`. This gives 80% of TraceQL's value with zero infrastructure.
4. **Trace comparison**: `hawk traces diff <session1> <session2>` to compare execution paths between two runs of the same task. Inspired by Jaeger's trace comparison.

---

## 3. Better Stack -- Log Management

### What it is

Better Stack (formerly Logtail) is a cloud log management platform built on a purpose-built data warehouse. Claims 30x cheaper than Datadog. Key differentiators: OTel-native ingestion, live-tail query language, VRL (Vector Remap Language) for data transformation, and an AI SRE for agentic root cause analysis.

### Key features

- **Wide events vs. metrics**: Stores high-cardinality structured events rather than pre-aggregated metrics. Allows querying any attribute at query time.
- **AI SRE**: Automated root cause analysis agent that correlates logs, traces, and metrics to identify the underlying cause of incidents.
- **Live tail**: Real-time log streaming with query filtering.
- **Alerting**: Threshold-based and anomaly-driven alerts.

### How it helps debugging

- The "wide events" model is relevant: rather than storing pre-defined metrics, store rich structured events and let the query engine find patterns. This is more flexible for unexpected debugging scenarios.
- The AI SRE concept (automated root cause analysis from logs) is directly applicable to hawk.

### How hawk should integrate

1. **Structured event logging**: Hawk's `logger/` package should emit structured JSON events to `~/.hawk/events.jsonl`, not just text logs. Each event carries: timestamp, level, trace_id, span_id, session_id, tool_name, and arbitrary attributes.
2. **Local log query engine**: `hawk logs --query "level=error AND tool=Bash AND session=abc"` with time range filtering. Implement using SQLite FTS5 or simple JSONL scanning for small volumes.
3. **OTLP log export**: When OTel is enabled, forward logs to the configured endpoint so they appear in Better Stack, Grafana Loki, or any OTLP-compatible backend.
4. **AI SRE for hawk**: When the user runs `/debug` or `/bughunter`, hawk should automatically collect recent error logs, traces, and metrics, feed them to the LLM, and produce a root cause analysis. This is hawk's version of Better Stack's AI SRE.

---

## 4. Highlight.io -- Full-Stack Observability

### What it is

- **Repo**: github.com/highlight/highlight (9.2k stars)
- Open-source full-stack monitoring: session replay, error monitoring, logging, and tracing in one platform.
- **Session replay**: DOM-based recording using rrweb, capturing browser interactions, network requests, and console logs.
- **Error monitoring**: Customizable error grouping with embedded session replay context.
- **Stack**: TypeScript (70.9%), Go (15.8%), JavaScript.

### Key innovation

The unified product -- session replay + errors + logs + traces in one view. When investigating an error, you see the user's exact actions, the network requests, the server-side traces, and the logs, all correlated by session/request ID.

### How it helps debugging

- **Replay context**: Seeing what the user did before an error eliminates guesswork about reproduction steps.
- **Full-stack correlation**: Jumping from a frontend error to its backend trace to the relevant log line, in one tool.

### How hawk should integrate

Session replay is not relevant to a CLI coding agent. But the correlation concept is critical:

1. **Session replay analog**: Hawk already records sessions as JSONL. Extend session playback (`hawk sessions replay <id>`) to show a timeline of: user prompts, agent responses, tool calls with inputs/outputs, LLM calls with latency/tokens, errors with context. This is hawk's "session replay."
2. **Error-to-context linking**: When hawk encounters an error (tool failure, LLM error, build failure), automatically capture the surrounding context: the last 5 tool calls, the current conversation state, the git diff, the file being edited. Store this as a structured error event. This is the coding-agent equivalent of Highlight's error-with-session-context.
3. **Unified timeline view**: `/timeline` command that shows a chronological interleaving of all signals -- prompts, tool calls, LLM responses, errors, file changes, git operations -- for the current session.

---

## 5. Axiom -- Log Analytics

### What it is

Axiom is a data platform for event and telemetry data at scale, serving 30k+ organizations. Key differentiators: extreme compression (25-50x), serverless querying via ephemeral runtimes, and separation of storage and compute.

### Key features

- **EventDB**: Seamless event ingestion with extreme compression.
- **MetricsDB**: High-cardinality time-series without performance degradation.
- **APL (Axiom Processing Language)**: KQL-compatible query language for log analysis.
- **Anomaly-driven alerts**: Automatic anomaly detection alongside threshold-based alerting.
- **Cost efficiency**: Dramatically lower ingestion and storage costs via columnar compression.

### How it helps debugging

- The extreme compression model matters for a solo dev: you can store months of agent telemetry locally without disk concerns.
- Anomaly-driven alerts are more useful than threshold alerts for a solo dev who cannot manually set thresholds for every metric.

### How hawk should integrate

1. **Columnar local storage**: For long-term trace/metrics storage, use a columnar format (Parquet via go-parquet, or SQLite with proper schema) rather than raw JSONL. This enables efficient queries over months of data while keeping disk usage minimal.
2. **Anomaly-driven alerts**: When hawk detects that a session's error rate, latency, or cost is statistically anomalous compared to the user's historical baseline, surface a warning automatically. No manual threshold configuration needed.
3. **APL-inspired query for local data**: The local query engine should support filtering, aggregation, and time-series functions over stored events. Model after APL/KQL syntax which developers already know.

---

## 6. Parca and Pyroscope -- Continuous Profiling

### Parca

- **Repo**: github.com/parca-dev/parca (4.9k stars)
- Continuous profiling via eBPF. Zero-instrumentation profiling of CPU and memory down to line number.
- **eBPF agent**: Automatically discovers targets from Kubernetes or systemd. Profiles C, C++, Rust, Go without code changes.
- **Storage**: Custom columnar storage with compression, retaining raw profile data.
- **Differentiator**: Compare profiles across time to identify performance regressions between deployments.

### Pyroscope (Grafana)

- **Repo**: github.com/grafana/pyroscope (11.4k stars)
- Continuous profiling with Grafana integration. Supports push (SDK) and pull (Alloy agent with eBPF) modes.
- **Language SDKs**: Go, Java, Python, Ruby, Node.js, .NET, Rust.
- **Go profiling**: Uses `runtime/pprof` under the hood. Periodically collects CPU, heap, goroutine, mutex, and block profiles.
- **Explore Profiles UI**: Queryless interface for browsing profiles. Flame graphs, diff flame graphs.
- **Key feature**: Line-level debugging -- identify the exact line of code causing CPU or memory issues.

### How they help debugging

- **CPU hotspots**: Identify functions consuming excessive CPU without manual profiling.
- **Memory leaks**: Track heap growth over time, pinpoint allocation sites.
- **Goroutine leaks**: Detect goroutine count growth, identify blocked goroutines.
- **Cross-version comparison**: Diff flame graphs between before/after a change to verify optimization or detect regression.

### How hawk should integrate

Hawk already has `profile/profile.go` with CPU, memory, and goroutine profiling using Go's `runtime/pprof`. Extensions:

1. **Continuous self-profiling**: Optionally enable continuous profiling of the hawk process itself. Collect CPU and heap profiles every 30s during a session. Store in `~/.hawk/profiles/`. This lets the agent diagnose its own performance issues.
2. **Profile analysis tool**: `hawk profile analyze <file>` that reads a pprof file and uses the LLM to explain the hotspots in plain English. "The function tool.executeBash is consuming 45% of CPU, primarily in subprocess spawning."
3. **User project profiling integration**: When hawk runs tests or builds, optionally collect profiles of the user's application. Expose via `/profile` command: "Profile the last test run and explain the CPU hotspots."
4. **Pyroscope/Parca export**: When continuous profiling is enabled and an OTLP endpoint is configured, export profiles in pprof format to Pyroscope or Parca for long-term storage and flame graph visualization.
5. **Diff flame graphs in terminal**: When comparing two profiling sessions, render a simplified diff (functions that got slower/faster) in the terminal.

---

## 7. AI-Assisted Debugging -- Automated Root Cause Analysis

### AutoSD -- Automated Scientific Debugging (arXiv:2304.02195)

The most relevant paper for hawk's debugging capabilities.

- **Technique**: Uses LLMs to perform the scientific debugging method -- hypothesis generation, active investigation (running tests, inspecting variables), and conclusion.
- **Process**: (1) Given buggy code and failing tests, LLM generates hypotheses about the bug cause. (2) System actively investigates by running modified tests or inserting print statements. (3) LLM reaches a conclusion and generates a patch with an explanation.
- **Key finding**: Matches established program repair baselines across three benchmarks. Human study (20 participants, 6 professional developers) showed 70% of developers want explanations alongside patches. Accuracy improved on 5/6 real-world bugs when explanations were provided.
- **Insight**: Explanations are not optional -- they are essential for developer trust in automated fixes.

### SWE-bench (arXiv:2310.06770)

- **Benchmark**: 2,294 real GitHub issues from 12 Python repos. Tests LLM ability to understand codebases and fix bugs.
- **Finding**: At time of publication, Claude 2 resolved only 1.96% of issues. The task requires multi-file reasoning, test understanding, and coordinated changes.
- **Relevance**: Demonstrates that automated debugging at the repository level is hard but tractable with the right tools and agent scaffolding.

### iAudit -- Agent-Based Vulnerability Detection (arXiv:2403.16073)

- **Technique**: Two-stage approach: (1) fine-tuned model for fault detection, (2) separate model for explanation generation, refined by Ranker and Critic agents.
- **Finding**: 91.21% F1 on vulnerability detection, but only 38% consistency on root cause explanations. Detection is easier than explanation.
- **Insight**: Use specialized models for detection, then a reasoning model for explanation. The two tasks have different requirements.

### How it helps debugging

AI-assisted debugging transforms the solo dev experience by:
- Automating the hypothesis-test-conclude cycle that developers do manually
- Providing explanations alongside fixes (the AutoSD insight)
- Scaling to multi-file, multi-system problems that would take a single developer hours

### How hawk should integrate

This is hawk's highest-value debugging feature. Implementation plan:

1. **`/debug` command -- Automated Scientific Debugging loop**:
   - Input: failing test, error message, or user description of the bug.
   - Step 1: Gather context -- read the failing test, the relevant source files (via repomap), recent git changes, and error logs.
   - Step 2: LLM generates 3-5 hypotheses about the root cause.
   - Step 3: For each hypothesis, hawk automatically runs an investigation (add debug logging, run subset of tests, inspect variable values) using tool calls.
   - Step 4: LLM evaluates investigation results, narrows to most likely cause, and generates a fix with explanation.
   - Step 5: Run tests to verify the fix. If tests pass, present the fix and explanation. If not, loop.
2. **Always explain**: Every automated fix must include a plain-English explanation of WHY the bug occurred, not just what was changed. This follows the AutoSD finding that developers need explanations.
3. **Confidence scoring**: The agent should express confidence in its diagnosis. "High confidence: the null pointer dereference is caused by the unchecked error on line 42" vs. "Low confidence: the race condition may involve the cache or the database connection pool."
4. **Investigation trace**: Record every hypothesis, investigation step, and result as spans in the session trace. This creates an audit trail the developer can review.

---

## 8. Time-Travel Debugging -- rr and Replay.io

### rr (Record and Replay)

- **Repo**: github.com/rr-debugger/rr (10.5k stars)
- **Latest**: v5.9.0 (February 2025)
- **Technique**: Records a program's execution (all system calls, signals, non-deterministic inputs) at the OS level. Replays the exact same execution deterministically. Extends gdb with reverse-execution (step backward, reverse-continue, reverse watchpoints).
- **Platform**: Linux kernel 4.7+. Intel Nehalem+ or select AMD Zen+, some AArch64 (Apple M-series).
- **Performance**: Low overhead recording (typically 1.2-2x slowdown). Replay is deterministic -- same behavior every time.
- **Key innovation**: Hardware performance counters for precise recording. Reverse execution lets you start at a crash and work backward to the cause.

### Replay.io

- **What**: Time-travel debugging for web applications. Records browser execution (DOM, network, JS) and lets developers step through the recording in a debugger.
- **Integration**: Cypress, Playwright, Puppeteer plugins for recording test runs. When a test fails, developers get a full recording of the browser state.
- **Differentiator**: Browser-level recording (not OS-level like rr). Designed for frontend/full-stack debugging.

### How they help debugging

- **Eliminates "cannot reproduce"**: Record the failing execution once, replay it unlimited times.
- **Reverse debugging**: Start from the symptom (crash, wrong value) and work backward to the cause. This is the opposite of traditional debugging (set breakpoints, run forward, hope to hit the right spot).
- **CI integration**: Record test runs in CI. When a test fails intermittently, you have a recording of the exact failure.

### How hawk should integrate

1. **`hawk debug record` -- Record-and-replay wrapper**: Wrap rr for recording test executions. `hawk debug record go test ./...` records the test run. If a test fails, hawk has a recording it can analyze.
2. **Automated reverse debugging**: When hawk has an rr recording of a crash, it can drive gdb/rr programmatically:
   - Set a breakpoint at the crash site
   - Reverse-continue to the point where the bad value was written
   - Inspect the state and report to the developer
   This turns a multi-hour manual debugging session into a 30-second automated one.
3. **Test failure recording in CI**: `hawk ci record` wraps test commands with rr. Failed tests produce recordings that hawk can analyze locally.
4. **Delve integration for Go**: For Go specifically, integrate with Delve (github.com/go-delve/delve, 24.7k stars, v1.26.3). Delve supports breakpoints, goroutine inspection, core dump analysis. Hawk can drive Delve programmatically to investigate failures.
5. **Browser recording for web projects**: For web projects, integrate Replay.io's Playwright plugin. When end-to-end tests fail, hawk has a browser recording to analyze.

---

## 9. AI-Powered Log Analysis -- LogAI and Drain

### LogAI (Salesforce)

- **Repo**: github.com/salesforce/logai (790 stars)
- **What**: Python library for log analysis using traditional ML and deep learning.
- **Techniques**:
  - **Parsing**: Drain algorithm for automatic log template extraction.
  - **Anomaly detection**: Time-series methods (ETS), One-class SVM for semantic anomaly detection, deep learning models.
  - **Clustering**: K-means and semantic clustering of log messages.
- **Data model**: Adopts OpenTelemetry's data model for cross-platform compatibility.
- **Key innovation**: Unified interface across statistical, ML, and DL approaches. Benchmark suite for comparing algorithms.

### Drain3 (IBM/logpai)

- **Repo**: github.com/logpai/Drain3 (797 stars)
- **What**: Online log template miner. Extracts templates from streaming log messages in real-time.
- **Algorithm**: Fixed-depth parse tree (default depth 4, max 100 children per node). New log messages traverse the tree by token; if similarity to an existing cluster exceeds threshold (default 0.4), the message is added to that cluster. Otherwise, a new template is created.
- **Example**: "user johndoe logged in" and "user jane logged in" both match template "user <*> logged in".
- **Persistence**: Save/load state to Kafka, Redis, or files. Supports inference-only mode.
- **Key features**: Regex masking for IPs, numbers, paths. LRU cache for memory efficiency.

### How they help debugging

- **Log parsing**: Converts unstructured logs into structured templates, enabling pattern matching and statistical analysis.
- **Anomaly detection**: Identifies log patterns that deviate from normal behavior (new error types, frequency spikes, missing expected patterns).
- **Root cause correlation**: Clusters related log messages to identify cascading failures.

### How hawk should integrate

1. **Drain-style log parsing in Go**: Implement a lightweight Drain parser in Go (the algorithm is simple: fixed-depth tree with similarity matching). Use it to parse build output, test output, and application logs into templates.
2. **Automatic error pattern extraction**: When a user's tests fail or build breaks, hawk parses the output, extracts error templates, and compares against known patterns:
   - "compilation error: undefined: X" --> missing import or declaration
   - "panic: runtime error: index out of range [X] with length Y" --> bounds check failure
   - "FAIL TestX (Y.YYs)" --> specific test failure
3. **Anomaly detection for recurring issues**: Track log templates across sessions. Alert when a new error pattern appears or an existing pattern increases in frequency.
4. **Semantic log clustering**: Group related error messages to identify the single root cause behind multiple symptoms. "Connection refused" + "timeout waiting for response" + "service unavailable" --> "upstream service is down."

---

## 10. Error Grouping and Deduplication

### Sentry's approach (43.8k stars)

Sentry's error grouping is the industry standard. The algorithm:

1. **Fingerprint generation** (priority order):
   - Custom fingerprint (if explicitly set)
   - Stack trace (primary default)
   - Exception type + value (fallback)
   - Message (final fallback)

2. **Stack trace normalization**:
   - Filter to in-app frames only (exclude third-party libraries)
   - Normalize filenames (remove revision hashes, build paths)
   - Normalize context lines (clean up source code references)
   - Hash the normalized frames to produce a fingerprint

3. **AI-enhanced grouping** (newer addition):
   - Generate vector embeddings of error messages and in-app stack frames using a transformer model
   - Compare against existing project errors via cosine similarity
   - Merge semantically similar errors within a configured threshold
   - Only applies to events using default fingerprinting

4. **Customization hierarchy** (least to most effort):
   - Manual issue merging
   - Fingerprint rules (project-level pattern matchers)
   - Stack trace rules (frame-level modification)
   - SDK fingerprinting (in-code)

### Bugsnag's approach

- Groups errors by error class + top in-app stack frame
- Simpler than Sentry but effective for most cases
- Custom grouping via callbacks in the SDK

### How it helps debugging

- **Reduces noise**: 1000 identical errors become 1 issue with a count. The developer sees the problem once, not 1000 times.
- **Identifies patterns**: Grouping reveals which errors are actually the same root cause vs. which are different.
- **Prioritization**: Issues ordered by frequency, user impact, and recency.

### How hawk should integrate

1. **Error fingerprinting for tool failures**: When hawk encounters errors (build failures, test failures, runtime errors), compute a fingerprint using the Sentry algorithm:
   - Parse stack traces from error output
   - Filter to in-app frames
   - Normalize file paths and line numbers
   - Hash to produce a fingerprint
2. **Cross-session deduplication**: Track error fingerprints across sessions. When the same error appears again, show: "This error has occurred 3 times across 2 sessions. Last seen: 2 hours ago." This prevents the developer from re-investigating known issues.
3. **Semantic similarity for novel errors**: When a new error does not match any existing fingerprint exactly, use LLM-based semantic similarity to find the closest known error. "This error is similar to issue #X from session Y -- same root cause (null pointer in database connection pool)."
4. **Error database**: Store error fingerprints, first/last occurrence, frequency, and resolution status in `~/.hawk/errors.db` (SQLite). The `/errors` command shows a dashboard of known errors, sorted by frequency.

---

## 11. Anomaly Detection in Metrics and Logs

### Techniques from research

**Statistical methods**:
- **Z-score / modified Z-score**: Flag values > 3 standard deviations from mean. Simple, fast, works for normally distributed metrics.
- **IQR (Interquartile Range)**: Flag values outside Q1 - 1.5*IQR to Q3 + 1.5*IQR. Robust to outliers in the baseline.
- **Exponential smoothing (ETS)**: Model expected value based on recent trend. Deviation from expected = anomaly. Used by LogAI.
- **Seasonal decomposition (STL)**: Separate trend, seasonal, and residual components. Anomalies are large residuals. Good for metrics with daily/weekly patterns.

**ML methods**:
- **Isolation Forest**: Randomly partition data; anomalies are isolated in fewer partitions. O(n log n), no label requirement.
- **One-class SVM**: Learn a boundary around normal data. Used by LogAI for semantic log anomaly detection.
- **LSTM autoencoders**: Learn to reconstruct normal sequences. High reconstruction error = anomaly. Best for time-series with complex patterns.

**For a coding agent**:
The most useful anomaly signals are:
| Signal | Method | Trigger |
|--------|--------|---------|
| LLM latency spike | ETS + Z-score | "API latency is 3x normal" |
| Error rate increase | Sliding window comparison | "Error rate jumped from 2% to 15%" |
| Cost anomaly | Z-score on session cost | "This session is 4x more expensive than average" |
| Token usage spike | IQR | "Prompt size grew unexpectedly" |
| Test failure rate | Proportion test | "Tests that normally pass are now failing" |

### How hawk should integrate

1. **Baseline collection**: After 10+ sessions, hawk has enough data to compute baselines for: session cost, LLM latency, error rate, tool execution time, token usage. Store in `~/.hawk/baselines.json`.
2. **Automatic anomaly detection**: On each LLM call and tool execution, compare against baseline. Use modified Z-score (robust, simple, zero-dependency). Surface warnings when anomalies are detected: "Warning: LLM latency is 3.2x your 30-day average. This may indicate provider degradation."
3. **Anomaly-as-context**: Feed detected anomalies to the LLM as context when debugging: "Note: 3 anomalies detected in this session: (1) Build time 2x normal, (2) Test failure rate 40% vs. baseline 5%, (3) Memory usage growing. These may be related."
4. **`/anomalies` command**: Show detected anomalies for the current session and recent sessions.

---

## 12. Performance Profiling Integration

### Go profiling ecosystem

**pprof** (built-in):
- CPU, heap, goroutine, mutex, block, threadcreate profiles
- `runtime/pprof` for programmatic collection
- `net/http/pprof` for HTTP endpoint
- `go tool pprof` for analysis (flame graphs, top functions, source view)

**Delve** (github.com/go-delve/delve, 24.7k stars):
- Go-specific debugger with goroutine-aware stepping
- Core dump analysis
- Conditional breakpoints
- IDE integration (VS Code, GoLand)
- Latest: v1.26.3 (April 2026)

**Datadog dd-trace-go** (845 stars):
- Continuous profiling: periodic CPU, heap, goroutine, mutex collection
- Integrated with distributed tracing
- Profiles annotated with trace context

**perf** (Linux):
- System-wide CPU profiling via hardware performance counters
- Call graph recording
- Works with any language/binary
- Low overhead (1-5%)

### How it helps debugging

- **CPU profiling** identifies slow functions and hot loops
- **Heap profiling** finds memory-hungry allocations
- **Goroutine profiling** detects goroutine leaks and scheduling issues
- **Mutex profiling** finds contention points
- **Integrated profiling + tracing** correlates performance issues with specific requests

### How hawk should integrate

Hawk's `profile/profile.go` already provides CPU, memory, and goroutine profiling. Extend:

1. **`/profile` command in REPL**: `/profile cpu start` begins CPU profiling. `/profile cpu stop` stops and analyzes. The agent reads the pprof output and explains: "Top CPU consumers: (1) regexp.Compile in tool/grep.go:142 -- 23% (consider pre-compiling), (2) json.Marshal in session/save.go:89 -- 15% (consider streaming encoder)."
2. **Automatic profiling on slow operations**: When a tool call takes >10s or an LLM call takes >30s, automatically capture a goroutine profile and a brief CPU profile. Attach to the span as metadata.
3. **Project profiling**: `hawk profile run "go test -bench ."` wraps the user's command with profiling, then analyzes the results. This gives the solo dev a one-command profiling workflow.
4. **Benchmark regression detection**: `hawk profile compare <before.pprof> <after.pprof>` diffs two profiles and highlights functions that got slower. Pairs with git bisect automation (section 15).

---

## 13. Memory Leak Detection Automation

### Techniques

**Heap growth tracking**:
- Take heap profiles at regular intervals
- If heap_in_use grows monotonically over N intervals without GC reclaiming, flag as potential leak
- Identify the allocation sites (functions) responsible for the growth

**Go-specific signals**:
- `runtime.MemStats.HeapInuse` growing while `HeapReleased` is flat
- Object count growing without bound (check via heap profile object counts)
- Goroutine count growing (goroutine leak = memory leak)
- Finalizer queue growing (objects with finalizers not being GC'd)

**Sanitizers** (C/C++/Go):
- **AddressSanitizer (ASan)**: Detects use-after-free, buffer overflow. Supported in Clang, GCC, Go (via CGO).
- **LeakSanitizer (LSan)**: Detects memory leaks at program exit. Part of ASan.
- **Go's built-in race detector**: `go test -race` detects data races (not leaks, but related).

### How hawk should integrate

1. **`/leakcheck` command**: Takes two heap profiles (start of session and current), diffs them, identifies allocation sites with monotonic growth. Uses the LLM to explain: "The buffer pool in net/http.Transport is growing because connections are not being closed. Consider setting Transport.IdleConnTimeout."
2. **Automatic goroutine leak detection**: Track goroutine count via `runtime.NumGoroutine()` at regular intervals. If count grows by >10 per minute sustained over 5 minutes, alert: "Goroutine leak detected. Current: 847 (was 12 at session start). Run /profile goroutine for details."
3. **Integrate with test runs**: `hawk test --leakcheck` runs the user's tests with heap profiling before and after, reporting any allocation growth.
4. **Self-monitoring**: Hawk monitors its own memory usage. If hawk's own heap exceeds a threshold (e.g., 500MB), alert and suggest compaction or session cleanup.

---

## 14. Deadlock and Race Condition Detection

### Techniques

**Go race detector** (`-race` flag):
- Instruments memory accesses at compile time
- Detects data races (concurrent unsynchronized access to shared memory)
- 5-10x slowdown, 5-10x memory overhead
- Exits with detailed report: goroutine stacks, memory address, read/write operations

**ThreadSanitizer (TSan)**:
- Available for C++, Go (via CGO), and other languages
- Detects data races and deadlocks
- Part of LLVM/GCC sanitizer suite

**Static analysis for deadlocks**:
- Go's `go vet` detects some concurrency issues
- `staticcheck` detects mutex misuse, unlocked access
- `golang.org/x/tools/go/analysis` framework for custom analyzers

**Dynamic deadlock detection**:
- Lock-order graph: track the order in which locks are acquired. If A->B and B->A orderings exist, deadlock is possible.
- Go's `runtime` can detect some deadlocks ("all goroutines are asleep - deadlock!")
- Mutex profiling (`runtime/pprof` mutex profile) shows contention hotspots

### How hawk should integrate

1. **`hawk test --race` default**: When running Go tests, default to enabling the race detector. Parse race detector output and explain findings: "Data race in cache.go:89: goroutine 7 writes cache.entries while goroutine 12 reads it. Add a sync.RWMutex or use sync.Map."
2. **Automated race report analysis**: When a race is detected, hawk:
   - Reads the two goroutine stacks from the report
   - Identifies the shared variable
   - Reads the relevant source code
   - Suggests a fix (mutex, atomic, channel, sync.Map)
3. **Deadlock explanation**: When Go's runtime reports "all goroutines are asleep - deadlock!", hawk:
   - Captures the goroutine dump
   - Identifies which goroutines are blocked and on what
   - Traces the lock dependency chain
   - Explains the cycle and suggests a fix
4. **Static analysis integration**: Run `go vet` and `staticcheck` automatically before test execution. Parse and present findings as part of the debugging context.

---

## 15. Git Bisect Automation for Regression Finding

### Technique

`git bisect` performs binary search across commit history to find the first commit that introduced a regression. Given a "good" and "bad" commit, it checks out the midpoint, runs a test, and narrows based on pass/fail. Time complexity: O(log n) for n commits.

**Automation**: `git bisect run <test-command>` fully automates the process. The test command must exit 0 for good, 1-124/126-127 for bad, 125 for skip.

### Existing tools

- `git bisect run` -- built-in automation
- `git bisect start --first-parent` -- follow only merge commits (faster for trunk-based development)
- `gh bisect` (GitHub CLI extension) -- bisect with PR awareness

### How hawk should integrate

1. **`/bisect` command**: Fully automated regression finding.
   - Input: A test command that currently fails (or a description of the regression).
   - Step 1: Hawk determines the "bad" commit (HEAD) and finds a "good" commit by running the test against recent commits (starting from the last known good state, or searching backward).
   - Step 2: Runs `git bisect run <test-command>` with the test the user provided or one hawk writes based on the regression description.
   - Step 3: Reports the first bad commit with its diff, message, and author.
   - Step 4: Analyzes the commit diff to explain what change caused the regression.
2. **Intelligent test generation**: If the user describes a regression but does not have a test for it ("the API response changed"), hawk writes a minimal test script that reproduces the issue, then uses it for bisect.
3. **Bisect with build caching**: For compiled projects, cache build artifacts per commit hash to avoid rebuilding the entire project at each bisect step.
4. **Integration with error fingerprints**: If an error fingerprint appeared in a specific session, and that session's git state is recorded (hawk already stores git branch per session), hawk can narrow the bisect range to only the commits between the last good session and the current bad one.

---

## 16. Stack Trace Analysis and Deduplication

### Techniques

**Frame normalization** (from Sentry):
1. Remove memory addresses and thread IDs (non-deterministic)
2. Normalize file paths (remove build directories, home paths)
3. Remove line numbers for grouping (or use them only as secondary signal)
4. Filter to in-app frames (remove stdlib, third-party)
5. Hash the normalized frame sequence

**Semantic deduplication**:
- Two stack traces with different line numbers but the same function call chain are the same error
- Two stack traces with the same function chain but different error messages may be different errors
- Use the deepest in-app frame as the primary grouping key

**Go-specific stack trace analysis**:
- Goroutine ID and state (running, chan receive, select, sleep, etc.)
- Goroutine creator chain (which goroutine spawned this one)
- Panic stack traces include the full goroutine dump
- Race detector output includes two goroutine stacks with the conflicting access

### How hawk should integrate

1. **Automatic stack trace parsing**: When hawk encounters a stack trace in tool output (build error, test failure, runtime panic, race report), automatically parse it into structured data: frames (file, function, line), goroutine state, error type/message.
2. **Fingerprinting**: Compute a fingerprint from the normalized in-app frames. Store in the error database.
3. **Stack trace explanation**: Feed the parsed stack trace to the LLM with the relevant source code for each frame. Produce a narrative: "The panic occurred in handler.go:89 (ServeHTTP). The nil pointer is db.conn, which was not initialized because the Setup() function on line 23 was never called. This happens when the server starts before the database connection is established."
4. **Cross-session stack trace matching**: When the same stack trace fingerprint appears across sessions, show the history: "This panic has occurred 5 times. First seen 3 days ago. Last attempted fix in session abc123 did not resolve it."
5. **Goroutine dump analysis**: Parse `goroutine dump` output from panics. Identify blocked goroutines, lock contention, and goroutine leak patterns. Present as a summary table rather than raw text.

---

## 17. Crash Reporting -- Sentry and Bugsnag Patterns

### Sentry patterns (43.8k stars)

**Architecture**:
- Client SDKs capture crashes, exceptions, and breadcrumbs
- Events sent to Sentry server with contextual data (device, OS, user, breadcrumbs)
- Server groups events into issues (see section 10)
- Issues have status (unresolved, resolved, ignored), assignment, and history

**Key patterns applicable to hawk**:
1. **Breadcrumbs**: A trail of events leading up to the crash (user actions, API calls, navigation, console logs). Limited to last N events. This is the most valuable debugging context.
2. **Release tracking**: Associate errors with specific versions/commits. Know which release introduced an error.
3. **Suspect commits**: Correlate errors with recent commits that touched the relevant code paths.
4. **Performance transactions**: Group spans into transactions that represent user-visible operations.

**SDK design patterns**:
- Automatic capture of uncaught exceptions
- Configurable before-send hooks for scrubbing PII
- Offline event buffering (when network is unavailable)
- Rate limiting (max events per second)
- Session tracking (crash-free session rate)

### How hawk should integrate

Hawk already has crash recovery (`cmd/errors.go` with `panicRecovery`, `signalHandler`, `errorLoggerT`). Extend with Sentry-inspired patterns:

1. **Breadcrumbs**: Maintain a ring buffer of the last 100 significant events (tool calls, file edits, git operations, LLM calls, user prompts). When a crash or error occurs, attach the breadcrumbs to the error report. This is the "what happened before" context.
2. **Release tracking**: Associate each error with hawk's version and the user's project commit. When hawk is updated, clear resolved errors and check if previously reported errors still occur.
3. **Suspect commits**: When an error occurs during or after file edits, automatically identify the recent changes that might have caused it. `git diff HEAD~5` + error stack trace = suspect commit analysis.
4. **Crash-free session rate**: Track what percentage of sessions complete without errors. Surface as a metric in `/doctor` or `/metrics`. A declining crash-free rate triggers investigation.
5. **Structured error events**: Replace the current text-based `~/.hawk/error.log` with structured JSONL events containing: timestamp, error type, message, stack trace, breadcrumbs, session ID, git state, file context.

---

## 18. Real User Monitoring (RUM) Patterns

### What RUM does

RUM collects performance and behavior data from real user sessions: page load times, interaction latency, JavaScript errors, network requests, and user flows. Key metrics: Core Web Vitals (LCP, FID, CLS), TTFB, TTI.

### Relevance to a coding agent

A coding agent does not have "users" in the browser sense, but the same principles apply to monitoring the developer's experience:

| RUM Metric | Hawk Equivalent | Why it matters |
|-----------|----------------|----------------|
| Page load time | Session startup time | Developer waits for hawk to be ready |
| Time to Interactive | Time to first response | How long until hawk answers the first prompt |
| Interaction latency | TTFT (time to first token) | Perceived responsiveness |
| Error rate | Tool failure rate | Reliability of hawk's operations |
| Session duration | Session duration | Understanding usage patterns |
| User flow | Tool call sequences | Understanding how developers use hawk |

### How hawk should integrate

1. **Developer Experience Metrics (DXM)**: Track the developer-facing performance metrics:
   - Startup time (already in `cmd/performance.go` via `startupProfile`)
   - Time to first token (already in `cmd/profiler.go` via `QueryProfile.RecordTTFT`)
   - Tool execution latency per tool type
   - End-to-end query time (prompt to final response)
   - Session completion rate (did the developer accomplish their goal?)
2. **DXM dashboard**: `/dx` command showing a dashboard of developer experience metrics for recent sessions. Highlight regressions: "Your average TTFT increased from 1.2s to 3.8s this week."
3. **Interaction replay**: `/replay <session-id>` replays a session's interaction sequence, showing timing for each step. Useful for understanding where time was spent.
4. **Usage analytics**: Track tool call frequencies, slash command usage, common prompt patterns. Use for `/learn` recommendations: "You frequently use Bash for file operations. Consider using the Read/Edit tools instead for better performance."

---

## 19. Synthetic Monitoring for Solo Developers

### What synthetic monitoring does

Synthetic monitoring probes services at regular intervals from external locations: HTTP health checks, API endpoint tests, SSL certificate expiry, DNS resolution, and multi-step transaction monitoring.

### Relevance to a coding agent

A solo developer lacks the team to notice when their deployed services go down. Hawk can fill this gap:

1. **Scheduled health checks**: Use hawk's existing `CronCreate/CronDelete/CronList` tools to schedule periodic checks of the developer's services.
2. **API endpoint monitoring**: `hawk monitor add https://api.myapp.com/health --interval 5m --alert slack` pings the endpoint and alerts on failure.
3. **SSL/DNS checks**: Detect certificate expiration before it happens. Alert 14 days before expiry.

### How hawk should integrate

1. **`/monitor` command**: Define synthetic checks that run on a schedule:
   - HTTP endpoint health (status code, response time, body match)
   - TCP port connectivity
   - DNS resolution
   - SSL certificate expiry
   - Custom command (e.g., `curl -s https://myapi.com/health | jq .status`)
2. **Cron-based execution**: Leverage hawk's existing cron system to run monitors at configurable intervals.
3. **Alert channels**: When a check fails, notify via:
   - Terminal notification (if hawk is running)
   - macOS notification center (via osascript)
   - Webhook (Slack, Discord, custom)
   - Email (via SMTP or SendGrid)
4. **Status page**: `hawk monitor status` shows a dashboard of all monitors with uptime history.
5. **Integration with debugging**: When a monitor fails, hawk automatically gathers diagnostic info (DNS lookup, curl verbose, traceroute) and presents it to the developer with a suggested fix.

---

## 20. Automated Performance Regression Detection

### Techniques

**Statistical comparison**:
- Compare benchmark results between two commits using statistical tests (Mann-Whitney U, bootstrapped confidence intervals)
- Flag regressions when the performance difference exceeds a threshold (e.g., >5% slowdown with p < 0.05)
- Tools: `benchstat` (Go), `criterion` (Rust), `hyperfine` (cross-language)

**Continuous benchmarking**:
- Run benchmarks on every commit (or PR)
- Store results in a time-series database
- Detect trends (gradual degradation) as well as step changes (sudden regression)
- Tools: `github.com/benchmark-action/github-action-benchmark`, Codspeed, benchmarking.rs

**Profile-guided regression detection**:
- Compare pprof profiles before/after a change
- Identify functions that got slower or allocate more
- More granular than benchmark-level detection -- shows WHERE the regression is, not just that it exists

### How hawk should integrate

1. **`/bench` command**: Run benchmarks and compare against baseline.
   - `hawk bench run` -- run project benchmarks, store results in `~/.hawk/benchmarks/`
   - `hawk bench compare` -- compare current results against the stored baseline
   - `hawk bench regression` -- flag statistically significant regressions
2. **Automatic baseline management**: After a successful PR merge or deployment, hawk stores benchmark results as the new baseline.
3. **Pre-commit regression check**: As a hook, run benchmarks before commit and warn if performance regressed: "Warning: BenchmarkParse is 23% slower than baseline (95% CI: [18%, 28%]). The change in parser.go:142 added an unnecessary allocation."
4. **Profile-guided analysis**: When a benchmark regression is detected, automatically profile both the baseline and current versions, diff the profiles, and explain the root cause.
5. **`benchstat` integration for Go**: Wrap Go's `benchstat` tool for statistical comparison. Parse its output and present in a developer-friendly format with explanations.
6. **Gradual degradation detection**: Track benchmark results over time. Alert when a metric has been degrading gradually (>1% per week for 4 consecutive weeks) even if no single commit caused a large regression.

---

## 21. Hawk Integration Strategy -- The Solo SRE

### The vision

A solo developer using hawk should have debugging capabilities equivalent to a team with a dedicated SRE. This means hawk must automate the activities an SRE performs:

| SRE Activity | Hawk Equivalent |
|-------------|----------------|
| Monitor dashboards | Automatic anomaly detection, `/metrics`, `/anomalies` |
| Triage incidents | Error fingerprinting, deduplication, priority ranking |
| Root cause analysis | `/debug` with automated scientific debugging |
| Performance analysis | `/profile`, `/bench`, continuous self-profiling |
| Log analysis | Drain-based parsing, structured event store, `/logs` |
| Trace analysis | Session trace trees, `/traces`, span filtering |
| Regression detection | `/bisect`, `/bench compare`, pre-commit checks |
| Uptime monitoring | `/monitor` with cron-based health checks |
| Post-incident review | Session replay, breadcrumbs, error history |
| Capacity planning | Cost tracking, token usage trends, `/cost analyze` |

### What hawk already has (strong foundation)

Hawk's existing infrastructure covers significant ground:

| Component | Location | Status |
|-----------|----------|--------|
| Distributed tracing | `trace/` | Built-in tracer + OTel SDK (build tag). Session trace trees with tree rendering. |
| Metrics | `metrics/` | Counters, gauges, timers with atomic operations. Registry with snapshot. |
| Structured logging | `logger/` | Leveled logging. Needs structured JSON output. |
| Profiling | `profile/` | CPU, memory, goroutine profiling via pprof. |
| Query profiling | `cmd/profiler.go` | TTFT, API call, tool exec timing per query. |
| Cost optimization | `analytics/optimize.go` | Wasted spend detection, model downgrade suggestions. |
| Task classification | `analytics/classify.go` | Prompt-based task type detection (simple/debug/refactor/etc). |
| Session insights | `analytics/insights.go` | Cross-session pattern extraction and recommendations. |
| Error handling | `cmd/errors.go` | Panic recovery, signal handling, error logging, friendly errors. |
| Health checks | `health/` | Registry with API key, config, disk space checks. |
| Circuit breaker | `circuit/` | Three-state circuit breaker for fault tolerance. |
| Retry | `retry/` | Exponential backoff with jitter. |
| Rate limiting | `ratelimit/` | Token bucket algorithm. |
| Fingerprinting | `fingerprint/` | Repository fingerprinting (languages, deps, git info). |
| Conversation DAG | `convodag/` | SQLite-backed DAG for conversation branching. |
| Code review | `sight/` | Bridge to AI code review library. |
| Site auditing | `inspect/` | Bridge to site auditing library. |
| Sandbox | `sandbox/` | Command isolation (namespace, docker, chroot, seatbelt). |

### What hawk needs to add (prioritized)

#### P0 -- Highest impact, implement first

1. **Automated Scientific Debugging (`/debug` command)**
   - Estimated value: Transforms debugging from hours to minutes for common issues.
   - Implementation: Hypothesis generation -> investigation via tool calls -> conclusion with explanation.
   - Depends on: repomap (already exists), tool execution (already exists), LLM reasoning (core capability).
   - See section 7 for detailed design.

2. **Error Fingerprinting and Cross-Session Deduplication**
   - Estimated value: Eliminates re-investigating known issues. Shows error history and trends.
   - Implementation: Stack trace parsing, frame normalization, hashing, SQLite error database.
   - Depends on: session persistence (already exists), error logging (already exists).
   - See sections 10 and 16 for algorithms.

3. **Structured Event Store**
   - Estimated value: Foundation for all other observability features. Enables querying, anomaly detection, and analysis.
   - Implementation: Extend `logger/` to emit structured JSON events to `~/.hawk/events.jsonl` with trace/span/session IDs. Add SQLite index for efficient querying.
   - Depends on: logger (already exists), trace (already exists).

4. **Breadcrumbs**
   - Estimated value: Provides "what happened before" context for every error. Eliminates guesswork.
   - Implementation: Ring buffer of last 100 significant events. Attach to error reports.
   - Depends on: structured event store (P0.3).

#### P1 -- High impact, implement next

5. **Drain-Based Log Parsing**
   - Estimated value: Automatically structures build output, test output, and application logs. Enables pattern matching.
   - Implementation: Fixed-depth tree parser in Go (algorithm is simple). Template extraction and anomaly detection.
   - See section 9 for algorithm details.

6. **Git Bisect Automation (`/bisect` command)**
   - Estimated value: Finds regression-causing commits in O(log n) time with zero manual effort.
   - Implementation: Wrapper around `git bisect run` with intelligent test generation.
   - See section 15 for design.

7. **Automatic Anomaly Detection**
   - Estimated value: Surfaces problems the developer has not yet noticed. Zero-config monitoring.
   - Implementation: Modified Z-score on session metrics (cost, latency, error rate). Baseline from historical data.
   - See section 11 for techniques.

8. **Session Timeline and Replay**
   - Estimated value: Full visibility into what happened during a session. Essential for post-incident review.
   - Implementation: `/timeline` command rendering chronological event stream. `/replay` for step-by-step playback.
   - See section 4 for design.

#### P2 -- Moderate impact, implement later

9. **Race and Deadlock Detection Integration**
   - Estimated value: Automates Go race detector usage and explains findings.
   - Implementation: Default `-race` for test runs, automated output parsing and fix suggestions.
   - See section 14 for techniques.

10. **Performance Regression Detection (`/bench` command)**
    - Estimated value: Catches performance regressions before deployment.
    - Implementation: `benchstat` integration, baseline management, profile-guided analysis.
    - See section 20 for design.

11. **Memory Leak Detection (`/leakcheck` command)**
    - Estimated value: Catches goroutine and heap leaks that are hard to find manually.
    - Implementation: Periodic heap/goroutine profiling, monotonic growth detection.
    - See section 13 for techniques.

12. **Synthetic Monitoring (`/monitor` command)**
    - Estimated value: Catches service outages without manual checking.
    - Implementation: HTTP/TCP/DNS/SSL checks on cron schedule with alerting.
    - See section 19 for design.

#### P3 -- Lower impact or high complexity

13. **Time-Travel Debugging Integration**
    - Estimated value: High for hard-to-reproduce bugs. Limited platform support (Linux only for rr).
    - Implementation: rr wrapper for recording test runs, automated reverse debugging.
    - See section 8 for design.

14. **Continuous Self-Profiling**
    - Estimated value: Moderate. Helps optimize hawk itself and long-running sessions.
    - Implementation: Periodic pprof collection, Pyroscope/Parca export.
    - See section 6 for design.

15. **OTel Collector Local Mode**
    - Estimated value: Moderate. Enables export to any backend without configuration.
    - Implementation: Embedded minimal collector that writes to local files and optionally forwards to OTLP endpoint.
    - See section 1 for design.

16. **Developer Experience Metrics Dashboard**
    - Estimated value: Moderate. Shows trends in developer productivity.
    - Implementation: DXM collection, `/dx` dashboard, regression alerts.
    - See section 18 for design.

### Architecture for the Solo SRE

```
                    +----------------------------------+
                    |         hawk agent loop          |
                    |  (engine, tools, LLM, sessions)  |
                    +----------------------------------+
                         |          |          |
                    spans/events  metrics   profiles
                         |          |          |
                    +----v----------v----------v----+
                    |     Unified Telemetry Bus      |
                    |  (structured events + traces)  |
                    +-------------------------------+
                         |          |          |
              +----------+    +----+----+    +-+--------+
              |               |         |    |          |
         Local Store     OTel Export  Anomaly     Error DB
         (JSONL/SQLite)  (optional)  Detector   (SQLite)
              |                          |          |
         /logs, /traces            /anomalies   /errors
         /timeline, /replay                     /debug
         /profile, /bench                       /bisect
```

All telemetry flows through a unified bus. The local store is always active (zero-config). OTel export is optional for developers who run Jaeger/Tempo/Grafana. The anomaly detector runs in the background. The error database tracks issues across sessions.

### Key design principles

1. **Zero-config by default**: Every observability feature works out of the box with local storage. No external services required.
2. **Progressive disclosure**: Start with simple `/errors` and `/debug`. Reveal `/traces`, `/anomalies`, `/bench` as the developer needs them.
3. **Agent-native**: The observability system is designed for an AI agent, not a traditional application. The LLM can read its own traces, analyze its own errors, and explain its own performance characteristics.
4. **Explain, do not just report**: Every finding comes with an explanation. "Error rate is 15%" is useless. "Error rate is 15%, up from 2% baseline. The errors are all in tool.Bash with 'permission denied'. This started after the sandbox mode change in session xyz. The fix is to add /tmp to the sandbox allow list." is useful.
5. **Cross-session memory**: Errors, baselines, and patterns persist across sessions. Hawk learns what is normal for this developer and this project, and alerts when something deviates.

---

## Key Repository References

| Project | URL | Stars | Key Technique |
|---------|-----|-------|---------------|
| OpenTelemetry Go | go.opentelemetry.io/otel | - | Stable traces + metrics SDK for Go |
| Jaeger | github.com/jaegertracing/jaeger | 22.7k | Distributed tracing, v2 built on OTel Collector |
| Grafana Tempo | github.com/grafana/tempo | 5.2k | Object-storage tracing, TraceQL |
| Highlight.io | github.com/highlight/highlight | 9.2k | Full-stack observability, session replay |
| Parca | github.com/parca-dev/parca | 4.9k | eBPF continuous profiling |
| Grafana Pyroscope | github.com/grafana/pyroscope | 11.4k | Continuous profiling with Grafana integration |
| rr | github.com/rr-debugger/rr | 10.5k | Record-and-replay time-travel debugging |
| Sentry | github.com/getsentry/sentry | 43.8k | Error grouping, fingerprinting, breadcrumbs |
| LogAI | github.com/salesforce/logai | 790 | AI-powered log analysis, anomaly detection |
| Drain3 | github.com/logpai/Drain3 | 797 | Online log template mining |
| Delve | github.com/go-delve/delve | 24.7k | Go debugger with goroutine support |
| Axiom | axiom.co | - | Log analytics with extreme compression |
| Better Stack | betterstack.com | - | Log management with AI SRE |

## Key Papers

| Paper | ID | Key Finding |
|-------|----|-------------|
| AutoSD | arXiv:2304.02195 | Automated Scientific Debugging: LLM-driven hypothesis-test-conclude cycle matches repair baselines. 70% of developers want explanations with fixes. |
| SWE-bench | arXiv:2310.06770 | Benchmark of 2,294 real GitHub issues. Repository-level bug fixing requires multi-file reasoning and is tractable with agent scaffolding. |
| iAudit | arXiv:2403.16073 | Two-stage detection + explanation with agent refinement achieves 91% F1. Detection is easier than explanation. |
