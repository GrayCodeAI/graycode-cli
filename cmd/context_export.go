package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExportContext generates a comprehensive context document about the current project.
// Output is optimized for pasting into any LLM chat.
func ExportContext(dir string, focus string) (string, error) {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("context export: get working dir: %w", err)
		}
	}

	var b strings.Builder

	b.WriteString("# Project Context\n\n")

	// Project type and language
	projectType, language := detectProjectType(dir)
	b.WriteString(fmt.Sprintf("**Type:** %s\n", projectType))
	b.WriteString(fmt.Sprintf("**Language:** %s\n", language))
	b.WriteString(fmt.Sprintf("**Directory:** %s\n\n", dir))

	// Directory structure (top 2 levels)
	b.WriteString("## Directory Structure\n\n```\n")
	tree := dirTree(dir, 2)
	b.WriteString(tree)
	b.WriteString("```\n\n")

	// Key files
	b.WriteString("## Key Files\n\n")
	for _, name := range keyFiles(dir) {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		ext := filepath.Ext(name)
		lang := langFromExt(ext)
		b.WriteString(fmt.Sprintf("### %s\n\n```%s\n%s\n```\n\n", name, lang, strings.TrimSpace(string(content))))
	}

	// Git status
	gitInfo := gitContextInfo(dir)
	if gitInfo != "" {
		b.WriteString("## Git Status\n\n")
		b.WriteString(gitInfo)
		b.WriteString("\n\n")
	}

	// HAWK.md / project instructions
	for _, instrFile := range []string{"HAWK.md", "CLAUDE.md", ".hawk.md"} {
		data, err := os.ReadFile(filepath.Join(dir, instrFile))
		if err == nil && len(data) > 0 {
			b.WriteString(fmt.Sprintf("## Project Instructions (%s)\n\n%s\n\n", instrFile, strings.TrimSpace(string(data))))
			break
		}
	}

	// Focus area files
	if focus != "" {
		b.WriteString(fmt.Sprintf("## Focus Area: %s\n\n", focus))
		focusFiles := findFocusFiles(dir, focus)
		for _, fp := range focusFiles {
			content, err := os.ReadFile(fp)
			if err != nil {
				continue
			}
			rel, _ := filepath.Rel(dir, fp)
			ext := filepath.Ext(fp)
			lang := langFromExt(ext)
			b.WriteString(fmt.Sprintf("### %s\n\n```%s\n%s\n```\n\n", rel, lang, strings.TrimSpace(string(content))))
		}
	}

	return b.String(), nil
}

// ExportContextToFile writes context to a .md file.
func ExportContextToFile(dir, focus, outputPath string) error {
	content, err := ExportContext(dir, focus)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(content), 0o644)
}

// detectProjectType returns the project type and primary language based on
// marker files in the directory.
func detectProjectType(dir string) (string, string) {
	markers := []struct {
		file     string
		projType string
		lang     string
	}{
		{"go.mod", "Go module", "Go"},
		{"Cargo.toml", "Rust crate", "Rust"},
		{"package.json", "Node.js project", "JavaScript/TypeScript"},
		{"pyproject.toml", "Python project", "Python"},
		{"requirements.txt", "Python project", "Python"},
		{"pom.xml", "Maven project", "Java"},
		{"build.gradle", "Gradle project", "Java/Kotlin"},
		{"Gemfile", "Ruby project", "Ruby"},
		{"mix.exs", "Elixir project", "Elixir"},
	}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(dir, m.file)); err == nil {
			return m.projType, m.lang
		}
	}
	return "unknown", "unknown"
}

// dirTree returns a tree-like string of the directory up to maxDepth levels.
func dirTree(dir string, maxDepth int) string {
	var b strings.Builder
	dirTreeRecurse(&b, dir, "", 0, maxDepth)
	return b.String()
}

func dirTreeRecurse(b *strings.Builder, dir, prefix string, depth, maxDepth int) {
	if depth > maxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Filter out hidden directories and common noise
	var visible []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
			continue
		}
		visible = append(visible, e)
	}

	for i, e := range visible {
		isLast := i == len(visible)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}
		b.WriteString(prefix + connector + e.Name())
		if e.IsDir() {
			b.WriteString("/")
		}
		b.WriteString("\n")

		if e.IsDir() {
			childPrefix := prefix + "│   "
			if isLast {
				childPrefix = prefix + "    "
			}
			dirTreeRecurse(b, filepath.Join(dir, e.Name()), childPrefix, depth+1, maxDepth)
		}
	}
}

// keyFiles returns the names of key project files that exist in the directory.
func keyFiles(dir string) []string {
	candidates := []string{
		"README.md", "go.mod", "package.json", "Cargo.toml",
		"pyproject.toml", "main.go", "main.py", "src/main.rs",
		"index.ts", "index.js", "app.py",
	}
	var found []string
	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			found = append(found, name)
		}
	}
	return found
}

// gitContextInfo returns git branch, recent commits, and diff summary.
func gitContextInfo(dir string) string {
	var b strings.Builder

	// Branch
	if out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		b.WriteString(fmt.Sprintf("**Branch:** %s\n", strings.TrimSpace(out)))
	}

	// Recent commits
	if out, err := runGit(dir, "log", "--oneline", "-5"); err == nil && strings.TrimSpace(out) != "" {
		b.WriteString("\n**Recent commits:**\n```\n")
		b.WriteString(strings.TrimSpace(out))
		b.WriteString("\n```\n")
	}

	// Diff summary
	if out, err := runGit(dir, "diff", "--stat"); err == nil && strings.TrimSpace(out) != "" {
		b.WriteString("\n**Current diff:**\n```\n")
		b.WriteString(strings.TrimSpace(out))
		b.WriteString("\n```\n")
	}

	return b.String()
}

// runGit executes a git command in the given directory.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// findFocusFiles finds files matching the focus string in the directory.
func findFocusFiles(dir, focus string) []string {
	var result []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		// Match focus against directory path or file name
		rel, _ := filepath.Rel(dir, path)
		if strings.Contains(rel, focus) {
			result = append(result, path)
		}
		return nil
	})
	// Limit to 20 files to avoid massive output
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}
