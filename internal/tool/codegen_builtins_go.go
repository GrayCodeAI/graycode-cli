package tool

// This file holds Go code-generation templates for the CodeGenerator.
// Go templates are organized by category for maintainability.

func registerGoTemplates(cg *CodeGenerator) {
	// HTTP Handler
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

	// Middleware
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

	// CRUD
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
	return &{{.Resource}}Store{items: make(map[string]*{{.Resource}})}}

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

	// Test Table
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
		{
			name:    "special chars",
			input:   "hello\nworld",
			want:    "expected with special chars",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := {{.Function}}(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("{{.Function}}() error = %v, wantErr %v", err, tt.wantErr)
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
			{Name: "Function", Description: "Function under test", Required: true, Default: ""},
		},
		Output: "{{.Function}}_table_test.go",
	}

	// Interface
	cg.Templates["go-interface"] = &CodeTemplate{
		Name:        "go-interface",
		Description: "Go interface definition",
		Language:    "go",
		Template: `package {{.Package}}

import "context"

// {{.Name}} is the interface for {{.Description}}.
type {{.Name}} interface {
	Execute(ctx context.Context, input {{.InputType}}) ({{.OutputType}}, error)
	Validate(input {{.InputType}}) error
}
`,
		Variables: []TemplateVar{
			{Name: "Package", Description: "Go package name", Required: true, Default: ""},
			{Name: "Name", Description: "Interface name (PascalCase)", Required: true, Default: ""},
			{Name: "InputType", Description: "Input parameter type", Required: false, Default: "string"},
			{Name: "OutputType", Description: "Return type", Required: false, Default: "string"},
		},
		Output: "{{.Name}}.go",
	}

	// Errors
	cg.Templates["go-errors"] = &CodeTemplate{
		Name:        "go-errors",
		Description: "Go error definitions",
		Language:    "go",
		Template: `package {{.Package}}

import "errors"

// {{.Name}}Error represents errors for {{.Name}}.
var (
	Err{{.Name}}NotFound     = errors.New("{{.Name}} not found")
	Err{{.Name}}InvalidInput = errors.New("invalid input for {{.Name}}")
	Err{{.Name}}Conflict      = errors.New("{{.Name}} conflict")
)

// Is{{.Name}}NotFound checks if error is Err{{.Name}}NotFound.
func Is{{.Name}}NotFound(err error) bool {
	return errors.Is(err, Err{{.Name}}NotFound)
}

// Is{{.Name}}InvalidInput checks if error is Err{{.Name}}InvalidInput.
func Is{{.Name}}InvalidInput(err error) bool {
	return errors.Is(err, Err{{.Name}}InvalidInput)
}
`,
		Variables: []TemplateVar{
			{Name: "Package", Description: "Go package name", Required: true, Default: ""},
			{Name: "Name", Description: "Resource name (PascalCase)", Required: true, Default: ""},
		},
		Output: "{{.Name}}_errors.go",
	}
}
