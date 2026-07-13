# Ecosystem message flow (eyrie · yaad · tok)

How one user message travels through hawk and the GrayCodeAI ecosystem libraries.

## Overview

```
User prompt (TUI or hawk exec)
        │
        ▼
┌───────────────────┐
│  buildSystemPrompt │  AGENTS.md + prompt templates (practices.md)
└─────────┬─────────┘
          │
          ▼
┌───────────────────┐     ┌─────────────┐
│  yaad recall       │◄────│ ~/.yaad/    │  conventions, decisions, skills
│  (if bridge ready) │     │ yaad.db     │
└─────────┬─────────┘     └─────────────┘
          │
          ▼
┌───────────────────┐     ┌─────────────┐
│  tok token budget  │     │ embedded    │  CountTokens, CompressForContext
│  (context sizing)  │     │ library     │
└─────────┬─────────┘     └─────────────┘
          │
          ▼
┌───────────────────┐     ┌─────────────┐
│  Hawk ChatClient   │────►│ eyrie/engine│  catalog, credentials, routing
│  port + adapter    │     │ generate/   │────► provider API
└─────────┬─────────┘     │ stream      │
                          └─────────────┘
          │
          ▼
    Tool calls (Read, Edit, Bash, CoreMemory*, …)
          │
          ├──► yaad Remember (CoreMemory tools, auto-remember)
          │
          ▼
    Response to user
          │
          ▼ (when context grows)
┌───────────────────┐
│  tok Compress      │  fast path before LLM summarization
│  + eyrie compact   │
└───────────────────┘
```

## Step by step

### 1. Session start (`hawk` or `hawk exec`)

- **eyrie**: The Hawk composition root creates an `eyrie/engine.Engine` with
  Eyrie-owned state paths, an injected secret store, and per-engine custom
  gateway metadata. The engine loads provider state and the model catalog, then
  builds transport behind Hawk's `ChatClient` port.
- **yaad**: `configureSession` creates `YaadBridge` → opens `~/.yaad/data/yaad.db`. If missing, hawk runs without persistent memory.
- **tok**: No startup step — linked at compile time.

### 2. System prompt assembly

- Hawk templates (`internal/prompts/templates/*.md`) define behavior, tools, and practices.
- Project `AGENTS.md` is appended via `hawkconfig.BuildContextWithDirs`.
- **yaad**: `Memory.Recall` injects relevant graph nodes into the system prompt.

### 3. User message → agent loop (`internal/engine/stream.go`)

Each turn:

1. **yaad** — recall memories matching the latest user message (token budget ~2000).
2. **eyrie** — Hawk's adapter calls engine generate/stream with Hawk-owned tool
   definitions; Eyrie normalizes provider events and tool requests.
3. Tools run with `YaadBridge` in context for `CoreMemory*` tools.
4. **yaad** — sleeptime consolidation, skill distillation, auto-remember after turns.

### 4. Context pressure

When messages exceed limits (`internal/engine/compact.go`):

1. **tok** — `tok.Compress()` tries a fast compression path for summaries.
2. **eyrie** — if tok reduction is insufficient, hawk calls the LLM to summarize, then keeps recent messages.

### 5. Token accounting

- `internal/engine/token/tokenizer.go` wraps **tok** for precise and fast estimates used in budget UI and compaction decisions.

## Verify locally

```bash
hawk doctor              # ecosystem panel + eyrie preflight + yaad status
hawk yaad                # inspect memory graph
./scripts/smoke-hawk.sh  # build + quick tests
```

## Module layout

| Module | Role in hawk | Required? |
|--------|----------------|-----------|
| **eyrie** | LLM APIs, catalog, credentials, routing | Yes |
| **yaad** | SQLite memory graph at `~/.yaad/data/` | No (degrades gracefully) |
| **tok** | Token estimate + context compression | Yes (embedded, no config) |

Pinned support-repo submodules live under `external/{eyrie,yaad,tok}` and are
wired via root `go.work`.

Production Hawk code imports Eyrie only through `eyrie/engine`. Conversation
history, WAL/resume, permissions, and tool execution remain in Hawk; provider
credentials, discovery, selection, transport, resilience, and normalized
streaming remain in Eyrie.
