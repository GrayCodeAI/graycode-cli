package plugin

import (
	"os"
	"path/filepath"
	"strings"
)

// Skill represents a project-local skill loaded from a markdown file
// with YAML front-matter metadata (name, description, invocation) and body content.
type Skill struct {
	Name         string
	Description  string
	Content      string
	Path         string
	Provider     string
	ResourceBase string
	Invocation   SkillInvocationPolicy
}

// LoadSkillsFromDir reads all .md files from a directory,
// parsing YAML front-matter (name, description) and body content.
// Returns an empty slice (not an error) if the directory does not exist.
func LoadSkillsFromDir(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(filePath) // #nosec G304 -- path is built from a caller-specified skills directory and an enumerated entry name, not raw external input
		if err != nil {
			continue
		}

		s, ok := parseSkillFrontMatter(string(data))
		if !ok {
			continue
		}
		s.Path = filePath
		s.ResourceBase = dir
		skills = append(skills, s)
	}
	return skills, nil
}

// parseSkillFrontMatter extracts a Skill from a markdown file with YAML front matter.
// Returns ok=false if the front matter is missing or lacks a name field.
func parseSkillFrontMatter(raw string) (Skill, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "---") {
		return Skill{}, false
	}

	// Find closing "---"
	rest := raw[3:]
	rest = strings.TrimLeft(rest, "\r\n")
	idx := strings.Index(rest, "---")
	if idx < 0 {
		return Skill{}, false
	}

	frontMatter := rest[:idx]
	content := strings.TrimSpace(rest[idx+3:])

	var s Skill
	var (
		modelInvocable *bool
		userInvocable  *bool
	)

	for _, line := range strings.Split(frontMatter, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, ":"); ok {
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.TrimSpace(v)
			switch k {
			case "name":
				s.Name = v
			case "description":
				s.Description = v
			case "model_invocable", "model-invocable":
				b := strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
				modelInvocable = &b
			case "user_invocable", "user-invocable":
				b := strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
				userInvocable = &b
			case "invocable":
				vLower := strings.ToLower(v)
				m := strings.Contains(vLower, "model") || strings.Contains(vLower, "all")
				u := strings.Contains(vLower, "user") || strings.Contains(vLower, "all") || strings.Contains(vLower, "human")
				if strings.EqualFold(vLower, "none") || strings.EqualFold(vLower, "false") {
					m = false
					u = false
				}
				modelInvocable = &m
				userInvocable = &u
			}
		}
	}

	if s.Name == "" {
		return Skill{}, false
	}

	s.Invocation = SkillInvocationPolicy{
		ModelInvocable: modelInvocable,
		UserInvocable:  userInvocable,
	}
	s.Content = content
	return s, true
}
