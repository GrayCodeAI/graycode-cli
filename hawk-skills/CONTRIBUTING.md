# Contributing to hawk-skills

## Adding a Skill

1. Create a directory: `mkdir your-skill-name`
2. Write `your-skill-name/SKILL.md` with YAML frontmatter
3. Add an entry to `registry.json`
4. Submit a PR

## SKILL.md Requirements

- **name**: lowercase, hyphens only, max 64 chars
- **description**: max 1024 chars, explain when to use it
- **version**: semver string
- **category**: one of `engineering`, `ops`, `workflow`, `security`, `devtools`, `testing`
- **tags**: array of lowercase keywords
- **allowed-tools**: space-separated hawk tool names

## Quality Checklist

- [ ] Clear "When to Use" section
- [ ] Concrete workflow steps (numbered)
- [ ] Code examples with language identifiers
- [ ] Verification checklist
- [ ] No hardcoded paths or assumptions about project structure
- [ ] Passes `hawk skills audit your-skill-name/SKILL.md`

## Review Process

PRs are reviewed for quality, accuracy, format, and scope. One technology per skill.
