package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewCodeGenerator(t *testing.T) {
	cg := NewCodeGenerator()
	if cg == nil {
		t.Fatal("NewCodeGenerator returned nil")
	}
	if len(cg.Templates) == 0 {
		t.Fatal("NewCodeGenerator should have built-in templates")
	}
}

func TestGenerateGoHandler(t *testing.T) {
	cg := NewCodeGenerator()
	vars := map[string]string{
		"Package": "api",
		"Name":    "CreateUser",
		"Method":  "Post",
		"Path":    "/api/users",
	}
	output, err := cg.Generate("go-handler", vars)
	if err != nil {
		t.Fatalf("Generate go-handler: %v", err)
	}
	if !strings.Contains(output, "package api") {
		t.Error("expected 'package api' in output")
	}
	if !strings.Contains(output, "CreateUserHandler") {
		t.Error("expected 'CreateUserHandler' in output")
	}
	if !strings.Contains(output, "CreateUserRequest") {
		t.Error("expected 'CreateUserRequest' in output")
	}
	if !strings.Contains(output, "CreateUserResponse") {
		t.Error("expected 'CreateUserResponse' in output")
	}
	if !strings.Contains(output, "http.MethodPost") {
		t.Error("expected 'http.MethodPost' in output")
	}
}

func TestGenerateGoMiddleware(t *testing.T) {
	cg := NewCodeGenerator()
	vars := map[string]string{
		"Package":     "mw",
		"Name":        "RateLimiter",
		"Description": "rate limits requests",
	}
	output, err := cg.Generate("go-middleware", vars)
	if err != nil {
		t.Fatalf("Generate go-middleware: %v", err)
	}
	if !strings.Contains(output, "package mw") {
		t.Error("expected 'package mw' in output")
	}
	if !strings.Contains(output, "func RateLimiter(next http.Handler) http.Handler") {
		t.Error("expected middleware function signature")
	}
	if !strings.Contains(output, "next.ServeHTTP(w, r)") {
		t.Error("expected next handler call")
	}
}

func TestGenerateGoCRUD(t *testing.T) {
	cg := NewCodeGenerator()
	vars := map[string]string{
		"Package":  "store",
		"Resource": "User",
	}
	output, err := cg.Generate("go-crud", vars)
	if err != nil {
		t.Fatalf("Generate go-crud: %v", err)
	}
	if !strings.Contains(output, "package store") {
		t.Error("expected 'package store' in output")
	}
	if !strings.Contains(output, "CreateUser") {
		t.Error("expected 'CreateUser' in output")
	}
	if !strings.Contains(output, "GetUser") {
		t.Error("expected 'GetUser' in output")
	}
	if !strings.Contains(output, "ListUsers") {
		t.Error("expected 'ListUsers' in output")
	}
	if !strings.Contains(output, "UpdateUser") {
		t.Error("expected 'UpdateUser' in output")
	}
	if !strings.Contains(output, "DeleteUser") {
		t.Error("expected 'DeleteUser' in output")
	}
	if !strings.Contains(output, "UserStore") {
		t.Error("expected 'UserStore' struct in output")
	}
}

func TestGenerateGoTestTable(t *testing.T) {
	cg := NewCodeGenerator()
	vars := map[string]string{
		"Package":  "parser",
		"Function": "ParseURL",
	}
	output, err := cg.Generate("go-test-table", vars)
	if err != nil {
		t.Fatalf("Generate go-test-table: %v", err)
	}
	if !strings.Contains(output, "package parser") {
		t.Error("expected 'package parser' in output")
	}
	if !strings.Contains(output, "func TestParseURL(t *testing.T)") {
		t.Error("expected test function")
	}
	if !strings.Contains(output, "t.Run(tt.name") {
		t.Error("expected subtests with t.Run")
	}
	if !strings.Contains(output, "tests := []struct") {
		t.Error("expected table-driven test structure")
	}
}

