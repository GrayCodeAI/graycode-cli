# Agent Mode (ACP) and IDE Integration

Agent mode runs Hawk as an ACP (Agent Client Protocol) server for IDE integration and custom tooling.

---

## What is ACP?

The [Agent Client Protocol (ACP)](https://agentclientprotocol.com) is a standard for AI agent communication. It enables:

- **Session management** — create, load, and resume conversations
- **Prompt submission** — send messages and receive streamed responses
- **Tool visibility** — see tool usage in real time
- **Permission handling** — approve/deny tool executions

---

## stdio Transport

Run Hawk as an ACP server over stdio:

```bash
hawk agent stdio
```

Clients include:
- IDE extensions (Zed, Neovim, Emacs)
- Custom automation tools
- ACP client libraries

### Options

```bash
hawk agent --model gpt-4o stdio
hawk agent --auto stdio
hawk agent --agent-profile path/to/profile.yaml stdio
```

| Flag | Description |
|------|-------------|
| `-m, --model` | Set the model ID |
| `--auto` | Auto-approve tool executions |
| `--agent-profile` | Load agent profile from file |

---

## Running the Agent Server

### WebSocket Server

```bash
hawk agent serve --bind 127.0.0.1:2419 --secret <token>
```

Connect clients over WebSocket using the secret token for authentication.

---

## Session Lifecycle

1. **Initialize** — client sends `initialize` with capabilities
2. **Create session** — client sends `session/new` with working directory
3. **Send prompts** — client sends `session/prompt`
4. **Receive updates** — agent streams `session/update` events
5. **Handle permissions** — approve/deny as needed

---

## Streaming Updates

ACP streams structured events with `sessionUpdate` types:

| Type | Description |
|------|-------------|
| `agent_message_chunk` | Response text |
| `agent_thought_chunk` | Reasoning tokens |
| `tool_call` | Tool invocation started |
| `tool_call_update` | Tool status/result |

---

## Extension Methods

Hawk provides `x.ai/*` extension methods:

| Category | Methods |
|----------|---------|
| Filesystem | `x.ai/fs/*` (read, write, list) |
| Git | `x.ai/git/*` (status, commit, diff) |
| Worktree | `x.ai/git/worktree/*` |
| Search | `x.ai/search/*` |
| Terminal | `x.ai/terminal/*` |

---

## Compatible Clients

| Client | Status |
|--------|--------|
| Zed | Supported |
| Neovim | Supported (via CodeCompanion, avante.nvim) |
| Emacs | Supported |
| JetBrains | Planned |

---

## Integration Example

```typescript
import { spawn } from "child_process";

// Start ACP server
const proc = spawn("hawk", ["agent", "stdio"]);

// Initialize
proc.stdin.write(JSON.stringify({
  jsonrpc: "2.0",
  id: 1,
  method: "initialize",
  params: { protocolVersion: 1, clientCapabilities: { terminal: true } }
}) + "\n");

// Create session
proc.stdin.write(JSON.stringify({
  jsonrpc: "2.0",
  id: 1,
  method: "session/new",
  params: { cwd: "." }
}) + "\n");
```

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Subagents](16-subagents.md) | Parallel agent sessions |
| [Sessions](17-sessions.md) | Session management |

---

© 2026 GrayCode AI. All rights reserved.