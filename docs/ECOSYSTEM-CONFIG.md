# graycode-eco Unified Config-as-Code

Status: Draft / shared spec
Applies to: hawk, eyrie, yaad, tok, trace

This document specifies a **single, unified configuration schema** for the
graycode-eco ecosystem: one declarative file (`graycode-eco.yaml`, with an equivalent
JSON form) that captures model/provider selection, memory, compression,
tracing, and gateway settings for all five repos. It is **config-as-code**: the
file is the source of truth, version-controlled alongside a project, and each
repo reads the slice of the schema it owns.

Today each repo configures itself independently through its own env vars,
flags, and config files (hawk: `config.json` + `HAWK_*`/`GRAYCODE_*` env; eyrie:
provider env vars; yaad: `~/.yaad/config.toml`; tok: `TOK_*` env; trace:
`TRACE_*` env). This spec does **not** replace those mechanisms — it defines a
superset schema and maps every setting back to the repo + existing env
var/flag that implements it today, so adoption can be incremental and the
unified file can be **rendered down** to per-repo env/config without changing
any runtime behavior.

## Design principles

1. **Additive, not breaking.** Every key maps to an env var/flag that already
   exists. A repo that ignores `graycode-eco.yaml` keeps working exactly as before.
2. **Env still wins at runtime.** Precedence: explicit flag > process env var >
   `graycode-eco.yaml` value > repo default. This preserves current behavior where
   env/flags are authoritative.
3. **Repo-owned sections.** Each top-level section is owned by one repo (with
   `model`/`providers` shared by hawk + eyrie). A repo only reads its sections.
4. **Two encodings, one schema.** YAML is canonical for humans; the identical
   structure is valid JSON for machine generation. (yaad's on-disk format is
   TOML; its section maps 1:1 to `~/.yaad/config.toml`.)
5. **Secrets by reference.** API keys are never inlined. Fields ending in
   `_env` name the environment variable that holds the secret.

## File location & precedence

Search order (first found wins for the file itself; values still follow the
runtime precedence above):

