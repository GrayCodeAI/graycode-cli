package engine

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

func (s *Scaffolder) registerBuiltins() {
	s.Templates["go-cli"] = &Template{
		Name:        "go-cli",
		Description: "Go CLI application with Cobra",
		Language:    "go",
		Framework:   "cobra",
		Variables: []TemplateVariable{
			{Name: "ProjectName", Description: "Name of the project", Required: true, Type: "string"},
			{Name: "Module", Description: "Go module path", Required: true, Type: "string"},
			{Name: "Author", Description: "Author name", Default: "Developer", Type: "string"},
			{Name: "License", Description: "License type", Default: "MIT", Type: "choice", Choices: []string{"MIT", "Apache-2.0", "BSD-3-Clause"}},
		},
		Files: []TemplateFile{
			{
				Path: "{{.ProjectName}}/cmd/main.go",
				Content: `package main

import (
	"fmt"
	"os"

	"{{.Module}}/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/internal/cmd/root.go",
				Content: `package cmd

import (
	"fmt"
	"os"
)

// Execute runs the root command.
func Execute() error {
	if len(os.Args) < 2 {
		fmt.Println("{{.ProjectName}} - A CLI application")
		fmt.Println("Usage: {{.ProjectName}} <command>")
		return nil
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("{{.ProjectName}} v0.1.0")
	default:
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}
	return nil
}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/go.mod",
				Content: `module {{.Module}}

go 1.21
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/Makefile",
				Content: `.PHONY: build test clean

BINARY={{.ProjectName}}

build:
	go build -o bin/$(BINARY) ./cmd/main.go

test:
	go test ./...

clean:
	rm -rf bin/

lint:
	golangci-lint run ./...
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/.gitignore",
				Content: `bin/
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
vendor/
.idea/
.vscode/
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/README.md",
				Content: `# {{.ProjectName}}

{{.ProjectName}} is a CLI application.

## Installation

` + "```bash" + `
go install {{.Module}}/cmd@latest
` + "```" + `

## Usage

` + "```bash" + `
{{.ProjectName}} version
` + "```" + `

## Author

{{.Author}}

## License

{{.License}}
`,
				Mode: 0644,
			},
		},
		PostCreate: []string{"cd {{.ProjectName}} && go mod tidy"},
	}

	s.Templates["go-api"] = &Template{
		Name:        "go-api",
		Description: "Go REST API with net/http",
		Language:    "go",
		Framework:   "net/http",
		Variables: []TemplateVariable{
			{Name: "ProjectName", Description: "Name of the project", Required: true, Type: "string"},
			{Name: "Module", Description: "Go module path", Required: true, Type: "string"},
			{Name: "Port", Description: "Server port", Default: "8080", Type: "string"},
			{Name: "WithDocker", Description: "Include Dockerfile", Default: "true", Type: "bool"},
		},
		Files: []TemplateFile{
			{
				Path: "{{.ProjectName}}/cmd/server/main.go",
				Content: `package main

import (
	"fmt"
	"log"
	"net/http"

	"{{.Module}}/internal/handler"
	"{{.Module}}/internal/middleware"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handler.Health)
	mux.HandleFunc("GET /api/v1/items", handler.ListItems)
	mux.HandleFunc("POST /api/v1/items", handler.CreateItem)

	wrapped := middleware.Logger(middleware.Recovery(mux))

	addr := ":{{.Port}}"
	fmt.Printf("Server starting on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, wrapped))
}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/internal/handler/handler.go",
				Content: `package handler

import (
	"encoding/json"
	"net/http"
)

// Health returns service health status.
func Health(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ListItems returns all items.
func ListItems(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]string{})
}

// CreateItem creates a new item.
func CreateItem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/internal/middleware/middleware.go",
				Content: `package middleware

import (
	"log"
	"net/http"
	"time"
)

// Logger logs incoming requests.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// Recovery recovers from panics.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic recovered: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/go.mod",
				Content: `module {{.Module}}

go 1.21
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/Dockerfile",
				Content: `FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE {{.Port}}
CMD ["./server"]
`,
				Mode: 0644,
				Condition: "{{.WithDocker}}",
			},
			{
				Path: "{{.ProjectName}}/.gitignore",
				Content: `bin/
*.exe
vendor/
.env
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/README.md",
				Content: `# {{.ProjectName}}

A REST API service.

## Running

` + "```bash" + `
go run ./cmd/server/main.go
` + "```" + `

## Endpoints

- GET /health
- GET /api/v1/items
- POST /api/v1/items
`,
				Mode: 0644,
			},
		},
		PostCreate: []string{"cd {{.ProjectName}} && go mod tidy"},
	}

	s.Templates["go-lib"] = &Template{
		Name:        "go-lib",
		Description: "Go library package",
		Language:    "go",
		Framework:   "stdlib",
		Variables: []TemplateVariable{
			{Name: "ProjectName", Description: "Name of the library", Required: true, Type: "string"},
			{Name: "Module", Description: "Go module path", Required: true, Type: "string"},
			{Name: "PackageName", Description: "Go package name", Required: true, Type: "string"},
			{Name: "WithCI", Description: "Include GitHub Actions CI", Default: "true", Type: "bool"},
		},
		Files: []TemplateFile{
			{
				Path: "{{.ProjectName}}/{{.PackageName}}.go",
				Content: `// Package {{.PackageName}} provides ...
package {{.PackageName}}

// Version is the library version.
const Version = "0.1.0"
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/{{.PackageName}}_test.go",
				Content: `package {{.PackageName}}

import "testing"

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/example_test.go",
				Content: `package {{.PackageName}}_test

import (
	"fmt"

	"{{.Module}}"
)

func Example() {
	fmt.Println({{.PackageName}}.Version)
	// Output: 0.1.0
}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/go.mod",
				Content: `module {{.Module}}

go 1.21
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/.github/workflows/ci.yml",
				Content: `name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: go test ./...
      - run: go vet ./...
`,
				Mode: 0644,
				Condition: "{{.WithCI}}",
			},
			{
				Path: "{{.ProjectName}}/README.md",
				Content: `# {{.ProjectName}}

` + "```go" + `
import "{{.Module}}"
` + "```" + `

## Installation

` + "```bash" + `
go get {{.Module}}
` + "```" + `
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/.gitignore",
				Content: `vendor/
*.test
`,
				Mode: 0644,
			},
		},
		PostCreate: []string{"cd {{.ProjectName}} && go mod tidy"},
	}

	s.Templates["ts-api"] = &Template{
		Name:        "ts-api",
		Description: "TypeScript API with Express",
		Language:    "typescript",
		Framework:   "express",
		Variables: []TemplateVariable{
			{Name: "ProjectName", Description: "Name of the project", Required: true, Type: "string"},
			{Name: "Port", Description: "Server port", Default: "3000", Type: "string"},
			{Name: "WithDocker", Description: "Include Dockerfile", Default: "true", Type: "bool"},
		},
		Files: []TemplateFile{
			{
				Path: "{{.ProjectName}}/src/index.ts",
				Content: `import express from 'express';

const app = express();
const port = process.env.PORT || {{.Port}};

app.use(express.json());

app.get('/health', (req, res) => {
  res.json({ status: 'ok' });
});

app.get('/api/v1/items', (req, res) => {
  res.json([]);
});

app.listen(port, () => {
  console.log(` + "`Server running on port ${port}`" + `);
});

export default app;
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/tsconfig.json",
				Content: `{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "lib": ["ES2020"],
    "outDir": "./dist",
    "rootDir": "./src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "declaration": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/package.json",
				Content: `{
  "name": "{{.ProjectName}}",
  "version": "0.1.0",
  "description": "{{.ProjectName}} API",
  "main": "dist/index.js",
  "scripts": {
    "build": "tsc",
    "start": "node dist/index.js",
    "dev": "ts-node src/index.ts",
    "test": "jest"
  },
  "dependencies": {
    "express": "^4.18.0"
  },
  "devDependencies": {
    "@types/express": "^4.17.0",
    "@types/node": "^20.0.0",
    "typescript": "^5.0.0",
    "ts-node": "^10.9.0"
  }
}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/Dockerfile",
				Content: `FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/package*.json ./
RUN npm ci --production
EXPOSE {{.Port}}
CMD ["node", "dist/index.js"]
`,
				Mode: 0644,
				Condition: "{{.WithDocker}}",
			},
			{
				Path: "{{.ProjectName}}/.gitignore",
				Content: `node_modules/
dist/
.env
*.js.map
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/README.md",
				Content: `# {{.ProjectName}}

TypeScript API service.

## Development

` + "```bash" + `
npm install
npm run dev
` + "```" + `

## Build

` + "```bash" + `
npm run build
npm start
` + "```" + `
`,
				Mode: 0644,
			},
		},
		PostCreate: []string{"cd {{.ProjectName}} && npm install"},
	}

	s.Templates["python-api"] = &Template{
		Name:        "python-api",
		Description: "Python API with FastAPI",
		Language:    "python",
		Framework:   "fastapi",
		Variables: []TemplateVariable{
			{Name: "ProjectName", Description: "Name of the project", Required: true, Type: "string"},
			{Name: "Port", Description: "Server port", Default: "8000", Type: "string"},
			{Name: "WithDocker", Description: "Include Dockerfile", Default: "true", Type: "bool"},
		},
		Files: []TemplateFile{
			{
				Path: "{{.ProjectName}}/app/main.py",
				Content: `from fastapi import FastAPI

app = FastAPI(title="{{.ProjectName}}")


@app.get("/health")
def health():
    return {"status": "ok"}


@app.get("/api/v1/items")
def list_items():
    return []


@app.post("/api/v1/items", status_code=201)
def create_item(item: dict):
    return {"status": "created", "item": item}
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/app/__init__.py",
				Content: `"""{{.ProjectName}} application."""
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/requirements.txt",
				Content: `fastapi>=0.100.0
uvicorn[standard]>=0.23.0
pydantic>=2.0.0
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/Dockerfile",
				Content: `FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE {{.Port}}
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "{{.Port}}"]
`,
				Mode: 0644,
				Condition: "{{.WithDocker}}",
			},
			{
				Path: "{{.ProjectName}}/.gitignore",
				Content: `__pycache__/
*.py[cod]
*$py.class
.env
venv/
.venv/
dist/
*.egg-info/
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/README.md",
				Content: `# {{.ProjectName}}

Python FastAPI service.

## Setup

` + "```bash" + `
python -m venv venv
source venv/bin/activate
pip install -r requirements.txt
` + "```" + `

## Run

` + "```bash" + `
uvicorn app.main:app --reload --port {{.Port}}
` + "```" + `
`,
				Mode: 0644,
			},
		},
		PostCreate: []string{"cd {{.ProjectName}} && python -m venv venv"},
	}

	s.Templates["python-cli"] = &Template{
		Name:        "python-cli",
		Description: "Python CLI with Click",
		Language:    "python",
		Framework:   "click",
		Variables: []TemplateVariable{
			{Name: "ProjectName", Description: "Name of the project", Required: true, Type: "string"},
			{Name: "Author", Description: "Author name", Default: "Developer", Type: "string"},
			{Name: "WithTests", Description: "Include test directory", Default: "true", Type: "bool"},
		},
		Files: []TemplateFile{
			{
				Path: "{{.ProjectName}}/{{.ProjectName}}/cli.py",
				Content: `"""CLI entry point for {{.ProjectName}}."""
import click


@click.group()
@click.version_option(version="0.1.0")
def main():
    """{{.ProjectName}} - A command line tool."""
    pass


@main.command()
@click.argument("name", default="World")
def hello(name):
    """Say hello."""
    click.echo(f"Hello, {name}!")


if __name__ == "__main__":
    main()
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/{{.ProjectName}}/__init__.py",
				Content: `"""{{.ProjectName}} package."""
__version__ = "0.1.0"
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/setup.py",
				Content: `from setuptools import setup, find_packages

setup(
    name="{{.ProjectName}}",
    version="0.1.0",
    author="{{.Author}}",
    packages=find_packages(),
    install_requires=[
        "click>=8.0.0",
    ],
    entry_points={
        "console_scripts": [
            "{{.ProjectName}}={{.ProjectName}}.cli:main",
        ],
    },
    python_requires=">=3.8",
)
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/tests/__init__.py",
				Content: ``,
				Mode:      0644,
				Condition: "{{.WithTests}}",
			},
			{
				Path: "{{.ProjectName}}/tests/test_cli.py",
				Content: `"""Tests for CLI."""
from click.testing import CliRunner
from {{.ProjectName}}.cli import main


def test_hello():
    runner = CliRunner()
    result = runner.invoke(main, ["hello"])
    assert result.exit_code == 0
    assert "Hello, World!" in result.output


def test_hello_name():
    runner = CliRunner()
    result = runner.invoke(main, ["hello", "Test"])
    assert result.exit_code == 0
    assert "Hello, Test!" in result.output
`,
				Mode:      0644,
				Condition: "{{.WithTests}}",
			},
			{
				Path: "{{.ProjectName}}/.gitignore",
				Content: `__pycache__/
*.py[cod]
*$py.class
.env
venv/
.venv/
dist/
*.egg-info/
build/
`,
				Mode: 0644,
			},
			{
				Path: "{{.ProjectName}}/README.md",
				Content: `# {{.ProjectName}}

A command line tool.

## Installation

` + "```bash" + `
pip install -e .
` + "```" + `

## Usage

` + "```bash" + `
{{.ProjectName}} hello
{{.ProjectName}} hello YourName
` + "```" + `

## Author

{{.Author}}
`,
				Mode: 0644,
			},
		},
		PostCreate: []string{"cd {{.ProjectName}} && pip install -e ."},
	}
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}

		// Determine file mode
		mode := f.Mode
		if mode == 0 {
			mode = 0644
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
	t, err := template.New("").Parse(text)
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
