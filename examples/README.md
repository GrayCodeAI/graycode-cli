# Hawk Examples

Hawk is an AI coding agent that understands your codebase.

## Basic Usage

### Start a chat session

```bash
hawk chat
> Explain how the authentication system works
```

### Generate code

```bash
hawk "Add input validation to the user registration endpoint"
```

### Review changes

```bash
hawk review
```

## Advanced Examples

### Use with skills

```bash
hawk --skill code-review "Review my latest changes"
```

### Analyze codebase

```bash
hawk analyze --depth full
```

### Fix issues

```bash
hawk fix --auto
```

## MCP Integration

Hawk can use MCP servers for extended capabilities:

```bash
# With yaad for persistent memory
yaad setup
hawk chat

# With trace for session capture
trace start
hawk "refactor the API layer"
trace stop
```

## Configuration

Create `.hawk/config.json`:

```json
{
  "model": "claude-sonnet-4-6",
  "maxTokens": 8192,
  "temperature": 0.7
}
```

See the [main README](../README.md) for full documentation.
