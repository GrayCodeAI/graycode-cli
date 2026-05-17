package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// PromptTemplate is a reusable prompt template.
type PromptTemplate struct {
	Name     string   `json:"name"`
	Template string   `json:"template"`
	Args     []string `json:"args,omitempty"`
}

// LoadTemplates loads prompt templates from ~/.hawk/templates/.
func LoadTemplates() []PromptTemplate {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	dir := filepath.Join(home, ".hawk", "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var templates []PromptTemplate
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		path := filepath.Join(dir, e.Name())

		switch ext {
		case ".json":
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var t PromptTemplate
			if json.Unmarshal(data, &t) == nil && t.Name != "" {
				templates = append(templates, t)
			}
		case ".txt", ".md":
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ext)
			templates = append(templates, PromptTemplate{
				Name:     name,
				Template: string(data),
			})
		}
	}
	return templates
}

// Apply fills template args using {{key}} placeholders.
func (t *PromptTemplate) Apply(args map[string]string) string {
	result := t.Template
	for key, value := range args {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}
