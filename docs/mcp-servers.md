# MCP Server Configuration

hawk supports connecting to external MCP (Model Context Protocol) servers to extend its capabilities with additional tools, resources, and prompts.

## Configuration

MCP servers are configured in `settings.json` (global: `~/.hawk/settings.json`, project: `.hawk/settings.json`).

```json
{
  "mcp_servers": [
    {
      "name": "server-name",
      "command": "executable",
      "args": ["arg1", "arg2"],
      "type": "stdio"
    }
  ]
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique identifier for the server |
| `command` | string | Yes | Executable name or path (stdio mode) |
| `args` | string[] | No | Arguments passed to the command |
| `type` | string | No | Transport type: `stdio` (default), `sse`, `http` |
| `url` | string | No | URL for SSE/HTTP transports |
| `headers` | map | No | Custom HTTP headers for SSE/HTTP transports |

## Known MCP Servers

### My-Jogyo (Scientific Research Lab)

My-Jogyo provides 12 MCP tools for scientific research workflows, including Python REPL execution, notebook management, and research session checkpointing.

**Installation:**

```bash
# Install My-Jogyo
npm install -g my-jogyo

# Add to hawk settings
```

**settings.json:**

```json
{
  "mcp_servers": [
    {
      "name": "my-jogyo",
      "command": "my-jogyo",
      "args": ["serve"],
      "type": "stdio"
    }
  ]
}
```

**Available tools:**
- `python_repl` — Execute Python code in a managed REPL
- `research_manager` — Create and manage research sessions
- `gyoshu_snapshot` — Snapshot current research state
- `checkpoint_manager` — Save and restore research checkpoints
- `notebook_writer` — Create and edit Jupyter notebooks
- `notebook_search` — Search notebook contents
- `research_export` — Export research results
- `data_loader` — Load datasets for analysis
- `visualization` — Generate plots and charts
- `environment` — Manage Python environment and packages
- `citation` — Manage research citations
- `session_handoff` — Transfer research context between sessions

### yaad (Memory Engine)

hawk's built-in memory engine. Configured automatically when yaad is installed.

```json
{
  "mcp_servers": [
    {
      "name": "yaad",
      "command": "yaad",
      "args": ["serve", "--stdio"],
      "type": "stdio"
    }
  ]
}
```

### sight (Code Review)

hawk's built-in code review engine.

```json
{
  "mcp_servers": [
    {
      "name": "sight",
      "command": "sight",
      "args": ["serve", "--stdio"],
      "type": "stdio"
    }
  ]
}
```

### inspect (Security Audit)

hawk's built-in security scanner.

```json
{
  "mcp_servers": [
    {
      "name": "inspect",
      "command": "inspect",
      "args": ["serve", "--stdio"],
      "type": "stdio"
    }
  ]
}
```

## CLI Management

```bash
# Add an MCP server
hawk mcp add <name> <command> [args...]

# List configured servers
hawk mcp list

# Remove a server
hawk mcp remove <name>

# Test a server connection
hawk mcp test <name>
```

## Troubleshooting

**Server fails to start:**
- Verify the command is in PATH: `which <command>`
- Check the server supports stdio transport
- Run the command manually to check for errors

**Tools not appearing:**
- Restart hawk after adding a new server
- Check `hawk mcp test <name>` for connection errors
- Verify the server's tools/list response is valid JSON-RPC

**Timeout errors:**
- Default tool timeout is 30 seconds
- Long-running tools may need server-side timeout configuration
- Check network connectivity for remote servers
