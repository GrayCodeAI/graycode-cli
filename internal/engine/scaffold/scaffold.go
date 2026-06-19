package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"
)

// Template defines a project template for scaffolding.
type Template struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Language    string             `json:"language"`
	Framework   string             `json:"framework"`
	Files       []TemplateFile     `json:"files"`
	Variables   []TemplateVariable `json:"variables"`
	PostCreate  []string           `json:"post_create"`
}

// TemplateFile defines a single file within a template.
type TemplateFile struct {
	Path      string      `json:"path"`
	Content   string      `json:"content"`
	Mode      os.FileMode `json:"mode"`
	Condition string      `json:"condition"`
}

// TemplateVariable defines a variable used in template rendering.
type TemplateVariable struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Default     string   `json:"default"`
	Required    bool     `json:"required"`
	Type        string   `json:"type"` // "string", "bool", "choice"
	Choices     []string `json:"choices,omitempty"`
}

// Scaffolder manages templates and generates projects.
type Scaffolder struct {
	Templates   map[string]*Template
	TemplateDir string
	mu          sync.RWMutex
}

// NewScaffolder creates a new Scaffolder with built-in templates.
func NewScaffolder() *Scaffolder {
	s := &Scaffolder{
		Templates: make(map[string]*Template),
	}
	s.registerBuiltins()
	return s
}

// Generate creates a project from a template.
func (s *Scaffolder) Generate(templateName string, vars map[string]string, outputDir string) error {
	s.mu.RLock()
	tmpl, ok := s.Templates[templateName]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("template %q not found", templateName)
	}

	// Apply defaults
	resolvedVars := make(map[string]string)
	for _, v := range tmpl.Variables {
		if val, exists := vars[v.Name]; exists {
			resolvedVars[v.Name] = val
		} else if v.Default != "" {
			resolvedVars[v.Name] = v.Default
		}
	}
	// Also include any extra vars passed in
	for k, v := range vars {
		if _, exists := resolvedVars[k]; !exists {
			resolvedVars[k] = v
		}
	}

	// Add built-in variables
	resolvedVars["Year"] = fmt.Sprintf("%d", time.Now().Year())
	resolvedVars["Date"] = time.Now().Format("2006-01-02")

	for _, f := range tmpl.Files {
		// Evaluate condition
		if f.Condition != "" {
			condResult, err := evalCondition(f.Condition, resolvedVars)
			if err != nil {
				return fmt.Errorf("evaluating condition for %s: %w", f.Path, err)
			}
			if !condResult {
				continue
			}
		}

		// Render path
		renderedPath, err := renderTemplate(f.Path, resolvedVars)
		if err != nil {
			return fmt.Errorf("rendering path %s: %w", f.Path, err)
		}

		// Render content
		renderedContent, err := renderTemplate(f.Content, resolvedVars)
		if err != nil {
			return fmt.Errorf("rendering content for %s: %w", f.Path, err)
		}

		// Create full path
		fullPath := filepath.Join(outputDir, renderedPath)

		// Create directories
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}

		// Determine file mode
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}

		// Write file
		if err := os.WriteFile(fullPath, []byte(renderedContent), mode); err != nil {
			return fmt.Errorf("writing file %s: %w", fullPath, err)
		}
	}

	return nil
}

// ListTemplates returns all registered templates sorted by name.
func (s *Scaffolder) ListTemplates() []*Template {
	s.mu.RLock()
	defer s.mu.RUnlock()

	templates := make([]*Template, 0, len(s.Templates))
	for _, t := range s.Templates {
		templates = append(templates, t)
	}
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Name < templates[j].Name
	})
	return templates
}

// RegisterTemplate adds a new template to the scaffolder.
func (s *Scaffolder) RegisterTemplate(t *Template) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Templates[t.Name] = t
}

// LoadTemplate loads a template from a JSON file.
func (s *Scaffolder) LoadTemplate(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading template file %s: %w", path, err)
	}

	var tmpl Template
	if err := json.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("parsing template file %s: %w", path, err)
	}

	if tmpl.Name == "" {
		return nil, fmt.Errorf("template file %s: name is required", path)
	}

	s.mu.Lock()
	s.Templates[tmpl.Name] = &tmpl
	s.mu.Unlock()

	return &tmpl, nil
}

// ValidateVars checks that required variables are provided and choices are valid.
func (s *Scaffolder) ValidateVars(tmpl *Template, vars map[string]string) []string {
	var errors []string

	for _, v := range tmpl.Variables {
		val, exists := vars[v.Name]
		if v.Required && (!exists || val == "") {
			errors = append(errors, fmt.Sprintf("required variable %q is not provided", v.Name))
			continue
		}

		if v.Type == "choice" && exists && val != "" && len(v.Choices) > 0 {
			found := false
			for _, c := range v.Choices {
				if c == val {
					found = true
					break
				}
			}
			if !found {
				errors = append(errors, fmt.Sprintf("variable %q value %q is not a valid choice (options: %s)", v.Name, val, strings.Join(v.Choices, ", ")))
			}
		}
	}

	return errors
}

