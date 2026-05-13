package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ConfigTemplate defines a project configuration template that generates
// hawk-specific files based on detected project characteristics.
type ConfigTemplate struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Language    string                 `json:"language"`
	Framework   string                 `json:"framework"`
	Content     map[string]interface{} `json:"content"`
	Files       map[string]string      `json:"files"` // additional files to generate
	Tags        []string               `json:"tags"`
}

// TemplateRegistry manages configuration templates.
type TemplateRegistry struct {
	Templates map[string]*ConfigTemplate
	mu        sync.RWMutex
}

// NewTemplateRegistry creates a registry pre-loaded with built-in templates.
func NewTemplateRegistry() *TemplateRegistry {
	r := &TemplateRegistry{
		Templates: make(map[string]*ConfigTemplate),
	}

	// go-default: standard Go project config
	r.Templates["go-default"] = &ConfigTemplate{
		Name:        "go-default",
		Description: "Standard Go project configuration",
		Language:    "go",
		Framework:   "",
		Content: map[string]interface{}{
			"model":    "sonnet",
			"sandbox":  "workspace",
			"auto_allow": []string{"bash(go build .)", "bash(go test ./...)", "bash(go vet ./...)"},
			"repo_map": true,
		},
		Files: map[string]string{
			".hawk/settings.json": `{
  "model": "{{model}}",
  "sandbox": "workspace",
  "auto_allow": ["bash(go build .)", "bash(go test ./...)", "bash(go vet ./...)"],
  "repo_map": true
}`,
			"AGENTS.md": `# {{project_name}}

## Language
Go

## Build
` + "```bash" + `
go build .
go test -race ./...
` + "```" + `

## Guidelines
- Use standard library where possible
- Run go vet before committing
- All exported functions must have doc comments
`,
			".hawk/rules/go.md": `---
paths: ["*.go"]
---
# Go Rules

- Use gofmt formatting
- Handle all errors explicitly
- Prefer table-driven tests
- No init() functions unless absolutely necessary
`,
		},
		Tags: []string{"go", "backend", "cli"},
	}

	// go-api: API project with chi/gin settings
	r.Templates["go-api"] = &ConfigTemplate{
		Name:        "go-api",
		Description: "Go API project with HTTP framework support",
		Language:    "go",
		Framework:   "chi/gin",
		Content: map[string]interface{}{
			"model":    "sonnet",
			"sandbox":  "workspace",
			"auto_allow": []string{"bash(go build .)", "bash(go test ./...)", "bash(curl localhost:*)"},
			"repo_map": true,
		},
		Files: map[string]string{
			".hawk/settings.json": `{
  "model": "{{model}}",
  "sandbox": "workspace",
  "auto_allow": ["bash(go build .)", "bash(go test ./...)", "bash(curl localhost:*)"],
  "repo_map": true
}`,
			"AGENTS.md": `# {{project_name}}

## Language
Go (API)

## Build
` + "```bash" + `
go build .
go test -race ./...
` + "```" + `

## Architecture
- HTTP API using {{framework}}
- Follow REST conventions
- Use middleware for auth, logging, recovery

## Guidelines
- All endpoints must have request/response structs
- Use context for cancellation
- Validate input at handler level
- Return structured JSON errors
`,
			".hawk/rules/api.md": `---
paths: ["**/handler*.go", "**/route*.go", "**/middleware*.go"]
---
# API Rules

- Every handler must validate input
- Use proper HTTP status codes
- Log errors with request context
- Never expose internal errors to clients
`,
		},
		Tags: []string{"go", "api", "http", "backend"},
	}

	// ts-react: React/Next.js project
	r.Templates["ts-react"] = &ConfigTemplate{
		Name:        "ts-react",
		Description: "React/Next.js TypeScript project",
		Language:    "typescript",
		Framework:   "react",
		Content: map[string]interface{}{
			"model":    "sonnet",
			"sandbox":  "workspace",
			"auto_allow": []string{"bash(npm run build)", "bash(npm test)", "bash(npx tsc --noEmit)"},
			"repo_map": true,
		},
		Files: map[string]string{
			".hawk/settings.json": `{
  "model": "{{model}}",
  "sandbox": "workspace",
  "auto_allow": ["bash(npm run build)", "bash(npm test)", "bash(npx tsc --noEmit)"],
  "repo_map": true
}`,
			"AGENTS.md": `# {{project_name}}

## Language
TypeScript (React)

## Build
` + "```bash" + `
npm install
npm run build
npm test
` + "```" + `

## Architecture
- React components in src/components/
- Pages/routes in src/pages/ or app/
- State management: {{state_management}}

## Guidelines
- Use functional components with hooks
- Prefer composition over inheritance
- Keep components small and focused
- Use TypeScript strict mode
`,
			".hawk/rules/react.md": `---
paths: ["src/**/*.tsx", "src/**/*.ts", "app/**/*.tsx"]
---
# React Rules

- Components must be typed with explicit props interface
- Use named exports for components
- Keep side effects in useEffect
- Memoize expensive computations with useMemo
`,
		},
		Tags: []string{"typescript", "react", "frontend", "nextjs"},
	}

	// ts-api: Express/Fastify API
	r.Templates["ts-api"] = &ConfigTemplate{
		Name:        "ts-api",
		Description: "TypeScript API with Express or Fastify",
		Language:    "typescript",
		Framework:   "express/fastify",
		Content: map[string]interface{}{
			"model":    "sonnet",
			"sandbox":  "workspace",
			"auto_allow": []string{"bash(npm run build)", "bash(npm test)", "bash(npx tsc --noEmit)"},
			"repo_map": true,
		},
		Files: map[string]string{
			".hawk/settings.json": `{
  "model": "{{model}}",
  "sandbox": "workspace",
  "auto_allow": ["bash(npm run build)", "bash(npm test)", "bash(npx tsc --noEmit)"],
  "repo_map": true
}`,
			"AGENTS.md": `# {{project_name}}

## Language
TypeScript (API)

## Build
` + "```bash" + `
npm install
npm run build
npm test
` + "```" + `

## Architecture
- Routes in src/routes/
- Controllers in src/controllers/
- Services in src/services/
- Middleware in src/middleware/

## Guidelines
- Validate all request inputs with schemas
- Use async/await consistently
- Structure errors with status codes
- Write integration tests for routes
`,
			".hawk/rules/api.md": `---
paths: ["src/**/*.ts"]
---
# TypeScript API Rules

- All route handlers must have input validation
- Use dependency injection for services
- Never throw untyped errors
- Log with structured metadata
`,
		},
		Tags: []string{"typescript", "api", "express", "fastify", "backend"},
	}

	// python-api: FastAPI/Django project
	r.Templates["python-api"] = &ConfigTemplate{
		Name:        "python-api",
		Description: "Python API with FastAPI or Django",
		Language:    "python",
		Framework:   "fastapi/django",
		Content: map[string]interface{}{
			"model":    "sonnet",
			"sandbox":  "workspace",
			"auto_allow": []string{"bash(python -m pytest)", "bash(python -m mypy .)", "bash(ruff check .)"},
			"repo_map": true,
		},
		Files: map[string]string{
			".hawk/settings.json": `{
  "model": "{{model}}",
  "sandbox": "workspace",
  "auto_allow": ["bash(python -m pytest)", "bash(python -m mypy .)", "bash(ruff check .)"],
  "repo_map": true
}`,
			"AGENTS.md": `# {{project_name}}

## Language
Python (API)

## Build
` + "```bash" + `
pip install -e ".[dev]"
python -m pytest
python -m mypy .
` + "```" + `

## Architecture
- API framework: {{framework}}
- Follow project layout conventions
- Use type hints everywhere

## Guidelines
- Type annotations on all functions
- Use pydantic models for request/response
- Write docstrings for public functions
- Keep business logic out of route handlers
`,
			".hawk/rules/python.md": `---
paths: ["**/*.py"]
---
# Python Rules

- All functions must have type annotations
- Use pydantic for data validation
- Prefer explicit imports over star imports
- Follow PEP 8 naming conventions
`,
		},
		Tags: []string{"python", "api", "fastapi", "django", "backend"},
	}

	// python-ml: ML/data science project
	r.Templates["python-ml"] = &ConfigTemplate{
		Name:        "python-ml",
		Description: "Python ML/data science project",
		Language:    "python",
		Framework:   "pytorch/sklearn",
		Content: map[string]interface{}{
			"model":    "sonnet",
			"sandbox":  "workspace",
			"auto_allow": []string{"bash(python -m pytest)", "bash(python -m mypy .)", "bash(jupyter nbconvert --execute *)"},
			"repo_map": true,
		},
		Files: map[string]string{
			".hawk/settings.json": `{
  "model": "{{model}}",
  "sandbox": "workspace",
  "auto_allow": ["bash(python -m pytest)", "bash(python -m mypy .)"],
  "repo_map": true
}`,
			"AGENTS.md": `# {{project_name}}

## Language
Python (ML/Data Science)

## Build
` + "```bash" + `
pip install -e ".[dev]"
python -m pytest
` + "```" + `

## Architecture
- Models in models/
- Data processing in data/
- Training scripts in scripts/
- Notebooks in notebooks/

## Guidelines
- Document model architectures clearly
- Use reproducible random seeds
- Track experiments with version info
- Keep data loading separate from model code
- Use numpy/pandas type stubs
`,
			".hawk/rules/ml.md": `---
paths: ["**/*.py", "notebooks/**/*.ipynb"]
---
# ML Rules

- Set random seeds for reproducibility
- Document tensor shapes in comments
- Keep training and evaluation separate
- Log metrics systematically
- Never commit large data files
`,
		},
		Tags: []string{"python", "ml", "data-science", "pytorch", "sklearn"},
	}

	// rust-cli: Rust CLI project
	r.Templates["rust-cli"] = &ConfigTemplate{
		Name:        "rust-cli",
		Description: "Rust CLI application",
		Language:    "rust",
		Framework:   "clap",
		Content: map[string]interface{}{
			"model":    "sonnet",
			"sandbox":  "workspace",
			"auto_allow": []string{"bash(cargo build)", "bash(cargo test)", "bash(cargo clippy)"},
			"repo_map": true,
		},
		Files: map[string]string{
			".hawk/settings.json": `{
  "model": "{{model}}",
  "sandbox": "workspace",
  "auto_allow": ["bash(cargo build)", "bash(cargo test)", "bash(cargo clippy)"],
  "repo_map": true
}`,
			"AGENTS.md": `# {{project_name}}

## Language
Rust (CLI)

## Build
` + "```bash" + `
cargo build
cargo test
cargo clippy -- -D warnings
` + "```" + `

## Architecture
- CLI args with clap in src/main.rs or src/cli.rs
- Business logic in src/lib.rs
- Error types in src/error.rs

## Guidelines
- Use thiserror for error types
- Prefer Result over unwrap/expect
- Write doc comments on public items
- Use clippy with all warnings as errors
`,
			".hawk/rules/rust.md": `---
paths: ["**/*.rs"]
---
# Rust Rules

- No unwrap() in library code
- Use proper error enums with thiserror
- All public items need doc comments
- Keep unsafe blocks minimal and documented
`,
		},
		Tags: []string{"rust", "cli", "systems"},
	}

	// monorepo: Multi-package monorepo
	r.Templates["monorepo"] = &ConfigTemplate{
		Name:        "monorepo",
		Description: "Multi-package monorepo configuration",
		Language:    "multi",
		Framework:   "workspace",
		Content: map[string]interface{}{
			"model":    "sonnet",
			"sandbox":  "workspace",
			"auto_allow": []string{"bash(make build)", "bash(make test)", "bash(make lint)"},
			"repo_map": true,
			"repo_map_max_tokens": 8000,
		},
		Files: map[string]string{
			".hawk/settings.json": `{
  "model": "{{model}}",
  "sandbox": "workspace",
  "auto_allow": ["bash(make build)", "bash(make test)", "bash(make lint)"],
  "repo_map": true,
  "repo_map_max_tokens": 8000
}`,
			"AGENTS.md": `# {{project_name}}

## Structure
Monorepo with multiple packages

## Build
` + "```bash" + `
make build
make test
make lint
` + "```" + `

## Packages
{{packages}}

## Guidelines
- Each package should be independently testable
- Shared code goes in packages/shared or libs/
- Use workspace-level tooling for consistency
- Cross-package changes need integration tests
`,
			".hawk/rules/monorepo.md": `# Monorepo Rules

- Changes spanning multiple packages need integration tests
- Respect package boundaries
- Shared dependencies must be version-aligned
- Each package maintains its own README
`,
		},
		Tags: []string{"monorepo", "workspace", "multi-language"},
	}

	return r
}

