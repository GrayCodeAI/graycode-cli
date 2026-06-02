<div align="center">

# 🦅 hawk Architecture

**AI Coding Agent for Your Terminal**

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![Port](https://img.shields.io/badge/Port-4590-orange)](https://www.iana.org/assignments/service-names-port-numbers/service-names-port-numbers.xhtml)
[![Protocol](https://img.shields.io/badge/Protocol-REST-blue)](https://swagger.io/specification/)

</div>

---

## 🎯 Overview

hawk is an AI-powered coding agent for the terminal. It reads codebases, writes and edits files, runs tests, and manages git — all through natural language. Zero CGO, single static binary for linux/darwin/windows on amd64/arm64.

---

## 🧱 Layered Architecture

```
hawk/
├── api/openapi.yaml           📜 Daemon REST API contract (OpenAPI 3.1)
├── cmd/                       🖥️ Cobra CLI commands (200+ files)
│   ├── hawk/main.go           ⚡ Entry point — calls cmd.Execute()
│   ├── root.go                ⚙️ Root command, flag definitions
│   ├── daemon.go              🔮 Daemon start/stop/status
│   ├── chat.go                💬 Interactive TUI chat
│   └── ...
├── internal/
│   ├── api/                   🌐 HTTP server (:4590) — 8 REST endpoints
│   ├── daemon/                🔮 Daemon lifecycle (PID file, socket)
│   ├── engine/                🧠 Agent execution loop
│   │   ├── session.go         🔄 Core agent loop (Stream, agentLoop)
│   │   ├── ctxmgr/            📦 Context packing and visualization
│   │   ├── token/             💰 Budget allocation and prediction
│   │   ├── streaming/         📡 Response cache and stream optimizer
│   │   ├── planning/          🎯 Goals and task decomposition
│   │   └── workflow/          🔧 JSON-defined automation pipelines
│   ├── tool/                  🛠️ 40+ built-in tools
│   ├── config/                ⚙️ Settings, env manager, migration
│   ├── session/               💾 SQLite persistence, search, export
│   ├── permissions/           🛡️ Guardian, rules DSL, boundary checker
│   ├── sandbox/               🏖️ Landlock + seccomp isolation
│   ├── intelligence/          🧬 Repo map, AST analysis, deps
│   ├── multiagent/            👥 Personas, inter-agent messaging
│   ├── mcp/                   🔌 MCP client and server
│   ├── bridge/                🌉 Bridges to ecosystem services
│   └── resilience/            🔄 Circuit breaker, retry, rate limit
├── shared/types/              📤 Cross-repo exported types
├── docs/                      📖 Architecture docs
└── external/                  🔗 Local go.work checkouts
```

---

## 🌐 Daemon HTTP API (:4590)

| | |
|---|---|
| **Contract** | [`api/openapi.yaml`](../api/openapi.yaml) |
| **Port** | `:4590` (default). Override: `HAWK_DAEMON_PORT` |
| **Auth** | Bearer token or `X-API-Key`. Set via `HAWK_DAEMON_API_KEY` |

<details>
<summary><b>📡 Endpoint Summary</b></summary>

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/health` | 🩺 Health check |
| `GET` | `/v1/version` | 🏷️ Version info |
| `POST` | `/v1/chat` | 💬 Send message (JSON or SSE) |
| `GET` | `/v1/sessions` | 📋 List sessions |
| `GET` | `/v1/sessions/{id}` | 🔍 Get session |
| `GET` | `/v1/sessions/{id}/messages` | 💬 Get messages |
| `DELETE` | `/v1/sessions/{id}` | 🗑️ Delete session |
| `GET` | `/v1/stats` | 📊 Usage statistics |

</details>

---

## 🔗 Ecosystem Integration

| Service | Role | Connection |
|---------|------|------------|
| 🦅 **eyrie** | LLM provider runtime | `:8080` — all LLM calls routed here |
| 🧠 **yaad** | Persistent memory | `:3456` — session context, recall |
| 👁️ **sight** | Code review | Library — diff-based review |
| 🔍 **inspect** | Security audit | Library — website scanning |
| ✂️ **tok** | Token optimization | Library — compression, secrets |
| 📸 **trace** | Session capture | CLI hook — git-native capture |

> 💡 **hawk never talks to LLM APIs directly** — all calls go through eyrie.

---

## 🛡️ Tool Safety Layer

Every tool call passes through the permission system before execution:

```
Tool Call → 🛡️ Guardian (rules DSL) → 🧱 Boundary Checker → 👤 User Approval → 🏖️ Sandbox (landlock/seccomp) → ✅ Execute
```

---

## 📐 Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| `cmd/hawk/main.go` entry point | Standard Go layout — goreleaser builds with `main: ./cmd/hawk` producing `hawk` binary |
| `cmd/` is CLI library | Not a binary sub-directory — holds 200+ cobra command files |
| Zero CGO | Pure Go, cross-compilable. Tree-sitter is optional |
| `internal/` is private | Other repos import `shared/types/` only |
| `external/` | go.work symlinks for local dev — not committed |
