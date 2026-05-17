# Eyrie: Universal LLM Provider Runtime -- State of the Art Research

Research compiled 2026-05-03. Sources: GitHub repos, API documentation, arXiv papers, and project wikis for 20+ systems.

---

## Table of Contents

1. [Universal LLM API Proxies](#1-universal-llm-api-proxies)
2. [Intelligent Model Routing](#2-intelligent-model-routing)
3. [Semantic and Response Caching](#3-semantic-and-response-caching)
4. [Provider Prompt Caching](#4-provider-prompt-caching)
5. [Cost Optimization: Cascading and FrugalGPT](#5-cost-optimization-cascading-and-frugalgpt)
6. [Retry and Fallback Strategies](#6-retry-and-fallback-strategies)
7. [Rate Limiting Algorithms](#7-rate-limiting-algorithms)
8. [Streaming, Connection Pooling, and Keep-Alive](#8-streaming-connection-pooling-and-keep-alive)
9. [Batch API Utilization](#9-batch-api-utilization)
10. [Request Routing: Latency/Cost/Quality Tradeoffs](#10-request-routing-latencycostquality-tradeoffs)
11. [Model Selection by Task Complexity](#11-model-selection-by-task-complexity)
12. [Multi-Turn Conversation Optimization](#12-multi-turn-conversation-optimization)
13. [Provider Health Monitoring and Circuit Breaking](#13-provider-health-monitoring-and-circuit-breaking)
14. [LLM Observability](#14-llm-observability)
15. [API Key Rotation and Security](#15-api-key-rotation-and-security)
16. [Eyrie Implementation Priorities](#16-eyrie-implementation-priorities)

---

## 1. Universal LLM API Proxies

### LiteLLM (BerriAI)
- **What**: Open-source gateway providing a unified OpenAI-compatible interface to 100+ LLM providers. Operates as both a Python SDK and a deployable proxy server.
- **Key features**: Unified chat/completion/embedding API, virtual keys, spend tracking, cost management, comprehensive logging callbacks (Lunary, MLflow, Langfuse).
- **Measured performance**: 8ms P95 latency at 1,000 RPS -- minimal overhead even under load.
- **Routing strategies implemented**: simple-shuffle (default), least-busy, lowest-latency, usage-based (lowest TPM), cost-based. Custom strategies via `CustomRoutingStrategyBase`.
- **Caching**: In-memory, Redis (with cluster), S3, GCS, Azure Blob, disk, and semantic (Redis + Qdrant with embeddings). Cache keys generated via SHA-256 of concatenated parameters. Dual-cache strategy for layered backends.
- **Reliability**: Cooldown after N failures (default 3/min, 5s cooldown), exponential backoff for rate limits, deployment health cache, pre-call rate limit checks, context-window fallbacks, content-policy fallbacks.
- **Architecture insight**: Cache key generation hashes all LLM API params into a deterministic key with optional namespace prefixing. Preset cache keys avoid duplicate work when params transform across providers.

### Portkey AI Gateway
- **What**: Open-source TypeScript gateway (96% TS codebase) supporting 1,600+ models across 45+ providers. Under 1ms added latency, 122KB footprint. Processes 10B+ tokens/day.
- **Key features**: Automatic retries (up to 5, exponential backoff 1s-16s), fallbacks between models/providers, weight-based load balancing, simple + semantic caching, 40+ guardrails, PII redaction.
- **Caching**: Simple (exact match) and semantic (cosine similarity, enterprise only). TTL range: 60s to 90 days, default 7 days. Semantic limited to 8,191 tokens, 4 messages, OpenAI-compatible embeddings only. Claims up to 20x faster responses on cache hits.
- **Circuit breaking**: Implemented via `isOpen` flag on targets; `handleCircuitBreakerResponse()` evaluates responses and updates health status. Unhealthy targets filtered from routing pool.
- **Load balancing**: Weight-based probabilistic selection (`Math.random() * totalWeight`). Sticky sessions via hash-based routing with TTL (default 3,600s) and dual-tier caching. Supports gradual migration (10% to new model).
- **Fallback**: Multi-tier chains with conditional triggers (customizable status codes, default: any non-2xx). Composable -- nest fallbacks inside load balancers.
- **Retry**: Uses provider `retry-after` headers when available; falls back to exponential backoff. Max cumulative wait 60s. Tracks via `x-portkey-retry-attempt-count` header.

### OpenRouter
- **What**: Commercial multi-provider routing API supporting hundreds of models through a single OpenAI-compatible interface.
- **Key features**: Automatic failover when providers experience downtime/rate-limiting/refusals. Model variants (`:free`, `:extended`, `:thinking`, `:nitro`). Auto Router for prompt-based model selection. BYOK (bring your own keys). Zero-charge policy on failed/empty responses.
- **Provider routing**: Intelligent routing across providers optimizing for cost, performance, and reliability. Real-time monitoring and routing maximize availability.
- **Observability**: Integrates with Langfuse, Datadog, and others for request tracing.

### Eyrie takeaway
Eyrie should implement a unified provider interface with the lightweight approach of Portkey (sub-ms overhead, small footprint) rather than the heavier proxy approach of LiteLLM. Key primitives: provider adapter trait, request/response normalization, and streaming passthrough. The hawk project already has `model/catalog.go` and `model/router.go` -- extend these with multi-provider support.

---

## 2. Intelligent Model Routing

### RouteLLM (LMSYS) -- arXiv:2406.18665
- **What**: Framework for routing between a strong (expensive) model and a weak (cheap) model based on query difficulty.
- **Techniques**: Matrix factorization (recommended), BERT classifier, semantic weighting (Elo-based), causal LLM classifier, random baseline.
- **Measured improvement**: Up to 85% cost reduction while maintaining 95% of GPT-4 performance on MT-Bench. Over 2x cost savings in general. Outperforms commercial offerings while being 40%+ cheaper.
- **Key insight**: Router models exhibit strong transfer -- they work well even when the underlying strong/weak models change at test time. Trained on human preference data with data augmentation.
- **Decision mechanism**: Cost threshold -- for each query, compute win-rate probability for the strong model. If above user-specified threshold, route to strong; else route to weak. Evaluated on MMLU, GSM8K, MT-Bench.

### Not Diamond
- **What**: Meta-model that learns when to call each LLM. Analyzes prompts and routes to the optimal model.
- **Claims**: Outperforms every individual foundation model on major benchmarks by consistently selecting the best model per prompt.
- **Features**: Cost/latency tradeoff configuration, custom router training on user data, Python/TypeScript/REST APIs.
- **Supported providers**: OpenAI, Anthropic, with user-defined model pools.

### Martian (RouterBench) -- arXiv:2403.12031
- **What**: Benchmarking framework for multi-LLM routing systems.
- **Dataset**: 405K+ inference outcomes from representative LLMs.
- **Key finding**: Establishes that no single model optimally addresses all tasks, especially when balancing performance with cost. Provides the first standardized evaluation framework for LLM routers.
- **Architecture**: Extensible via abstract base classes for new routing strategies and data formats.

### Hybrid LLM Serving -- ICLR 2024 (arXiv:2404.14618)
- **What**: Routes queries between large and small models based on predicted difficulty.
- **Measured improvement**: Up to 40% fewer calls to the large model with no drop in response quality.
- **Mechanism**: Router predicts query difficulty and desired quality level; dynamic quality tuning at inference time.

### Eyrie takeaway
Implement a two-tier router as the primary cost optimization. For a coding agent, this means: route simple queries (file reads, grep, explanations of short code) to Haiku/small models; reserve Opus/large models for complex reasoning, multi-file refactors, architecture decisions. The matrix factorization approach from RouteLLM is the most practical -- it can be distilled into a lightweight classifier that adds negligible latency. Start with a simpler heuristic (token count, tool complexity, conversation depth) and graduate to a learned router.

---

## 3. Semantic and Response Caching

### GPTCache (Zilliz)
- **What**: Semantic cache layer using embeddings + vector similarity search for LLM responses.
- **Measured improvement**: ~10x cost reduction, ~100x speed improvement on cache hits.
- **How it works**: Convert queries to embeddings, store in vector DB, find K most similar cached requests, evaluate semantic similarity, return cached response if above threshold.
- **Eviction**: LRU, FIFO, LFU, random replacement. Supports in-memory and Redis-distributed.
- **Embeddings**: OpenAI API, ONNX, HuggingFace, SentenceTransformers.
- **Vector stores**: Milvus, FAISS, Chroma, PGVector.
- **Metrics**: Hit ratio, latency, recall.

### LiteLLM Caching
- **Implementations**: In-memory, Redis, Redis semantic, Qdrant semantic, S3, GCS, Azure Blob, disk.
- **Semantic**: Uses configurable similarity threshold. Redis variant defaults to `text-embedding-ada-002` for embeddings. Qdrant variant supports quantization.
- **Key generation**: SHA-256 hash of concatenated `param: value` pairs, with optional namespace prefix.
- **TTL**: Global default, type-specific defaults (in-memory vs Redis), and per-request overrides. Validates freshness via `max_age` comparison.

### Portkey Caching
- **Simple cache**: Exact match, works across all models including image generation.
- **Semantic cache**: Cosine similarity matching (enterprise only). Limited to 8,191 tokens, 4 messages.
- **TTL**: 60s min to 90 days max, default 7 days. Per-request namespacing.
- **Claims**: Up to 20x faster responses on cache hits.

### Helicone Caching
- **What**: Edge caching on Cloudflare's network for LLM responses.
- **Cache key**: Hash of seed + request URL + body + headers + bucket index. Any component change creates new entry.
- **Configuration**: 7-day default TTL, max 365 days. Bucket max size for multiple responses per identical request (useful for temperature > 0).
- **Metrics**: Dashboard shows hit counts, cost savings, and time saved.

### Eyrie takeaway
For a coding agent, semantic caching has limited value -- most queries involve unique code context. Instead, implement:
1. **Exact-match response cache** with SHA-256 of (model, system prompt hash, messages hash, tools hash, temperature). Use for deterministic calls (temperature=0).
2. **Tool result caching** -- cache file reads, grep results, and other deterministic tool outputs with file-mtime-based invalidation. This is more impactful than response caching.
3. **Prefix deduplication** -- detect when conversation prefix is identical across requests and reuse via provider prompt caching rather than response caching.

Skip semantic caching initially. The embedding overhead and imprecise matching make it unsuitable for code-heavy contexts where small changes in code are semantically similar but functionally different.

---

## 4. Provider Prompt Caching

### Anthropic Prompt Caching (the highest-impact optimization for eyrie)
- **How it works**: Cache prompt prefixes at breakpoints. On cache hit, skip prefill of cached content. On miss, process and cache for future use.
- **TTL**: 5-minute default (ephemeral), 1-hour option at 2x base input price. Cache refreshed at no cost on each use within TTL.
- **Pricing**:
  - Base input: 1.0x (e.g., $5/M tokens for Opus 4.7)
  - 5-min cache write: 1.25x ($6.25/M)
  - 1-hour cache write: 2.0x ($10/M)
  - Cache hit/read: 0.1x ($0.50/M) -- **90% savings**
- **Minimum tokens**: 1,024 (Sonnet 4/4.5, Opus 4/4.1), 2,048 (Sonnet 4.6, Haiku 3.5), 4,096 (Opus 4.6/4.7, Haiku 4.5).
- **Breakpoint limit**: Up to 4 explicit breakpoints per request.
- **Lookback window**: 20-block lookback for finding previously written cache entries.
- **Cache invalidation**: Tool definition changes invalidate all caches. Web search/citations/speed toggle changes invalidate system+messages. Tool choice and image changes invalidate messages only.
- **Rate limit benefit**: Cached tokens do NOT count toward ITPM rate limits on most models. With 80% cache hit rate, effective throughput is 5x the nominal limit (e.g., 2M ITPM becomes 10M effective).
- **Pre-warming**: Send `max_tokens: 0` requests to pre-populate cache before user arrives.
- **Best practice**: Place `cache_control` on the LAST block that remains identical across requests. Static content first, varying content after. Use automatic caching for multi-turn conversations.

### Google Gemini Context Caching
- **Implicit caching**: Enabled by default on Gemini 2.5+ models. Automatic cost savings on cache hits.
- **Explicit caching**: Manual, cache once and reference for subsequent requests.
- **Minimum tokens**: 1,024 (Flash models), 4,096 (Pro models).
- **TTL**: Default 1 hour. Pricing covers cached tokens (reduced), storage duration, plus standard charges.
- **Best use cases**: System instructions, recurring video/document analysis, code repository analysis.

### Eyrie takeaway -- THIS IS THE SINGLE HIGHEST-ROI FEATURE
For a coding agent, prompt caching is transformative. The system prompt + tool definitions + repository context can be 50K-200K tokens. At 90% discount on cache hits, this saves $4.50-$9.00 per million cached tokens on every request.

Implementation plan:
1. **Structure prompts for cacheability**: System instructions and tool definitions FIRST (rarely change), then repo context (changes per-session), then conversation history (grows), then user message (changes every turn).
2. **Use up to 4 breakpoints**: (a) tool definitions, (b) system prompt, (c) repo context, (d) end of conversation history. Each changes at different frequencies.
3. **Automatic breakpoint advancement**: For multi-turn conversations, use automatic caching so the breakpoint moves forward as conversation grows.
4. **1-hour cache for tool definitions**: These rarely change; use 1-hour TTL. Use 5-min for conversation context that updates frequently.
5. **Pre-warm on session start**: Fire `max_tokens: 0` request to cache system prompt + tools before the user's first query.
6. **Monitor cache performance**: Track `cache_read_input_tokens` vs `cache_creation_input_tokens` to measure hit rates. Alert if hit rate drops below 70%.
7. **Avoid cache-busting**: Never put timestamps, request IDs, or per-request context before the cache breakpoint. Keep the 20-block lookback window in mind for long conversations -- add explicit breakpoints if conversation exceeds 20 blocks from previous write.

---

## 5. Cost Optimization: Cascading and FrugalGPT

### FrugalGPT (arXiv:2305.05176)
- **Three strategies**:
  1. **Prompt adaptation**: Modify queries to reduce computational demands (shorter prompts, fewer examples).
  2. **LLM approximation**: Use efficient model alternatives.
  3. **LLM cascade**: Route queries through a chain of models, starting cheap, escalating only when needed.
- **Measured results**: Matches GPT-4 performance with up to 98% cost reduction. Or improves accuracy by 4% over GPT-4 at equivalent cost.
- **Key insight**: The cascade approach -- try cheap model first, evaluate confidence, escalate if needed -- is the most practical for production systems.

### Model Cascading for Coding Agents
For eyrie, cascading is particularly effective because coding tasks have highly variable complexity:

| Task Type | Appropriate Model | Relative Cost |
|-----------|------------------|---------------|
| File read/write, simple grep | No LLM needed (tool-only) | 0x |
| Code explanation, simple questions | Haiku 4.5 | 1x |
| Single-file edits, test writing | Sonnet 4.x | 5-8x |
| Multi-file refactors, architecture | Opus 4.x | 25-50x |

### Eyrie takeaway
Implement a three-tier cascade:
1. **Tier 0 -- Tool-only**: Detect requests that can be fulfilled by tools alone (file reads, directory listings, grep). Skip the LLM entirely.
2. **Tier 1 -- Fast/cheap model**: Use Haiku/Flash for classification, simple Q&A, formatting, and confidence scoring.
3. **Tier 2 -- Strong model**: Escalate to Sonnet/Opus when Tier 1 confidence is below threshold or task complexity is detected.

The hawk project already has `engine/cost.go` and `engine/cost_table.go` -- extend these to track per-tier spending and measure cascade effectiveness. Use `model/health_router.go` as the foundation for cascade routing logic.

---

## 6. Retry and Fallback Strategies

### Research findings consolidated

**Portkey approach**:
- Up to 5 retries with exponential backoff (1s, 2s, 4s, 8s, 16s).
- Retries on [429, 500, 502, 503, 504] by default, customizable.
- Respects provider `retry-after` headers, overriding exponential backoff. Max cumulative 60s.
- Multi-tier fallback chains: GPT-4o -> Claude Sonnet -> Gemini Pro. Composable with load balancing.

**LiteLLM approach**:
- Cooldown after failure threshold (default 3 failures/minute, 5s cooldown).
- Exponential backoff for rate limits, immediate retry for generic failures.
- Error-specific policies via `RetryPolicy` and `AllowedFailsPolicy` (separate handling for ContentPolicyViolation, RateLimitError, etc.).
- Pre-call checks filter out deployments that would exceed rate limits.
- Priority-ordered fallback with `order` parameter.

**OpenRouter approach**:
- Automatic failover when providers experience downtime, rate limiting, or refusals.
- Zero-charge on failed/empty responses.

### Best practices synthesized
1. **Classify errors**: Distinguish transient (429, 502, 503) from permanent (400, 401, 404). Only retry transient.
2. **Respect `retry-after`**: Providers set this header for a reason. Always honor it.
3. **Jitter**: Add random jitter to backoff to prevent thundering herd.
4. **Budget-aware retries**: Set a total retry time budget (e.g., 30s for interactive, 120s for background). Abandon rather than keeping the user waiting.
5. **Fallback ordering**: Same model on different provider first (preserves behavior), then different model (may change behavior).
6. **Context-window fallback**: If request exceeds model context, automatically try a model with larger context rather than failing.

### Eyrie takeaway
The hawk project already has `retry/retry.go` and `circuit/circuit.go`. Extend with:
1. **Error classification enum**: Transient, RateLimit, ContextOverflow, ContentPolicy, AuthFailure, Unknown.
2. **Per-error-class retry policy**: Different backoff curves and max attempts per class.
3. **Provider-aware fallback chain**: Define primary -> fallback paths per model capability tier.
4. **Retry budget**: Total time budget per request, not just per-attempt timeout.
5. **Hedged requests**: For latency-critical paths, fire request to two providers simultaneously, use first response (cancel the other). Costs 2x but halves tail latency.

---

## 7. Rate Limiting Algorithms

### Anthropic's Rate Limit Structure
- **Algorithm**: Token bucket (continuous replenishment, not fixed-window reset).
- **Dimensions**: RPM (requests/min), ITPM (input tokens/min), OTPM (output tokens/min). Separate limits per model family.
- **Cache-aware ITPM**: Only uncached input tokens count toward ITPM. With 80% cache hit rate, effective throughput is 5x nominal. This makes prompt caching a rate-limit multiplier, not just a cost saver.
- **Headers**: Full suite -- `anthropic-ratelimit-{requests,tokens,input-tokens,output-tokens}-{limit,remaining,reset}` plus `retry-after`.
- **Tier structure**: Tier 1 (50 RPM, 30K ITPM for Sonnet) through Tier 4 (4,000 RPM, 2M ITPM for Sonnet). Solo developers typically at Tier 2-3.
- **Acceleration limits**: Sharp usage increases trigger additional 429s even within nominal limits. Ramp up gradually.

### Rate Limiting Algorithms for Eyrie

**Token Bucket** (what providers use):
- Tokens added at constant rate up to max capacity.
- Each request consumes tokens equal to its estimated cost.
- Predictable, fair, allows bursts up to bucket capacity.
- Best for: tracking provider-side limits locally.

**Sliding Window Log**:
- Track exact timestamps of each request in a window.
- Most accurate but highest memory.
- Best for: precise local rate limiting of outgoing requests.

**Sliding Window Counter** (recommended for eyrie):
- Hybrid: fixed window counters + sliding window interpolation.
- Low memory, good accuracy (within ~1% of true sliding window).
- Best for: client-side rate limiting with low overhead.

**Adaptive Rate Limiting** (what eyrie should innovate on):
- Track remaining capacity from response headers.
- Predict time until limit reset.
- Pre-emptively throttle when approaching limits rather than waiting for 429s.
- Smooth request distribution across the minute to avoid burst-then-wait patterns.

### Eyrie takeaway
The hawk project has `ratelimit/ratelimit.go`. Enhance with:
1. **Header-driven capacity tracking**: Parse every response's rate limit headers. Maintain a real-time model of remaining capacity per provider/model.
2. **Predictive throttling**: When remaining capacity < estimated next request cost, delay rather than risk 429. Calculate optimal delay from reset time.
3. **Cache-aware budgeting**: Factor in expected cache hit rate when estimating ITPM consumption. A request with 100K cached tokens and 1K uncached tokens costs only 1K against ITPM.
4. **Cross-request coordination**: If running parallel tool calls, ensure aggregate rate stays within limits.
5. **Graceful degradation**: When approaching limits, automatically downgrade to cheaper models rather than queuing.

---

## 8. Streaming, Connection Pooling, and Keep-Alive

### Key findings from research

**Streaming SSE best practices**:
- All major providers use Server-Sent Events (SSE) over HTTP/1.1 or HTTP/2.
- Critical for coding agent UX: stream tokens as they arrive, don't wait for full response.
- Portkey gateway adds <1ms latency to streaming passthrough.

**Connection pooling**:
- HTTP/2 multiplexing eliminates the need for multiple TCP connections to the same provider.
- Keep connections alive between requests to the same provider (avoid TCP+TLS handshake per request: ~50-100ms savings).
- Go's `http.Client` with `Transport.MaxIdleConnsPerHost` handles this natively.
- Set `IdleConnTimeout` to match session duration (e.g., 5 minutes) rather than default 90s.

**Streaming implementation considerations**:
- Buffer SSE events and forward immediately -- don't accumulate.
- Handle partial JSON chunks in SSE `data:` fields.
- Implement heartbeat detection: if no event for 30s, the connection may be dead.
- Track time-to-first-token (TTFT) separately from total latency.

### Eyrie takeaway
The hawk project has `engine/stream.go`. Extend with:
1. **Per-provider connection pool**: Reuse HTTP/2 connections. One pool per provider base URL.
2. **Streaming multiplexer**: For parallel tool calls that each stream, multiplex SSE streams into a unified event stream.
3. **TTFT tracking**: Measure and report time-to-first-token as a key latency metric.
4. **Dead connection detection**: Timeout if no SSE event within provider-specific threshold (Anthropic ~30s, OpenAI ~60s).
5. **Backpressure**: If consumer (terminal) can't keep up, buffer up to N events then apply backpressure to the stream.

---

## 9. Batch API Utilization

### Anthropic Message Batches API
- **Pricing**: 50% discount on both input and output tokens compared to real-time.
- **Completion window**: Up to 24 hours (typically faster).
- **Rate limits**: Separate from real-time. Tier 4: 4,000 RPM to batch endpoints, 500K batch requests in processing queue.
- **Batch size**: Up to 100,000 requests per batch.
- **Use cases**: Bulk processing, evaluation, data extraction, non-interactive tasks.

### OpenAI Batch API
- **Pricing**: 50% discount.
- **Completion window**: 24 hours.
- **Use cases**: Classification, embedding generation, bulk summarization.

### When to use batch vs real-time for a coding agent

| Scenario | Mode | Reason |
|----------|------|--------|
| Interactive coding assistance | Real-time streaming | User is waiting |
| Running test suite analysis | Batch | Can wait, 50% savings |
| Bulk file analysis on session start | Batch | Background prep work |
| Generating commit messages for N files | Batch | Can parallelize |
| Repository indexing/understanding | Batch | Background, large volume |
| Code review of multiple files | Real-time + batch hybrid | Show first result immediately, batch the rest |

### Eyrie takeaway
Implement a batch scheduler that:
1. **Detects batchable work**: When a task generates N independent LLM calls (e.g., analyze N files), batch them.
2. **Hybrid mode**: Fire the first request as real-time (show progress immediately), batch the remaining N-1.
3. **Background pre-computation**: On session start, batch-analyze the codebase and cache results.
4. **Automatic batching of tool call results**: When multiple tool calls return and each needs LLM processing, batch them.

---

## 10. Request Routing: Latency/Cost/Quality Tradeoffs

### Routing dimensions from research

**LiteLLM's five strategies**:
1. Simple-shuffle (random, weighted by RPM/TPM)
2. Latency-based (route to fastest, with buffer to prevent overload)
3. Usage-based (route to lowest TPM deployment)
4. Least-busy (fewest concurrent requests)
5. Cost-based (cheapest deployment from cost map)

**Portkey's approach**:
- Weight-based probabilistic selection across providers.
- Sticky sessions for conversation continuity.
- Conditional routing based on metadata/request parameters.

**OpenRouter's approach**:
- Auto Router selects model based on prompt content.
- Model variants for specific optimization goals (`:nitro` for speed, `:free` for cost).

### Optimal routing strategy for a coding agent

A coding agent has distinct routing needs vs. a general chatbot:

| Dimension | Priority | Rationale |
|-----------|----------|-----------|
| Quality | Highest for code generation | Incorrect code wastes more time than it saves |
| Latency | High for interactive, low for background | User waiting = highest priority |
| Cost | Medium | Solo dev budget matters, but correctness matters more |

### Eyrie takeaway
Implement a multi-objective router with mode switching:
1. **Interactive mode**: Optimize for (quality * 0.5 + latency * 0.4 + cost * 0.1). Prefer the best model that can respond quickly.
2. **Background mode**: Optimize for (quality * 0.4 + cost * 0.5 + latency * 0.1). Prefer cheaper models, use batch API.
3. **Budget-constrained mode**: Hard cost ceiling per session. Automatically downgrade models as budget depletes.

The existing `model/router.go` in hawk is the right place to add this. The `model/health_router.go` already tracks provider health -- extend with latency and cost tracking.

---

## 11. Model Selection by Task Complexity

### Research on task-complexity routing

**RouteLLM finding**: Matrix factorization on human preference data effectively predicts which queries need strong models. Transfers well to new model pairs.

**Hybrid LLM (ICLR 2024)**: Router predicts query difficulty. Achieves 40% fewer calls to large model with no quality drop.

**FrugalGPT**: Cascade through models cheapest-first, escalate when confidence is low. Up to 98% cost reduction matching GPT-4.

### Complexity signals for coding tasks

| Signal | Indicates | Route to |
|--------|-----------|----------|
| Single file mentioned | Simple edit | Sonnet/Haiku |
| Multiple files mentioned | Cross-file refactor | Opus |
| "Explain" / "What does" | Comprehension | Haiku |
| "Refactor" / "Redesign" | Architecture | Opus |
| Short conversation (< 5 turns) | Simple task | Sonnet |
| Long conversation (> 15 turns) | Complex/stuck | Opus |
| Tool call count > 5 in plan | Complex orchestration | Opus |
| Error in previous response | Needs better reasoning | Upgrade model |
| User explicitly requests quality | Direct signal | Opus |
| Test generation | Structured, pattern-heavy | Sonnet |

### Eyrie takeaway
Build a lightweight task classifier:
1. **Heuristic classifier first**: Use the signals above as rules. Fast, zero-cost, no external dependency.
2. **Confidence-based escalation**: If Haiku/Sonnet response includes hedging language ("I'm not sure", "this might not work"), automatically escalate to a stronger model.
3. **User override**: Always let user force a specific model tier (e.g., `/opus` command).
4. **Learning from corrections**: When user rejects a response from a cheaper model and the stronger model succeeds, log this as a training signal for future routing.

---

## 12. Multi-Turn Conversation Optimization

### KV Cache Reuse Research

**SGLang (RadixAttention)** -- arXiv:2312.07104:
- Technique for reusing KV cache across multiple generation calls.
- Achieves up to 6.4x higher throughput vs. state-of-the-art inference systems.
- Uses a radix tree to store and retrieve cached KV states by prefix.

**StreamingLLM (Attention Sinks)** -- ICLR 2024 (arXiv:2309.17453):
- Keeping KV of initial tokens recovers performance of window attention.
- 22.2x speedup over sliding window recomputation.
- Enables processing up to 4M tokens without fine-tuning.

### Multi-turn optimization for API-based usage

For API consumers (not self-hosted), KV cache is managed server-side. The optimization levers are:

1. **Prompt caching alignment**: Structure conversation so that the growing prefix remains cache-aligned. Anthropic's automatic caching handles this, but explicit breakpoints give finer control.
2. **Conversation compaction**: When conversation exceeds a threshold, summarize earlier turns. The hawk project already has `engine/compact.go` with multiple strategies -- this is excellent.
3. **Context window management**: Track token usage and proactively compact before hitting limits, not after.
4. **Stateful sessions**: Maintain conversation state so reconnections reuse cached context rather than re-sending everything.

### Anthropic's cache-aware multi-turn pattern
```
Turn 1: [system + tools | cached] [user msg]
Turn 2: [system + tools | cached from T1] [T1 history] [user msg]  
Turn 3: [system + tools | cached from T1] [T1-T2 history | cached from T2] [user msg]
```
Each turn, the previous conversation prefix is cached. New turns only process new content. With 90% cache discount, a 100-turn conversation where the system prompt is 50K tokens costs the same as if the system prompt were 5K tokens after the first turn.

### Eyrie takeaway
1. **Prompt structure contract**: Define a strict ordering -- tools, system prompt, repo context, conversation history, user message. Never deviate.
2. **Breakpoint management**: Automatically place breakpoints at the boundary between stable and changing content. Advance as conversation grows.
3. **Compaction trigger**: Use the existing compact strategies in hawk. Trigger compaction when: (a) approaching context limit, (b) cache hit rate drops (stale breakpoints), or (c) conversation exceeds 20 blocks from last explicit breakpoint.
4. **Session persistence**: Save conversation state to disk so that restarting the agent doesn't lose cached context.

---

## 13. Provider Health Monitoring and Circuit Breaking

### LiteLLM's implementation
- `DeploymentHealthCache` manages health states per deployment.
- `health_check_staleness_threshold` determines when to re-check.
- Cooldown: deployments go offline after N failures (default 3/min). Auto-recover after cooldown period (default 5s).
- Separate handling for transient vs. persistent errors.

### Portkey's implementation
- Circuit breaker via `isOpen` flag. `handleCircuitBreakerResponse()` hook evaluates every response.
- Unhealthy targets filtered from routing pool before selection.
- Fallback chains engage when primary circuit is open.

### Circuit breaker states (standard pattern)
```
CLOSED (healthy) --[failure threshold]--> OPEN (unhealthy, reject requests)
OPEN --[timeout]--> HALF-OPEN (allow one probe request)
HALF-OPEN --[probe succeeds]--> CLOSED
HALF-OPEN --[probe fails]--> OPEN
```

### Eyrie takeaway
The hawk project has `circuit/circuit.go`. Ensure it implements:
1. **Per-provider, per-model circuit breakers**: Separate circuits for `anthropic/opus` vs `anthropic/sonnet` vs `openai/gpt-4o`.
2. **Sliding window failure tracking**: Count failures in a 60s window. Open circuit at 3+ failures (configurable).
3. **Half-open probing**: After cooldown (5-30s), send one probe request. Close circuit on success.
4. **Health score**: Rather than binary open/closed, maintain a health score (0.0-1.0) based on recent success rate, latency percentiles, and error types. Route proportionally to health.
5. **Provider status page integration**: Poll Anthropic/OpenAI status pages for known outages. Pre-emptively open circuits.
6. **Cascade on circuit open**: When primary circuit opens, automatically engage fallback chain.

---

## 14. LLM Observability

### Helicone
- **What**: LLM observability proxy with 0% markup. Unified API for 100+ models.
- **Features**: Automatic logging, request dashboard, cost tracking, automatic fallbacks, caching on Cloudflare edge.
- **Caching**: Edge-cached responses. Keys hashed from URL + body + headers. Dashboard tracks hits, savings, time saved.

### LangSmith
- **What**: Full observability platform for LLM applications.
- **Features**: Trace filtering/export/comparison via UI and API. Production dashboards with alerts. AI-powered trace analysis (Polly). Automated actions via rules/webhooks.
- **Integration**: Works with OpenAI, Anthropic, and many frameworks.

### Key metrics for a coding agent

| Metric | Why It Matters | Target |
|--------|---------------|--------|
| Cost per session | Budget management | Track and alert |
| Cost per successful edit | Efficiency | Minimize |
| Cache hit rate | Cost optimization | > 70% |
| TTFT (time to first token) | UX responsiveness | < 2s |
| Total response time | UX | < 30s for complex |
| Error rate by provider | Reliability | < 1% |
| Model downgrade rate | Quality monitoring | Track |
| Retry rate | Provider health | < 5% |
| Tokens wasted (failed generations) | Waste detection | Minimize |

### Eyrie takeaway
The hawk project already has `metrics/`, `trace/`, and `analytics/`. Extend with:
1. **Per-request cost tracking**: Calculate and log cost of every LLM call using the provider's pricing.
2. **Cache performance dashboard**: Track prompt cache hit rates, response cache hit rates, and estimated savings.
3. **Session cost budget**: Running total with configurable alerts/limits.
4. **Wasted spend detection**: Track tokens spent on generations that were rejected, retried, or discarded.
5. **Provider health timeline**: Log latency, error rate, and availability per provider over time.

---

## 15. API Key Rotation and Security

### OWASP LLM Top 10 relevant risks
- **Prompt injection**: Untrusted inputs leading to unauthorized access.
- **Sensitive information disclosure**: LLM outputs leaking credentials.
- **Supply chain vulnerabilities**: Compromised dependencies.

### Best practices for API key management in a coding agent

1. **Key storage**: Never in code or config files. Use OS keychain (macOS Keychain, Linux secret-tool) or encrypted environment files.
2. **Key rotation**: Support multiple keys per provider. Rotate on schedule or on suspected compromise.
3. **Key scoping**: Use workspace-scoped keys with spend limits. Anthropic supports per-workspace limits.
4. **Key validation**: Test key validity on startup without making a billable request (use lightweight endpoint or metadata call).
5. **Credential isolation**: Provider keys should never be passed to LLM context, tool outputs, or logged in traces.
6. **Multiple key load balancing**: Distribute requests across multiple API keys to multiply effective rate limits. LiteLLM and Portkey both support this.
7. **BYOK (Bring Your Own Key)**: Let users provide their own keys for each provider. Store encrypted, never log.

### Eyrie takeaway
The hawk project has `auth/`. Ensure:
1. **Encrypted key storage**: Keys at rest encrypted with a machine-specific key.
2. **Key rotation support**: Accept multiple keys per provider, round-robin across them.
3. **Spend limit enforcement**: Client-side spend tracking with configurable per-session and per-day limits.
4. **Key leak detection**: Scan LLM outputs for patterns matching API key formats. Redact before displaying.
5. **Minimal privilege**: Each key should have the minimum tier needed. Don't use a Tier 4 key for testing.

---

## 16. Eyrie Implementation Priorities

Ordered by impact for a solo developer's API spend efficiency:

### P0 -- Implement immediately (highest ROI)

1. **Prompt caching optimization** (Section 4)
   - Expected savings: 70-90% on input token costs for multi-turn conversations.
   - Implementation: Strict prompt ordering, automatic breakpoint management, pre-warming.
   - Already relevant: hawk's `engine/compact.go` and `engine/stream.go`.

2. **Two-tier model cascade** (Section 5, 11)
   - Expected savings: 40-85% on model costs by routing simple tasks to cheap models.
   - Implementation: Heuristic classifier + confidence-based escalation.
   - Already relevant: hawk's `model/router.go` and `model/catalog.go`.

3. **Adaptive rate limiting** (Section 7)
   - Expected improvement: Eliminate 429 errors, maximize throughput within limits.
   - Implementation: Parse response headers, predict capacity, throttle proactively.
   - Already relevant: hawk's `ratelimit/ratelimit.go`.

### P1 -- Implement next (high ROI)

4. **Provider fallback chains** (Section 6)
   - Expected improvement: 99.9%+ effective availability across providers.
   - Implementation: Error classification, multi-provider fallback, context-window fallback.
   - Already relevant: hawk's `retry/retry.go` and `circuit/circuit.go`.

5. **Exact-match response caching** (Section 3)
   - Expected savings: 10-30% cost reduction from repeated identical queries.
   - Implementation: SHA-256 cache keys, file-mtime-aware tool result caching.

6. **Cost tracking and budgeting** (Section 14)
   - Expected improvement: Spend visibility and control.
   - Implementation: Per-request cost calculation, session budgets, alerts.
   - Already relevant: hawk's `engine/cost.go`, `metrics/`, `analytics/`.

### P2 -- Implement later (moderate ROI)

7. **Batch API for background work** (Section 9)
   - Expected savings: 50% on non-interactive LLM calls.
   - Implementation: Detect batchable work, hybrid first-result-streaming + batch.

8. **Circuit breaking with health scores** (Section 13)
   - Expected improvement: Faster failover, fewer wasted requests to unhealthy providers.
   - Already relevant: hawk's `circuit/circuit.go`, `model/health_router.go`.

9. **Multi-key load balancing** (Section 15)
   - Expected improvement: 2-Nx effective rate limits with N keys.
   - Implementation: Round-robin across keys, per-key capacity tracking.

10. **Connection pooling optimization** (Section 8)
    - Expected improvement: 50-100ms latency reduction per request from connection reuse.
    - Implementation: Per-provider HTTP/2 connection pools with appropriate keepalive.

### P3 -- Evaluate later (lower ROI or high complexity)

11. **Learned routing model** (Section 2): Train on user's own preference data. High complexity, moderate improvement over heuristics.
12. **Semantic caching** (Section 3): Low hit rates for code-heavy contexts. Not worth the embedding cost.
13. **Hedged requests** (Section 6): Fire to two providers, use first response. Doubles cost for latency improvement.
14. **Provider status page integration** (Section 13): Nice-to-have, low urgency.

---

## Appendix: Key Numbers for Cost Modeling

### Anthropic pricing (as of research date)
| Model | Input (/M tokens) | Output (/M tokens) | Cache Write | Cache Read |
|-------|-------------------|---------------------|-------------|------------|
| Opus 4.7 | $15 | $75 | $18.75 (5m) / $30 (1h) | $1.50 |
| Sonnet 4.6 | $3 | $15 | $3.75 (5m) / $6 (1h) | $0.30 |
| Haiku 4.5 | $0.80 | $4 | $1.00 (5m) / $1.60 (1h) | $0.08 |

### Cost savings scenarios for a coding agent session

**Scenario: 50-turn conversation, 100K system+tools, 50K repo context**

Without optimization:
- 50 turns x 150K input tokens = 7.5M input tokens
- At Opus pricing: 7.5M x $15/M = $112.50

With prompt caching (90% hit rate):
- First turn: 150K tokens cached (write cost: 150K x $18.75/M = $2.81)
- Turns 2-50: 135K cached (read: $1.50/M) + 15K uncached ($15/M)
  = 49 x (135K x $1.50/M + 15K x $15/M) = 49 x ($0.20 + $0.23) = $21.07
- Total: $2.81 + $21.07 = $23.88
- **Savings: $88.62 (79%)**

With prompt caching + model cascade (40% to Haiku):
- Same caching savings PLUS 20 turns handled by Haiku
- Haiku turns: 20 x (135K x $0.08/M + 15K x $0.80/M) = 20 x ($0.011 + $0.012) = $0.46
- Opus turns: 30 x (135K x $1.50/M + 15K x $15/M) = 30 x ($0.20 + $0.23) = $12.90
- Total: $2.81 + $0.46 + $12.90 = $16.17
- **Savings: $96.33 (86%)**

With all optimizations (caching + cascade + batch for background):
- Interactive: $16.17
- Background work (e.g., repo indexing): 50% batch discount
- Estimated total session savings: **80-90%**

---

## Key Repository References

| Project | URL | Stars | Key Technique |
|---------|-----|-------|--------------|
| LiteLLM | github.com/BerriAI/litellm | 18K+ | Unified proxy, routing, caching |
| Portkey Gateway | github.com/Portkey-AI/gateway | 7K+ | Lightweight gateway, circuit breaking |
| RouteLLM | github.com/lm-sys/RouteLLM | 4K+ | Cost-effective model routing |
| GPTCache | github.com/zilliztech/GPTCache | 7K+ | Semantic response caching |
| Not Diamond | github.com/Not-Diamond/notdiamond-python | - | Intelligent model selection |
| RouterBench | github.com/withmartian/routerbench | - | Router evaluation framework |
| SGLang | github.com/sgl-project/sglang | 8K+ | KV cache reuse, efficient serving |

## Key Papers

| Paper | ID | Key Finding |
|-------|----|-|
| RouteLLM | arXiv:2406.18665 | 85% cost reduction, 95% quality preservation via MF router |
| FrugalGPT | arXiv:2305.05176 | 98% cost reduction via model cascading |
| Hybrid LLM | arXiv:2404.14618 | 40% fewer large-model calls, no quality drop (ICLR 2024) |
| RouterBench | arXiv:2403.12031 | First standardized LLM router evaluation (405K outcomes) |
| StreamingLLM | arXiv:2309.17453 | 22.2x speedup via attention sinks (ICLR 2024) |
| SGLang | arXiv:2312.07104 | 6.4x throughput via RadixAttention KV reuse |
