# Graycode Examples

Graycode is an AI coding agent that understands your codebase.

## Basic Usage

### Start a chat session

```bash
graycode chat
> Explain how the authentication system works
```

### Generate code

```bash
graycode "Add input validation to the user registration endpoint"
```

### Review changes

```bash
graycode review
```

## Advanced Examples

### Use with skills

```bash
graycode --skill code-review "Review my latest changes"
```

### Analyze codebase

```bash
graycode analyze --depth full
```

### Fix issues

```bash
graycode fix --auto
```

### Headless agent in CI

Copy [graycode-ci-exec.yml](github/graycode-ci-exec.yml) to run `graycode exec --ephemeral --json`
on pull requests (summarize diff, risk list, or your own prompt). Pin your
graycode install step and provider secrets before enabling the job.

### Report CI delivery context

Copy [graycode-delivery-context.yml](github/graycode-delivery-context.yml) to your
repository to report GitHub Actions runs to Graycode Cloud. Create a dedicated,
revocable device token for CI and keep its endpoint, device ID, project ID, and
token in GitHub Actions secrets.

## MCP Integration

Graycode can use MCP servers for extended capabilities:

```bash
# With harrier for persistent memory
harrier setup
graycode chat

# With swift for session capture
swift start
graycode "refactor the API layer"
swift stop
```

## Configuration

Create `.graycode/config.json`:

```json
{
  "model": "claude-sonnet-4-6",
  "maxTokens": 8192,
  "temperature": 0.7
}
```

See the [main README](../README.md) for full documentation.
