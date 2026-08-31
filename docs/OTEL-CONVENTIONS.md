# graycode-eco OpenTelemetry Semantic Conventions for AI Agent Spans

Status: Draft / shared spec
Applies to: hawk, eyrie, harrier, shrike, swift

This document defines the **ecosystem-wide** OpenTelemetry (OTel) semantic
conventions that every graycode-eco repo should follow when emitting spans for AI
agent and LLM operations. The goal is that a single swift backend (Jaeger,
Tempo, Honeycomb, an OTLP collector, etc.) can correlate model calls, tool
invocations, token usage, and cost **across all five repos** using one common
attribute vocabulary.

These conventions track the upstream OpenTelemetry **GenAI** semantic
conventions (`gen_ai.*`) where they exist, and add a small set of ecosystem
extensions (`cost.usd`, `session.id`, `agent.id`) for the cross-cutting
concerns that the GenAI spec does not yet standardize.

- Upstream spec: <https://opentelemetry.io/docs/specs/semconv/gen-ai/>

## Reference ownership

Eyrie owns provider-call instrumentation behind its `eyrie/engine` facade.
Its lower provider layer contains the reference OTel decorator for chat and
stream calls: it starts a client span, records provider/model/usage attributes,
sets status from the result, and ends a streamed span on completion. That
decorator is an Eyrie implementation detail; Hawk must not import or compose it
directly.

- `eyrie/internal/observability/observability.go` provides a stdlib-only,
  zero-dependency telemetry/metrics layer (spans, latency histograms,
  Prometheus + JSON export) for environments that cannot pull in the OTel SDK.
- `eyrie/internal/observability/genai_semconv.go` exports the canonical
  attribute-key constants defined below, so Go code can reference them instead
  of hard-coding strings. A pinning test
  (`genai_semconv_test.go`) guards the exact key values.

When adding tracing to Hawk, propagate swift context through the Engine call and
use the attribute keys in this document. Eyrie wraps provider operations;
Hawk wraps product turns and tools. Harrier, Shrike, and Swift instrument only their
own operations.

## Span kinds and names

| Operation                         | Span name (recommended) | OTel `gen_ai.operation.name` |
|-----------------------------------|-------------------------|------------------------------|
| Chat / completion request         | `chat <model>`          | `chat`                       |
| Streaming chat                    | `chat <model>`          | `chat`                       |
| Embeddings request                | `embeddings <model>`    | `embeddings`                 |
| Tool / function invocation        | `tool <tool.name>`      | `tool_use`                   |
| Agent step / turn                 | `agent <agent.id>`      | `agent`                      |

LLM/provider request spans SHOULD use span kind `CLIENT`. Tool invocations and
agent steps that represent internal work SHOULD use span kind `INTERNAL`.

eyrie's existing stdlib layer also defines short span names
(`llm.chat`, `llm.stream`, `llm.retry`, `llm.cache_hit`) — these remain valid
for the internal metrics collector; the names above are the cross-repo
convention for OTLP-exported spans.

## Required / recommended attributes

The required attribute set for any AI agent span. Keys are exported as Go
constants in `eyrie/internal/observability/genai_semconv.go` (constant name in
parentheses).