func TestGenerateGoInterface(t *testing.T) {
	cg := NewCodeGenerator()
	vars := map[string]string{
		"Package":     "repo",
		"Name":        "Repository",
		"Description": "data access",
	}
	output, err := cg.Generate("go-interface", vars)
	if err != nil {
		t.Fatalf("Generate go-interface: %v", err)
	}
	if !strings.Contains(output, "type Repository interface") {
		t.Error("expected interface definition")
	}
	if !strings.Contains(output, "type MockRepository struct") {
		t.Error("expected mock struct")
	}
	if !strings.Contains(output, "GetFunc") {
		t.Error("expected mock function fields")
	}
}

func TestGenerateGoErrors(t *testing.T) {
	cg := NewCodeGenerator()
	vars := map[string]string{
		"Package": "apierr",
		"Name":    "API",
		"Domain":  "API",
	}
	output, err := cg.Generate("go-errors", vars)
	if err != nil {
		t.Fatalf("Generate go-errors: %v", err)
	}
	if !strings.Contains(output, "type APIError struct") {
		t.Error("expected error struct")
	}
	if !strings.Contains(output, "func (e *APIError) Error() string") {
		t.Error("expected Error() method")
	}
	if !strings.Contains(output, "func (e *APIError) Unwrap() error") {
		t.Error("expected Unwrap() method")
	}
	if !strings.Contains(output, "ErrNotFound") {
		t.Error("expected ErrNotFound constructor")
	}
	if !strings.Contains(output, "ErrValidation") {
		t.Error("expected ErrValidation constructor")
	}
}

func TestGenerateGoConfig(t *testing.T) {
	cg := NewCodeGenerator()
	vars := map[string]string{
		"Package":     "config",
		"Name":        "Server",
		"Prefix":      "SERVER",
		"Description": "the HTTP server",
	}
	output, err := cg.Generate("go-config", vars)
	if err != nil {
		t.Fatalf("Generate go-config: %v", err)
	}
	if !strings.Contains(output, "type ServerConfig struct") {
		t.Error("expected config struct")
	}
	if !strings.Contains(output, "SERVER_HOST") {
		t.Error("expected env var with prefix")
	}
	if !strings.Contains(output, "func (c *ServerConfig) Validate() error") {
		t.Error("expected Validate method")
	}
	if !strings.Contains(output, "DefaultServerConfig") {
		t.Error("expected Default function")
	}
}

func TestVariableSubstitution(t *testing.T) {
	cg := NewCodeGenerator()
	vars := map[string]string{
		"Package": "mypackage",
		"Name":    "MyHandler",
		"Method":  "Get",
		"Path":    "/custom/path",
	}
	output, err := cg.Generate("go-handler", vars)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(output, "package mypackage") {
		t.Error("Package variable not substituted")
	}
	if !strings.Contains(output, "MyHandlerHandler") {
		t.Error("Name variable not substituted")
	}
	if !strings.Contains(output, "http.MethodGet") {
		t.Error("Method variable not substituted")
	}
	if !strings.Contains(output, "/custom/path") {
		t.Error("Path variable not substituted")
	}
}