// Register adds a new template to the registry.
func (r *TemplateRegistry) Register(template *ConfigTemplate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Templates[template.Name] = template
}

// List returns all registered templates sorted by name.
func (r *TemplateRegistry) List() []*ConfigTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	templates := make([]*ConfigTemplate, 0, len(r.Templates))
	for _, t := range r.Templates {
		templates = append(templates, t)
	}
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Name < templates[j].Name
	})
	return templates
}

// Generate creates all config files from the named template with variable substitution.
func (r *TemplateRegistry) Generate(templateName string, vars map[string]string) (map[string]string, error) {
	r.mu.RLock()
	tmpl, ok := r.Templates[templateName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("template %q not found", templateName)
	}

	result := make(map[string]string)
	for path, content := range tmpl.Files {
		resolved := applyVars(content, vars)
		resolvedPath := applyVars(path, vars)
		result[resolvedPath] = resolved
	}
	return result, nil
}

// DetectAndGenerate auto-detects the project type from projectDir,
// selects the best matching template, and generates configs.
func (r *TemplateRegistry) DetectAndGenerate(projectDir string) (map[string]string, error) {
	lang, framework := detectProject(projectDir)

	templateName := selectTemplate(lang, framework)
	if templateName == "" {
		return nil, fmt.Errorf("could not detect project type in %q", projectDir)
	}

	projectName := filepath.Base(projectDir)
	vars := map[string]string{
		"project_name":     projectName,
		"model":            "sonnet",
		"framework":        framework,
		"state_management": "context",
		"packages":         detectPackages(projectDir),
	}

	return r.Generate(templateName, vars)
}

