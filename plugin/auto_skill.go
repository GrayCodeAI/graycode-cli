package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectSignal represents a detected project characteristic.
type ProjectSignal struct {
	Category string // language, framework, pattern, tool
	Name     string // e.g. "go", "react", "docker"
}

// AnalyzeProject scans the current directory for project signals.
func AnalyzeProject(dir string) []ProjectSignal {
	var signals []ProjectSignal
	checks := []struct {
		file     string
		category string
		name     string
	}{
		{"go.mod", "language", "go"},
		{"package.json", "language", "javascript"},
		{"tsconfig.json", "language", "typescript"},
		{"Cargo.toml", "language", "rust"},
		{"requirements.txt", "language", "python"},
		{"pyproject.toml", "language", "python"},
		{"Gemfile", "language", "ruby"},
		{"pom.xml", "language", "java"},
		{"build.gradle", "language", "java"},
		{"Dockerfile", "pattern", "docker"},
		{"docker-compose.yml", "pattern", "docker"},
		{"docker-compose.yaml", "pattern", "docker"},
		{".github/workflows", "pattern", "ci-cd"},
		{".gitlab-ci.yml", "pattern", "ci-cd"},
		{"Makefile", "tool", "make"},
		{".eslintrc.json", "tool", "eslint"},
		{".eslintrc.js", "tool", "eslint"},
		{"jest.config.js", "pattern", "testing"},
		{"jest.config.ts", "pattern", "testing"},
		{"vitest.config.ts", "pattern", "testing"},
		{"pytest.ini", "pattern", "testing"},
		{".env", "pattern", "env-config"},
		{".env.example", "pattern", "env-config"},
		{"terraform", "tool", "terraform"},
		{"k8s", "tool", "kubernetes"},
		{"openapi.yaml", "pattern", "api"},
		{"openapi.json", "pattern", "api"},
		{"swagger.yaml", "pattern", "api"},
	}

	seen := map[string]bool{}
	for _, c := range checks {
		p := filepath.Join(dir, c.file)
		if _, err := os.Stat(p); err == nil {
			key := c.category + ":" + c.name
			if !seen[key] {
				signals = append(signals, ProjectSignal{Category: c.category, Name: c.name})
				seen[key] = true
			}
		}
	}

	// Detect frameworks from package.json.
	if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		content := string(data)
		frameworks := map[string]string{
			"react": "react", "next": "nextjs", "vue": "vue",
			"angular": "angular", "svelte": "svelte", "express": "express",
			"fastify": "fastify", "nestjs": "nestjs",
		}
		for dep, name := range frameworks {
			if strings.Contains(content, `"`+dep+`"`) {
				key := "framework:" + name
				if !seen[key] {
					signals = append(signals, ProjectSignal{Category: "framework", Name: name})
					seen[key] = true
				}
			}
		}
	}

	return signals
}

// RecommendSkills matches project signals against the registry index.
func RecommendSkills(signals []ProjectSignal, skills []SkillEntry) []SkillEntry {
	signalSet := map[string]bool{}
	for _, s := range signals {
		signalSet[strings.ToLower(s.Name)] = true
	}

	type scored struct {
		entry SkillEntry
		score int
	}
	var candidates []scored
	for _, skill := range skills {
		score := 0
		// Match tags against signals.
		for _, tag := range skill.Tags {
			if signalSet[strings.ToLower(tag)] {
				score += 2
			}
		}
		// Match category keywords.
		if signalSet[strings.ToLower(skill.Category)] {
			score++
		}
		// Match description words.
		descWords := strings.Fields(strings.ToLower(skill.Description))
		for _, w := range descWords {
			if len(w) > 3 && signalSet[w] {
				score++
			}
		}
		if score > 0 {
			candidates = append(candidates, scored{skill, score})
		}
	}

	// Sort by score descending, then installs.
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score ||
				(candidates[j].score == candidates[i].score && candidates[j].entry.Installs > candidates[i].entry.Installs) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	var result []SkillEntry
	for _, c := range candidates {
		result = append(result, c.entry)
		if len(result) >= 5 {
			break
		}
	}
	return result
}

// RunAutoSkill analyzes the project and installs recommended skills.
func RunAutoSkill(dir string) (string, error) {
	signals := AnalyzeProject(dir)
	if len(signals) == 0 {
		return "No project signals detected. Skipping auto-skill.", nil
	}

	var sigNames []string
	for _, s := range signals {
		sigNames = append(sigNames, s.Name)
	}

	rc := NewRegistryClient()
	idx, err := rc.FetchIndex()
	if err != nil {
		return fmt.Sprintf("Detected: %s\nRegistry unavailable: %v", strings.Join(sigNames, ", "), err), nil
	}

	recommended := RecommendSkills(signals, idx.Skills)
	if len(recommended) == 0 {
		return fmt.Sprintf("Detected: %s\nNo matching skills found in registry.", strings.Join(sigNames, ", ")), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Detected: %s\n", strings.Join(sigNames, ", "))

	installed := 0
	for _, skill := range recommended {
		msg, err := rc.Install(skill.Repo, skill.Name, "project")
		if err != nil {
			fmt.Fprintf(&b, "  ✗ %s — %v\n", skill.Name, err)
			continue
		}
		_ = msg
		fmt.Fprintf(&b, "  ✓ %s — %s\n", skill.Name, skill.Description)
		installed++
	}

	if installed > 0 {
		fmt.Fprintf(&b, "\nInstalled %d skill(s) to .hawk/skills/", installed)
	}
	return b.String(), nil
}
