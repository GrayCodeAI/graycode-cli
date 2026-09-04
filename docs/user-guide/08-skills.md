# Skills

Skills are reusable prompt packages that extend Graycode with task-specific instructions. They let you capture a repeatable procedure once, instead of re-explaining it each session.

---

## What Are Skills?

A skill is a directory containing a `SKILL.md` file. Its markdown body tells Graycode how to handle a specific type of task: step-by-step instructions, conventions, and tool-usage patterns.

Use a skill for a repeatable procedure that's too specific for AGENTS.md but too long to retype each time. Graycode activates a skill when it applies to your current task.

---

## Skill Locations

Graycode discovers skills from these directories, in priority order:

| Location | Scope | Priority |
|----------|-------|----------|
| `.graycode/skills/`, `.graycode/commands/` | Local (CWD) | Highest |
| `<repo-root>/.graycode/skills/` | Repo | Medium |
| `~/.graycode/skills/` | User | Lowest |

Higher-priority locations override skills with the same name.

---

## Creating a Skill

### Directory Structure

Each skill lives in its own directory with a `SKILL.md` file:

```
~/.graycode/skills/
  commit/
    SKILL.md
  review-pr/
    SKILL.md
```

### SKILL.md Format

A skill file has YAML frontmatter followed by markdown instructions:

```markdown
---
name: commit
description: Create well-formatted git commits following conventional commit standards. Use when the user wants to commit changes or asks for /commit.
user-invocable: true
---

# Git Commit Skill

Review staged changes and create a commit with a clear, conventional message.

## Steps

1. Run `git diff --staged` to see changes
2. Summarize what changed and why
3. Create commit message following conventional commits format
4. Run `git commit -m "..."` with the message
```

### Core Frontmatter Fields

| Field | Description |
|-------|-------------|
| `name` | Skill identifier (lowercase letters, digits, hyphens) |
| `description` | What the skill does and when to use it |

### Optional Frontmatter Fields

| Field | Description |
|-------|-------------|
| `allowed-tools` | Tools the skill uses |
| `user-invocable` | Whether you can run the skill as a slash command (default: `true`) |
| `disable-model-invocation` | When `true`, only slash command runs the skill |
| `license` | License identifier |

---

## Using Skills

### Run a Skill

Each skill is a slash command named after the skill:

```
/commit              # Runs the "commit" skill
/review-pr           # Runs the "review-pr" skill
```

Pass arguments after the name:

```
/commit fix the build
```

### Qualified Names

When names collide, use qualified forms:

```
/local:commit        # From ./.graycode/skills/
/user:commit         # From ~/.graycode/skills/
```

### Automatic Invocation

Graycode can invoke a skill automatically when it recognizes a relevant task. Write specific descriptions in the `description` and `when-to-use` fields.

---

## CLI Management

Manage skills from the command line:

```bash
# Search the community registry
graycode skills search go

# Install a skill from a source
graycode skills install go-review

# Audit installed skills for security
graycode skills audit
```

---

## Installing Skills

Graycode ships **no bundled skills** by default. Skills are installed on demand
from the separate `GrayCodeAI/starling` repo (or any GitHub repo):

```bash
graycode skills search <query>                  # find a skill in the registry
graycode skills install <owner/repo> [skill]    # install after user approval
graycode skills audit                           # security-scan installed skills
```

Once installed, skills are discovered from the locations listed above.

### Install-time security and the lockfile

Every install runs two scans on skill content before it is written:

1. **Unicode audit** — dangerous homoglyph/injection characters are
   stripped (reported as `sanitized`).
2. **Threat scan** — pattern scan for command injection, data
   exfiltration, encoded payloads, and network calls, scored 0-100.
   Content scoring below 30 is **refused**; scores in the warning band
   install with a printed report.

Each install is recorded in a `skills-lock.json` next to the installed
skills (user or project scope), pinning the source repo, the cloned
commit, and the SHA-256 of the installed content. Removing a skill drops
its lock entry.

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Plugins](09-plugins.md) | Multi-component plugins and marketplace |
| [Hooks](10-hooks.md) | Event-driven automation |

---

© 2026 GrayCode AI. All rights reserved.