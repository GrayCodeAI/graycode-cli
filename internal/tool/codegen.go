package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// CodeGenerator manages code generation templates and renders them on demand.
type CodeGenerator struct {
	Templates map[string]*CodeTemplate
	Language  string
	mu        sync.RWMutex
}

// CodeTemplate defines a single code generation template.
type CodeTemplate struct {
	Name        string
	Description string
	Language    string
	Template    string
	Variables   []TemplateVar
	Output      string
}

// TemplateVar describes a variable used in a template.
type TemplateVar struct {
	Name        string
	Description string
	Required    bool
	Default     string
}

// NewCodeGenerator creates a CodeGenerator pre-loaded with built-in templates.
func NewCodeGenerator() *CodeGenerator {
	cg := &CodeGenerator{
		Templates: make(map[string]*CodeTemplate),
		Language:  "go",
	}
	cg.registerBuiltins()
	return cg
}

func (cg *CodeGenerator) registerBuiltins() {
	// Go templates
	cg.Templates["go-handler"] = &CodeTemplate{
		Name:        "go-handler",
		Description: "HTTP handler function with request parsing, validation, and response",
		Language:    "go",
		Template: `package {{.Package}}

import (
	"encoding/json"
	"net/http"
)

// {{.Name}}Request represents the request body for {{.Name}}.
type {{.Name}}Request struct {
	// TODO: define request fields
}

// {{.Name}}Response represents the response body for {{.Name}}.
type {{.Name}}Response struct {
	// TODO: define response fields
}

// {{.Name}}Handler handles {{.Method}} requests for {{.Path}}.
func {{.Name}}Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.Method{{.Method}} {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req {{.Name}}Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// TODO: implement handler logic

	resp := {{.Name}}Response{}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
`,
		Variables: []TemplateVar{
			{Name: "Package", Description: "Go package name", Required: true, Default: "handlers"},
			{Name: "Name", Description: "Handler name (PascalCase)", Required: true, Default: ""},
			{Name: "Method", Description: "HTTP method (Get, Post, Put, Delete)", Required: false, Default: "Post"},
			{Name: "Path", Description: "URL path for the endpoint", Required: false, Default: "/api/resource"},
		},
		Output: "{{.Name | lower}}_handler.go",
	}

	cg.Templates["go-middleware"] = &CodeTemplate{
		Name:        "go-middleware",
		Description: "HTTP middleware with next handler chaining",
		Language:    "go",
		Template: `package {{.Package}}

import (
	"log"
	"net/http"
	"time"
)

// {{.Name}} is a middleware that {{.Description}}.
func {{.Name}}(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Pre-processing
		log.Printf("[{{.Name}}] %s %s started", r.Method, r.URL.Path)

		// TODO: implement middleware logic

		// Call next handler
		next.ServeHTTP(w, r)

		// Post-processing
		duration := time.Since(start)
		log.Printf("[{{.Name}}] %s %s completed in %v", r.Method, r.URL.Path, duration)
	})
}
`,
		Variables: []TemplateVar{
			{Name: "Package", Description: "Go package name", Required: true, Default: "middleware"},
			{Name: "Name", Description: "Middleware function name", Required: true, Default: ""},
			{Name: "Description", Description: "What the middleware does", Required: false, Default: "processes requests"},
		},
		Output: "{{.Name | lower}}.go",
	}

	cg.Templates["go-crud"] = &CodeTemplate{
		Name:        "go-crud",
		Description: "Full CRUD functions for a resource (Create, Get, List, Update, Delete)",
		Language:    "go",
		Template: `package {{.Package}}

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// {{.Resource}} represents the {{.Resource}} entity.
type {{.Resource}} struct {
	ID   string ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
	// TODO: add fields
}

// {{.Resource}}Store manages {{.Resource}} persistence.
type {{.Resource}}Store struct {
	mu    sync.RWMutex
	items map[string]*{{.Resource}}
}

// New{{.Resource}}Store creates a new store.
func New{{.Resource}}Store() *{{.Resource}}Store {
	return &{{.Resource}}Store{items: make(map[string]*{{.Resource}})}
}

// Create{{.Resource}} adds a new {{.Resource}}.
func (s *{{.Resource}}Store) Create{{.Resource}}(item *{{.Resource}}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[item.ID]; exists {
		return fmt.Errorf("{{.Resource}} with ID %s already exists", item.ID)
	}
	s.items[item.ID] = item
	return nil
}

// Get{{.Resource}} retrieves a {{.Resource}} by ID.
func (s *{{.Resource}}Store) Get{{.Resource}}(id string) (*{{.Resource}}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("{{.Resource}} with ID %s not found", id)
	}
	return item, nil
}

// List{{.Resource}}s returns all {{.Resource}} items.
func (s *{{.Resource}}Store) List{{.Resource}}s() []*{{.Resource}} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*{{.Resource}}, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	return result
}

// Update{{.Resource}} updates an existing {{.Resource}}.
func (s *{{.Resource}}Store) Update{{.Resource}}(item *{{.Resource}}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[item.ID]; !exists {
		return fmt.Errorf("{{.Resource}} with ID %s not found", item.ID)
	}
	s.items[item.ID] = item
	return nil
}

// Delete{{.Resource}} removes a {{.Resource}} by ID.
func (s *{{.Resource}}Store) Delete{{.Resource}}(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[id]; !exists {
		return fmt.Errorf("{{.Resource}} with ID %s not found", id)
	}
	delete(s.items, id)
	return nil
}

// Handle{{.Resource}}s returns an HTTP handler for {{.Resource}} CRUD operations.
func (s *{{.Resource}}Store) Handle{{.Resource}}s(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		items := s.List{{.Resource}}s()
		_ = json.NewEncoder(w).Encode(items)
	case http.MethodPost:
		var item {{.Resource}}
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Create{{.Resource}}(&item); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(item)
	case http.MethodPut:
		var item {{.Resource}}
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.Update{{.Resource}}(&item); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(item)
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if err := s.Delete{{.Resource}}(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
`,
		Variables: []TemplateVar{
			{Name: "Package", Description: "Go package name", Required: true, Default: "models"},
			{Name: "Resource", Description: "Resource name (PascalCase)", Required: true, Default: ""},
		},
		Output: "{{.Resource | lower}}_crud.go",
	}

	cg.Templates["go-test-table"] = &CodeTemplate{
		Name:        "go-test-table",
		Description: "Table-driven test with subtests",
		Language:    "go",
		Template: `package {{.Package}}

import (
	"testing"
)

func Test{{.Function}}(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "valid input",
			input:   "hello",
			want:    "expected",
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   "",
			want:    "",
			wantErr: true,
		},
		// TODO: add more test cases
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := {{.Function}}(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("{{.Function}}() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("{{.Function}}() = %v, want %v", got, tt.want)
			}
		})
	}
}
`,
		Variables: []TemplateVar{
			{Name: "Package", Description: "Go package name", Required: true, Default: ""},
			{Name: "Function", Description: "Function name to test", Required: true, Default: ""},
		},
		Output: "{{.Function | lower}}_test.go",
	}

	cg.Templates["go-interface"] = &CodeTemplate{
		Name:        "go-interface",
		Description: "Interface definition with mock implementation",
		Language:    "go",
		Template: `package {{.Package}}

import (
	"context"
	"sync"
)

// {{.Name}} defines the interface for {{.Description}}.
type {{.Name}} interface {
	Get(ctx context.Context, id string) (interface{}, error)
	List(ctx context.Context) ([]interface{}, error)
	Create(ctx context.Context, item interface{}) error
	Update(ctx context.Context, id string, item interface{}) error
	Delete(ctx context.Context, id string) error
}

// Mock{{.Name}} is a test double for {{.Name}}.
type Mock{{.Name}} struct {
	mu          sync.Mutex
	GetFunc     func(ctx context.Context, id string) (interface{}, error)
	ListFunc    func(ctx context.Context) ([]interface{}, error)
	CreateFunc  func(ctx context.Context, item interface{}) error
	UpdateFunc  func(ctx context.Context, id string, item interface{}) error
	DeleteFunc  func(ctx context.Context, id string) error
	Calls       []string
}

func (m *Mock{{.Name}}) Get(ctx context.Context, id string) (interface{}, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "Get")
	m.mu.Unlock()
	if m.GetFunc != nil {
		return m.GetFunc(ctx, id)
	}
	return nil, nil
}

func (m *Mock{{.Name}}) List(ctx context.Context) ([]interface{}, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, "List")
	m.mu.Unlock()
	if m.ListFunc != nil {
		return m.ListFunc(ctx)
	}
	return nil, nil
}

func (m *Mock{{.Name}}) Create(ctx context.Context, item interface{}) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "Create")
	m.mu.Unlock()
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, item)
	}
	return nil
}

func (m *Mock{{.Name}}) Update(ctx context.Context, id string, item interface{}) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "Update")
	m.mu.Unlock()
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, id, item)
	}
	return nil
}

func (m *Mock{{.Name}}) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, "Delete")
	m.mu.Unlock()
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}
`,
		Variables: []TemplateVar{
			{Name: "Package", Description: "Go package name", Required: true, Default: ""},
			{Name: "Name", Description: "Interface name (PascalCase)", Required: true, Default: ""},
			{Name: "Description", Description: "What the interface represents", Required: false, Default: "a service"},
		},
		Output: "{{.Name | lower}}.go",
	}

	cg.Templates["go-errors"] = &CodeTemplate{
		Name:        "go-errors",
		Description: "Custom error type with constructors",
		Language:    "go",
		Template: `package {{.Package}}

import "fmt"

// {{.Name}}Error represents an error in the {{.Domain}} domain.
type {{.Name}}Error struct {
	Code    string
	Message string
	Err     error
}

func (e *{{.Name}}Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *{{.Name}}Error) Unwrap() error {
	return e.Err
}

// New{{.Name}}Error creates a new {{.Name}}Error.
func New{{.Name}}Error(code, message string) *{{.Name}}Error {
	return &{{.Name}}Error{Code: code, Message: message}
}

// Wrap{{.Name}}Error wraps an existing error with a {{.Name}}Error.
func Wrap{{.Name}}Error(code, message string, err error) *{{.Name}}Error {
	return &{{.Name}}Error{Code: code, Message: message, Err: err}
}

// ErrNotFound creates a not-found error.
func ErrNotFound(resource, id string) *{{.Name}}Error {
	return New{{.Name}}Error("NOT_FOUND", fmt.Sprintf("%s with ID %s not found", resource, id))
}

// ErrValidation creates a validation error.
func ErrValidation(field, reason string) *{{.Name}}Error {
	return New{{.Name}}Error("VALIDATION", fmt.Sprintf("field %s: %s", field, reason))
}

// ErrInternal creates an internal error wrapping the cause.
func ErrInternal(message string, err error) *{{.Name}}Error {
	return Wrap{{.Name}}Error("INTERNAL", message, err)
}

// Is{{.Name}}Error checks if an error is a {{.Name}}Error with the given code.
func Is{{.Name}}Error(err error, code string) bool {
	if e, ok := err.(*{{.Name}}Error); ok {
		return e.Code == code
	}
	return false
}
`,
		Variables: []TemplateVar{
			{Name: "Package", Description: "Go package name", Required: true, Default: ""},
			{Name: "Name", Description: "Error type prefix (PascalCase)", Required: true, Default: ""},
			{Name: "Domain", Description: "Domain this error belongs to", Required: false, Default: "application"},
		},
		Output: "{{.Name | lower}}_errors.go",
	}

	cg.Templates["go-config"] = &CodeTemplate{
		Name:        "go-config",
		Description: "Config struct with environment variable loading and validation",
		Language:    "go",
		Template: `package {{.Package}}

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// {{.Name}}Config holds configuration for {{.Description}}.
type {{.Name}}Config struct {
	Host     string
	Port     int
	Debug    bool
	LogLevel string
	// TODO: add more config fields
}

// Default{{.Name}}Config returns the default configuration.
func Default{{.Name}}Config() *{{.Name}}Config {
	return &{{.Name}}Config{
		Host:     "localhost",
		Port:     8080,
		Debug:    false,
		LogLevel: "info",
	}
}

// Load{{.Name}}Config loads configuration from environment variables.
// Variables are prefixed with {{.Prefix}}_.
func Load{{.Name}}Config() (*{{.Name}}Config, error) {
	cfg := Default{{.Name}}Config()

	if v := os.Getenv("{{.Prefix}}_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("{{.Prefix}}_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid {{.Prefix}}_PORT: %w", err)
		}
		cfg.Port = port
	}
	if v := os.Getenv("{{.Prefix}}_DEBUG"); v != "" {
		cfg.Debug = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("{{.Prefix}}_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks that the configuration is valid.
func (c *{{.Name}}Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host must not be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Port)
	}
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.LogLevel)] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.LogLevel)
	}
	return nil
}

// Address returns the host:port address string.
func (c *{{.Name}}Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
`,
		Variables: []TemplateVar{
			{Name: "Package", Description: "Go package name", Required: true, Default: "config"},
			{Name: "Name", Description: "Config name (PascalCase)", Required: true, Default: ""},
			{Name: "Prefix", Description: "Environment variable prefix (UPPER_CASE)", Required: true, Default: "APP"},
			{Name: "Description", Description: "What this config is for", Required: false, Default: "the application"},
		},
		Output: "{{.Name | lower}}_config.go",
	}

	// Python templates
	cg.Templates["py-fastapi-endpoint"] = &CodeTemplate{
		Name:        "py-fastapi-endpoint",
		Description: "FastAPI route with Pydantic model",
		Language:    "python",
		Template: `from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field
from typing import Optional

router = APIRouter(prefix="/{{.Prefix}}", tags=["{{.Tag}}"])


class {{.Name}}Request(BaseModel):
    """Request model for {{.Name}}."""
    name: str = Field(..., description="Name field")
    # TODO: add request fields


class {{.Name}}Response(BaseModel):
    """Response model for {{.Name}}."""
    id: str
    name: str
    # TODO: add response fields


@router.post("/", response_model={{.Name}}Response, status_code=201)
async def create_{{.NameLower}}(request: {{.Name}}Request) -> {{.Name}}Response:
    """Create a new {{.Name}}."""
    # TODO: implement creation logic
    return {{.Name}}Response(id="generated-id", name=request.name)


@router.get("/{item_id}", response_model={{.Name}}Response)
async def get_{{.NameLower}}(item_id: str) -> {{.Name}}Response:
    """Get a {{.Name}} by ID."""
    # TODO: implement retrieval logic
    raise HTTPException(status_code=404, detail="{{.Name}} not found")


@router.put("/{item_id}", response_model={{.Name}}Response)
async def update_{{.NameLower}}(item_id: str, request: {{.Name}}Request) -> {{.Name}}Response:
    """Update a {{.Name}}."""
    # TODO: implement update logic
    return {{.Name}}Response(id=item_id, name=request.name)


@router.delete("/{item_id}", status_code=204)
async def delete_{{.NameLower}}(item_id: str) -> None:
    """Delete a {{.Name}}."""
    # TODO: implement deletion logic
    pass
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Resource name (PascalCase)", Required: true, Default: ""},
			{Name: "NameLower", Description: "Resource name (lowercase)", Required: true, Default: ""},
			{Name: "Prefix", Description: "URL prefix", Required: false, Default: "api"},
			{Name: "Tag", Description: "OpenAPI tag", Required: false, Default: "default"},
		},
		Output: "{{.NameLower}}_router.py",
	}

	cg.Templates["py-test-class"] = &CodeTemplate{
		Name:        "py-test-class",
		Description: "Pytest test class with setup/teardown",
		Language:    "python",
		Template: `import pytest


class Test{{.Name}}:
    """Tests for {{.Name}}."""

    def setup_method(self):
        """Set up test fixtures."""
        # TODO: initialize test fixtures
        self.subject = None

    def teardown_method(self):
        """Clean up after tests."""
        # TODO: clean up resources
        pass

    def test_{{.MethodUnderTest}}_with_valid_input(self):
        """Test {{.MethodUnderTest}} with valid input."""
        # Arrange
        expected = None  # TODO: set expected value

        # Act
        result = self.subject.{{.MethodUnderTest}}()

        # Assert
        assert result == expected

    def test_{{.MethodUnderTest}}_with_invalid_input(self):
        """Test {{.MethodUnderTest}} raises on invalid input."""
        with pytest.raises(ValueError):
            self.subject.{{.MethodUnderTest}}(None)

    def test_{{.MethodUnderTest}}_edge_case(self):
        """Test {{.MethodUnderTest}} handles edge cases."""
        # TODO: implement edge case test
        pass
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Class under test (PascalCase)", Required: true, Default: ""},
			{Name: "MethodUnderTest", Description: "Primary method to test", Required: true, Default: "execute"},
		},
		Output: "test_{{.Name | lower}}.py",
	}

	cg.Templates["py-dataclass"] = &CodeTemplate{
		Name:        "py-dataclass",
		Description: "Dataclass with validation",
		Language:    "python",
		Template: `from dataclasses import dataclass, field
from typing import Optional, List


@dataclass
class {{.Name}}:
    """{{.Description}}"""

    name: str
    value: int = 0
    tags: List[str] = field(default_factory=list)
    metadata: Optional[str] = None

    def __post_init__(self):
        """Validate fields after initialization."""
        if not self.name:
            raise ValueError("name must not be empty")
        if self.value < 0:
            raise ValueError("value must be non-negative")
        # TODO: add more validation

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "name": self.name,
            "value": self.value,
            "tags": list(self.tags),
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict) -> "{{.Name}}":
        """Create instance from dictionary."""
        return cls(
            name=data["name"],
            value=data.get("value", 0),
            tags=data.get("tags", []),
            metadata=data.get("metadata"),
        )
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Class name (PascalCase)", Required: true, Default: ""},
			{Name: "Description", Description: "Class description", Required: false, Default: "A data model"},
		},
		Output: "{{.Name | lower}}.py",
	}

	// TypeScript templates
	cg.Templates["ts-react-component"] = &CodeTemplate{
		Name:        "ts-react-component",
		Description: "Functional React component with props interface",
		Language:    "typescript",
		Template: `import React from 'react';

interface {{.Name}}Props {
  title: string;
  className?: string;
  children?: React.ReactNode;
  onClick?: () => void;
}

/**
 * {{.Description}}
 */
export const {{.Name}}: React.FC<{{.Name}}Props> = ({
  title,
  className = '',
  children,
  onClick,
}) => {
  return (
    <div className={` + "`{{.Name}} ${className}`" + `} onClick={onClick}>
      <h2>{title}</h2>
      {children}
    </div>
  );
};

export default {{.Name}};
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Component name (PascalCase)", Required: true, Default: ""},
			{Name: "Description", Description: "Component description", Required: false, Default: "A React component"},
		},
		Output: "{{.Name}}.tsx",
	}

	cg.Templates["ts-express-router"] = &CodeTemplate{
		Name:        "ts-express-router",
		Description: "Express router with middleware",
		Language:    "typescript",
		Template: `import { Router, Request, Response, NextFunction } from 'express';

const router = Router();

// Middleware for this router
function validate{{.Name}}(req: Request, res: Response, next: NextFunction): void {
  // TODO: implement validation
  next();
}

// GET /{{.Path}}
router.get('/', async (req: Request, res: Response) => {
  try {
    // TODO: implement list
    res.json({ items: [] });
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// GET /{{.Path}}/:id
router.get('/:id', async (req: Request, res: Response) => {
  try {
    const { id } = req.params;
    // TODO: implement get by id
    res.json({ id });
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// POST /{{.Path}}
router.post('/', validate{{.Name}}, async (req: Request, res: Response) => {
  try {
    // TODO: implement create
    res.status(201).json({ id: 'new-id', ...req.body });
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// PUT /{{.Path}}/:id
router.put('/:id', validate{{.Name}}, async (req: Request, res: Response) => {
  try {
    const { id } = req.params;
    // TODO: implement update
    res.json({ id, ...req.body });
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

// DELETE /{{.Path}}/:id
router.delete('/:id', async (req: Request, res: Response) => {
  try {
    const { id } = req.params;
    // TODO: implement delete
    res.status(204).send();
  } catch (error) {
    res.status(500).json({ error: 'Internal server error' });
  }
});

export default router;
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Resource name (PascalCase)", Required: true, Default: ""},
			{Name: "Path", Description: "Route path", Required: false, Default: "resources"},
		},
		Output: "{{.Name | lower}}.router.ts",
	}

	cg.Templates["ts-test-describe"] = &CodeTemplate{
		Name:        "ts-test-describe",
		Description: "Jest/Vitest describe block with test cases",
		Language:    "typescript",
		Template: `import { describe, it, expect, beforeEach, afterEach } from 'vitest';

describe('{{.Name}}', () => {
  let subject: any;

  beforeEach(() => {
    // TODO: set up test fixtures
    subject = null;
  });

  afterEach(() => {
    // TODO: clean up
  });

  describe('{{.Method}}', () => {
    it('should handle valid input', () => {
      // Arrange
      const input = {};

      // Act
      const result = subject.{{.Method}}(input);

      // Assert
      expect(result).toBeDefined();
    });

    it('should throw on invalid input', () => {
      expect(() => subject.{{.Method}}(null)).toThrow();
    });

    it('should handle edge cases', () => {
      // TODO: implement edge case test
      expect(true).toBe(true);
    });
  });
});
`,
		Variables: []TemplateVar{
			{Name: "Name", Description: "Module/class under test", Required: true, Default: ""},
			{Name: "Method", Description: "Method being tested", Required: true, Default: "execute"},
		},
		Output: "{{.Name | lower}}.test.ts",
	}
}

// Generate renders a template with the given variables.
func (cg *CodeGenerator) Generate(templateName string, vars map[string]string) (string, error) {
	cg.mu.RLock()
	tmpl, ok := cg.Templates[templateName]
	cg.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("template %q not found", templateName)
	}

	// Apply defaults for missing non-required variables
	resolved := make(map[string]string)
	for _, v := range tmpl.Variables {
		if val, exists := vars[v.Name]; exists && val != "" {
			resolved[v.Name] = val
		} else if v.Required && v.Default == "" {
			return "", fmt.Errorf("required variable %q not provided for template %q", v.Name, templateName)
		} else if v.Default != "" {
			resolved[v.Name] = v.Default
		} else {
			resolved[v.Name] = ""
		}
	}

	// Also include any extra vars passed in
	for k, v := range vars {
		if _, exists := resolved[k]; !exists {
			resolved[k] = v
		}
	}

	funcMap := template.FuncMap{
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"title": cases.Title(language.English).String,
	}

	t, err := template.New(templateName).Funcs(funcMap).Parse(tmpl.Template)
	if err != nil {
		return "", fmt.Errorf("parsing template %q: %w", templateName, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, resolved); err != nil {
		return "", fmt.Errorf("executing template %q: %w", templateName, err)
	}

	return buf.String(), nil
}

// ListTemplates returns templates filtered by language.
// If language is empty, all templates are returned.
func (cg *CodeGenerator) ListTemplates(language string) []*CodeTemplate {
	cg.mu.RLock()
	defer cg.mu.RUnlock()

	var result []*CodeTemplate
	for _, tmpl := range cg.Templates {
		if language == "" || strings.EqualFold(tmpl.Language, language) {
			result = append(result, tmpl)
		}
	}
	return result
}

// Register adds a custom template to the generator.
func (cg *CodeGenerator) Register(template *CodeTemplate) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.Templates[template.Name] = template
}

// Preview shows what would be generated without performing full rendering.
// It shows the template with variable placeholders highlighted.
func (cg *CodeGenerator) Preview(templateName string, vars map[string]string) string {
	cg.mu.RLock()
	tmpl, ok := cg.Templates[templateName]
	cg.mu.RUnlock()
	if !ok {
		return fmt.Sprintf("template %q not found", templateName)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Template: %s\n", tmpl.Name))
	sb.WriteString(fmt.Sprintf("Language: %s\n", tmpl.Language))
	sb.WriteString(fmt.Sprintf("Description: %s\n", tmpl.Description))
	sb.WriteString("\nVariables:\n")
	for _, v := range tmpl.Variables {
		value := v.Default
		if val, exists := vars[v.Name]; exists {
			value = val
		}
		required := ""
		if v.Required {
			required = " (required)"
		}
		sb.WriteString(fmt.Sprintf("  %s = %q%s\n", v.Name, value, required))
	}
	sb.WriteString("\n--- Generated Preview ---\n")

	// Attempt to render
	output, err := cg.Generate(templateName, vars)
	if err != nil {
		sb.WriteString(fmt.Sprintf("Error: %v\n", err))
	} else {
		sb.WriteString(output)
	}

	return sb.String()
}

// SuggestTemplate suggests the best template given a natural language description.
func (cg *CodeGenerator) SuggestTemplate(description string) string {
	desc := strings.ToLower(description)

	// Keyword matching rules with weights.
	// More specific (language-prefixed) rules use higher weights so they win ties.
	type rule struct {
		keywords []string
		weights  []int
		template string
	}

	rules := []rule{
		// Python templates (high weight so they beat generic Go matches)
		{keywords: []string{"fastapi", "pydantic", "python api", "python endpoint"}, weights: []int{10, 10, 10, 10}, template: "py-fastapi-endpoint"},
		{keywords: []string{"pytest", "python test"}, weights: []int{10, 10}, template: "py-test-class"},
		{keywords: []string{"dataclass", "python model", "python class"}, weights: []int{10, 10, 10}, template: "py-dataclass"},

		// TypeScript templates (high weight)
		{keywords: []string{"react", "component", "jsx", "tsx"}, weights: []int{10, 5, 10, 10}, template: "ts-react-component"},
		{keywords: []string{"express", "node router", "node api"}, weights: []int{10, 10, 10}, template: "ts-express-router"},
		{keywords: []string{"jest", "vitest", "typescript test", "ts test"}, weights: []int{10, 10, 10, 10}, template: "ts-test-describe"},

		// Go templates (standard weight)
		{keywords: []string{"middleware"}, weights: []int{5}, template: "go-middleware"},
		{keywords: []string{"crud", "create read update delete", "resource operations"}, weights: []int{5, 5, 5}, template: "go-crud"},
		{keywords: []string{"test", "testing", "unit test", "add tests"}, weights: []int{3, 3, 5, 5}, template: "go-test-table"},
		{keywords: []string{"interface", "mock", "abstraction"}, weights: []int{5, 5, 5}, template: "go-interface"},
		{keywords: []string{"error", "custom error", "error type"}, weights: []int{3, 5, 5}, template: "go-errors"},
		{keywords: []string{"config", "configuration", "environment", "env var"}, weights: []int{5, 5, 5, 5}, template: "go-config"},
		{keywords: []string{"handler", "endpoint", "api endpoint", "route", "http"}, weights: []int{3, 3, 5, 3, 3}, template: "go-handler"},
	}

	// Score each rule by summing weights of matched keywords
	bestTemplate := "go-handler"
	bestScore := 0

	for _, r := range rules {
		score := 0
		for i, kw := range r.keywords {
			if strings.Contains(desc, kw) {
				score += r.weights[i]
			}
		}
		if score > bestScore {
			bestScore = score
			bestTemplate = r.template
		}
	}

	return bestTemplate
}

// CodeGenTool implements the Tool interface for code generation.
type CodeGenTool struct {
	generator *CodeGenerator
}

// NewCodeGenTool creates a new CodeGenTool.
func NewCodeGenTool() *CodeGenTool {
	return &CodeGenTool{generator: NewCodeGenerator()}
}

func (t *CodeGenTool) Name() string {
	return "CodeGen"
}

func (t *CodeGenTool) Description() string {
	return "Generate common code patterns from templates. Supports Go, Python, and TypeScript templates for handlers, tests, middleware, CRUD operations, and more."
}

func (t *CodeGenTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"generate", "list", "preview", "suggest"},
				"description": "Action to perform",
			},
			"template": map[string]interface{}{
				"type":        "string",
				"description": "Template name (e.g., go-handler, py-fastapi-endpoint)",
			},
			"variables": map[string]interface{}{
				"type":        "object",
				"description": "Template variables as key-value pairs",
			},
			"language": map[string]interface{}{
				"type":        "string",
				"description": "Filter templates by language (go, python, typescript)",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Natural language description for template suggestion",
			},
		},
		"required": []string{"action"},
	}
}

type codeGenInput struct {
	Action      string            `json:"action"`
	Template    string            `json:"template"`
	Variables   map[string]string `json:"variables"`
	Language    string            `json:"language"`
	Description string            `json:"description"`
}

func (t *CodeGenTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in codeGenInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	switch in.Action {
	case "generate":
		if in.Template == "" {
			return "", fmt.Errorf("template name is required for generate action")
		}
		code, err := t.generator.Generate(in.Template, in.Variables)
		if err != nil {
			return "", err
		}
		return code, nil

	case "list":
		templates := t.generator.ListTemplates(in.Language)
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Available templates (%d):\n\n", len(templates)))
		for _, tmpl := range templates {
			sb.WriteString(fmt.Sprintf("  %s [%s]\n    %s\n", tmpl.Name, tmpl.Language, tmpl.Description))
			for _, v := range tmpl.Variables {
				req := ""
				if v.Required {
					req = " *"
				}
				def := ""
				if v.Default != "" {
					def = fmt.Sprintf(" (default: %s)", v.Default)
				}
				sb.WriteString(fmt.Sprintf("    - %s: %s%s%s\n", v.Name, v.Description, req, def))
			}
			sb.WriteString("\n")
		}
		return sb.String(), nil

	case "preview":
		if in.Template == "" {
			return "", fmt.Errorf("template name is required for preview action")
		}
		return t.generator.Preview(in.Template, in.Variables), nil

	case "suggest":
		if in.Description == "" {
			return "", fmt.Errorf("description is required for suggest action")
		}
		suggested := t.generator.SuggestTemplate(in.Description)
		return fmt.Sprintf("Suggested template: %s", suggested), nil

	default:
		return "", fmt.Errorf("unknown action: %s (valid: generate, list, preview, suggest)", in.Action)
	}
}

// Generator returns the underlying CodeGenerator for direct use.
func (t *CodeGenTool) Generator() *CodeGenerator {
	return t.generator
}
