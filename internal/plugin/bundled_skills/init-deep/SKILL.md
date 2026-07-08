---
name: init-deep
description: Auto-generate hierarchical AGENTS.md files at key directory levels
version: "1.0.0"
author: graycode
license: MIT
category: workflow
tags: ["context", "agents", "bootstrapping", "documentation"]
allowed-tools: Read Grep Glob Bash Write
---

# Init-Deep: Hierarchical AGENTS.md Generation

Generate AGENTS.md files at strategic directory levels throughout a project so that AI coding agents automatically read relevant conventions when working in any subdirectory.

## Phase 1: Explore Project Structure

Scan the project to identify key directories:
- Project root
- Source directories (src/, lib/, pkg/, cmd/, internal/, app/)
- Component/package subdirectories with significant code
- Test directories with distinct conventions
- Configuration directories

Use `Glob` and `Bash` to map the directory tree. Focus on directories that:
- Contain 3+ source files
- Have distinct conventions or tech stacks
- Are commonly navigated by developers

Score directories by complexity (file count, unique extensions, subdirectory depth).

## Phase 2: Analyze Each Directory

For each key directory, read 2-3 representative files to identify:
- Language(s) and frameworks used
- Naming conventions (files, functions, types)
- Import/module patterns
- Testing patterns and frameworks
- Common design patterns
- Error handling approaches
- Documentation style

Use `Grep` to find patterns:
- Import statements
- Function signatures
- Test patterns
- Error types

## Phase 3: Generate AGENTS.md Files

Create AGENTS.md at each identified level. Follow this template:

### Root AGENTS.md
```markdown
# Project Conventions

## Overview
[Brief project description, tech stack, architecture]

## Structure
[Key directories and their purposes]

## Coding Standards
[Language-specific conventions, formatting, naming]

## Testing
[Testing framework, patterns, coverage expectations]

## Build & Run
[How to build, test, run locally]

## Common Patterns
[Shared patterns, utilities, abstractions]
```

### Subdirectory AGENTS.md
```markdown
# [Directory Name] Conventions

## Purpose
[What this directory contains and its role]

## Key Files
[Important files and what they do]

## Patterns
[Directory-specific patterns and conventions]

## Dependencies
[What this depends on, what depends on it]

## Testing
[Directory-specific testing conventions]
```

## Phase 4: Validate

Review generated files for:
- Accuracy: conventions match actual code
- No contradictions between levels (root vs subdirectory)
- Appropriate detail level (not too verbose, not too sparse)
- Correct cross-references between directories

## Phase 5: Report

Summarize what was generated:
- List of files created with paths
- Brief description of each file's focus
- Any inconsistencies found in the codebase
- Suggestions for manual review

## Constraints

- Never overwrite existing AGENTS.md files — skip and report
- Keep each file under 500 lines
- Focus on conventions that help AI agents, not human onboarding
- Include concrete examples from the actual codebase
- Prefer specific rules over general advice ("use `errors.Is()`" over "handle errors properly")
