# Hawk ACP Client Guide

Hawk speaks the **Agent Client Protocol (ACP)** — newline-delimited JSON-RPC 2.0
over stdio — so editors (e.g. Zed) and custom tooling can drive the agent the
same way the TUI does, including the control plane: work modes, isolation
profiles, and background tasks.

This guide is a worked reference for writing an ACP client. It pairs with the
protocol surface documented in `docs/architecture/control-plane.md`.

---

## Starting the server

```bash
hawk acp
```

Hawk reads newline-delimited JSON-RPC 2.0 requests from stdin and writes
responses and notifications to stdout. **Do not log to stdout** — it is the
protocol channel. All other diagnostics go to stderr.

A minimal client starts the process and exchanges `initialize`:

```bash
hawk acp <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
EOF
```

Response:

```json
{"jsonrpc":"2.0","id":1,"result":{
  "protocolVersion": 1,
  "agentCapabilities": {
    "loadSession": false,
    "promptCapabilities": {"image": false, "audio": false}
  },
  "hawkCapabilities": {
    "workModes": ["plan", "act", "review"],
    "isolation": ["dev", "workspace", "strict", "container"],
    "folderTrust": true,
    "lazyTools": true,
    "autoCommit": true,
    "spawnController": true
  }
}}
```

`hawkCapabilities` is hawk-specific metadata IDE clients can use to show the
control plane in their UI. It is additive — ignore it if you do not need it.

---

## Protocol summary

| Method | Direction | Purpose |
|--------|-----------|---------|
| `initialize` | client → server | handshake, capability negotiation |
| `session/new` | client → server | create a session, get `sessionId` |
| `session/setMode` | client → server | switch work mode (`plan` \| `act` \| `review`) |
| `session/setIsolation` | client → server | apply an isolation profile |
| `session/status` | client → server | control-plane snapshot |
| `session/prompt` | client → server | run a prompt (streams `session/update`) |
| `session/cancel` | client → server | cancel the in-flight prompt |
| `session/update` | server → client | streaming progress notification |
| `session/request_permission` | server → client | ask the client to approve a tool call |

---

## Session lifecycle with the control plane

The canonical flow for a controlled edit session:

### 1. Create a session

```json
{"jsonrpc":"2.0","id":2,"method":"session/new","params":{}}
```

Response:

```json
{"jsonrpc":"2.0","id":2,"result":{
  "sessionId": "sess_1",
  "hawk": {
    "workMode": "act",
    "isolation": "dev",
    "autoCommit": false
  }
}}
```

New sessions default to `act` mode with the `dev` isolation profile — the same
defaults as the interactive TUI. Prompts are a structured array
(`[{"type":"text","text":"..."}]`) so clients can later send non-text blocks;
hawk currently consumes the `text` blocks.

### 2. Switch to plan mode

```json
{"jsonrpc":"2.0","id":3,"method":"session/setMode","params":{
  "sessionId": "sess_1",
  "mode": "plan"
}}
```

Response:

```json
{"jsonrpc":"2.0","id":3,"result":{"sessionId":"sess_1","workMode":"plan"}}
```

`plan` restricts the model surface to read + plan tools and makes Bash
read-only — useful before letting the agent touch files. `act` is the full
surface; `review` is a read-only review surface.

### 3. Raise the isolation profile

```json
{"jsonrpc":"2.0","id":4,"method":"session/setIsolation","params":{
  "sessionId": "sess_1",
  "profile": "workspace"
}}
```

Response:

```json
{"jsonrpc":"2.0","id":4,"result":{"sessionId":"sess_1","isolation":"workspace"}}
```

Profiles: `dev` (no sandbox), `workspace` (sandboxed to the workspace),
`strict` (stricter sandbox), `container` (requires a container). An invalid
profile returns a JSON-RPC error with code `-32602`.

### 4. Check the snapshot

```json
{"jsonrpc":"2.0","id":5,"method":"session/status","params":{
  "sessionId": "sess_1"
}}
```

Response:

```json
{"jsonrpc":"2.0","id":5,"result":{
  "sessionId": "sess_1",
  "workMode": "plan",
  "isolation": "workspace",
  "autoCommit": false,
  "messages": 0
}}
```

### 5. Run a prompt

```json
{"jsonrpc":"2.0","id":6,"method":"session/prompt","params":{
  "sessionId": "sess_1",
  "prompt": [{"type":"text","text":"Summarize the retry logic in internal/engine/stream.go"}]
}}
```

The server streams `session/update` notifications while the agent works and
answers `id:6` when the turn finishes. A prompt that wants to use a tool the
client must approve triggers `session/request_permission`; the client replies
to the server's request with the same `id`.

### 6. Cancel if needed

```json
{"jsonrpc":"2.0","id":7,"method":"session/cancel","params":{
  "sessionId": "sess_1"
}}
```

---

## Reference client (Python, stdlib only)

A complete, dependency-free client showing the full lifecycle above:

```python
#!/usr/bin/env python3
"""Minimal hawk ACP client: initialize → new → setMode → status → prompt."""
import json
import subprocess
import sys

proc = subprocess.Popen(
    ["hawk", "acp"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    text=True,
    bufsize=1,
)


def call(method, params=None, rid=None):
    """Send one request, return the decoded result (or raise on error)."""
    global _rid
    rid = rid if rid is not None else _rid
    _rid += 1
    body = {"jsonrpc": "2.0", "id": rid, "method": method}
    if params is not None:
        body["params"] = params
    proc.stdin.write(json.dumps(body) + "\n")
    proc.stdin.flush()
    while True:
        line = proc.stdout.readline()
        if not line:
            raise RuntimeError("hawk acp closed the stream")
        msg = json.loads(line)
        if msg.get("id") != rid:
            continue  # notifications / other responses
        if "error" in msg:
            raise RuntimeError(f"{method}: {msg['error']}")
        return msg["result"]


_rid = 1

call("initialize", {})
sess = call("session/new", {})["sessionId"]
print("session:", sess)
print("setMode ->", call("session/setMode", {"sessionId": sess, "mode": "plan"}))
print("isolation ->", call("session/setIsolation", {"sessionId": sess, "profile": "workspace"}))
print("status ->", call("session/status", {"sessionId": sess}))

# Streaming prompt: read notifications until the response for id=6 arrives.
proc.stdin.write(json.dumps({
    "jsonrpc": "2.0", "id": 6, "method": "session/prompt",
    "params": {"sessionId": sess, "prompt": [{"type": "text", "text": "Say hello."}]},
}) + "\n")
proc.stdin.flush()
while True:
    line = proc.stdout.readline()
    if not line:
        break
    msg = json.loads(line)
    if msg.get("id") == 6:
        print("prompt done:", msg["result"])
        break
    # session/update notifications stream past here

proc.stdin.close()
proc.wait()
```

---

## Errors

Errors follow JSON-RPC 2.0:

| Code | Meaning |
|------|---------|
| `-32700` | Parse error |
| `-32600` | Invalid request |
| `-32601` | Method not found |
| `-32602` | Invalid params (e.g. unknown `sessionId`, bad mode/profile) |
| `-32603` | Internal error (e.g. session factory failure) |

Unknown sessions and invalid `mode`/`profile` values return `-32602` with a
human-readable message in `error.message`.

---

## Where to go next

| Document | What You Will Learn |
|----------|-------------------|
| [Control plane](../architecture/control-plane.md) | The full control-plane design |
| [Headless mode](../user-guide/14-headless-mode.md) | Scripting without ACP |

© 2026 GrayCode AI. All rights reserved.
