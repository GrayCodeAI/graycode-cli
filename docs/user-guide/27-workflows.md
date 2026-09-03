# Workflows

## Interactive Input

- `/command` invokes a Graycode slash command.
- `!command` runs a direct shell command when the current permission mode allows it.
- `@path` adds file or directory context.
- `Esc` interrupts the current operation.
- Use session, checkpoint, branch, rewind, and compact commands for long work.

## Headless Review

Use a bounded, machine-readable review in CI:

```bash
graycode review run HEAD --output-format json --max-turns 8
```

Review findings remain structured and severity-based. Provider retries,
permissions, and tool timeouts are still enforced in headless mode.

## MCP-Backed Analysis

1. Establish project trust.
2. Merlin configured MCP servers with `graycode mcp`.
3. Run the review or scan command.
4. Check the persisted findings and event output.

Do not bypass trust or permission controls to make an MCP tool convenient.

## Session Recovery

Use `/session`, `/resume`, `/continue`, `/checkpoint`, and `/rewind` to recover
from interruptions. Graycode trims incomplete tool turns before a cancelled session
is reused, preserving provider transcript invariants.

## Recording and Replay

Use `--record` when diagnosing a terminal or provider interaction, then replay
the resulting artifact without making another live provider request. Keep
recordings free of credentials and sensitive tool output.

## Skills and Preferences

Install skills through the registry, audit them before activation, and keep
large references in `references/` rather than bloating `SKILL.md`. Use learned
preferences for style alignment only; never use them to override security or
explicit project policy.
