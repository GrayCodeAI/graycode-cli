# Ecosystem message flow (eyrie · harrier · shrike)

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
│  harrier recall       │◄────│ ~/.harrier/    │  conventions, decisions, skills
│  (if bridge ready) │     │ harrier.db     │
└─────────┬─────────┘     └─────────────┘
          │
          ▼
┌───────────────────┐     ┌─────────────┐
│  shrike token budget  │     │ embedded    │  CountTokens, CompressForContext
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
          ├──► harrier Remember (CoreMemory tools, auto-remember)
          │
          ▼
    Response to user
          │
          ▼ (when context grows)
┌───────────────────┐
│  shrike Compress      │  fast path before LLM summarization
│  + eyrie compact   │
└───────────────────┘
```

## Step by step

### 1. Session start (`hawk` or `hawk exec`)

- **eyrie**: The Hawk composition root creates an `eyrie/engine.Engine` with
  Eyrie-owned state paths, an injected secret store, and per-engine custom
  gateway metadata. The engine loads provider state and the model catalog, then
  builds transport behind Hawk's `ChatClient` port.
- **harrier**: `configureSession` creates `HarrierBridge` → opens `~/.harrier/data/harrier.db`. If missing, hawk runs without persistent memory.
- **shrike**: No startup step — linked at compile time.

### 2. System prompt assembly

- Hawk templates (`internal/prompts/templates/*.md`) define behavior, tools, and practices.
- Project `AGENTS.md` is appended via `hawkconfig.BuildContextWithDirs`.
- **harrier**: `Memory.Recall` injects relevant graph nodes into the system prompt.

### 3. User message → agent loop (`internal/engine/stream.go`)

Each turn:

1. **harrier** — recall memories matching the latest user message (token budget ~2000).
2. **eyrie** — Hawk's adapter calls engine generate/stream with Hawk-owned tool
   definitions; Eyrie normalizes provider events and tool requests.
3. Tools run with `HarrierBridge` in context for `CoreMemory*` tools.
4. **harrier** — sleeptime consolidation, skill distillation, auto-remember after turns.

### 4. Context pressure

When messages exceed limits (`internal/engine/compact.go`):

1. **shrike** — `shrike.Compress()` tries a fast compression path for summaries.
2. **eyrie** — if shrike reduction is insufficient, hawk calls the LLM to summarize, then keeps recent messages.

### 5. Token accounting

- `internal/engine/token/tokenizer.go` wraps **shrike** for precise and fast estimates used in budget UI and compaction decisions.

## Verify locally

```bash
hawk doctor              # ecosystem panel + eyrie preflight + harrier status
hawk harrier                # merlin memory graph
./scripts/smoke-hawk.sh  # build + quick tests
```

## Module layout

| Module | Role in hawk | Required? |
|--------|----------------|-----------|
| **eyrie** | LLM APIs, catalog, credentials, routing | Yes |
| **harrier** | SQLite memory graph at `~/.harrier/data/` | No (degrades gracefully) |
| **shrike** | Token estimate + context compression | Yes (embedded, no config) |

The support repositories are independent sibling checkouts: `eyrie`,
`harrier` (Harrier), and `shrike` (Shrike), with the parent `go.work` wiring the
local Go workspace.

Production Hawk code imports Eyrie only through `eyrie/engine`. Conversation
history, WAL/resume, permissions, and tool execution remain in Hawk; provider
credentials, discovery, selection, transport, resilience, and normalized
streaming remain in Eyrie.
