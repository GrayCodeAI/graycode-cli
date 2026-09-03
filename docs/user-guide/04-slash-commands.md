# Slash Commands

Type `/` in the prompt to access commands. Each command runs an action immediately and autocompletes as you type.

---

## Session Management

### `/new`

Start a new session, clearing the current conversation.

```
/new
```

### `/resume`

Open the session picker to load a previous session.

```
/resume
```

### `/compact [context]`

Compress conversation history to save context window space. Optionally specify what to preserve.

```
/compact
/compact keep the auth implementation details
```

### `/context`

Show context window usage and session stats.

```
/context
```

### `/session-info`

Show session details including model, turn count, and context usage.

```
/session-info
```

### `/fork`

Branch the current session into a new agent, preserving history up to this point.

```
/fork
```

### `/home`

Return to the welcome screen.

```
/home
```

---

## Model and Mode

### `/model <name>`

Switch to a different model. Accepts model IDs or display names.

```
/model grok-build
/model gpt-4o
/model claude-3-5-sonnet
```

### `/effort <level>`

Set reasoning effort on the current model. Levels: `low`, `medium`, `high`, `xhigh`.

```
/effort high
```

Only works when the active model supports reasoning effort.

### `/autonomy`

Control the autonomy tier and sandbox profile. Opens an interactive picker.

```
/autonomy
```

### `/autonomy tier <tier>`

Set autonomy tier without opening the picker.

```
/autonomy tier scout
/autonomy tier builder
/autonomy tier operator
/autonomy tier autonomous
```

### `/autonomy sandbox <profile>`

Set sandbox profile.

```
/autonomy sandbox strict
/autonomy sandbox workspace
/autonomy sandbox off
```

### `/autonomy dry-run <on|off>`

Enable or disable dry-run mode (denies all tools unconditionally).

```
/autonomy dry-run on
/autonomy dry-run off
```

### `/multiline`

Toggle multiline input mode. When enabled, `Enter` inserts a newline and `Shift+Enter` sends.

```
/multiline
```

---

## Memory (via harrier)

### `/harrier`

Browse persistent memory stored in harrier.

```
/harrier
```

### `/harrier search <query>`

Search harrier memories.

```
/harrier search auth implementation
```

### `/remember <note>`

Save a note to memory immediately.

```
/remember the staging deploy uses the eu-west cluster
```

---

## Hooks and Plugins

### `/hooks`

Manage hooks — file-triggered and HTTP-based event handlers.

```
/hooks
```

### `/plugins`

View and manage installed plugins.

```
/plugins
```

### `/skills`

Browse installed skills from the community registry.

```
/skills
```

### `/skills search <query>`

Search the community registry for skills.

```
/skills search go
/skills search review
```

### `/skills install <source>`

Install a skill from a source.

```
/skills install go-review
graycode skills install go-review
```

### `/skills audit`

Security scan installed skills.

```
/skills audit
graycode skills audit
```

---

## Planning and Tasks

### `/spec [description]`

Enter spec mode. Walks through Specify → Plan → Tasks workflow.

```
/spec Add authentication to the API
```

### `/spec status`

Show current spec workflow status.

```
/spec status
```

### `/spec reset`

Reset the spec workflow.

```
/spec reset
```

### `/plan`

Enter plan mode (alias for `/spec`).

```
/plan Refactor the auth module
```

### `/todo`

Create and manage task lists.

```
/todo
```

---

## Asking Questions

### `/ask <question>`

Ask the user a clarifying question. Model uses the `AskUserQuestion` tool internally.

```
/ask What framework should I use for the frontend?
```

With options (in-tool):

```
/ask What database should I use? --options postgresql mysql sqlite
```

---

## Sandbox and Permissions

### `/sandbox strict`

Apply strict sandbox profile (cwd only, no network).

```
/sandbox strict
```

### `/sandbox workspace`

Apply workspace sandbox profile (project directory only).

```
/sandbox workspace
```

### `/sandbox off`

Disable sandbox (full access).

```
/sandbox off
```

### `/trust`

Manage folder trust for project automation.

```
/trust
```

---

## Scheduling

### `/loop [interval] <prompt>`

Run a prompt on a recurring interval.

```
/loop 30m check deploy status
/loop 1 hour review latest changes
```

Interval formats: `30s`, `30m`, `1h`, `1d`. Minimum 60 seconds.

### `/cron`

Open the cron/scheduler management.

```
/cron
```

---

## Diagnostics

### `/path`

Check developer path readiness (setup + security + sandbox).

```
/path
```

### `/preflight`

Quick ready-to-chat check.

```
/preflight
```

### `/ecosystem`

Show ecosystem component status (Eyrie, harrier, shrike).

```
/ecosystem
```

---

## Configuration

### `/config`

Open settings/configuration modal.

```
/config
```

Aliases: `/settings`, `/preferences`

### `/theme`

Switch TUI color theme.

```
/theme
```

### `/scroll-speed <n>`

Set scroll speed (1-100).

```
/scroll-speed 75
```

### `/scroll-mode <mode>`

Set scroll mode (auto, wheel, trackpad).

```
/scroll-mode auto
/scroll-mode wheel
/scroll-mode trackpad
```

### `/scroll-invert`

Toggle natural scrolling (invert direction).

```
/scroll-invert
```

### `/compact-mode`

Toggle compact mode (reduces outer padding).

```
/compact-mode
```

### `/pager-config <option> <value>`

Configure scrollback pager settings.

```
/pager-config lines 5000
/pager-config linenumbers true
```

### `/prompt-queue <subcommand>`

Manage queued prompts.

```
/prompt-queue add refactor this codebase
/prompt-queue list
/prompt-queue clear
/prompt-queue remove 1
```

### `/vim-mode`

Toggle vim-style keybindings.

```
/vim-mode
```

---

## Other Commands

### `/refresh-model-catalog`

Fetch the latest deployment-aware model catalog from Eyrie.

```
/refresh-model-catalog
```

### `/terminal-setup`

Show terminal capability detection and setup info.

```
/terminal-setup
```

### `/help`

Show help information.

```
/help
```

### `/btw <note>`

Send an aside to the agent without interrupting the current task.

```
/btw also check the error handling
```

---

## Skills as Slash Commands

Any enabled skill with `user-invocable: true` in its SKILL.md frontmatter appears as a slash command. Skills from plugins also appear. When multiple skills share the same name, use qualified form:

```
/local:my-skill      # Project-scoped skill
/user:my-skill       # User-scoped skill
```

---

© 2026 GrayCode AI. All rights reserved.