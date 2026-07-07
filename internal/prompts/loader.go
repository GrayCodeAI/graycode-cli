package prompts

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
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
var mainSections = []string{"role.md", "execution.md", "tools.md", "practices.md", "examples.md", "communication.md"}

var (
	embeddedTemplateMu    sync.RWMutex
	embeddedTemplateCache = make(map[string]*template.Template)
)

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
// It checks Hawk user config first for user overrides, then falls back to embedded templates.
func BuildSystemPrompt(ctx PromptContext) (string, error) {
	var sections []string
	for _, name := range mainSections {
		tmpl, err := loadTemplateForRender(name)
		if err != nil {
			return "", err
		}
		rendered, err := renderTemplate(name, tmpl, ctx)
		if err != nil {
			return "", err
		}
		sections = append(sections, strings.TrimSpace(rendered))
	}
	return strings.Join(sections, "\n\n---\n\n"), nil
}

// BuildSubAgentPrompt assembles the sub-agent variant of the system prompt.
func BuildSubAgentPrompt(ctx PromptContext) (string, error) {
	tmpl, err := loadTemplateForRender("subagent.md")
	if err != nil {
		return "", err
	}
	return renderTemplate("subagent.md", tmpl, ctx)
}

// LoadTemplate loads a single template by name.
// It checks Hawk user config prompts first, then falls back to embedded.
func LoadTemplate(name string) (string, error) {
	return loadTemplateSource(name)
}

func loadTemplateSource(name string) (string, error) {
	overridePath := filepath.Join(storage.ConfigDir(), "prompts", name)
	// #nosec G304 -- name is a fixed internal template identifier, not external input
	if data, readErr := os.ReadFile(overridePath); readErr == nil {
		return string(data), nil
	}

	// Fall back to embedded templates
	data, err := embeddedTemplates.ReadFile("templates/" + name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func loadTemplateForRender(name string) (*template.Template, error) {
	overridePath := filepath.Join(storage.ConfigDir(), "prompts", name)
	// #nosec G304 -- name is a fixed internal template identifier, not external input
	if data, readErr := os.ReadFile(overridePath); readErr == nil {
		return template.New(name).Parse(string(data))
	}
	return cachedEmbeddedTemplate(name)
}

func cachedEmbeddedTemplate(name string) (*template.Template, error) {
	embeddedTemplateMu.RLock()
	tmpl := embeddedTemplateCache[name]
	embeddedTemplateMu.RUnlock()
	if tmpl != nil {
		return tmpl, nil
	}
	data, err := embeddedTemplates.ReadFile("templates/" + name)
	if err != nil {
		return nil, err
	}
	parsed, err := template.New(name).Parse(string(data))
	if err != nil {
		return nil, err
	}
	embeddedTemplateMu.Lock()
	if existing := embeddedTemplateCache[name]; existing != nil {
		parsed = existing
	} else {
		embeddedTemplateCache[name] = parsed
	}
	embeddedTemplateMu.Unlock()
	return parsed, nil
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
func renderTemplate(name string, tmpl *template.Template, ctx PromptContext) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}