1. `--config <path>` flag (where a repo's CLI supports it)
2. `$HAWK_ECO_CONFIG`
3. `./graycode-eco.yaml` (project root)
4. `~/.config/graycode-eco/config.yaml`

## Top-level schema

```yaml
version: 1

# ─── Shared: model + providers (hawk + eyrie) ───────────────────────────────
model:
  default: anthropic/claude-sonnet-4-5   # provider/model the agent uses
  small_fast: anthropic/claude-haiku     # cheap model for trivial steps

providers:
  - name: anthropic
    api_key_env: ANTHROPIC_API_KEY
    base_url_env: ANTHROPIC_BASE_URL     # optional override
  - name: openai
    api_key_env: OPENAI_API_KEY
    base_url_env: OPENAI_BASE_URL
    model: gpt-4o
  - name: gemini
    api_key_env: GEMINI_API_KEY
    model: gemini-2.0-flash

# ─── eyrie: gateway / runtime ───────────────────────────────────────────────
gateway:
  base_url: http://localhost:8080        # eyrie endpoint hawk talks to
  api_key_env: EYRIE_API_KEY
  allow_insecure_public_api: false
  deployment_routing: ""                 # EYRIE_DEPLOYMENT_ROUTING
  model_catalog:
    path_env: EYRIE_MODEL_CATALOG_PATH
    url_env: EYRIE_MODEL_CATALOG_URL
    refresh: EYRIE_MODEL_CATALOG_REFRESH

# ─── yaad: memory ───────────────────────────────────────────────────────────
memory:
  addr: 127.0.0.1:3456
  api_key_env: YAAD_API_KEY
  data_dir: ~/.yaad
  hot_token_budget: 800
  warm_token_budget: 800
  max_memories: 10000
  embeddings:
    enabled: true
    provider: local
    model: all-MiniLM-L6-v2
  search:
    bm25_weight: 0.5
    vector_weight: 0.5
    default_limit: 10
  decay:
    enabled: true
    half_life_days: 30

# ─── tok: compression ───────────────────────────────────────────────────────
compression:
  enabled: true
  preset: ""                             # TOK_PRESET
  mode: ""                               # TOK_MODE
  max_context: 0                         # TOK_MAX_CONTEXT
  budget: 0                              # TOK_BUDGET
  db_path: ~/.tok/usage.db               # TOK_DATABASE_PATH / TOK_DB_PATH
  tracking_disabled: false               # TOK_TRACKING_DISABLED / TOK_TELEMETRY_DISABLED

# ─── trace + telemetry (shared OTel) ────────────────────────────────────────
trace:
  search_url: ""                         # TRACE_SEARCH_URL
  log_level: info                        # TRACE_LOG_LEVEL
  telemetry_optout: false                # TRACE_TELEMETRY_OPTOUT / TRACE_NO_TELEMETRY
  posthog:
    api_key_env: POSTHOG_API_KEY
    endpoint: https://app.posthog.com

telemetry:
  # OTel exporter settings shared by all repos. Span attribute keys follow
  # docs/OTEL-CONVENTIONS.md.
  enabled: false                         # hawk: HAWK_CODE_ENABLE_TELEMETRY
  otlp_endpoint: ""                      # OTEL_EXPORTER_OTLP_ENDPOINT
  shutdown_timeout_ms: 0                 # HAWK_CODE_OTEL_SHUTDOWN_TIMEOUT_MS
```

## Setting → repo → existing mechanism

The authoritative mapping. "Mechanism today" is what already implements the
setting; the unified key is rendered down to it.

### Shared: model / providers (hawk + eyrie)

| Unified key                     | Repo        | Mechanism today                                  |
|---------------------------------|-------------|--------------------------------------------------|
| `model.default`                 | hawk        | `HAWK_MODEL` env / `config.json`                 |
| `model.small_fast`              | hawk        | `GRAYCODE_SMALL_FAST_MODEL` env                  |
| `providers[].api_key_env`       | eyrie/hawk  | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `OPENROUTER_API_KEY`, `XAI_API_KEY`, `ZAI_API_KEY`, `CANOPYWAVE_API_KEY` |
| `providers[].base_url_env`      | eyrie       | `ANTHROPIC_BASE_URL`, `OPENAI_BASE_URL` / `OPENAI_API_BASE`, `OLLAMA_BASE_URL` |
| `providers[].model` (openai)    | eyrie       | `OPENAI_MODEL` env                               |
| `providers[].model` (gemini)    | eyrie       | `GEMINI_MODEL` env                               |
| `providers[].model` (anthropic) | eyrie       | `ANTHROPIC_MODEL` env                            |

### eyrie: gateway / runtime

| Unified key                          | Mechanism today (eyrie)              |
|--------------------------------------|--------------------------------------|
| `gateway.base_url`                   | `EYRIE_BASE_URL` (hawk→eyrie link)   |
| `gateway.api_key_env`                | `EYRIE_API_KEY`                      |
| `gateway.allow_insecure_public_api`  | `EYRIE_ALLOW_INSECURE_PUBLIC_API`    |
| `gateway.deployment_routing`         | `EYRIE_DEPLOYMENT_ROUTING` (also `HAWK_DEPLOYMENT_ROUTING`) |
| `gateway.model_catalog.path_env`     | `EYRIE_MODEL_CATALOG_PATH`           |
| `gateway.model_catalog.url_env`      | `EYRIE_MODEL_CATALOG_URL`            |
| `gateway.model_catalog.refresh`      | `EYRIE_MODEL_CATALOG_REFRESH` / `HAWK_AUTO_REFRESH_CATALOG` / `HAWK_CATALOG_REFRESH_ALWAYS` |
| `gateway` config dir                 | `HAWK_CONFIG_DIR` (default `~/.eyrie`) |

### yaad: memory

yaad's on-disk format is `~/.yaad/config.toml` (struct in
`yaad/config/config.go`). The `memory.*` section maps 1:1 to that file plus a
few env vars.

| Unified key                       | Mechanism today (yaad)                          |
|-----------------------------------|-------------------------------------------------|
| `memory.addr`                     | `YAAD_ADDR` env                                 |
| `memory.api_key_env`              | `YAAD_API_KEY` env                              |
| `memory.data_dir`                 | `YAAD_DATA_DIR` env                             |
| `memory.hot_token_budget`         | `config.toml` `[memory].hot_token_budget`       |
| `memory.warm_token_budget`        | `config.toml` `[memory].warm_token_budget`      |
| `memory.max_memories`             | `config.toml` `[memory].max_memories`           |
| `memory.embeddings.enabled`       | `config.toml` `[embeddings].enabled`            |
| `memory.embeddings.provider`      | `config.toml` `[embeddings].provider`           |
| `memory.embeddings.model`         | `config.toml` `[embeddings].model`              |
| `memory.search.bm25_weight`       | `config.toml` `[search].bm25_weight`            |
| `memory.search.vector_weight`     | `config.toml` `[search].vector_weight`          |
| `memory.search.default_limit`     | `config.toml` `[search].default_limit`          |
| `memory.decay.enabled`            | `config.toml` `[decay].enabled`                 |
| `memory.decay.half_life_days`     | `config.toml` `[decay].half_life_days`          |
| TLS cert/key                      | `YAAD_TLS_CERT`, `YAAD_TLS_KEY` / `[server]` TLS |
| agent identity                    | `YAAD_AGENT_ID`, `YAAD_ADD_ONLY` env            |

