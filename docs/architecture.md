# Hawk Architecture

## Overview

Hawk is a terminal-native AI coding agent built in Go. It reads, writes, and runs code in your terminal through natural language interaction.

```
┌─────────────────────────────────────────────────┐
│                    hawk CLI                       │
│        cmd/ → cobra + bubbletea TUI             │
├─────────────────────────────────────────────────┤
│                 engine/                           │
│  Agent loop, compaction, tools, permissions      │
├──────────┬──────────┬──────────┬────────────────┤
│  eyrie   │   tok    │   yaad   │  sight/inspect │
│  (LLM)   │ (tokens) │ (memory) │  (review/sec)  │
└──────────┴──────────┴──────────┴────────────────┘
```

## Package Map

### Entry Point
- **cmd/** — CLI commands (cobra), TUI (bubbletea), session management

### Core Engine
- **engine/** — Agent loop, streaming, compaction, tools orchestration
- **tool/** — 40+ built-in tools (Bash, Read, Write, Edit, Grep, Glob, etc.)
- **permissions/** — User approval system, auto-learning, injection scanning

### LLM Layer
- **eyrie** (external) — Multi-provider LLM client (Anthropic, OpenAI, Gemini, local)
- **routing/** — Model selection, cascade routing, health-aware fallback

### Intelligence
- **repomap/** — Code intelligence (PageRank, BM25, file relevance ranking)
- **memory/** — Cross-session memory via yaad bridge
- **planner/** — Multi-step task decomposition

### Persistence
- **session/** — JSONL + WAL crash recovery, SQLite index, snapshots
- **config/** — Layered config (global + project), validation

### Infrastructure
- **daemon/** — HTTP API server for programmatic/CI access
- **sandbox/** — Command isolation (landlock, seccomp, seatbelt)
- **mcp/** — Model Context Protocol client
- **parallel/** — Git worktree parallel execution
- **circuit/** — Circuit breaker pattern
- **ratelimit/** — Token bucket rate limiting
- **retry/** — Exponential backoff
- **shutdown/** — Graceful shutdown with hook registration

## Data Flow

1. User types prompt → cmd/ captures via bubbletea
2. engine/ builds message array with context (repomap, memory, system prompt)
3. eyrie sends to LLM provider (streaming)
4. LLM responds with text + tool calls
5. engine/ executes tools via tool.Registry (with permission checks)
6. Tool results fed back to LLM for next turn
7. Final response displayed, session persisted to WAL

## Key Design Decisions

- **Single binary**: Go compilation, zero runtime dependencies
- **Streaming-first**: All LLM responses stream token-by-token
- **Crash recovery**: WAL ensures no data loss on unexpected exit
- **Permission sandboxing**: All tool calls gated by configurable permission engine
- **Model-agnostic**: eyrie abstracts all provider differences
- **Offline-capable**: Works with local models (Ollama) when no API key configured
