# Sandbox

Sandbox mode restricts what Hawk and its spawned commands can access on your filesystem using OS-level kernel primitives.

---

## Quick Start

```bash
# Workspace sandbox (recommended for development)
hawk --sandbox workspace

# Read-only mode
hawk --sandbox read-only

# Strict mode
hawk --sandbox strict
```

---

## Built-in Profiles

| Profile | FS Read | FS Write | Child Network | Use Case |
|---------|---------|----------|---------------|----------|
| `off` | All | All | All | No restrictions |
| `workspace` | All | CWD + `~/.hawk/` + temp | Allowed | Normal development |
| `read-only` | All | `~/.hawk/` + temp | Blocked (Linux) | Exploration, reviews |
| `strict` | CWD + system | CWD + `~/.hawk/` + temp | Blocked (Linux) | Untrusted code |

---

## Custom Profiles

Create `.hawk/sandbox.toml`:

```toml
[profiles.project]
extends = "workspace"
restrict_network = true
deny = ["**/.env", "**/*.pem"]
```

Use with:

```bash
hawk --sandbox project
```

---

## Profile Details

### workspace (recommended)

Read anywhere, write only to the project directory and Hawk's config. Allows network access for LLM calls and web search.

### read-only

Read anywhere, write only to `~/.hawk/` and temp directories. Good for code exploration without risk of modification.

### strict

Most restrictive. Read only CWD and essential system paths. Write only to CWD, `~/.hawk/`, and temp.

---

## Platform Support

| Platform | Mechanism |
|----------|-----------|
| Linux | Landlock + bubblewrap |
| macOS | Seatbelt |

On Linux, network blocking for child processes requires seccomp. On macOS, network blocking is a no-op.

---

## When to Use Sandbox

**Use `workspace` for:**
- Normal development work
- Shared environments

**Use `read-only` for:**
- Code review
- Exploration of unfamiliar code

**Use `strict` for:**
- Analyzing untrusted code
- Security-sensitive environments

---

## Profile Locking

Once a session starts with a sandbox profile, that profile is fixed for the session lifetime. Resuming a session uses its saved profile automatically.

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Plan Mode](19-plan-mode.md) | Spec workflow |
| [Background Tasks](20-background-tasks.md) | Task management |

---

© 2026 GrayCode AI. All rights reserved.