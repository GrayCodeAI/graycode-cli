package plugin

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/home"
)

// SkillSource tracks where an installed skill came from.
type SkillSource struct {
	Repo        string `json:"repo,omitempty"`
	Ref         string `json:"ref,omitempty"`
	InstalledAt string `json:"installed_at,omitempty"`
}

// SmartSkill is a skill that can be auto-invoked based on file paths or user
// prompt context. Follows the Agent Skills spec (agentskills.io).
type SmartSkill struct {
	Name          string
	Description   string   // used for auto-matching against user prompts
	Paths         []string // glob patterns that trigger this skill
	Content       string   // skill prompt content (body of SKILL.md)
	AutoInvoke    bool     // if true, model can trigger without user /command
	Compatibility string   // environment requirements (per spec)
	AllowedTools  string   // pre-approved tools, space-separated (per spec)
	Version       string   // semver for update tracking
	Author        string   // skill author
	License       string   // license identifier (MIT, Apache-2.0, etc.)
	Category      string   // engineering, ops, testing, security, devtools, workflow
	Tags          []string // discovery tags
	Agents        []string // cross-agent compatibility (hawk, claude-code, etc.)
	Source        SkillSource
	Invoke        string   // namespaced invocation pattern (e.g. "/vendor:skill")
	Refs          []string // declared @ref() references in SKILL.md
	RefDir        string   // path to references/ directory
	Chain         SkillChain
}

// SkillChain declares relationships between skills.
type SkillChain struct {
	After     []string // skills that should run before this one
	Before    []string // skills to suggest after this one completes
	Conflicts []string // skills that cannot be active simultaneously
	Enhances  []string // skills that work well together (advisory)
}

// LoadSmartSkills scans the given directories for SKILL.md files with YAML
// frontmatter and returns the parsed skills.
//
// Frontmatter format:
//
//	---
//	name: api-review
//	description: Reviews API endpoints for consistency
//	paths: ["src/api/**", "routes/**"]
//	auto-invoke: true
//	---
func LoadSmartSkills(dirs []string) []SmartSkill {
	var skills []SmartSkill
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
			data, err := os.ReadFile(skillFile)
			if err != nil {
				continue
			}
			skill := parseSmartSkill(string(data))
			if skill.Name == "" {
				skill.Name = e.Name()
			}
			// Set reference directory if it exists
			refDir := filepath.Join(dir, e.Name(), "references")
			if info, err := os.Stat(refDir); err == nil && info.IsDir() {
				skill.RefDir = refDir
			}
			skills = append(skills, skill)
		}
	}
	return skills
}

// parseSmartSkill parses a SKILL.md file with YAML frontmatter.
func parseSmartSkill(content string) SmartSkill {
	var skill SmartSkill

	// Split frontmatter from body.
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		skill.Content = content
		return skill
	}

	// Find the closing ---
	rest := content[3:]
	idx := strings.Index(rest, "---")
	if idx < 0 {
		skill.Content = content
		return skill
	}

	frontmatter := rest[:idx]
	body := strings.TrimSpace(rest[idx+3:])
	skill.Content = body

	// Simple line-by-line YAML parsing (avoids external dependency).
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := parseYAMLLine(line)
		if !ok {
			continue
		}
		switch key {
		case "name":
			skill.Name = val
		case "description":
			skill.Description = val
		case "auto-invoke":
			skill.AutoInvoke = val == "true"
		case "paths":
			skill.Paths = parseYAMLStringArray(val)
		case "compatibility":
			skill.Compatibility = val
		case "allowed-tools":
			skill.AllowedTools = val
		case "version":
			skill.Version = val
		case "author":
			skill.Author = val
		case "license":
			skill.License = val
		case "category":
			skill.Category = val
		case "tags":
			skill.Tags = parseYAMLStringArray(val)
		case "agents":
			skill.Agents = parseYAMLStringArray(val)
		case "source-repo":
			skill.Source.Repo = val
		case "source-installed-at":
			skill.Source.InstalledAt = val
		case "source-ref":
			skill.Source.Ref = val
		case "invoke":
			skill.Invoke = val
		case "chain-after":
			skill.Chain.After = parseYAMLStringArray(val)
		case "chain-before":
			skill.Chain.Before = parseYAMLStringArray(val)
		case "chain-conflicts":
			skill.Chain.Conflicts = parseYAMLStringArray(val)
		case "chain-enhances":
			skill.Chain.Enhances = parseYAMLStringArray(val)
		}
	}

	// Extract @ref() declarations from content
	skill.Refs = extractRefs(skill.Content)

	return skill
}

