---
name: changelog
description: Generates user-facing changelogs from git commits using conventional commit format
version: "1.0.0"
author: graycode
license: MIT
category: workflow
tags: ["changelog", "git", "release"]
allowed-tools: Read Bash Grep
---

# Changelog Generator

## When to Use
- Preparing release notes
- Creating weekly update summaries
- Documenting changes between versions

## Workflow
1. Identify commit range using `git log --oneline <from>..<to>`
2. Categorize commits by conventional commit prefix:
   - `feat:` → **Features**
   - `fix:` → **Bug Fixes**
   - `BREAKING CHANGE:` → **Breaking Changes**
   - `docs:` → **Documentation**
   - `refactor:` → **Refactoring**
   - `perf:` → **Performance**
3. Rewrite technical commit messages into user-friendly language
4. Format as markdown with date and version headers

## Output Format
```markdown
## [1.2.0] - 2026-05-03

### Features
- Added dark mode support for all themes

### Bug Fixes
- Fixed login timeout on slow connections

### Breaking Changes
- Removed deprecated `oldApi()` — use `newApi()` instead
```

## Verification
- All commits in range are accounted for
- Breaking changes are prominently highlighted
- No internal/technical jargon in user-facing entries
