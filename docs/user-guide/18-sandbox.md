# Sandbox

Sandbox mode restricts what Graycode and its spawned commands can access on your filesystem using OS-level kernel primitives.

---

## Quick Start

```bash
# Workspace sandbox (recommended for development)
graycode --sandbox workspace

# Read-only mode
graycode --sandbox read-only

# Strict mode
graycode --sandbox strict
```

---

## Built-in Profiles

| Profile | FS Read | FS Write | Child Network | Use Case |
|---------|---------|----------|---------------|----------|
| `off` | All | All | All | No restrictions |
| `workspace` | All | CWD + `~/.graycode/` + temp | Allowed | Normal development |
| `read-only` | All | `~/.graycode/` + temp | Blocked (Linux) | Exploration, reviews |
| `strict` | CWD + system | CWD + `~/.graycode/` + temp | Blocked (Linux) | Untrusted code |

---

## Custom Profiles

Create `.graycode/sandbox.toml`:

```toml
[profiles.project]
extends = "workspace"
restrict_network = true
deny = ["**/.env", "**/*.pem"]
```

Use with:

```bash
graycode --sandbox project
```

---

## Profile Details

### workspace (recommended)

Read anywhere, write only to the project directory and Graycode's config. Allows network access for LLM calls and web search.

### read-only

Read anywhere, write only to `~/.graycode/` and temp directories. Good for code exploration without risk of modification.

### strict

Most restrictive. Read only CWD and essential system paths. Write only to CWD, `~/.graycode/`, and temp.

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