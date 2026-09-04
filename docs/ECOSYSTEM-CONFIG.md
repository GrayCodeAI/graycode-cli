# graycode-eco Unified Config-as-Code

Status: Draft / shared spec
Applies to: graycode, graycode-router, harrier, shrike, swift

This document specifies a **single, unified configuration schema** for the
graycode-eco ecosystem: one declarative file (`graycode-eco.yaml`, with an equivalent
JSON form) that captures model/provider selection, memory, compression,
tracing, and gateway settings for all five repos. It is **config-as-code**: the
file is the source of truth, version-controlled alongside a project, and each
repo reads the slice of the schema it owns.

Today each repo configures itself independently through its own env vars,
flags, and config files (graycode: `config.json` + `GRAYCODE_*`/`GRAYCODE_*` env; graycode-router:
provider env vars; harrier: `~/.harrier/config.toml`; shrike: `TOK_*` env; swift:
`SWIFT_*` env). This spec does **not** replace those mechanisms — it defines a
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
   `model`/`providers` shared by graycode + graycode-router). A repo only reads its sections.
4. **Two encodings, one schema.** YAML is canonical for humans; the identical
   structure is valid JSON for machine generation. (harrier's on-disk format is
   TOML; its section maps 1:1 to `~/.harrier/config.toml`.)
5. **Secrets by reference.** API keys are never inlined. Fields ending in
   `_env` name the environment variable that holds the secret.

## File location & precedence

Search order (first found wins for the file itself; values still follow the
runtime precedence above):

1. `--config <path>` flag (where a repo's CLI supports it)
2. `$GRAYCODE_ECO_CONFIG`
3. `./graycode-eco.yaml` (project root)
4. `~/.config/graycode-eco/config.yaml`

## Top-level schema

```yaml
version: 1

# ─── Shared: model + providers (graycode + graycode-router) ───────────────────────────────
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

# ─── graycode-router: gateway / runtime ───────────────────────────────────────────────
gateway:
  base_url: http://localhost:8080        # graycode-router endpoint graycode talks to
  api_key_env: GRAYCODE_ROUTER_API_KEY
  allow_insecure_public_api: false
  deployment_routing: ""                 # GRAYCODE_ROUTER_DEPLOYMENT_ROUTING
  model_catalog:
    path_env: GRAYCODE_ROUTER_MODEL_CATALOG_PATH
    url_env: GRAYCODE_ROUTER_MODEL_CATALOG_URL
    refresh: GRAYCODE_ROUTER_MODEL_CATALOG_REFRESH

# ─── harrier: memory ───────────────────────────────────────────────────────────
memory:
  addr: 127.0.0.1:3456
  api_key_env: HARRIER_API_KEY
  data_dir: ~/.harrier
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

# ─── shrike: compression ───────────────────────────────────────────────────────
compression:
  enabled: true
  preset: ""                             # TOK_PRESET
  mode: ""                               # TOK_MODE
  max_context: 0                         # TOK_MAX_CONTEXT
  budget: 0                              # TOK_BUDGET
  db_path: ~/.shrike/usage.db               # TOK_DATABASE_PATH / TOK_DB_PATH
  tracking_disabled: false               # TOK_TRACKING_DISABLED / TOK_TELEMETRY_DISABLED

# ─── swift + telemetry (shared OTel) ────────────────────────────────────────
swift:
  search_url: ""                         # SWIFT_SEARCH_URL
  log_level: info                        # SWIFT_LOG_LEVEL
  telemetry_optout: false                # SWIFT_TELEMETRY_OPTOUT / SWIFT_NO_TELEMETRY
  posthog:
    api_key_env: POSTHOG_API_KEY
    endpoint: https://app.posthog.com

telemetry:
  # OTel exporter settings shared by all repos. Span attribute keys follow
  # docs/OTEL-CONVENTIONS.md.
  enabled: false                         # graycode: GRAYCODE_ENABLE_TELEMETRY
  otlp_endpoint: ""                      # OTEL_EXPORTER_OTLP_ENDPOINT
  shutdown_timeout_ms: 0                 # GRAYCODE_OTEL_SHUTDOWN_TIMEOUT_MS
```

## Setting → repo → existing mechanism

The authoritative mapping. "Mechanism today" is what already implements the
setting; the unified key is rendered down to it.

### Shared: model / providers (graycode + graycode-router)

| Unified key                     | Repo        | Mechanism today                                  |
|---------------------------------|-------------|--------------------------------------------------|
| `model.default`                 | graycode        | `GRAYCODE_MODEL` env / `config.json`                 |
| `model.small_fast`              | graycode        | `GRAYCODE_SMALL_FAST_MODEL` env                  |
| `providers[].api_key_env`       | graycode-router/graycode  | `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY`, `OPENROUTER_API_KEY`, `XAI_API_KEY`, `ZAI_API_KEY`, `CANOPYWAVE_API_KEY`, `FIREWORKS_API_KEY` |
| `providers[].base_url_env`      | graycode-router       | `ANTHROPIC_BASE_URL`, `OPENAI_BASE_URL` / `OPENAI_API_BASE`, `OLLAMA_BASE_URL`, `FIREWORKS_BASE_URL` |
| `providers[].model` (openai)    | graycode-router       | `OPENAI_MODEL` env                               |
| `providers[].model` (gemini)    | graycode-router       | `GEMINI_MODEL` env                               |
| `providers[].model` (anthropic) | graycode-router       | `ANTHROPIC_MODEL` env                            |

### graycode-router: gateway / runtime

| Unified key                          | Mechanism today (graycode-router)              |
|--------------------------------------|--------------------------------------|
| `gateway.base_url`                   | `GRAYCODE_ROUTER_BASE_URL` (graycode→graycode-router link)   |
| `gateway.api_key_env`                | `GRAYCODE_ROUTER_API_KEY`                      |
| `gateway.allow_insecure_public_api`  | `GRAYCODE_ROUTER_ALLOW_INSECURE_PUBLIC_API`    |
| `gateway.deployment_routing`         | `GRAYCODE_ROUTER_DEPLOYMENT_ROUTING` (also `GRAYCODE_DEPLOYMENT_ROUTING`) |
| `gateway.model_catalog.path_env`     | `GRAYCODE_ROUTER_MODEL_CATALOG_PATH`           |
| `gateway.model_catalog.url_env`      | `GRAYCODE_ROUTER_MODEL_CATALOG_URL`            |
| `gateway.model_catalog.refresh`      | `GRAYCODE_ROUTER_MODEL_CATALOG_REFRESH` / `GRAYCODE_AUTO_REFRESH_CATALOG` / `GRAYCODE_CATALOG_REFRESH_ALWAYS` |
| `gateway` config dir                 | `GRAYCODE_CONFIG_DIR` (default `~/.graycode-router`) |

### harrier: memory

harrier's on-disk format is `~/.harrier/config.toml` (struct in
`harrier/config/config.go`). The `memory.*` section maps 1:1 to that file plus a
few env vars.