func TestMissingRequiredVar(t *testing.T) {
	cg := NewCodeGenerator()

	// go-handler requires Name (no default)
	vars := map[string]string{
		"Package": "api",
		// Name is missing and has no default
	}
	_, err := cg.Generate("go-handler", vars)
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
	if !strings.Contains(err.Error(), "required variable") {
		t.Errorf("expected 'required variable' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Name") {
		t.Errorf("expected variable name 'Name' in error, got: %v", err)
	}
}

func TestDefaultValuesApplied(t *testing.T) {
	cg := NewCodeGenerator()

	// go-handler has Method default "Post" and Path default "/api/resource"
	vars := map[string]string{
		"Package": "api",
		"Name":    "Create",
	}
	output, err := cg.Generate("go-handler", vars)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(output, "http.MethodPost") {
		t.Error("expected default Method 'Post' to be applied")
	}
	if !strings.Contains(output, "/api/resource") {
		t.Error("expected default Path '/api/resource' to be applied")
	}
}

func TestTemplateNotFound(t *testing.T) {
	cg := NewCodeGenerator()
	_, err := cg.Generate("nonexistent-template", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestSuggestTemplate(t *testing.T) {
	cg := NewCodeGenerator()

	tests := []struct {
		description string
		expected    string
	}{
		{"create an API endpoint", "go-handler"},
		{"I need a handler for HTTP requests", "go-handler"},
		{"add tests for my function", "go-test-table"},
		{"need middleware for authentication", "go-middleware"},
		{"CRUD operations for user resource", "go-crud"},
		{"custom error type for validation", "go-errors"},
		{"configuration loading from environment", "go-config"},
		{"interface with mock", "go-interface"},
		{"FastAPI endpoint with pydantic", "py-fastapi-endpoint"},
		{"pytest class", "py-test-class"},
		{"python dataclass model", "py-dataclass"},
		{"react component", "ts-react-component"},
		{"express router for API", "ts-express-router"},
		{"vitest describe block", "ts-test-describe"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := cg.SuggestTemplate(tt.description)
			if result != tt.expected {
				t.Errorf("SuggestTemplate(%q) = %q, want %q", tt.description, result, tt.expected)
			}
		})
	}
}

func TestListTemplatesByLanguage(t *testing.T) {
	cg := NewCodeGenerator()

	goTemplates := cg.ListTemplates("go")
	if len(goTemplates) != 7 {
		t.Errorf("expected 7 Go templates, got %d", len(goTemplates))
	}
	for _, tmpl := range goTemplates {
		if tmpl.Language != "go" {
			t.Errorf("expected language 'go', got %q for template %q", tmpl.Language, tmpl.Name)
		}
	}

	pyTemplates := cg.ListTemplates("python")
	if len(pyTemplates) != 3 {
		t.Errorf("expected 3 Python templates, got %d", len(pyTemplates))
	}

	tsTemplates := cg.ListTemplates("typescript")
	if len(tsTemplates) != 3 {
		t.Errorf("expected 3 TypeScript templates, got %d", len(tsTemplates))
	}

	allTemplates := cg.ListTemplates("")
	if len(allTemplates) != 13 {
		t.Errorf("expected 13 total templates, got %d", len(allTemplates))
	}
}

func TestCustomTemplateRegistration(t *testing.T) {
	cg := NewCodeGenerator()

	custom := &CodeTemplate{
		Name:        "custom-template",
		Description: "A custom template for testing",
		Language:    "go",
		Template:    "package {{.Package}}\n\n// {{.Name}} is custom.\ntype {{.Name}} struct{}\n",
		Variables: []TemplateVar{
			{Name: "Package", Description: "Package name", Required: true, Default: ""},
			{Name: "Name", Description: "Type name", Required: true, Default: ""},
		},
		Output: "{{.Name | lower}}.go",
	}

	cg.Register(custom)

	// Verify it's in the template list
	templates := cg.ListTemplates("go")
	found := false
	for _, tmpl := range templates {
		if tmpl.Name == "custom-template" {
			found = true
			break
		}
	}
	if !found {
		t.Error("custom template not found after registration")
	}

	// Generate from it
	output, err := cg.Generate("custom-template", map[string]string{
		"Package": "mylib",
		"Name":    "Widget",
	})
	if err != nil {
		t.Fatalf("Generate custom template: %v", err)
	}
	if !strings.Contains(output, "package mylib") {
		t.Error("expected custom template output to contain package")
	}
	if !strings.Contains(output, "type Widget struct{}") {
		t.Error("expected custom template output to contain type")
	}
}

func TestPreviewOutput(t *testing.T) {
	cg := NewCodeGenerator()

	vars := map[string]string{
		"Package": "handlers",
		"Name":    "GetUser",
	}
	preview := cg.Preview("go-handler", vars)

	if !strings.Contains(preview, "Template: go-handler") {
		t.Error("expected template name in preview")
	}
	if !strings.Contains(preview, "Language: go") {
		t.Error("expected language in preview")
	}
	if !strings.Contains(preview, "Variables:") {
		t.Error("expected variables section in preview")
	}
	if !strings.Contains(preview, "--- Generated Preview ---") {
		t.Error("expected preview separator")
	}
	if !strings.Contains(preview, "GetUserHandler") {
		t.Error("expected rendered code in preview")
	}
}

func TestPreviewNotFound(t *testing.T) {
	cg := NewCodeGenerator()
	preview := cg.Preview("nonexistent", nil)
	if !strings.Contains(preview, "not found") {
		t.Error("expected 'not found' in preview for missing template")
	}
}

func TestPreviewWithMissingRequired(t *testing.T) {
	cg := NewCodeGenerator()
	// Missing Name which is required
	preview := cg.Preview("go-handler", map[string]string{
		"Package": "api",
	})
	if !strings.Contains(preview, "Error:") {
		t.Error("expected error in preview when required var is missing")
	}
}

func TestCodeGenToolInterface(t *testing.T) {
	tool := NewCodeGenTool()

	if tool.Name() != "CodeGen" {
		t.Errorf("expected Name() = 'CodeGen', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
	params := tool.Parameters()
	if params == nil {
		t.Fatal("expected non-nil parameters")
	}
}

func TestCodeGenToolGenerate(t *testing.T) {
	tool := NewCodeGenTool()
	ctx := context.Background()

	input := codeGenInput{
		Action:   "generate",
		Template: "go-handler",
		Variables: map[string]string{
			"Package": "api",
			"Name":    "Health",
			"Method":  "Get",
		},
	}
	data, _ := json.Marshal(input)
	output, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("Execute generate: %v", err)
	}
	if !strings.Contains(output, "HealthHandler") {
		t.Error("expected generated handler in output")
	}
}

func TestCodeGenToolList(t *testing.T) {
	tool := NewCodeGenTool()
	ctx := context.Background()

	input := codeGenInput{
		Action:   "list",
		Language: "go",
	}
	data, _ := json.Marshal(input)
	output, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("Execute list: %v", err)
	}
	if !strings.Contains(output, "go-handler") {
		t.Error("expected go-handler in list output")
	}
	if !strings.Contains(output, "go-crud") {
		t.Error("expected go-crud in list output")
	}
}

func TestCodeGenToolPreview(t *testing.T) {
	tool := NewCodeGenTool()
	ctx := context.Background()

	input := codeGenInput{
		Action:   "preview",
		Template: "go-middleware",
		Variables: map[string]string{
			"Package": "mw",
			"Name":    "Logger",
		},
	}
	data, _ := json.Marshal(input)
	output, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("Execute preview: %v", err)
	}
	if !strings.Contains(output, "Template: go-middleware") {
		t.Error("expected template info in preview")
	}
}

func TestCodeGenToolSuggest(t *testing.T) {
	tool := NewCodeGenTool()
	ctx := context.Background()

	input := codeGenInput{
		Action:      "suggest",
		Description: "I need middleware for logging",
	}
	data, _ := json.Marshal(input)
	output, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("Execute suggest: %v", err)
	}
	if !strings.Contains(output, "go-middleware") {
		t.Errorf("expected suggestion 'go-middleware', got: %s", output)
	}
}