// Preview shows what would be created without actually creating files.
func (s *Scaffolder) Preview(templateName string, vars map[string]string) string {
	s.mu.RLock()
	tmpl, ok := s.Templates[templateName]
	s.mu.RUnlock()

	if !ok {
		return fmt.Sprintf("Template %q not found", templateName)
	}

	// Apply defaults
	resolvedVars := make(map[string]string)
	for _, v := range tmpl.Variables {
		if val, exists := vars[v.Name]; exists {
			resolvedVars[v.Name] = val
		} else if v.Default != "" {
			resolvedVars[v.Name] = v.Default
		}
	}
	for k, v := range vars {
		if _, exists := resolvedVars[k]; !exists {
			resolvedVars[k] = v
		}
	}
	resolvedVars["Year"] = fmt.Sprintf("%d", time.Now().Year())
	resolvedVars["Date"] = time.Now().Format("2006-01-02")

	var files []string
	for _, f := range tmpl.Files {
		if f.Condition != "" {
			condResult, err := evalCondition(f.Condition, resolvedVars)
			if err != nil || !condResult {
				continue
			}
		}

		renderedPath, err := renderTemplate(f.Path, resolvedVars)
		if err != nil {
			continue
		}
		files = append(files, renderedPath)
	}

	sort.Strings(files)

	var sb strings.Builder
	sb.WriteString("Would create:\n")
	sb.WriteString(RenderTree(files))

	// Count files and directories
	dirSet := make(map[string]bool)
	for _, f := range files {
		dir := filepath.Dir(f)
		for dir != "." && dir != "" {
			dirSet[dir] = true
			dir = filepath.Dir(dir)
		}
	}
	sb.WriteString(fmt.Sprintf("\n%d files, %d directories\n", len(files), len(dirSet)))

	return sb.String()
}

// RenderTree creates an ASCII tree visualization from a list of file paths.
func RenderTree(files []string) string {
	if len(files) == 0 {
		return ""
	}

	sort.Strings(files)

	// Build tree structure
	type node struct {
		name     string
		children []*node
		isDir    bool
	}

	root := &node{name: "", isDir: true}

	addPath := func(path string) {
		parts := strings.Split(filepath.ToSlash(path), "/")
		current := root
		for i, part := range parts {
			isDir := i < len(parts)-1
			found := false
			for _, child := range current.children {
				if child.name == part {
					current = child
					found = true
					break
				}
			}
			if !found {
				newNode := &node{name: part, isDir: isDir}
				current.children = append(current.children, newNode)
				current = newNode
			}
		}
	}

	for _, f := range files {
		addPath(f)
	}

	var sb strings.Builder
	var renderNode func(n *node, prefix string, isLast bool, isRoot bool)
	renderNode = func(n *node, prefix string, isLast bool, isRoot bool) {
		if !isRoot {
			connector := "├── "
			if isLast {
				connector = "└── "
			}
			name := n.name
			if n.isDir {
				name += "/"
			}
			sb.WriteString(prefix + connector + name + "\n")
		} else {
			if n.name != "" {
				name := n.name
				if n.isDir {
					name += "/"
				}
				sb.WriteString("  " + name + "\n")
			}
		}

		childPrefix := prefix
		if !isRoot {
			if isLast {
				childPrefix += "    "
			} else {
				childPrefix += "│   "
			}
		} else {
			childPrefix = "  "
		}

		for i, child := range n.children {
			isChildLast := i == len(n.children)-1
			renderNode(child, childPrefix, isChildLast, false)
		}
	}

	// If there's a single top-level directory, render from it
	if len(root.children) == 1 && root.children[0].isDir {
		topDir := root.children[0]
		sb.WriteString("  " + topDir.name + "/\n")
		for i, child := range topDir.children {
			isLast := i == len(topDir.children)-1
			renderNode(child, "  ", isLast, false)
		}
	} else {
		for i, child := range root.children {
			isLast := i == len(root.children)-1
			renderNode(child, "", isLast, false)
		}
	}

	return sb.String()
}

// renderTemplate executes a Go template with the given variables.
func renderTemplate(text string, vars map[string]string) (string, error) {
	t, err := template.New("").Option("missingkey=error").Parse(text)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// evalCondition evaluates a template condition string.
// The condition is a Go template that should render to "true" to be truthy.
func evalCondition(condition string, vars map[string]string) (bool, error) {
	result, err := renderTemplate(condition, vars)
	if err != nil {
		return false, err
	}

	result = strings.TrimSpace(strings.ToLower(result))
	return result == "true" || result == "yes" || result == "1", nil
}
