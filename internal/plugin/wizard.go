package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildNewSkillPrompt creates an LLM prompt for the skill creator wizard.
func BuildNewSkillPrompt(description string) string {
	return fmt.Sprintf(`You are a skill creator for the Hawk AI coding agent. Create a complete SKILL.md file based on this description:

"%s"

Generate a well-structured skill with:
1. YAML frontmatter (name, description, version, author, license, category, tags, allowed-tools)
2. Clear "When to Use" section
3. Concrete workflow steps
4. Code examples where relevant
5. Verification checklist

Output ONLY the complete SKILL.md content, starting with the --- frontmatter delimiter. Use this exact format:

---
name: skill-name-here
description: Brief description (max 1024 chars)
version: "1.0.0"
author: user
license: MIT
category: engineering
tags: ["tag1", "tag2"]
allowed-tools: Read Write Bash Grep
---

# Skill Name

## When to Use This Skill
...

## Workflow
1. ...
2. ...

## Examples
...

## Verification
...`, description)
}

// SaveNewSkill writes a SKILL.md to the project skills directory.
func SaveNewSkill(name, content string) (string, error) {
	dir := filepath.Join(".hawk", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create skill dir: %w", err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("skill %q already exists at %s", name, path)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write skill: %w", err)
	}
	return path, nil
}

// ExtractSkillName tries to extract the skill name from generated SKILL.md content.
func ExtractSkillName(content string) string {
	skill := parseSmartSkill(content)
	if skill.Name != "" {
		return strings.TrimSpace(skill.Name)
	}
	return ""
}