### tok: compression

| Unified key                       | Mechanism today (tok)                           |
|-----------------------------------|-------------------------------------------------|
| `compression.preset`              | `TOK_PRESET` env                                |
| `compression.mode`                | `TOK_MODE` env                                  |
| `compression.max_context`         | `TOK_MAX_CONTEXT` env                           |
| `compression.budget`              | `TOK_BUDGET` env (also `TOK_PLAN_BUDGET`, `TOK_ROLE_BUDGET`) |
| `compression.db_path`             | `TOK_DATABASE_PATH` / `TOK_DB_PATH` env         |
| `compression.tracking_disabled`   | `TOK_TRACKING_DISABLED` / `TOK_TELEMETRY_DISABLED` env |
| (advanced tuning knobs)           | `TOK_*` family: `TOK_COMPACTION`, `TOK_ENTROPY_THRESHOLD`, `TOK_CACHE_SIZE`, `TOK_ATTENTION_SINK`, `TOK_STRUCTURAL_COLLAPSE`, etc. (left out of the top-level schema; pass through via `compression.advanced` map if needed) |

### trace + telemetry

| Unified key                       | Mechanism today (trace / hawk)                  |
|-----------------------------------|-------------------------------------------------|
| `trace.search_url`                | `TRACE_SEARCH_URL` env                          |
| `trace.log_level`                 | `TRACE_LOG_LEVEL` env                           |
| `trace.telemetry_optout`          | `TRACE_TELEMETRY_OPTOUT` / `TRACE_NO_TELEMETRY` env |
| `trace.posthog.api_key_env`       | `POSTHOG_API_KEY` env                           |
| `trace.posthog.endpoint`          | `POSTHOG_ENDPOINT` env                          |
| `telemetry.enabled`               | `HAWK_CODE_ENABLE_TELEMETRY` env (hawk)         |
| `telemetry.shutdown_timeout_ms`   | `HAWK_CODE_OTEL_SHUTDOWN_TIMEOUT_MS` env (hawk) |
| `telemetry.otlp_endpoint`         | `OTEL_EXPORTER_OTLP_ENDPOINT` (standard OTel)   |

## Rendering down to per-repo config

The unified file is designed to be **resolved** into the existing mechanisms:

- **env-based repos** (hawk, eyrie, tok, trace): export the mapped env var for
  any key set in `graycode-eco.yaml` that is not already present in the process
  environment (preserving "env wins" precedence).
- **file-based repos** (yaad): write/merge the `memory.*` section into
  `~/.yaad/config.toml` using the field names above.

A reference resolver can live in any repo as an additive, stdlib-only helper
(`encoding/json` for the JSON form; a small hand-rolled reader or
`gopkg.in/yaml.v3` only if a repo already depends on it). No repo is required
to implement it to remain spec-compliant — the mapping table is the contract.

## JSON equivalent

The schema is encoding-agnostic. The YAML above is identical in structure to:

```json
{
  "version": 1,
  "model": { "default": "anthropic/claude-sonnet-4-5", "small_fast": "anthropic/claude-haiku" },
  "providers": [
    { "name": "anthropic", "api_key_env": "ANTHROPIC_API_KEY" }
  ],
  "gateway": { "base_url": "http://localhost:8080", "api_key_env": "EYRIE_API_KEY" },
  "memory": { "addr": "127.0.0.1:3456", "data_dir": "~/.yaad" },
  "compression": { "enabled": true, "db_path": "~/.tok/usage.db" },
  "trace": { "telemetry_optout": false },
  "telemetry": { "enabled": false }
}
```

## Relationship to OTel conventions

The `telemetry` and `trace` sections only configure *transport/opt-out*. The
*attribute vocabulary* emitted on spans is defined separately in
[`OTEL-CONVENTIONS.md`](./OTEL-CONVENTIONS.md), with eyrie's
`internal/observability` package as the reference implementation.
