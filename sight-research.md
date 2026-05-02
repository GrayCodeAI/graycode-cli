# Sight: AI-Powered Code Review Library -- State of the Art Research

Research compiled 2026-05-03. Sources: arXiv papers, GitHub repos, tool documentation, and engineering blogs for 30+ systems and papers.

---

## Table of Contents

1. [Automated Code Review: Foundation Papers](#1-automated-code-review-foundation-papers)
2. [Static Analysis Tools: Architecture and Techniques](#2-static-analysis-tools-architecture-and-techniques)
3. [AI Code Review Tools: Commercial and Open-Source](#3-ai-code-review-tools-commercial-and-open-source)
4. [Security-Focused Analysis](#4-security-focused-analysis)
5. [Diff Understanding and Change Semantics](#5-diff-understanding-and-change-semantics)
6. [Multi-Concern Review Architecture](#6-multi-concern-review-architecture)
7. [Incremental Analysis: Review Only What Changed](#7-incremental-analysis-review-only-what-changed)
8. [False Positive Reduction and Review Quality](#8-false-positive-reduction-and-review-quality)
9. [Auto-Fix Generation](#9-auto-fix-generation)
10. [Cross-File Dependency Analysis](#10-cross-file-dependency-analysis)
11. [Review Prioritization](#11-review-prioritization)
12. [Historical Pattern Learning](#12-historical-pattern-learning)
13. [Bug Prediction from Diffs](#13-bug-prediction-from-diffs)
14. [Sight Implementation Priorities](#14-sight-implementation-priorities)

---

## 1. Automated Code Review: Foundation Papers

### CodeReviewer (Microsoft) -- ESEC/FSE 2022, arXiv:2203.09095
- **Authors**: Zhiyu Li, Shuai Lu, Daya Guo, Nan Duan, Shailesh Jannu, Grant Jenks, Deep Majumder, Jared Green, Alexey Svyatkovskiy, Shengyu Fu, Neel Sundaresan.
- **Architecture**: Pre-trained model built on CodeT5 base, specifically designed for code review automation. Trained on a large-scale dataset of real-world code changes and reviews from open-source projects spanning nine programming languages.
- **Four pre-training objectives**: (1) Diff tag prediction -- learning to identify meaningful change patterns in unified diffs, (2) Code change denoising -- reconstructing corrupted code changes, (3) Review comment generation -- learning to produce human-like review feedback, (4) Code refinement -- learning to apply reviewer suggestions.
- **Three downstream tasks evaluated**: Code change quality estimation, review comment generation, and code refinement (applying review feedback to modify code).
- **Performance**: Outperformed all previous state-of-the-art pre-training approaches on all three tasks.
- **Key insight for sight**: The pre-training objectives demonstrate that understanding diffs requires specific training -- generic code models do not naturally understand the semantics of additions and deletions. Sight should structure its prompts to make diff semantics explicit rather than assuming an LLM understands unified diff format natively.

### Google AutoCommenter -- ICSE 2024
- **What**: An LLM-based system deployed internally at Google for automated detection of coding best practice violations during code review.
- **Architecture**: Uses a large language model (evolved from the earlier T5-based DIDACT approach) to detect violations of Google's coding guidelines and standards during the code review process.
- **Deployment scale**: Used by tens of thousands of developers at Google across the full codebase.
- **Key design decisions**: (1) Focused specifically on best practice detection rather than bug finding -- narrower scope yields higher precision. (2) Comments include specific guideline references, making them verifiable. (3) System learns from developer responses (thumbs up/down, whether suggestions are applied) to improve over time.
- **Key metric**: Developer acceptance/resolution rate is the north-star metric rather than precision/recall on a static dataset. Google found that producing comments developers actually act on matters more than finding every possible issue.
- **Key insight for sight**: Narrow the scope of each review concern. A system that detects 5 types of issues with 90% precision is more useful than one that detects 50 types with 40% precision. Best practices with clear, citable guidelines achieve the highest acceptance rates.

### Tufano et al. -- Qualitative Study of Code Review Automation (arXiv:2401.05136, Jan 2024)
- **Authors**: Rosalia Tufano, Ozren Dabic, Antonio Mastropaolo, Matteo Ciniselli, Gabriele Bavota.
- **Methodology**: 105 man-hours manually inspecting 2,291 predictions across three automated code review techniques on two tasks: commenting on changes and addressing reviewer comments.
- **Critical finding**: Quantitative metrics alone are misleading. Achieving 10% accuracy on review comment generation is meaningless without knowing what types of comments succeed. The paper developed taxonomies of success/failure patterns.
- **ChatGPT comparison**: ChatGPT struggles specifically at commenting on code as a human reviewer would. It generates plausible but often generic comments. Specialized models outperform general-purpose LLMs on review tasks.
- **Dataset quality**: Found significant data quality issues in existing benchmarks that inflate reported metrics.
- **Key insight for sight**: Do not rely on BLEU/exact-match metrics for evaluating review comment quality. Develop a taxonomy of review comment types and measure per-type accuracy. Prioritize generating the types of comments that have the highest human acceptance rate.

### Code Review Benchmark Survey -- arXiv:2602.13377 (Feb 2026)
- **Authors**: Taufiqul Islam Khan, Shaowei Wang, Haoxiang Zhang, Tse-Hsun Chen.
- **Scope**: Analyzed 99 papers (58 pre-LLM era, 41 LLM era) covering 2015-2025.
- **Taxonomy**: Multi-level taxonomy organizing code review research into 5 domains and 18 fine-grained tasks.
- **Key trends**: (1) Movement toward end-to-end generative peer review. (2) Expanded multilingual coverage. (3) Decline in standalone change understanding tasks (subsumed by end-to-end systems).
- **Identified gaps**: Current benchmarks lack comprehensive task coverage, dynamic runtime evaluation, and fine-grained assessment frameworks. Most benchmarks evaluate on static snapshots rather than realistic incremental review scenarios.
- **Key insight for sight**: The field is moving toward end-to-end review systems rather than piecemeal analyzers. Sight should be designed as a pipeline that handles the full review lifecycle: parse diff -> enrich context -> analyze multi-concern -> generate comments -> suggest fixes -> prioritize findings.

### Rasheed et al. -- AI-Powered Code Review with LLMs (arXiv:2404.18496, Apr 2024)
- **Technique**: LLM-based agent trained on code repositories including reviews, bug reports, and documentation to detect code smells, identify bugs, and suggest improvements.
- **Differentiator**: Predicts future potential risks in code, going beyond static analysis to anticipate maintenance and reliability issues.
- **Dual objective**: Improve code quality AND enhance developer education by explaining best practices in review comments.
- **Finding**: Developer sentiment analysis supports that educational review comments increase developer engagement with the review process.
- **Key insight for sight**: Review comments should be educational, not just diagnostic. For solo developers who lack a human reviewer, sight should explain WHY something is a problem, not just WHAT is wrong. Include links to relevant documentation or patterns.

### ContextCRBench -- arXiv:2511.07017 (Nov 2025)
- **Authors**: Ruida Hu, Xinchen Wang, Xin-Cheng Wen, Zhao Zhang, Bo Jiang, Pengfei Gao, Chao Peng, Cuiyun Gao.
- **Dataset**: 67,910 context-enriched entries derived from 153.7K issues and pull requests from top-tier repositories.
- **Three evaluation tasks**: (1) Hunk-level quality assessment, (2) Line-level defect localization, (3) Line-level comment generation.
- **Critical finding**: Textual context (issue descriptions, PR descriptions, commit messages) yields GREATER performance gains than code context alone. Developer intent information improves review quality more than expanded code snippets.
- **Deployment**: When deployed at ByteDance, improved system performance by 61.98%.
- **Gap identified**: Current LLMs remain far from human-level review ability, even with enriched context.
- **Key insight for sight**: Context enrichment is critical and should prioritize textual context (commit messages, issue links, PR descriptions) over expanding the code window. Sight should extract and include: (a) commit message intent, (b) related issue/ticket text, (c) PR description, (d) recent git history for the changed files. This is more valuable than including more surrounding code.

---

## 2. Static Analysis Tools: Architecture and Techniques

### Semgrep
- **Architecture**: Pattern-based static analysis using a lightweight, fast pattern matching engine. Rules are written in YAML with a grep-like syntax that operates on ASTs rather than raw text.
- **Rule system**: 30+ supported languages. Rules combine pattern matching with data flow analysis. Patterns use metavariables ($X, $Y) to capture and cross-reference code elements.
- **Taint mode**: Full taint tracking with configurable sources, sinks, sanitizers, and propagators. Tracks data flow from user input to dangerous operations (SQL injection, XSS, command injection).
- **Diff-aware scanning**: Supports `SEMGREP_BASELINE_REF` environment variable to scan only changes relative to a baseline branch. In CI, this means only newly introduced issues are reported on PRs.
- **Autofix**: Rules can include fix patterns that automatically remediate detected issues.
- **Precision approach**: Designed for high precision (low false positive rate) at the cost of some recall. Rules are community-contributed and vetted. Default timeout of 5 seconds per rule prevents runaway analysis.
- **Key numbers**: 30+ languages, thousands of community rules covering OWASP Top 10, CWE patterns, and framework-specific issues.
- **Key insight for sight**: Semgrep's pattern+taint approach achieves high precision because each rule is narrowly scoped and human-vetted. Sight should integrate Semgrep-style pattern matching for deterministic checks (known vulnerability patterns, banned APIs) and reserve LLM analysis for semantic/contextual issues that patterns cannot catch.

### CodeQL (GitHub)
- **Architecture**: Builds a relational database from source code by extracting ASTs, data flow graphs, and control flow graphs. For compiled languages, it monitors the build process; for interpreted languages, it runs extractors directly on source.
- **Query language**: QL -- a declarative, object-oriented query language. Queries express relationships between code elements (e.g., "find all calls to `eval()` where the argument contains user input").
- **Key capabilities**: Variant analysis (find all instances of a known vulnerability pattern), taint tracking (full interprocedural data flow), cross-file analysis (queries span the entire database).
- **Integration**: GitHub code scanning runs CodeQL on every push and PR. Copilot Autofix generates fix suggestions for CodeQL findings.
- **Multi-repository**: Can run queries at scale across many repositories.
- **Limitation**: Requires building a database, making incremental analysis harder. Analysis time scales with codebase size.
- **Key insight for sight**: CodeQL's strength is deep cross-file taint tracking that LLMs cannot replicate. Sight should not try to replace CodeQL but should complement it: use LLM analysis for semantic issues (logic errors, design problems, naming) and delegate security vulnerability detection to CodeQL/Semgrep when available. Provide an integration point for piping CodeQL results into sight's review output.

### SonarQube (SonarSource)
- **Multi-concern analysis**: Categorizes issues into four types: bugs, code smells, vulnerabilities, and security hotspots. This four-category taxonomy has become the de facto standard.
- **Rule system**: 7,000+ language-specific rules across 40+ languages and IaC technologies.
- **Quality gates**: Customizable go/no-go thresholds that block merges when standards are not met. Gates can enforce criteria like "zero new bugs," "code coverage > 80%," or "no new security hotspots."
- **PR analysis**: Branch analysis and pull request decoration with actionable summaries. Integrates with GitHub, GitLab, Bitbucket, Azure DevOps.
- **Taint analysis**: Tracks data flow for injection vulnerability detection (SQL injection, XSS).
- **Incremental**: Focuses on new issues in PRs -- does not re-report existing technical debt in unchanged code.
- **Key insight for sight**: SonarQube's four-category taxonomy (bugs, code smells, vulnerabilities, security hotspots) is well-understood by developers. Sight should adopt a similar multi-concern taxonomy but add LLM-specific categories: logic errors, performance concerns, test coverage gaps, and cross-file consistency issues. Quality gates are valuable for solo developers as automated standards enforcement.

### Snyk Code
- **AI-powered SAST**: Uses a machine learning engine trained on millions of open-source commits and vulnerability fixes.
- **Analysis types**: Data flow (source-to-sink taint tracking), control flow (null dereferences, race conditions), API usage (misuse detection), type inference (handles dynamic typing).
- **False positive reduction**: The ML engine achieves lower false positive rates than traditional rule-based SAST tools by learning from real-world code patterns and developer fixes.
- **Interfile analysis**: Available for all supported languages except Ruby.
- **Integration**: PR checks, IDE plugins (JetBrains, VS Code, Visual Studio), CLI scanning, Jira export.
- **Key insight for sight**: ML-trained vulnerability detection significantly outperforms rule-only approaches on false positive rates. Sight's security concern should leverage both pattern matching (Semgrep-style for known patterns) and LLM reasoning (for novel vulnerability patterns) to minimize false positives.

---

## 3. AI Code Review Tools: Commercial and Open-Source

### CodeRabbit (coderabbitai/ai-pr-reviewer)
- **Architecture**: GitHub Action triggering on pull_request events. Two-tier model approach: lightweight model (GPT-3.5-turbo) for PR summarization and release notes, heavyweight model (GPT-4) for detailed line-by-line code review.
- **Review pipeline**: (1) Track changed files between commits, (2) Compare against PR base for incremental review, (3) Generate context-aware line-by-line feedback with actionable code suggestions.
- **Review concerns**: Functional correctness, code quality, maintainability, security, performance, typos, documentation.
- **Smart review skipping**: Bypasses in-depth analysis for trivial changes (typo fixes) unless configured otherwise.
- **Context enrichment**: ast-grep for syntax-aware pattern matching, multi-repo analysis for cross-repository dependency detection, issue tracker integration (Jira, Linear, GitHub Issues), code guidelines from .cursorrules/CLAUDE.md/AGENTS.md.
- **Auto-fix**: Agentic code modifications, custom recipes for reusable fix patterns, specialized generators for docstrings, tests, simplification, and merge conflict resolution.
- **Learnable patterns**: Natural language learnings from chat interactions, path-based review instructions using glob patterns.
- **Integrated tools**: 40+ linters and scanners (ESLint, Ruff, Semgrep, Trivy).
- **Key numbers**: Multi-platform (GitHub, GitLab, Azure DevOps, Bitbucket).
- **Key insight for sight**: CodeRabbit's two-tier model approach (cheap model for summarization, expensive model for analysis) is a proven cost optimization. Sight should adopt this: use a fast/cheap model for diff summarization and impact classification, then route to a strong model only for hunks that need deep analysis. The learnable patterns feature (teaching preferences via chat) is important for solo developers who want to encode their personal standards.

### PR-Agent / Qodo (formerly CodiumAI)
- **Architecture**: Open-source Python-based tool. Platform-agnostic (GitHub, GitLab, BitBucket, Azure DevOps, Gitea). Supports multiple LLM providers (OpenAI, Claude, Deepseek).
- **PR Compression strategy**: Adaptive approach for handling PRs of varying sizes. Fits file patches into model context with token-aware compression. Dynamically builds context for each file.
- **Tool suite**: (1) Describe -- PR summary generation, (2) Review -- detailed code quality assessment, (3) Improve -- code enhancement suggestions, (4) Ask -- interactive Q&A, (5) Update CHANGELOG.
- **Multi-concern review**: Configurable JSON-based prompting allows teams to customize review categories and focus areas.
- **Self-reflection**: After generating review comments, the system performs a self-reflection pass to verify and refine suggestions before posting.
- **Ticket context**: Fetches linked issue/ticket content to validate changes against requirements.
- **Performance**: Each tool completes in approximately 30 seconds using a single LLM API call.
- **Key insight for sight**: PR-Agent's self-reflection mechanism is critical for quality. After generating findings, sight should run a verification pass: "Given this code context, is this finding actually correct? Would a senior developer agree?" This reduces false positives. The PR compression strategy (fitting relevant context into the model window) is an essential technique.

### Sourcery
- **Architecture**: Uses OpenAI and Anthropic LLMs for code review. Integrates with GitHub as an app, IDE plugins for PyCharm, VS Code, Sublime, Vim.
- **Review output**: Three components per review: change summaries, high-level feedback, and line-by-line suggestions.
- **Additional capabilities**: Unit test generation, docstring generation, code optimization (readability and performance).
- **Key insight for sight**: Sourcery's three-component output structure (summary, high-level, line-level) maps well to how developers consume reviews. Sight should generate: (a) a brief summary of what the diff does and its risk level, (b) high-level architectural/design comments, (c) line-specific findings with suggested fixes.

### Greptile
- **Core philosophy**: The tool that generates code should NOT be the same tool that reviews it. Separation prevents shared assumptions.
- **Codebase understanding**: Reads the full codebase (not just the diff) to catch logic issues spanning multiple files. Identifies gaps between assumptions in the diff and reality in the broader system.
- **Issue distribution**: 48% logic errors, remainder style/syntax. Prioritizes logic errors that matter.
- **Confidence scoring**: Rather than flooding PRs with comments, provides severity-based triage. High-confidence PRs (63% in their data) merge 3.6x faster than those requiring attention.
- **Scale**: 700,000+ monthly PRs processed. Developed signal control to prevent low-value comment noise.
- **Real-world catches**: Detected production bugs in Netflix, NVIDIA, and Meta repositories -- nested retries, data leaks, conflicting implementations invisible in single-file review.
- **Key finding**: Tools generating excessive feedback get disabled by teams. Restraint is essential.
- **Key insight for sight**: Greptile's confidence scoring and restraint philosophy is crucial. Sight must aggressively filter findings. A useful rule: if you would not bet $100 that a senior developer would agree with the finding, do not post it. The 48% logic errors finding suggests sight should weight its analysis toward logic/correctness over style. Solo developers care more about "this will break" than "this could be named better."

---

## 4. Security-Focused Analysis

### SAST Architecture (consolidated from Snyk, OWASP, Semgrep, CodeQL)

**Five analysis techniques**:
1. **Data flow / Taint analysis**: Track data from untrusted sources through the program to dangerous sinks. The core technique for injection vulnerabilities (SQLi, XSS, command injection, path traversal).
2. **Control flow analysis**: Model all execution paths. Detect null dereferences, race conditions, resource leaks, unreachable code.
3. **Semantic analysis**: Identify logical errors and insecure patterns based on code meaning rather than syntax.
4. **Configuration analysis**: Detect misconfigured settings, insecure defaults, hardcoded credentials.
5. **Structural analysis**: Find code organization flaws, excessive complexity, deep nesting.

**False positive rates in traditional SAST**: Legacy tools historically suffered 50-80% false positive rates (per Snyk's analysis). This is the primary reason developers abandon SAST tools. Modern AI-native tools (Snyk Code, Semgrep) have significantly reduced this, but the problem remains the central challenge.

**CWE/OWASP coverage**: OWASP notes that current SAST tools can only automatically identify a relatively small percentage of application security flaws. Authentication, access control, and cryptography issues remain difficult to automate.

### LLM Security Code Review (arXiv:2401.16310, updated Dec 2025)
- **Authors**: Jiaxin Yu, Peng Liang, Yujia Fu, Amjed Tahir, Mojtaba Shahin, Chong Wang, Yangxiao Cai.
- **Finding**: LLMs significantly outperform state-of-the-art static analysis tools on security defect detection. Reasoning-optimized models (DeepSeek-R1, GPT-4) perform best.
- **Model-specific behaviors**: GPT-4 produces vague expressions and struggles to follow instructions precisely. DeepSeek-R1 generates inaccurate code details.
- **Influential factors**: (1) Files with fewer tokens have better detection rates. (2) Security-relevant annotations in code improve detection. (3) Prompt design is critical -- DeepSeek-R1 benefits from commit messages + chain-of-thought; GPT-4 benefits from CWE lists in the prompt.
- **Key insight for sight**: For security review, include a CWE taxonomy excerpt relevant to the language/framework in the system prompt. Use chain-of-thought prompting that walks through data flow from source to sink. Break large files into smaller analysis windows for better detection accuracy.

### Contextual Bias in LLM Security Review (arXiv:2603.18740, Mar 2026)
- **Authors**: Dimitris Mitropoulos, Nikolaos Alexopoulos, Georgios Alexopoulos, Diomidis Spinellis.
- **Attack vector**: Contextual-bias injection -- a supply-chain attack where attackers craft misleading metadata (commit messages, comments) to make vulnerable code appear safe to LLM reviewers.
- **Finding**: Framing effects are systematic and widespread across all 6 LLMs tested. "Bug-free framing" produces the strongest effect, causing LLMs to miss vulnerabilities.
- **Attack success**: Iterative refinement attacks achieved 100% success rate on 17 CVEs across 10 production projects.
- **Mitigation**: Metadata redaction and explicit counter-instructions restore detection in all affected cases.
- **Key insight for sight**: CRITICAL for security review: sight must redact or quarantine potentially adversarial metadata (commit messages, code comments, variable names) when performing security analysis. Run security checks with and without metadata context and flag discrepancies. Never trust developer-provided framing for security assessments.

### CodeSecEval (arXiv:2407.02395, Jul 2024)
- **Dataset**: 44 critical vulnerability types with 180 samples.
- **Finding**: Current LLMs frequently overlook security issues during both code generation and repair processes.
- **Proposed mitigation**: Vulnerability-aware contextual information and insecure code explanations improve detection.
- **Key insight for sight**: When reviewing for security, explicitly prompt the model with vulnerability-aware context: "This function handles user input. Check for: SQL injection, XSS, path traversal, command injection, and SSRF." Do not rely on the model to spontaneously identify the threat model.

### ChatGPT Vulnerability Detection Limitations (arXiv:2304.07232, Apr 2023)
- **Authors**: Anton Cheshkov, Pavel Zadorozhny, Rodion Levichev.
- **Finding**: ChatGPT performed no better than a dummy classifier on vulnerability detection tasks, for both binary and multi-label CWE classification.
- **Implication**: General-purpose LLMs without specialized prompting or fine-tuning are unreliable for automated vulnerability detection. Specialized techniques are required.
- **Key insight for sight**: Do NOT rely on naive LLM prompting ("find vulnerabilities in this code") for security review. Sight must use structured prompting with explicit CWE taxonomies, taint-path descriptions, and step-by-step data flow analysis. Consider hybrid approach: use pattern matching (Semgrep rules) for known patterns, LLM analysis for novel/complex patterns.

---

## 5. Diff Understanding and Change Semantics

### ReDef: Do CLMs Truly Understand Code Changes? (arXiv:2509.09192, Sep 2025)
- **Authors**: Doha Nam, Taehyoun Kim, Duksan Ryu, Jongmoon Baik.
- **Dataset**: 3,164 defective and 10,268 clean modifications from 22 large C/C++ projects, with GPT-assisted triage for ambiguous cases.
- **Critical finding on diff encoding**: Compact diff-style encodings consistently outperform whole-function formats across all tested models (CodeBERT, CodeT5+, UniXcoder, Qwen2.5). Statistically confirmed.
- **Shocking finding on semantic understanding**: When researchers applied counterfactual perturbations (swapping added/deleted blocks, inverting diff polarity), model performance remained effectively stable. This means current code LMs exploit SURFACE-LEVEL CUES rather than genuinely understanding change semantics.
- **Models tested**: CodeBERT, CodeT5+, UniXcoder, Qwen2.5.
- **Key insight for sight**: This is a foundational result. LLMs do not inherently understand diff semantics -- they match surface patterns. Sight must compensate by making semantics explicit in its prompts. Instead of just passing a unified diff, sight should include: (1) a natural language description of what changed ("function X was modified to add parameter Y"), (2) explicit before/after code blocks for complex changes, (3) the intent behind the change (from commit message/PR description). Compact diff format is better than whole-function format for model consumption.

### Change-Aware Defect Prediction (arXiv:2512.23875, Dec 2025)
- **Authors**: Mohsen Hesamolhokama, Behnam Rohani, Amirahmad Shafiee, MohammadAmin Fazli, Jafar Habibi.
- **Problem exposed**: Traditional defect prediction suffers from "illusion of accuracy" -- standard evaluation rewards label-persistence bias (most files stay the same between versions) rather than reasoning about actual code changes. Inflated F1 scores.
- **Architecture**: LLM-driven, change-aware, multi-agent debate framework that reasons over code changes between successive versions rather than static snapshots.
- **Key result**: Traditional models show inflated F1 but fail on defect-transition cases (where code changes introduce or remove defects). The proposed framework achieves balanced performance across evolution subsets with significantly improved sensitivity to defect introductions.
- **Key insight for sight**: Sight should focus on change transitions, not static snapshots. The review question should be: "Does this specific change introduce a defect that was not present before?" rather than "Does this file contain defects?" This requires both the before and after states, plus understanding of what the change intended to accomplish.

### Comment Classification Taxonomy (arXiv:2604.23667, Apr 2026)
- **Authors**: Semih Caglar, Sukru Eren Gokirmak, Eray Tuzun.
- **Nine-label taxonomy**: 6 review comment smells (problems) + 3 useful intent types. Captures redundancy, vagueness, and lack of constructiveness.
- **LLM classification results**: Zero-shot macro-F1 between 0.360-0.374 (moderate). One-shot improved GPT-5-mini and DeepSeek-R1 but slightly degraded LLaMA-3.3.
- **Finding**: Comment-diff context suffices for some classifications but falls short for "evidence-sensitive labels" -- those that require broader thread context or understanding of project conventions.
- **Key insight for sight**: Sight's generated comments should be self-checked against the smell taxonomy before posting. Reject comments that are vague, non-actionable, or redundant with what the diff already makes obvious. Each comment should pass the test: "Does this tell the developer something they do not already know from reading the diff?"

---

## 6. Multi-Concern Review Architecture

### Concern Taxonomy (synthesized from all tools and papers)

The following concern categories appear across CodeRabbit, SonarQube, PR-Agent, and the research literature:

| Concern | What to detect | Technique | FP Risk |
|---------|---------------|-----------|---------|
| **Correctness** | Logic errors, off-by-ones, null handling, edge cases, wrong conditions | LLM reasoning with before/after context | Medium |
| **Security** | Injection, auth bypass, data exposure, insecure crypto, secrets | Pattern matching + LLM taint analysis | High |
| **Performance** | O(n^2) in hot paths, unnecessary allocations, N+1 queries, blocking I/O | Pattern matching + LLM reasoning | Medium |
| **Maintainability** | Dead code, excessive complexity, poor naming, missing abstraction | LLM reasoning with project style context | High |
| **Style** | Formatting, naming conventions, import ordering | Linter integration, pattern matching | Low |
| **Test gaps** | Changed behavior without test updates, untested edge cases | LLM analysis of test coverage intent | Medium |
| **API consistency** | Breaking changes, incompatible modifications, contract violations | Cross-file dependency analysis | Low |
| **Documentation** | Public API missing docs, stale comments contradicting code | Pattern matching + LLM | Medium |

### How existing tools implement multi-concern review

**CodeRabbit**: Runs all concerns in a single LLM call with structured output. 40+ integrated linters handle deterministic concerns.

**PR-Agent**: JSON-based prompt configuration allows enabling/disabling specific concern categories. Single LLM call per tool invocation (~30 seconds).

**SonarQube**: 7,000+ rules organized by category (bugs, vulnerabilities, code smells, security hotspots). Each rule maps to a specific concern with severity.

**Greptile**: Prioritizes logic errors (48% of findings). Explicitly deprioritizes style issues. Confidence scoring per finding.

### Sight takeaway: parallel multi-concern architecture

Sight should run concerns in parallel with different prompts and different temperature/model settings per concern:

1. **Deterministic layer** (zero LLM cost): Run pattern matchers and linters for style, known vulnerability patterns, banned APIs. These produce zero false positives.
2. **Focused LLM analysis** (parallel): Send separate prompts for each major concern (correctness, security, performance). Each prompt includes concern-specific context (CWE taxonomy for security, complexity metrics for performance, test file list for test gaps).
3. **Cross-concern deduplication**: After parallel analysis, deduplicate findings that were flagged by multiple concerns (e.g., a security issue that is also a correctness issue).
4. **Self-reflection filter**: Run a final verification pass on all findings to remove false positives and improve actionability.

Different concerns benefit from different model strengths:
- Correctness: needs strong reasoning (Opus-class)
- Security: needs structured analysis (pattern matching + Sonnet-class with CWE prompting)
- Performance: needs reasoning + knowledge (Sonnet-class)
- Style/maintainability: fast model or pattern matching (Haiku-class or linters)
- Test gaps: needs understanding of test intent (Sonnet-class)

---

## 7. Incremental Analysis: Review Only What Changed

### How existing tools handle incremental review

**Semgrep**: `SEMGREP_BASELINE_REF` compares current branch against baseline. Only reports issues introduced in the diff, not pre-existing issues. This is the gold standard for incremental SAST.

**SonarQube**: PR analysis focuses on new issues. Does not re-report existing technical debt in unchanged code. Quality gates can be set to "zero new issues" independent of existing debt.

**CodeQL / GitHub Code Scanning**: Runs on every push and PR. Distinguishes new alerts from pre-existing alerts. Copilot Autofix generates suggestions only for new findings.

**CodeRabbit**: Tracks changed files between commits. Compares against PR base for incremental review. Smart review skipping for trivial changes.

**Greptile**: Analyzes the diff but enriches with full codebase context. The analysis scope is the diff; the context scope is the entire repository.

### Key principle: narrow analysis, wide context

All effective tools follow the same pattern:
- **Analysis scope**: Only the changed code (hunks in the diff)
- **Context scope**: As wide as needed to understand the change (surrounding functions, called functions, type definitions, test files, git history)

This is not the same as "only looking at the diff." It means looking at the diff WITH sufficient context to understand whether the change is correct.

### Sight takeaway: incremental analysis architecture

Sight should implement a three-layer context model:

1. **Diff layer** (what changed): Parse unified diff into structured hunks. Each hunk becomes an analysis unit.
2. **Local context layer** (immediate surroundings): For each changed hunk, fetch:
   - The complete function/method containing the change
   - The class/module containing the change
   - Direct imports and type definitions referenced in the change
   - The corresponding test file (if it exists) and whether tests were also modified
3. **Extended context layer** (cross-file): For each changed function:
   - Find all callers of the changed function (impact analysis)
   - Find all callees of the changed function (dependency analysis)
   - Check for interface/contract changes that affect other files
   - Git blame for the changed lines (who wrote this, when, associated with what issue)

Context should be fetched lazily -- only expand beyond the diff layer when the LLM analysis of the diff layer identifies potential cross-file concerns.

---

## 8. False Positive Reduction and Review Quality

### The Central Problem

False positives are the #1 reason code review tools get disabled. Greptile processes 700K+ PRs monthly and explicitly invested in signal control because excessive noise leads to tool abandonment. Legacy SAST tools with 50-80% FP rates are routinely ignored.

### Reducing FPs with LLMs in Static Analysis (arXiv:2601.18844, Jan 2026)
- **Authors**: Xueying Du, Jiayi Feng, Yi Zou, Wei Xu, Jie Ma, Wei Zhang, Sisi Liu, Xin Peng, Yiling Lou.
- **Deployment**: Industrial study at Tencent on enterprise-customized static analysis tools.
- **Technique**: Hybrid -- run static analysis first (for high recall), then use LLM as a filter to eliminate false positives.
- **Result**: Eliminates 94-98% of false positives with high recall maintained. That is, the LLM correctly identifies and removes almost all false alarms while preserving almost all true positives.
- **Cost**: 2.1-109.5 seconds per alarm, $0.0011-$0.12 per alarm. Orders of magnitude cheaper than manual review (10-20 minutes per false alarm for human developers).
- **Key insight for sight**: This is the most important result for sight's architecture. Use a TWO-PASS approach: (1) generate findings aggressively (higher recall, accept some FPs), (2) use a separate LLM call to validate each finding against the full context ("Is this finding correct? Could there be a reason this code is intentionally written this way?"). The validation pass eliminates 94-98% of FPs.

### iCodeReviewer: Mixture of Prompts (arXiv:2510.12186, Oct 2025)
- **Authors**: Yun Peng, Kisub Kim, Linghan Meng, Kui Liu.
- **Architecture**: Multiple specialized "prompt experts," each designed to detect a specific category of security issue. A routing algorithm activates only the relevant experts based on code features.
- **FP reduction mechanism**: By activating only necessary prompt experts, prevents irrelevant analyses from generating hallucinated findings.
- **Metrics**: F1 of 63.98% on internal datasets. 84% acceptance rate in production deployment.
- **Key insight for sight**: Rather than one mega-prompt that looks for everything, sight should use specialized sub-prompts for each concern category, activated only when relevant. A router/classifier determines which concerns apply to each hunk (e.g., no need to run security analysis on a README change, no need to run performance analysis on a type definition).

### LLM Overcorrection Problem (arXiv:2603.00539, Feb 2026)
- **Authors**: Haolin Jin, Huaming Chen.
- **Problem**: LLMs systematically overcorrect -- they classify correct code as non-compliant or defective. This is a false positive generator.
- **Counterintuitive finding**: More detailed prompts (requiring explanations and proposed corrections) lead to HIGHER misjudgment rates. When LLMs explain their reasoning for flagging code, they talk themselves into seeing problems that do not exist.
- **Proposed mitigation**: "Fix-guided Verification Filter" -- validate both the original code AND the LLM's proposed fix using tests. If the original passes and the fix does not change behavior, the finding is likely a false positive.
- **Key insight for sight**: CRITICAL -- do not ask the review LLM to explain and fix issues in the same pass. Separate detection from explanation. First, detect issues with minimal prompting. Then, for each finding, separately generate an explanation and fix. Finally, validate: if the suggested fix would not change any test outcomes, the finding is likely a false positive. For solo developers, this automated validation is especially important because there is no human reviewer to filter FPs.

### STAF: Sentence Transformer-based Actionability Filtering (arXiv:2604.18525, Apr 2026)
- **Architecture**: Uses transformer-based sentence embeddings to classify static analysis findings into actionable vs non-actionable.
- **F1**: 89%. At least 11% improvement over existing methods within-project, 6% cross-project.
- **Key insight for sight**: Train or use a classifier to predict whether a finding is actionable before presenting it to the developer. Actionability factors: (1) Does it point to a specific line? (2) Is the fix clear? (3) Is the finding about new code (not pre-existing debt)? (4) Is it in code the developer actually wrote (not generated/vendored)?

### Sight's false positive reduction pipeline

Based on all research, sight should implement a four-stage filter:

```
Stage 1: Generate findings aggressively (recall-optimized)
    |
Stage 2: Self-reflection verification
    "Given the full context, is this finding actually correct?"
    Eliminates ~50% of FPs (based on PR-Agent's self-reflection results)
    |
Stage 3: Fix-guided validation
    "If I apply the suggested fix, does anything actually change?"
    Eliminates overcorrection FPs
    |
Stage 4: Confidence scoring and threshold
    Score each finding on: specificity, severity, actionability, confidence
    Only surface findings above threshold (default: high confidence)
    Greptile data: 63% of PRs need no comments at all
```

---

## 9. Auto-Fix Generation

### Meta SapFix -- ICSE 2019
- **Three fix strategies**:
  1. **Revert**: Full or partial revert of the code submission that introduced the bug. Highest confidence, simplest approach.
  2. **Template-based**: Draws from a collection of fix templates automatically harvested from past human fixes. Patterns like "add null check before dereference" or "replace == with .equals()."
  3. **Mutation-based**: Small modifications to the AST of the crash-causing statement. Iteratively adjusts patches when templates do not apply.
- **Validation**: Every candidate fix is validated by (a) compilation, (b) checking that the original crash no longer occurs, (c) checking that no new failures are introduced using both developer-written tests and Sapienz-generated tests.
- **Human oversight**: SapFix never deploys fixes to production autonomously. Human engineers review and approve. The system tracks acceptance rates to improve.
- **Deployment**: First automated end-to-end repair system deployed at Meta's scale (tens of millions of lines, hundreds of millions of users).
- **Key insight for sight**: SapFix's three-strategy hierarchy (revert > template > mutation) maps well to LLM-based fixes. Sight should generate fixes with confidence tiers: (a) High confidence: the fix follows a known pattern (null check, error handling, type annotation). (b) Medium confidence: LLM-generated fix with clear rationale. (c) Low confidence: LLM suggestion that requires human judgment. Always validate fixes do not introduce new issues.

### REFINE: Context-Aware Patch Refinement (arXiv:2510.03588, Oct 2025)
- **Authors**: Anvith Pabba, Simin Chen, Alex Mathai, Anindya Chakraborty, Baishakhi Ray.
- **Three-phase pipeline**: (1) Context disambiguation -- clarify vague code context. (2) Candidate diversification -- generate multiple fix variants. (3) Partial fix aggregation -- combine incomplete fixes via LLM-powered code review.
- **Performance**: Improved AutoCodeRover by 14.67% on SWE-Bench Lite (51.67% resolution). Average 14% improvement across multiple repair systems.
- **Key insight for sight**: Generate MULTIPLE fix candidates, not just one. Then use a separate LLM call to evaluate and select the best. The diversity of candidates is crucial -- a single greedy fix often misses the best solution.

### LLM-Based Code Review Defect Repair (arXiv:2312.17485, Dec 2023)
- **Authors**: Zelin Zhao, Zhaogui Xu, Jialong Zhu, Peng Di, Yuan Yao, Xiaoxing Ma.
- **Repair rate**: 72.97% using optimized prompt strategies.
- **Key finding**: Review comments are an under-utilized resource for generating fixes. The natural language description of the problem in a review comment provides valuable guidance for the LLM to generate a correct fix.
- **Key insight for sight**: When sight generates a review comment explaining a problem, it should FEED THAT COMMENT BACK to the fix generation step as input. The comment acts as a specification for the fix. This creates a pipeline: detect -> explain -> fix, where each stage builds on the previous.

### Sight auto-fix architecture

```
Finding detected
    |
Generate natural language explanation of the problem
    |
Feed (code + finding + explanation) to fix generator
    |
Generate 3 candidate fixes with confidence scores
    |
Validate each candidate:
  - Parses correctly (syntax check)
  - Does not obviously break callers (type-compatible)
  - Addresses the specific finding
    |
Rank by confidence, present best candidate as suggestion
```

Fix confidence scoring should consider:
- **Pattern match**: Fix follows a well-known pattern (null check, error handling) -> high confidence
- **Locality**: Fix only modifies the flagged code, no side effects -> higher confidence
- **Complexity**: Simple one-line fix -> higher confidence than multi-file refactor
- **Test validation**: If tests exist and the fix maintains them -> highest confidence

---

## 10. Cross-File Dependency Analysis

### Change Impact Analysis Research

**Interprocedural Semantic Change-Impact Analysis (arXiv:1609.08734, 2016)**:
- Formalizes change-impact analysis as determining which program elements are affected by modifications.
- Uses equivalence relations to compute impact sets.
- Achieved approximately 35% improvement in precision for semantics-preserving transformations.
- Key finding: Many changes that look impactful are actually semantics-preserving refactors. Filtering these out dramatically reduces noise.

**RippleGUItester (arXiv:2603.03121, Mar 2026)**:
- Uses LLM-based change-impact analysis to generate test scenarios by treating code modifications as epicenters that "ripple" through system behavior.
- Differential analysis compares execution between pre-change and post-change versions.
- Found 26 previously unknown bugs across Firefox, Zettlr, JabRef, and Godot (16 subsequently fixed).
- Key insight: Existing testing approaches miss change-induced issues because they follow predefined paths rather than exploring change-aware scenarios.

### Cross-file analysis in existing tools

**CodeRabbit**: Multi-repo analysis identifies breaking changes, API mismatches, and dependency issues across repository boundaries.

**Greptile**: Full codebase indexing enables detection of logic issues spanning multiple files. Caught production bugs involving conflicting implementations across files in Netflix, NVIDIA, Meta repos.

**CodeQL**: Full database queries enable interprocedural taint tracking and call graph analysis across the entire codebase.

### Sight takeaway: practical cross-file analysis

Full dependency graph construction is expensive. Sight should use a targeted approach:

1. **Function signature changes**: If a function's signature changed (parameters added/removed, types changed, return type changed), automatically find all callers.
2. **Interface/trait/protocol changes**: If an interface changed, find all implementers and verify compliance.
3. **Export changes**: If a module's exports changed, find all importers.
4. **Shared state changes**: If a global/shared variable's type or invariants changed, find all readers/writers.
5. **Configuration changes**: If config schema changed, find all config consumers.

For each category, sight should:
- Use AST parsing or language-server-protocol queries (fast, no LLM needed) to find affected files
- Include affected callsites in the LLM context for impact assessment
- Flag potential breaking changes with the specific affected locations

This avoids the cost of indexing the entire codebase while catching the highest-value cross-file issues.

---

## 11. Review Prioritization

### What the research says about prioritization

**Greptile's approach**: Confidence scoring with severity-based triage. 63% of PRs receive no comments at all (high confidence of correctness). High-confidence PRs merge 3.6x faster.

**STAF (arXiv:2604.18525)**: Classifier achieving 89% F1 on predicting whether a static analysis finding is actionable. Non-actionable findings are suppressed.

**SonarQube quality gates**: Binary pass/fail based on configurable thresholds. Simple but effective for preventing the worst issues.

**Google AutoCommenter**: Tracks developer response to each comment type. Deprioritizes categories with low acceptance rates. The system learns what matters over time.

### Prioritization dimensions for sight

Each finding should be scored on multiple axes:

| Dimension | Weight | Signal |
|-----------|--------|--------|
| **Severity** | High | Could this cause data loss, security breach, or crash? |
| **Confidence** | High | How certain is the analysis? (Pattern match > LLM reasoning) |
| **Actionability** | High | Is the fix clear and specific? |
| **Locality** | Medium | Is the issue in newly written code? (Higher priority than pre-existing) |
| **Blast radius** | Medium | How many users/features are affected? |
| **Reversibility** | Low | Can this be easily fixed later, or is it an architectural decision? |

### Sight's prioritization system

```
Priority = severity * 0.35 + confidence * 0.30 + actionability * 0.20 + locality * 0.15

P0 (must fix): Security vulnerabilities in new code with clear fix
P1 (should fix): Logic errors, correctness issues, breaking changes
P2 (consider): Performance issues, maintainability concerns
P3 (optional): Style issues, naming suggestions, documentation gaps

Default behavior:
- Solo developer mode: Show P0 and P1 only. P2 on request. Never show P3 automatically.
- Team mode: Show P0, P1, P2. P3 in a collapsed section.
```

For solo developers, the priority threshold should be HIGH by default. A solo developer using sight as their only reviewer wants a trusted senior engineer who speaks up only when it matters, not a pedantic lint tool that comments on every line.

---

## 12. Historical Pattern Learning

### Approaches from research and tools

**Google AutoCommenter**: Tracks which comments developers accept (apply the suggestion) vs dismiss. Categories with consistently low acceptance are deprioritized or retrained. This creates a feedback loop: the system improves precision over time by learning from developer responses.

**CodeRabbit learnable patterns**: Users teach preferences via chat interactions ("always check for null before accessing .length in our codebase"). These are stored persistently and applied to future reviews. Path-based instructions (glob patterns) enable context-aware customization.

**iCodeReviewer routing**: Learns which prompt experts are relevant for which code patterns, avoiding unnecessary expert activation over time.

### What sight should learn from history

For a solo developer, historical patterns come from two sources:

1. **The developer's own PR history**: What issues does this developer commonly introduce? If they frequently forget error handling, weight that concern higher. If they never have security issues, weight security lower (but never zero).
2. **The repository's patterns**: What coding conventions exist? What does the test infrastructure look like? What frameworks and patterns are used?

### Sight implementation

**Phase 1 (config-driven)**: Allow developers to specify review preferences in a configuration file:
```yaml
concerns:
  security: high
  correctness: high
  performance: medium
  style: off
custom_rules:
  - pattern: "TODO without issue link"
    severity: low
    message: "TODOs should reference an issue tracker link"
  - pattern: "error swallowed without logging"
    severity: high
    message: "Errors should be logged or returned, not swallowed"
```

**Phase 2 (implicit learning)**: Track which findings the developer acknowledges vs dismisses. After N dismissed findings of a type, reduce that type's priority. After N acknowledged findings, increase confidence in that finding type.

**Phase 3 (repository learning)**: On first run, analyze the codebase to learn:
- Naming conventions (snake_case vs camelCase vs PascalCase)
- Error handling patterns (return error vs throw vs Result type)
- Test patterns (test file naming, assertion style, mock patterns)
- Import organization
- Commit message conventions

Use these learned patterns as baseline expectations for review.

---

## 13. Bug Prediction from Diffs

### Key research findings

**ReDef (arXiv:2509.09192)**: Compact diff-style encodings outperform whole-function formats for defect prediction. However, current CLMs exploit surface-level cues rather than genuine semantic understanding. Counterfactual perturbations (swapping add/delete) do not degrade performance, proving shallow matching.

**Change-Aware Defect Prediction (arXiv:2512.23875)**: Multi-agent debate framework for reasoning over code changes between versions. Traditional models show inflated metrics due to label-persistence bias. Change-aware evaluation reveals real predictive capability.

### Bug prediction signals from diffs

Research and practice identify these signals as predictive of defect-introducing changes:

| Signal | Why predictive | How to detect |
|--------|---------------|--------------|
| **High churn files** | Files that change frequently contain more bugs | Git log frequency analysis |
| **Author unfamiliarity** | Changes to code the author did not write | Git blame vs commit author |
| **Large diffs** | More changes = more opportunities for bugs | Hunk count, line count |
| **Cross-concern changes** | Mixing refactoring with feature work | Hunk semantic analysis |
| **Missing test changes** | Behavior change without test update | Diff file list analysis |
| **Complex conditionals** | New nested if/else, switch statements | AST complexity metrics |
| **Error handling changes** | Modified catch/error blocks | Pattern matching on diff |
| **Concurrent code changes** | Modifications to shared state, locks | Pattern matching + LLM |
| **Late-night commits** | Developer fatigue indicator | Commit timestamp |
| **Reverted-then-modified** | Prior failed attempt at same change | Git history analysis |

### Sight implementation for bug prediction

Sight should compute a "risk score" for each PR/commit based on these signals:

```
risk_score = weighted_sum(
    file_churn_percentile * 0.15,
    author_unfamiliarity * 0.10,
    diff_size_normalized * 0.15,
    missing_test_updates * 0.20,
    conditional_complexity_delta * 0.15,
    error_handling_changes * 0.10,
    cross_concern_mixing * 0.15
)
```

Use the risk score to:
1. Determine how much LLM analysis budget to spend on this change (high risk = use Opus, low risk = use Haiku or skip)
2. Choose which concerns to analyze (high risk = all concerns, low risk = correctness only)
3. Set the confidence threshold for surfacing findings (high risk = lower threshold to catch more, low risk = higher threshold to reduce noise)

---

## 14. Sight Implementation Priorities

Ordered by impact for a solo developer who needs a reliable automated reviewer:

### P0 -- Implement First (highest impact, table stakes)

1. **Unified diff parser with structured hunk extraction** (Section 5, 7)
   - Parse unified diffs into structured hunks with file path, line ranges, added/deleted/context lines.
   - Use compact diff encoding (proven superior by ReDef).
   - Include commit message and PR description as first-class context.
   - Implementation: Pure library, no LLM needed.

2. **Context enrichment pipeline** (Section 1, 7)
   - For each hunk: fetch containing function, class, imports, and type definitions.
   - Fetch corresponding test file and whether tests were modified.
   - Include git blame summary and file churn metrics.
   - Textual context (commit messages, issue descriptions) yields greater performance gains than code context (ContextCRBench).
   - Implementation: Git operations + file reading, no LLM needed.

3. **Parallel multi-concern LLM analysis** (Section 6)
   - Separate prompts for correctness, security, and performance (the three concerns solo devs care about most).
   - Concern-specific system prompts with relevant taxonomies (CWE for security, complexity heuristics for performance).
   - Run in parallel to minimize wall-clock time.
   - Implementation: LLM calls via eyrie.

4. **Self-reflection false positive filter** (Section 8)
   - After generating findings, run verification pass: "Is this finding actually correct given the full context?"
   - The Tencent study shows this eliminates 94-98% of false positives.
   - Implementation: Additional LLM call per finding.

### P1 -- Implement Next (high impact)

5. **Confidence scoring and prioritization** (Section 11)
   - Score each finding on severity, confidence, actionability.
   - Default to high threshold -- show only P0/P1 findings.
   - Solo developer mode: be a trusted senior engineer, not a pedantic linter.
   - Greptile data: 63% of PRs need zero comments. Aspire to this.

6. **Auto-fix generation with confidence tiers** (Section 9)
   - Generate fix suggestions for each finding.
   - Feed the finding explanation back as fix specification (72.97% repair rate from Zhao et al.).
   - Score fix confidence: pattern-based > LLM-generated > speculative.
   - Generate multiple candidates for high-severity findings.

7. **Risk-based analysis depth** (Section 13)
   - Compute risk score from diff signals (size, churn, missing tests, author unfamiliarity).
   - High-risk changes get deep analysis (Opus-class model, all concerns).
   - Low-risk changes get fast scan (Haiku-class, correctness only).
   - Cost optimization: spend analysis budget where it matters.

8. **Cross-file impact detection** (Section 10)
   - Detect function signature changes, interface changes, export changes.
   - Use AST/grep to find affected callsites (no LLM needed for detection).
   - Include affected callsites in LLM context for impact assessment.
   - Flag potential breaking changes.

### P2 -- Implement Later (moderate impact)

9. **Security metadata quarantine** (Section 4)
   - Run security analysis with redacted metadata to prevent adversarial framing bias.
   - Compare redacted vs non-redacted analysis results. Discrepancies flag potential issues.
   - Include CWE taxonomy in security prompts.
   - Critical per arXiv:2603.18740: iterative refinement attacks achieve 100% bypass.

10. **Smart hunk routing** (Section 6)
    - Classify each hunk to determine which concerns apply.
    - Skip security analysis for README changes. Skip style analysis for generated code.
    - Reduces LLM cost and false positives from irrelevant analysis.

11. **Historical pattern learning** (Section 12)
    - Config file for developer preferences and custom rules.
    - Track finding acceptance/dismissal rates.
    - Adjust concern weights and confidence thresholds over time.
    - Repository convention detection on first run.

12. **Test coverage gap detection** (Section 6)
    - Detect behavior changes without corresponding test modifications.
    - Identify untested edge cases from changed conditionals.
    - Suggest specific test cases for new code paths.

### P3 -- Evaluate Later (lower impact or high complexity)

13. **Multi-agent debate for complex findings**: Use two LLMs with different prompts to debate whether a finding is valid. Higher cost but higher precision. Per arXiv:2512.23875, multi-agent debate improves sensitivity to defect introductions.

14. **Learned risk model**: Train a classifier on the developer's own commit history to predict defect-introducing changes. Requires sufficient history (100+ commits).

15. **Full call graph construction**: Build complete dependency graphs for deep impact analysis. High cost, diminishing returns beyond the targeted approach in P1.

16. **Cross-repository analysis**: Detect breaking changes across dependent repositories. Requires access to multiple repos and their dependency relationships.

---

## Key Numbers for Sight's Design Decisions

### Precision/Recall/Acceptance from research

| System/Paper | Metric | Value | Context |
|-------------|--------|-------|---------|
| Tencent LLM FP filter | FP elimination | 94-98% | Hybrid static analysis + LLM filter |
| iCodeReviewer | F1 | 63.98% | Security review with mixture of prompts |
| iCodeReviewer | Production acceptance | 84% | Human acceptance of review comments |
| STAF alert filter | F1 | 89% | Actionability classification |
| Zhao et al. LLM repair | Repair rate | 72.97% | Code review defect repair with optimized prompts |
| REFINE patch refinement | SWE-Bench Lite | 51.67% | Repository-level program repair |
| ContextCRBench at ByteDance | Performance improvement | 61.98% | With enriched textual context |
| Greptile | No-comment PRs | 63% | PRs passing with high confidence |
| Greptile | Merge speed improvement | 3.6x | For high-confidence PRs |
| Legacy SAST tools | False positive rate | 50-80% | Industry-wide problem |
| Tencent manual review | Time per FP alarm | 10-20 min | Developer cost of false positives |
| LLM FP filter | Cost per alarm | $0.001-$0.12 | Orders of magnitude less than manual |
| Comment classification | Zero-shot macro-F1 | 0.360-0.374 | Review comment type classification |

### Threat model for sight's security review

| Attack | Source | Mitigation |
|--------|--------|------------|
| Adversarial metadata | Commit messages, PR descriptions | Redact for security analysis (arXiv:2603.18740) |
| LLM overcorrection | Model tendency to flag correct code | Fix-guided verification (arXiv:2603.00539) |
| Prompt injection in code | Malicious comments/strings in diff | Sanitize code strings before analysis |
| Shallow pattern matching | LLMs not understanding diff semantics | Explicit semantic annotation (ReDef) |

---

## Key Repository and Tool References

| Project | URL | Key Feature for Sight |
|---------|-----|----------------------|
| PR-Agent (Qodo) | github.com/Codium-ai/pr-agent | PR compression, self-reflection, multi-tool review |
| CodeRabbit | github.com/coderabbitai/ai-pr-reviewer | Two-tier models, learnable patterns, 40+ linters |
| Sourcery | github.com/sourcery-ai/sourcery | Three-component review output structure |
| Semgrep | github.com/semgrep/semgrep | Pattern matching, taint mode, diff-aware scanning |
| CodeQL | github.com/github/codeql | Relational code database, cross-file taint tracking |
| SonarQube | github.com/SonarSource/sonarqube | 7K+ rules, quality gates, four-category taxonomy |

## Key Papers

| Paper | ID/Venue | Key Finding for Sight |
|-------|----------|----------------------|
| CodeReviewer | arXiv:2203.09095 / ESEC/FSE 2022 | Diff-specific pre-training outperforms generic models |
| ContextCRBench | arXiv:2511.07017 | Textual context > code context for review quality |
| ReDef | arXiv:2509.09192 | Compact diffs beat whole-function; LLMs use surface cues |
| FP Reduction (Tencent) | arXiv:2601.18844 | LLM filter eliminates 94-98% of static analysis FPs |
| iCodeReviewer | arXiv:2510.12186 | Mixture of prompts with routing reduces hallucination |
| LLM Overcorrection | arXiv:2603.00539 | Detailed prompts increase misjudgment; validate fixes |
| Security Bias | arXiv:2603.18740 | Metadata framing bypasses LLM security review |
| LLM Security Review | arXiv:2401.16310 | LLMs outperform SAST; CWE prompting helps |
| Change-Aware Defect | arXiv:2512.23875 | Multi-agent debate on changes beats static prediction |
| Review Comment Taxonomy | arXiv:2604.23667 | 9-label taxonomy; evidence-sensitive labels need more context |
| Review Benchmark Survey | arXiv:2602.13377 | 99 papers analyzed; field moving to end-to-end systems |
| REFINE | arXiv:2510.03588 | Multi-candidate fix generation + review = 14% improvement |
| LLM Defect Repair | arXiv:2312.17485 | Review comments as fix specs = 72.97% repair rate |
| SapFix (Meta) | ICSE 2019 | Revert > template > mutation fix hierarchy |
| STAF | arXiv:2604.18525 | 89% F1 on actionability filtering of SA findings |
| CodeSecEval | arXiv:2407.02395 | LLMs miss vulnerabilities without explicit threat context |
| Grounded Copilot | arXiv:2206.15000 | Acceleration vs exploration modes in AI-assisted coding |