// GenerateHawkConfig generates .hawk/settings.json content from a template.
func GenerateHawkConfig(template *ConfigTemplate, vars map[string]string) string {
	content, ok := template.Files[".hawk/settings.json"]
	if !ok {
		// Fallback: generate from Content map
		data, err := json.MarshalIndent(template.Content, "", "  ")
		if err != nil {
			return "{}"
		}
		return applyVars(string(data), vars)
	}
	return applyVars(content, vars)
}

// GenerateAgentsmd generates AGENTS.md content from a template.
func GenerateAgentsmd(template *ConfigTemplate, vars map[string]string) string {
	content, ok := template.Files["AGENTS.md"]
	if !ok {
		return fmt.Sprintf("# %s\n\nProject using %s.\n", vars["project_name"], template.Language)
	}
	return applyVars(content, vars)
}

// GenerateRules generates .hawk/rules permission file content from a template.
func GenerateRules(template *ConfigTemplate) string {
	var sb strings.Builder
	for path, content := range template.Files {
		if strings.HasPrefix(path, ".hawk/rules/") {
			if sb.Len() > 0 {
				sb.WriteString("\n---\n\n")
			}
			sb.WriteString(fmt.Sprintf("# File: %s\n\n", path))
			sb.WriteString(content)
		}
	}
	if sb.Len() == 0 {
		return fmt.Sprintf("# Rules for %s project\n\n- Follow %s best practices\n", template.Name, template.Language)
	}
	return sb.String()
}