func TestCodeGenToolErrors(t *testing.T) {
	tool := NewCodeGenTool()
	ctx := context.Background()

	tests := []struct {
		name  string
		input codeGenInput
		errIn string
	}{
		{
			name:  "unknown action",
			input: codeGenInput{Action: "unknown"},
			errIn: "unknown action",
		},
		{
			name:  "generate without template",
			input: codeGenInput{Action: "generate"},
			errIn: "template name is required",
		},
		{
			name:  "preview without template",
			input: codeGenInput{Action: "preview"},
			errIn: "template name is required",
		},
		{
			name:  "suggest without description",
			input: codeGenInput{Action: "suggest"},
			errIn: "description is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := json.Marshal(tt.input)
			_, err := tool.Execute(ctx, data)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.errIn) {
				t.Errorf("expected error containing %q, got: %v", tt.errIn, err)
			}
		})
	}
}

func TestCodeGenToolInvalidJSON(t *testing.T) {
	tool := NewCodeGenTool()
	ctx := context.Background()

	_, err := tool.Execute(ctx, json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestPythonTemplates(t *testing.T) {
	cg := NewCodeGenerator()

	t.Run("fastapi-endpoint", func(t *testing.T) {
		output, err := cg.Generate("py-fastapi-endpoint", map[string]string{
			"Name":      "Product",
			"NameLower": "product",
			"Prefix":    "products",
			"Tag":       "products",
		})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.Contains(output, "from fastapi import") {
			t.Error("expected fastapi import")
		}
		if !strings.Contains(output, "class ProductRequest(BaseModel)") {
			t.Error("expected request model")
		}
		if !strings.Contains(output, "async def create_product") {
			t.Error("expected create endpoint")
		}
	})

	t.Run("test-class", func(t *testing.T) {
		output, err := cg.Generate("py-test-class", map[string]string{
			"Name":            "Parser",
			"MethodUnderTest": "parse",
		})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.Contains(output, "class TestParser") {
			t.Error("expected test class")
		}
		if !strings.Contains(output, "def test_parse_with_valid_input") {
			t.Error("expected test method")
		}
		if !strings.Contains(output, "def setup_method") {
			t.Error("expected setup_method")
		}
	})

	t.Run("dataclass", func(t *testing.T) {
		output, err := cg.Generate("py-dataclass", map[string]string{
			"Name":        "Config",
			"Description": "Application configuration",
		})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.Contains(output, "@dataclass") {
			t.Error("expected @dataclass decorator")
		}
		if !strings.Contains(output, "class Config") {
			t.Error("expected class name")
		}
		if !strings.Contains(output, "__post_init__") {
			t.Error("expected validation in __post_init__")
		}
	})
}

func TestTypeScriptTemplates(t *testing.T) {
	cg := NewCodeGenerator()

	t.Run("react-component", func(t *testing.T) {
		output, err := cg.Generate("ts-react-component", map[string]string{
			"Name":        "Button",
			"Description": "A clickable button",
		})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.Contains(output, "interface ButtonProps") {
			t.Error("expected props interface")
		}
		if !strings.Contains(output, "React.FC<ButtonProps>") {
			t.Error("expected functional component type")
		}
		if !strings.Contains(output, "export const Button") {
			t.Error("expected named export")
		}
	})

	t.Run("express-router", func(t *testing.T) {
		output, err := cg.Generate("ts-express-router", map[string]string{
			"Name": "User",
			"Path": "users",
		})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.Contains(output, "import { Router") {
			t.Error("expected Router import")
		}
		if !strings.Contains(output, "validateUser") {
			t.Error("expected validation middleware")
		}
		if !strings.Contains(output, "router.get('/'") {
			t.Error("expected GET route")
		}
		if !strings.Contains(output, "router.post('/'") {
			t.Error("expected POST route")
		}
	})

	t.Run("test-describe", func(t *testing.T) {
		output, err := cg.Generate("ts-test-describe", map[string]string{
			"Name":   "Calculator",
			"Method": "add",
		})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if !strings.Contains(output, "describe('Calculator'") {
			t.Error("expected describe block")
		}
		if !strings.Contains(output, "describe('add'") {
			t.Error("expected nested describe for method")
		}
		if !strings.Contains(output, "beforeEach") {
			t.Error("expected beforeEach")
		}
	})
}

func TestConcurrentAccess(t *testing.T) {
	cg := NewCodeGenerator()

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = cg.Generate("go-handler", map[string]string{
				"Package": "api",
				"Name":    "Test",
			})
			cg.ListTemplates("go")
			cg.Register(&CodeTemplate{
				Name:     "concurrent-test",
				Language: "go",
				Template: "package main",
			})
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