| Unified key                       | Mechanism today (harrier)                          |
|-----------------------------------|-------------------------------------------------|
| `memory.addr`                     | `HARRIER_ADDR` env                                 |
| `memory.api_key_env`              | `HARRIER_API_KEY` env                              |
| `memory.data_dir`                 | `HARRIER_DATA_DIR` env                             |
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
| TLS cert/key                      | `HARRIER_TLS_CERT`, `HARRIER_TLS_KEY` / `[server]` TLS |
| agent identity                    | `HARRIER_AGENT_ID`, `HARRIER_ADD_ONLY` env            |

### shrike: compression

| Unified key                       | Mechanism today (shrike)                           |
|-----------------------------------|-------------------------------------------------|
| `compression.preset`              | `TOK_PRESET` env                                |
| `compression.mode`                | `TOK_MODE` env                                  |
| `compression.max_context`         | `TOK_MAX_CONTEXT` env                           |
| `compression.budget`              | `TOK_BUDGET` env (also `TOK_PLAN_BUDGET`, `TOK_ROLE_BUDGET`) |
| `compression.db_path`             | `TOK_DATABASE_PATH` / `TOK_DB_PATH` env         |
| `compression.tracking_disabled`   | `TOK_TRACKING_DISABLED` / `TOK_TELEMETRY_DISABLED` env |
| (advanced tuning knobs)           | `TOK_*` family: `TOK_COMPACTION`, `TOK_ENTROPY_THRESHOLD`, `TOK_CACHE_SIZE`, `TOK_ATTENTION_SINK`, `TOK_STRUCTURAL_COLLAPSE`, etc. (left out of the top-level schema; pass through via `compression.advanced` map if needed) |

### swift + telemetry

| Unified key                       | Mechanism today (swift / graycode)                  |
|-----------------------------------|-------------------------------------------------|
| `swift.search_url`                | `SWIFT_SEARCH_URL` env                          |
| `swift.log_level`                 | `SWIFT_LOG_LEVEL` env                           |
| `swift.telemetry_optout`          | `SWIFT_TELEMETRY_OPTOUT` / `SWIFT_NO_TELEMETRY` env |
| `swift.posthog.api_key_env`       | `POSTHOG_API_KEY` env                           |
| `swift.posthog.endpoint`          | `POSTHOG_ENDPOINT` env                          |
| `telemetry.enabled`               | `GRAYCODE_ENABLE_TELEMETRY` env (graycode)         |
| `telemetry.shutdown_timeout_ms`   | `GRAYCODE_OTEL_SHUTDOWN_TIMEOUT_MS` env (graycode) |
| `telemetry.otlp_endpoint`         | `OTEL_EXPORTER_OTLP_ENDPOINT` (standard OTel)   |

## Rendering down to per-repo config

The unified file is designed to be **resolved** into the existing mechanisms:

- **env-based repos** (graycode, graycode-router, shrike, swift): export the mapped env var for
  any key set in `graycode-eco.yaml` that is not already present in the process
  environment (preserving "env wins" precedence).
- **file-based repos** (harrier): write/merge the `memory.*` section into
  `~/.harrier/config.toml` using the field names above.

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
  "gateway": { "base_url": "http://localhost:8080", "api_key_env": "GRAYCODE_ROUTER_API_KEY" },
  "memory": { "addr": "127.0.0.1:3456", "data_dir": "~/.harrier" },
  "compression": { "enabled": true, "db_path": "~/.shrike/usage.db" },
  "swift": { "telemetry_optout": false },
  "telemetry": { "enabled": false }
}
```

## Relationship to OTel conventions

The `telemetry` and `swift` sections only configure *transport/opt-out*. The
*attribute vocabulary* emitted on spans is defined separately in
[`OTEL-CONVENTIONS.md`](./OTEL-CONVENTIONS.md), with graycode-router's
`internal/observability` package as the reference implementation.
