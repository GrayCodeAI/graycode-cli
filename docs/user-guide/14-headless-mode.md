# Headless Mode and Scripting

Headless mode runs Hawk non-interactively from the command line. It accepts a prompt, executes tools, and returns results — ideal for automation and CI/CD.

---

## Basic Usage

Pass a prompt to run headless:

```bash
hawk -p "Your prompt here"
```

Hawk processes the prompt and prints the result to stdout.

---

## Output Formats

### plain (default)

Human-readable text:

```bash
hawk -p "Summarize this codebase"
```

### json

Single JSON object after completion:

```bash
hawk -p "Summarize this codebase" --output-format json | jq -r '.text'
```

Output includes:
- `text` — Response content
- `stopReason` — Why the response ended
- `sessionId` — Session ID for resuming

### streaming-json

NDJSON events in real time:

```bash
hawk -p "Summarize" --output-format streaming-json | jq -r 'select(.type=="text") | .data'
```

Event types:
- `text` — Response chunk
- `thought` — Reasoning (thinking tokens)
- `end` — Final event with metadata
- `error` — Error occurred

---

## Session Management

### Named Session

Each `hawk -p` creates a fresh session by default. To continue a session:

```bash
# Get session ID
hawk -p "Initial prompt" --output-format json | jq -r '.sessionId'

# Resume the session
hawk -p "Follow-up" --resume <session-id>
```

### Continue Most Recent

```bash
hawk -p "Continue" --continue
```

---

## Tool Filtering

Restrict available tools:

```bash
# Allow only read tools
hawk -p "Explain this" --tools "Read,Grep,LS"

# Deny specific tools
hawk -p "Review" --disallowed-tools "Bash,WebSearch"
```

---

## Permission Rules

Control tool permissions:

```bash
# Allow shell commands
hawk -p "Build" --allow "Bash(git*)" --allow "Bash(npm*)"

# Deny dangerous commands
hawk -p "Clean" --deny "Bash(rm*)" --deny "Bash(sudo*)"
```

---

## Auto-Approve Mode

Use `--auto` for fully automated runs:

```bash
hawk -p "Format all files" --auto
hawk exec --auto full "Add error handling"
```

**Warning:** This grants full autonomy. Use only in trusted environments.

---

## CI/CD Integration

### Pre-Commit Hook

```bash
#!/bin/bash
hawk -p "Review staged changes for bugs. Reply OK if fine." \
  --auto --output-format json | jq -r '.text' | grep -q "^OK" || exit 1
```

### Code Review

```bash
hawk -p "Review PR for security issues" \
  --output-format json --auto | jq -r '.text' > review.md
```

### Batch Processing

```bash
for file in src/*.go; do
  hawk -p "Format $file" --auto
done
```

---

## Environment Variables

```bash
export XAI_API_KEY="xai-..."     # API key
export HAWK_HOME="/path"          # Custom config location
export HAWK_LOG_FILE="/tmp/hawk.log"  # Log file
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Error |
| `130` | Interrupted (SIGINT) |
| `143` | Terminated (SIGTERM) |

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Agent Mode](15-agent-mode.md) | Agent configuration |
| [Subagents](16-subagents.md) | Parallel agent sessions |

---

© 2026 GrayCode AI. All rights reserved.