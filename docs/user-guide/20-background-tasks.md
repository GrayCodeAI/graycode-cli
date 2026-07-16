# Background Tasks and Monitoring

Hawk runs long-lived processes without blocking the conversation. This covers background commands, `/loop`, and the `monitor` tool.

---

## Background Commands

Run commands in the background with `background: true`:

```
Run the dev server in background mode.
```

### Getting Output

Use `GetTaskOutput` to check on a background task:

```json
{
  "task": "task_id_here"
}
```

The tool returns current output and status.

### Waiting for Tasks

Use `WaitTasks` to block on multiple tasks:

| Parameter | Description |
|-----------|-------------|
| `task_ids` | List of task IDs |
| `mode` | `any` or `all` |
| `timeout_ms` | Max wait time |

### Killing Tasks

Use `KillTask` to terminate a running task:

```
Kill the task running in the background.
```

---

## The /loop Command

Run prompts on recurring intervals:

```
/loop 5m Check test status
/loop 2h Summarize new commits
/loop 60s Check dev server health
```

Interval formats: `Ns`, `Nm`, `Nh`, `Nd` (minimum 60 seconds).

### Behavior

- Fires immediately on creation
- Repeats at specified interval
- Auto-expires after 7 days
- Max 50 concurrent scheduled tasks

---

## The Monitor Tool

Stream events from long-running scripts:

```
Monitor the application log for errors.
```

Use case:
- Log tailing with `tail -f | grep --line-buffered`
- File watching with `inotifywait`
- API polling loops

### Guidelines

- Use `grep --line-buffered` in pipes
- Handle transient failures (`|| true`)
- Set poll intervals to 30s+ for APIs
- Both stdout and stderr generate events

---

## Tasks Pane

In the TUI, press **Ctrl+B** to toggle the tasks pane:

- Running subagents
- Active background tasks
- Monitor and /loop tasks
- Task IDs for reference

---

## Use Cases

### Dev Server + Coding

```
Start the dev server in the background, then implement the login form.
```

### Continuous Monitoring

```
/loop 5m Run tests and report new failures
```

### Log Monitoring

```
Monitor /var/log/app.log for ERROR entries with grep --line-buffered.
```

---

## Best Practices

- Use `background` for one-shot long commands
- Use `/loop` for periodic checks
- Use `monitor` for real-time streams
- Keep monitor filters tight
- Use reasonable poll intervals

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Terminal Support](21-terminal-support.md) | Terminal compatibility |
| [Permissions](22-permissions-and-safety.md) | Safety controls |

---

© 2026 GrayCode AI. All rights reserved.