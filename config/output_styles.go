package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OutputStyle defines a custom output format loaded from a markdown file.
type OutputStyle struct {
	Name             string
	Description      string
	Template         string
	KeepInstructions bool
}

// LoadOutputStyles loads .md files from .hawk/output-styles/ (project-local)
// and ~/.hawk/output-styles/ (global), merging both. Project-local styles
// take priority over global styles with the same name.
func LoadOutputStyles() []OutputStyle {
	var styles []OutputStyle
	seen := make(map[string]bool)

	// Project-local styles first (higher priority)
	cwd, _ := os.Getwd()
	localDir := filepath.Join(cwd, ".hawk", "output-styles")
	for _, s := range loadStylesFromDir(localDir) {
		seen[s.Name] = true
		styles = append(styles, s)
	}

	// Global styles
	home, _ := os.UserHomeDir()
	globalDir := filepath.Join(home, ".hawk", "output-styles")
	for _, s := range loadStylesFromDir(globalDir) {
		if !seen[s.Name] {
			styles = append(styles, s)
		}
	}

	return styles
}

func loadStylesFromDir(dir string) []OutputStyle {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var styles []OutputStyle
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		content := string(data)
		name := strings.TrimSuffix(entry.Name(), ".md")

		style := OutputStyle{
			Name:     name,
			Template: content,
		}

		// Parse front-matter style description from the first line if it
		// starts with "# " (markdown heading).
		lines := strings.SplitN(content, "\n", 2)
		if len(lines) > 0 && strings.HasPrefix(lines[0], "# ") {
			style.Description = strings.TrimPrefix(lines[0], "# ")
			if len(lines) > 1 {
				style.Template = lines[1]
			}
		}

		// Check for keep-instructions marker
		if strings.Contains(content, "<!-- keep-instructions -->") {
			style.KeepInstructions = true
		}

		styles = append(styles, style)
	}

	return styles
}

// ApplyOutputStyle wraps content with the style template. The template may
// contain {{content}} as a placeholder for the actual content.
func ApplyOutputStyle(content string, style OutputStyle) string {
	tmpl := style.Template
	if strings.Contains(tmpl, "{{content}}") {
		return strings.ReplaceAll(tmpl, "{{content}}", content)
	}
	// If no placeholder, prepend the template as instructions.
	return tmpl + "\n\n" + content
}

// OutputStyleModTime returns the latest modification time of any style file,
// useful for cache invalidation.
func OutputStyleModTime() time.Time {
	var latest time.Time

	dirs := []string{}
	cwd, _ := os.Getwd()
	dirs = append(dirs, filepath.Join(cwd, ".hawk", "output-styles"))
	home, _ := os.UserHomeDir()
	dirs = append(dirs, filepath.Join(home, ".hawk", "output-styles"))

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if info, err := entry.Info(); err == nil {
				if info.ModTime().After(latest) {
					latest = info.ModTime()
				}
			}
		}
	}

	return latest
}