| Attribute key                   | Go const (`eyrie` observability)   | Type   | Required | Meaning                                                        |
|---------------------------------|------------------------------------|--------|----------|----------------------------------------------------------------|
| `gen_ai.system`                 | `AttrGenAISystem`                  | string | yes      | Provider/system: `openai`, `anthropic`, `gemini`, etc.         |
| `gen_ai.request.model`          | `AttrGenAIRequestModel`            | string | yes      | Model requested by the caller, e.g. `gpt-4o`.                  |
| `gen_ai.response.model`         | `AttrGenAIResponseModel`           | string | when known | Model that actually served the response.                    |
| `gen_ai.usage.input_tokens`     | `AttrGenAIUsageInputTokens`        | int    | when known | Prompt/input tokens consumed.                                |
| `gen_ai.usage.output_tokens`    | `AttrGenAIUsageOutputTokens`       | int    | when known | Completion/output tokens generated.                          |
| `gen_ai.operation.name`         | `AttrGenAIOperationName`           | string | recommended | `chat` / `embeddings` / `tool_use` / `agent`.              |
| `cost.usd`                      | `AttrCostUSD`                      | double | when known | Computed monetary cost in USD (ecosystem extension).         |
| `tool.name`                     | `AttrToolName`                     | string | for tool spans | Name of the tool/function invoked.                       |
| `session.id`                    | `AttrSessionID`                    | string | recommended | Correlates spans in one logical session/conversation.      |
| `agent.id`                      | `AttrAgentID`                      | string | recommended | Identifies the agent instance producing the span.          |

### Notes

- **Tokens as integers.** Emit `gen_ai.usage.input_tokens` /
  `gen_ai.usage.output_tokens` as OTel integer attributes. eyrie's stdlib
  `Span.Attributes` is `map[string]string`; when bridging that layer to OTLP,
  convert to integer attributes.
- **Cost is a double in USD.** `cost.usd` is the ecosystem standard.
  eyrie's metrics collector stores cost internally in micro-USD for precision
  (`costMicroUSD`) but exports USD; emit the USD value on spans.
- **No prompt/response bodies by default.** Following eyrie's audit design
  (`observability/audit.go`), spans MUST NOT carry raw prompt/response text by
  default. If content capture is enabled, use the OTel GenAI event/log channel,
  not span attributes, and gate it behind explicit opt-in.
- **Status.** Set span status to `Error` and record the error on failure,
  mirroring `TracingProvider.Chat`. Otherwise set `Ok`.

## Legacy attribute mapping

eyrie's older stdlib constants (`AttrLLMProvider`, `AttrLLMModel`,
`AttrLLMInputTokens`, …, keyed `llm.*`) predate this spec and remain for
backwards compatibility. New instrumentation should use the `gen_ai.*` keys.
Mapping:

| Legacy key (`llm.*`)   | Canonical key (`gen_ai.*` / ecosystem) |
|------------------------|----------------------------------------|
| `llm.provider`         | `gen_ai.system`                        |
| `llm.model`            | `gen_ai.request.model`                 |
| `llm.input_tokens`     | `gen_ai.usage.input_tokens`            |
| `llm.output_tokens`    | `gen_ai.usage.output_tokens`           |
| `llm.cost_usd`         | `cost.usd`                             |
| `llm.latency_ms`       | (use span duration)                    |
| `llm.status`           | (use OTel span status)                 |

## Per-repo guidance

- **eyrie** — owns provider/model/usage spans behind `eyrie/engine`; align
  attribute keys to `gen_ai.*` over time.
- **hawk** — daemon/orchestrator. Already has OTel hooks
  (`HAWK_CODE_ENABLE_TELEMETRY`, `HAWK_CODE_OTEL_SHUTDOWN_TIMEOUT_MS`). Emit
  `agent.id` and `session.id` on agent-turn spans; propagate them downstream to
  eyrie via context so provider spans inherit the same IDs.
- **harrier** — memory service. Add `embeddings` spans with `gen_ai.system` +
  `gen_ai.request.model` from its embeddings config; tag retrieval spans with
  `session.id`.
- **shrike** — compression library/CLI. Token accounting is its domain; when it
  emits spans, populate `gen_ai.usage.input_tokens` / `output_tokens` and
  `cost.usd` so compression savings are visible in the same swift.
- **swift** — CLI / replay. When ingesting third-party agent sessions, map
  vendor fields onto these keys so replayed traces share the ecosystem schema.

## Versioning

This spec tracks OTel GenAI conventions, which are still evolving. Pin changes
to the constants via `genai_semconv_test.go` in eyrie and update this table in
the same change.
