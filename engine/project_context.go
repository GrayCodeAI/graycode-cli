package engine

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectContext loads and manages persistent project knowledge files.
type ProjectContext struct {
	ProjectDir string
	Loaded     map[string]string // filename → content
}

// ProjectContextFiles are the files hawk auto-loads for project context.
var ProjectContextFiles = []string{
	".hawk/project-context.md",
	".hawk/conventions.md",
	".hawk/architecture.md",
	".hawk/debt.md",
}

// NewProjectContext creates a context loader for the given project directory.
func NewProjectContext(projectDir string) *ProjectContext {
	return &ProjectContext{
		ProjectDir: projectDir,
		Loaded:     make(map[string]string),
	}
}

// Load reads all project context files and returns combined content.
func (pc *ProjectContext) Load() string {
	var sections []string
	for _, relPath := range ProjectContextFiles {
		fullPath := filepath.Join(pc.ProjectDir, relPath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		pc.Loaded[relPath] = content
		sections = append(sections, "# ["+relPath+"]\n"+content)
	}
	if len(sections) == 0 {
		return ""
	}
	return strings.Join(sections, "\n\n---\n\n")
}

// HasContext reports whether any project context files exist.
func (pc *ProjectContext) HasContext() bool {
	for _, relPath := range ProjectContextFiles {
		if _, err := os.Stat(filepath.Join(pc.ProjectDir, relPath)); err == nil {
			return true
		}
	}
	return false
}

// InitPrompt returns a prompt for hawk to generate initial project-context.md.
func (pc *ProjectContext) InitPrompt() string {
	return `Analyze this project and generate a .hawk/project-context.md file with:

## Technology Stack & Versions
- List all languages, frameworks, and key dependencies with versions

## Critical Implementation Rules
- Coding conventions (naming, structure, patterns)
- Testing patterns (framework, coverage expectations)
- Architecture decisions (module boundaries, data flow)
- Things that are NOT obvious from reading the code

Focus on what's UNOBVIOUS — things an AI agent might get wrong without this context.
Keep it concise. No generic advice — only project-specific rules.`
}
