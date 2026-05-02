package prompts

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/template"
	"time"
)

//go:embed templates/*.md
var embeddedTemplates embed.FS

// PromptContext holds the variables available to prompt templates.
type PromptContext struct {
	Date          string
	WorkDir       string
	OS            string
	Shell         string
	Model         string
	Provider      string
	GitBranch     string
	GitStatus     string
	RecentCommits string
	TopFiles      string
	MaxTurns      int
	Task          string
}

// mainSections lists the template files assembled into the system prompt, in order.
var mainSections = []string{"role.md", "tools.md", "practices.md", "communication.md"}

// DefaultContext builds a PromptContext from the current environment.
func DefaultContext() PromptContext {
	wd, _ := os.Getwd()
	return PromptContext{
		Date:    time.Now().Format("Monday, 2006-01-02"),
		WorkDir: wd,
		OS:      runtime.GOOS,
		Shell:   os.Getenv("SHELL"),
	}
}

// BuildSystemPrompt assembles the main template sections into a complete system prompt.
// It checks ~/.hawk/prompts/ first for user overrides, then falls back to embedded templates.
func BuildSystemPrompt(ctx PromptContext) (string, error) {
	var sections []string
	for _, name := range mainSections {
		raw, err := LoadTemplate(name)
		if err != nil {
			return "", err
		}
		rendered, err := renderTemplate(name, raw, ctx)
		if err != nil {
			return "", err
		}
		sections = append(sections, strings.TrimSpace(rendered))
	}
	return strings.Join(sections, "\n\n---\n\n"), nil
}

// BuildSubAgentPrompt assembles the sub-agent variant of the system prompt.
func BuildSubAgentPrompt(ctx PromptContext) (string, error) {
	raw, err := LoadTemplate("subagent.md")
	if err != nil {
		return "", err
	}
	return renderTemplate("subagent.md", raw, ctx)
}

// LoadTemplate loads a single template by name.
// It checks ~/.hawk/prompts/<name> first (user overrides), then falls back to embedded.
func LoadTemplate(name string) (string, error) {
	// Check user override directory first
	home, err := os.UserHomeDir()
	if err == nil {
		overridePath := filepath.Join(home, ".hawk", "prompts", name)
		if data, readErr := os.ReadFile(overridePath); readErr == nil {
			return string(data), nil
		}
	}

	// Fall back to embedded templates
	data, err := embeddedTemplates.ReadFile("templates/" + name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ListTemplates returns all available template names from the embedded templates.
func ListTemplates() []string {
	entries, err := embeddedTemplates.ReadDir("templates")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// renderTemplate executes a Go text/template against the given context.
func renderTemplate(name, raw string, ctx PromptContext) (string, error) {
	tmpl, err := template.New(name).Parse(raw)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}
