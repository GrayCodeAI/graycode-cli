package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BundledSkill defines a skill that ships with hawk.
type BundledSkill struct {
	Name        string
	Description string
	Category    string
	Content     string
	Files       map[string]string // additional files beyond SKILL.md
}

// bundledSkills returns the set of skills that ship with hawk.
func bundledSkills() []BundledSkill {
	return []BundledSkill{
		{
			Name:        "git-workflow",
			Description: "Best practices for git branching, commits, and PRs",
			Category:    "general",
			Content: `---
name: git-workflow
description: Best practices for git branching, commits, and PRs
category: general
---

# Git Workflow

## Commit Messages
- Use conventional commits: feat, fix, docs, style, refactor, test, chore
- Keep subject line under 72 characters
- Use imperative mood: "Add feature" not "Added feature"

## Branching
- Feature branches: feat/description
- Bugfix branches: fix/description
- Always rebase onto main before opening PR
- Squash merge PRs to keep history clean

## PRs
- Small, focused PRs (< 400 lines)
- Include test coverage for new features
- Link related issues
`,
		},
		{
			Name:        "test-driven",
			Description: "Test-first development workflow",
			Category:    "general",
			Content: `---
name: test-driven
description: Test-first development workflow
category: general
---

# Test-Driven Development

## Workflow
1. Write a failing test for the desired behavior
2. Write the minimum code to make it pass
3. Refactor while keeping tests green
4. Repeat

## Rules
- Never write implementation before tests
- Tests should be named descriptively (TestFunctionDoesX)
- Each test should have a single assertion
- Use table-driven tests for multiple cases
- Run tests after every change
`,
		},
		{
			Name:        "security-checklist",
			Description: "Security review checklist for code changes",
			Category:    "security",
			Content: `---
name: security-checklist
description: Security review checklist for code changes
category: security
---

# Security Checklist

## Input Validation
- [ ] All user input is validated and sanitized
- [ ] SQL queries use parameterized statements
- [ ] File paths are validated against traversal

## Authentication & Authorization
- [ ] Sensitive endpoints require authentication
- [ ] Role-based access control is enforced
- [ ] Session tokens are httpOnly and secure

## Secrets
- [ ] No hardcoded API keys or passwords
- [ ] Environment variables used for secrets
- [ ] .env files are in .gitignore

## Error Handling
- [ ] Errors don't expose internal details
- [ ] Stack traces not sent to clients
- [ ] Rate limiting on auth endpoints
`,
		},
		{
			Name:        "code-review",
			Description: "Systematic code review process",
			Category:    "general",
			Content: `---
name: code-review
description: Systematic code review process
category: general
---

# Code Review Process

## Review Checklist
1. **Correctness**: Does it do what it's supposed to?
2. **Readability**: Can another developer understand it?
3. **Performance**: Are there obvious inefficiencies?
4. **Security**: Any vulnerabilities?
5. **Testing**: Are edge cases covered?
6. **Maintainability**: Will this be easy to change later?

## Feedback Style
- Be specific and actionable
- Suggest alternatives, not just problems
- Praise good patterns too
- Focus on code, not the author
`,
		},
		{
			Name:        "debugging",
			Description: "Systematic debugging methodology",
			Category:    "general",
			Content: `---
name: debugging
description: Systematic debugging methodology
category: general
---

# Debugging Methodology

## Steps
1. **Reproduce**: Can you trigger the bug consistently?
2. **Isolate**: What's the minimal reproduction?
3. **Hypothesize**: What could cause this?
4. **Test**: Add logging/breakpoints to verify
5. **Fix**: Make the smallest change that works
6. **Verify**: Test the fix and check for regressions

## Tips
- Read the full error message and stack trace
- Check recent changes (git log, git diff)
- Use binary search to isolate the problem
- Rubber duck explanation often reveals the issue
`,
		},
	}
}

// BundledSkillsDir returns the directory where bundled skills are extracted.
func BundledSkillsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "bundled-skills")
}

// ExtractBundledSkills extracts bundled skills to the user directory.
// Returns the number of skills extracted.
func ExtractBundledSkills() (int, error) {
	dir := BundledSkillsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("create bundled skills directory: %w", err)
	}

	skills := bundledSkills()
	extracted := 0

	for _, skill := range skills {
		skillDir := filepath.Join(dir, skill.Name)

		// Skip if already extracted
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); err == nil {
			continue
		}

		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			continue
		}

		if err := os.WriteFile(skillFile, []byte(skill.Content), 0o644); err != nil {
			continue
		}

		// Write additional files
		for name, content := range skill.Files {
			if err := os.WriteFile(filepath.Join(skillDir, name), []byte(content), 0o644); err != nil {
				continue
			}
		}

		extracted++
	}

	return extracted, nil
}

// BundledSkillsSummary returns a human-readable list of bundled skills.
func BundledSkillsSummary() string {
	skills := bundledSkills()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Bundled skills (%d):\n\n", len(skills)))
	for _, s := range skills {
		b.WriteString(fmt.Sprintf("  • %s — %s [%s]\n", s.Name, s.Description, s.Category))
	}
	return b.String()
}