// FormatTemplate returns a human-readable summary of a template.
func FormatTemplate(template *ConfigTemplate) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Template: %s\n", template.Name))
	sb.WriteString(fmt.Sprintf("Description: %s\n", template.Description))
	sb.WriteString(fmt.Sprintf("Language: %s\n", template.Language))
	if template.Framework != "" {
		sb.WriteString(fmt.Sprintf("Framework: %s\n", template.Framework))
	}
	if len(template.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(template.Tags, ", ")))
	}
	sb.WriteString(fmt.Sprintf("Files generated: %d\n", len(template.Files)))
	for path := range template.Files {
		sb.WriteString(fmt.Sprintf("  - %s\n", path))
	}
	return sb.String()
}

// Preview shows what files would be generated without writing them.
func (r *TemplateRegistry) Preview(templateName string, vars map[string]string) string {
	files, err := r.Generate(templateName, vars)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Preview for template %q:\n\n", templateName))

	// Sort file paths for deterministic output
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, path := range paths {
		content := files[path]
		sb.WriteString(fmt.Sprintf("--- %s ---\n", path))
		sb.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// applyVars substitutes {{key}} placeholders in text with values from vars.
func applyVars(text string, vars map[string]string) string {
	result := text
	for key, value := range vars {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// detectProject inspects projectDir for language/framework indicators.
func detectProject(projectDir string) (language, framework string) {
	// Go detection
	if fileExists(filepath.Join(projectDir, "go.mod")) {
		language = "go"
		// Check for API frameworks
		if containsInFile(filepath.Join(projectDir, "go.mod"), "chi") ||
			containsInFile(filepath.Join(projectDir, "go.mod"), "gin-gonic") ||
			containsInFile(filepath.Join(projectDir, "go.mod"), "echo") {
			framework = "api"
		}
		return
	}

	// Rust detection
	if fileExists(filepath.Join(projectDir, "Cargo.toml")) {
		language = "rust"
		framework = "cli"
		return
	}

	// TypeScript/JavaScript detection
	if fileExists(filepath.Join(projectDir, "package.json")) {
		language = "typescript"
		pkgData, err := os.ReadFile(filepath.Join(projectDir, "package.json"))
		if err == nil {
			pkgStr := string(pkgData)
			if strings.Contains(pkgStr, "react") || strings.Contains(pkgStr, "next") {
				framework = "react"
			} else if strings.Contains(pkgStr, "express") || strings.Contains(pkgStr, "fastify") {
				framework = "api"
			}
		}
		return
	}

	// Python detection
	if fileExists(filepath.Join(projectDir, "pyproject.toml")) ||
		fileExists(filepath.Join(projectDir, "setup.py")) ||
		fileExists(filepath.Join(projectDir, "requirements.txt")) {
		language = "python"
		// Check for ML libraries
		for _, f := range []string{"pyproject.toml", "requirements.txt", "setup.py"} {
			path := filepath.Join(projectDir, f)
			if containsInFile(path, "torch") || containsInFile(path, "tensorflow") ||
				containsInFile(path, "sklearn") || containsInFile(path, "pandas") {
				framework = "ml"
				return
			}
			if containsInFile(path, "fastapi") || containsInFile(path, "django") ||
				containsInFile(path, "flask") {
				framework = "api"
				return
			}
		}
		return
	}

	// Monorepo detection
	if fileExists(filepath.Join(projectDir, "pnpm-workspace.yaml")) ||
		fileExists(filepath.Join(projectDir, "lerna.json")) ||
		dirExists(filepath.Join(projectDir, "packages")) {
		language = "multi"
		framework = "monorepo"
		return
	}

	return "", ""
}

// selectTemplate maps detected language/framework to a template name.
func selectTemplate(language, framework string) string {
	switch language {
	case "go":
		if framework == "api" {
			return "go-api"
		}
		return "go-default"
	case "typescript":
		if framework == "react" {
			return "ts-react"
		}
		if framework == "api" {
			return "ts-api"
		}
		return "ts-react" // default for TS
	case "python":
		if framework == "ml" {
			return "python-ml"
		}
		if framework == "api" {
			return "python-api"
		}
		return "python-api" // default for python
	case "rust":
		return "rust-cli"
	case "multi":
		return "monorepo"
	}
	return ""
}

// detectPackages lists subdirectories in packages/ or apps/ for monorepos.
func detectPackages(projectDir string) string {
	var pkgs []string
	for _, dir := range []string{"packages", "apps", "libs"} {
		entries, err := os.ReadDir(filepath.Join(projectDir, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				pkgs = append(pkgs, fmt.Sprintf("- %s/%s", dir, e.Name()))
			}
		}
	}
	if len(pkgs) == 0 {
		return "- (auto-detect packages)"
	}
	return strings.Join(pkgs, "\n")
}

// fileExists checks if a file exists at path.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dirExists checks if a directory exists at path.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// containsInFile checks if a file contains the given substring.
func containsInFile(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
}
