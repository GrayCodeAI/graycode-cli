# 🦅 hawk-skills

Community skill registry for [Hawk](https://github.com/GrayCodeAI/hawk) — the AI coding agent.

## Install Skills

```bash
# From hawk CLI (non-interactive)
hawk skills install GrayCodeAI/hawk-skills go-review
hawk skills install GrayCodeAI/hawk-skills changelog

# From hawk REPL (interactive)
/skills install GrayCodeAI/hawk-skills go-review
/skills search api
/skills trending
```

## Browse Skills

| Skill | Category | Description |
|---|---|---|
| `go-review` | engineering | Reviews Go code for idioms, error handling, and performance |
| `changelog` | workflow | Generates changelogs from git commits |
| `docker-deploy` | ops | Docker build, optimize, and deploy workflows |
| `api-design` | engineering | RESTful API design patterns and review |
| `security-scan` | security | Scans code for common security vulnerabilities |

## Skill Format

Each skill is a directory with a `SKILL.md`:

```
skill-name/
└── SKILL.md
```

SKILL.md uses YAML frontmatter:

```yaml
---
name: skill-name
description: Brief description
version: "1.0.0"
author: your-name
license: MIT
category: engineering
tags: ["tag1", "tag2"]
allowed-tools: Read Write Bash Grep
---

# Skill Name

Instructions for the AI agent...
```

## Contributing

1. Fork this repo
2. Create `your-skill/SKILL.md`
3. Add an entry to `registry.json`
4. Submit a PR

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT
