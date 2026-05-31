# hawk

CLI/REPL and orchestration hub for the hawk-eco AI coding agent platform.

## Build & Test
- go build ./cmd/hawk — build CLI binary
- go test ./... -count=1 — run all tests
- go test ./shared/types/... -count=1 — shared types only
- go test ./internal/engine/... -count=1 — engine tests

## Architecture
- cmd/ — CLI commands (chat, review, inspect, models, credentials)
- internal/engine/ — REPL session, magic commands, conversation adapter
- internal/bridge/ — bridges to eyrie, sight, inspect, yaad, trace
- internal/intelligence/ — repomap, memory subsystem
- shared/types/ — unified types (Severity, Finding)

## Key Patterns
- Hub-and-spoke: orchestrates eyrie, sight, inspect, yaad, tok, trace
- Bridge pattern: each external tool has a bridge in internal/bridge/
- REPL magic commands: %reset, %undo, %tokens, %history, %copy, %save, %compact, %model, %clear
- Unified Finding type for cross-tool interoperability

## Ecosystem
- eyrie — LLM provider layer
- sight — code review
- inspect — web auditing
- yaad — memory/recall
- tok — token counting
- trace — session capture (via subprocess bridge)

## Recent Additions
- REPL magic commands
- Prompt cache keep-alive
- Unified Finding type in shared/types
- Conversation adapter for eyrie integration