// extractRefs finds all @ref(filename.md) patterns in content.
func extractRefs(content string) []string {
	var refs []string
	for _, line := range strings.Split(content, "\n") {
		for {
			idx := strings.Index(line, "@ref(")
			if idx < 0 {
				break
			}
			end := strings.Index(line[idx:], ")")
			if end < 0 {
				break
			}
			ref := line[idx+5 : idx+end]
			refs = append(refs, ref)
			line = line[idx+end+1:]
		}
	}
	return refs
}

// LoadRef loads a reference document on-demand from the skill's references/ dir.
func (s *SmartSkill) LoadRef(name string) (string, error) {
	if s.RefDir == "" {
		return "", os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(s.RefDir, name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseYAMLLine splits "key: value" and returns (key, value, true).
func parseYAMLLine(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	val := strings.TrimSpace(line[idx+1:])
	return key, val, true
}

// parseYAMLStringArray parses a JSON-ish array: ["a", "b", "c"].
func parseYAMLStringArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ResolveChainConflicts checks if activating a skill conflicts with already-active skills.
func ResolveChainConflicts(candidate SmartSkill, active map[string]SmartSkill) []string {
	var conflicts []string
	for name, a := range active {
		for _, c := range candidate.Chain.Conflicts {
			if name == c {
				conflicts = append(conflicts, name)
			}
		}
		for _, c := range a.Chain.Conflicts {
			if candidate.Name == c {
				conflicts = append(conflicts, name)
			}
		}
	}
	return conflicts
}

// SuggestChainSkills returns skill names that should be suggested based on chain declarations.
func SuggestChainSkills(skill SmartSkill) (after []string, enhances []string) {
	return skill.Chain.Before, skill.Chain.Enhances
}

// MatchSkillsByPath returns skills whose Paths glob patterns match activePath.
func MatchSkillsByPath(skills []SmartSkill, activePath string) []SmartSkill {
	var matched []SmartSkill
	for _, skill := range skills {
		for _, pattern := range skill.Paths {
			ok, _ := filepath.Match(pattern, activePath)
			if ok {
				matched = append(matched, skill)
				break
			}
			// Also try matching just the base name for simple patterns.
			ok, _ = filepath.Match(pattern, filepath.Base(activePath))
			if ok {
				matched = append(matched, skill)
				break
			}
		}
	}
	return matched
}

// MatchSkillsByContext returns skills whose Description keywords appear in the
// user prompt. Uses simple case-insensitive word overlap.
func MatchSkillsByContext(skills []SmartSkill, userPrompt string) []SmartSkill {
	promptLower := strings.ToLower(userPrompt)
	promptWords := strings.Fields(promptLower)

	var matched []SmartSkill
	for _, skill := range skills {
		if skill.Description == "" {
			continue
		}
		descWords := strings.Fields(strings.ToLower(skill.Description))
		if hasWordOverlap(promptWords, descWords) {
			matched = append(matched, skill)
		}
	}
	return matched
}

// hasWordOverlap returns true if at least one non-trivial word from b appears in a.
func hasWordOverlap(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, w := range a {
		if len(w) > 3 { // skip short/common words
			set[w] = true
		}
	}
	for _, w := range b {
		if len(w) > 3 && set[w] {
			return true
		}
	}
	return false
}

// FormatSkillsForPrompt formats matched skills into text suitable for
// injection into the system prompt.
func FormatSkillsForPrompt(skills []SmartSkill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Available Skills\n\n")
	for _, s := range skills {
		b.WriteString("### ")
		b.WriteString(s.Name)
		b.WriteString("\n")
		if s.Description != "" {
			b.WriteString(s.Description)
			b.WriteString("\n")
		}
		if s.Content != "" {
			b.WriteString("\n")
			b.WriteString(s.Content)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ParseSmartSkillPublic is the exported version of parseSmartSkill.
func ParseSmartSkillPublic(content string) SmartSkill {
	return parseSmartSkill(content)
}

// DefaultSkillDirs returns directories to scan for SKILL.md files.
// Includes hawk's own paths plus cross-agent standard paths for interoperability.
// Follows the agentskills.io spec and supports gh skill install placement.
func DefaultSkillDirs() []string {
	home := home.Dir()
	if home == "" {
		return []string{".hawk/skills", ".agents/skills"}
	}
	return []string{
		// Project-level directories.
		".hawk/skills",   // hawk project skills
		".agents/skills", // agentskills.io shared dir (gh skill install default)
		".claude/skills", // Claude Code project skills
		".codex/skills",  // Codex project skills
		// User-level directories.
		filepath.Join(home, ".hawk", "skills"),   // hawk global skills
		filepath.Join(home, ".agents", "skills"), // agentskills.io global shared
		filepath.Join(home, ".claude", "skills"), // Claude Code global skills
		filepath.Join(home, ".codex", "skills"),  // Codex global skills
	}
}
