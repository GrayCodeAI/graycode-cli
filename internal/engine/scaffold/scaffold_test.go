package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewScaffolder(t *testing.T) {
	s := NewScaffolder()
	if s == nil {
		t.Fatal("NewScaffolder returned nil")
	}
	if len(s.Templates) == 0 {
		t.Fatal("NewScaffolder should have built-in templates")
	}
}

func TestBuiltinTemplatesValid(t *testing.T) {
	s := NewScaffolder()
	expectedTemplates := []string{"go-cli", "go-api", "go-lib", "ts-api", "python-api", "python-cli"}

	for _, name := range expectedTemplates {
		tmpl, ok := s.Templates[name]
		if !ok {
			t.Errorf("built-in template %q not found", name)
			continue
		}
		if tmpl.Name == "" {
			t.Errorf("template %q has empty name", name)
		}
		if tmpl.Description == "" {
			t.Errorf("template %q has empty description", name)
		}
		if tmpl.Language == "" {
			t.Errorf("template %q has empty language", name)
		}
		if len(tmpl.Files) == 0 {
			t.Errorf("template %q has no files", name)
		}
		if len(tmpl.Variables) == 0 {
			t.Errorf("template %q has no variables", name)
		}
		// Check all files have paths
		for i, f := range tmpl.Files {
			if f.Path == "" {
				t.Errorf("template %q file %d has empty path", name, i)
			}
		}
	}
}

func TestGenerateCreatesAllFiles(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	vars := map[string]string{
		"ProjectName": "myapp",
		"Module":      "github.com/test/myapp",
	}

	err := s.Generate("go-cli", vars, outputDir)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	expectedFiles := []string{
		"myapp/cmd/main.go",
		"myapp/internal/cmd/root.go",
		"myapp/go.mod",
		"myapp/Makefile",
		"myapp/.gitignore",
		"myapp/README.md",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(outputDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %q was not created", f)
		}
	}
}

func TestGenerateVariableSubstitution(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	vars := map[string]string{
		"ProjectName": "testproj",
		"Module":      "github.com/user/testproj",
		"Author":      "Jane Smith",
	}

	err := s.Generate("go-cli", vars, outputDir)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Check main.go contains the module path
	mainContent, err := os.ReadFile(filepath.Join(outputDir, "testproj/cmd/main.go"))
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	if !strings.Contains(string(mainContent), "github.com/user/testproj/internal/cmd") {
		t.Error("main.go does not contain expected module import")
	}

	// Check README contains the author
	readmeContent, err := os.ReadFile(filepath.Join(outputDir, "testproj/README.md"))
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	if !strings.Contains(string(readmeContent), "Jane Smith") {
		t.Error("README.md does not contain author name")
	}

	// Check go.mod contains module
	modContent, err := os.ReadFile(filepath.Join(outputDir, "testproj/go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	if !strings.Contains(string(modContent), "github.com/user/testproj") {
		t.Error("go.mod does not contain module path")
	}
}

func TestGenerateConditionEvaluation(t *testing.T) {
	s := NewScaffolder()

	// Test with condition true
	outputDir1 := t.TempDir()
	vars := map[string]string{
		"ProjectName": "myapi",
		"Module":      "github.com/test/myapi",
		"WithDocker":  "true",
	}

	err := s.Generate("go-api", vars, outputDir1)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	dockerPath := filepath.Join(outputDir1, "myapi/Dockerfile")
	if _, statErr := os.Stat(dockerPath); os.IsNotExist(statErr) {
		t.Error("Dockerfile should be created when WithDocker is true")
	}

	// Test with condition false
	outputDir2 := t.TempDir()
	vars["WithDocker"] = "false"

	err = s.Generate("go-api", vars, outputDir2)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	dockerPath2 := filepath.Join(outputDir2, "myapi/Dockerfile")
	if _, err := os.Stat(dockerPath2); !os.IsNotExist(err) {
		t.Error("Dockerfile should NOT be created when WithDocker is false")
	}
}

func TestGenerateTemplateNotFound(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	err := s.Generate("nonexistent", nil, outputDir)
	if err == nil {
		t.Fatal("Generate should fail for unknown template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestPreviewOutput(t *testing.T) {
	s := NewScaffolder()

	vars := map[string]string{
		"ProjectName": "myapp",
		"Module":      "github.com/test/myapp",
	}

	result := s.Preview("go-cli", vars)

	if !strings.Contains(result, "Would create:") {
		t.Error("Preview should start with 'Would create:'")
	}
	if !strings.Contains(result, "myapp/") {
		t.Error("Preview should contain project directory")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("Preview should contain main.go")
	}
	if !strings.Contains(result, "files") {
		t.Error("Preview should contain file count")
	}
	if !strings.Contains(result, "directories") {
		t.Error("Preview should contain directory count")
	}
}

func TestPreviewTemplateNotFound(t *testing.T) {
	s := NewScaffolder()
	result := s.Preview("nonexistent", nil)
	if !strings.Contains(result, "not found") {
		t.Error("Preview should indicate template not found")
	}
}

func TestRenderTree(t *testing.T) {
	files := []string{
		"myapp/cmd/main.go",
		"myapp/internal/handler/handler.go",
		"myapp/go.mod",
		"myapp/README.md",
	}

	result := RenderTree(files)

	if !strings.Contains(result, "myapp/") {
		t.Error("Tree should contain root directory")
	}
	if !strings.Contains(result, "cmd/") {
		t.Error("Tree should contain cmd directory")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("Tree should contain main.go")
	}
	if !strings.Contains(result, "├") || !strings.Contains(result, "└") {
		t.Error("Tree should contain tree drawing characters")
	}
}

func TestRenderTreeEmpty(t *testing.T) {
	result := RenderTree(nil)
	if result != "" {
		t.Error("RenderTree with nil should return empty string")
	}

	result = RenderTree([]string{})
	if result != "" {
		t.Error("RenderTree with empty slice should return empty string")
	}
}

func TestValidateVarsMissingRequired(t *testing.T) {
	s := NewScaffolder()
	tmpl := s.Templates["go-cli"]

	// No vars provided
	errors := s.ValidateVars(tmpl, map[string]string{})
	if len(errors) == 0 {
		t.Error("ValidateVars should report errors for missing required vars")
	}

	// Check that ProjectName and Module are reported
	found := false
	for _, e := range errors {
		if strings.Contains(e, "ProjectName") {
			found = true
			break
		}
	}
	if !found {
		t.Error("ValidateVars should report missing ProjectName")
	}
}

func TestValidateVarsInvalidChoice(t *testing.T) {
	s := NewScaffolder()
	tmpl := s.Templates["go-cli"]

	vars := map[string]string{
		"ProjectName": "test",
		"Module":      "github.com/test/test",
		"License":     "INVALID",
	}

	errors := s.ValidateVars(tmpl, vars)
	found := false
	for _, e := range errors {
		if strings.Contains(e, "License") && strings.Contains(e, "not a valid choice") {
			found = true
			break
		}
	}
	if !found {
		t.Error("ValidateVars should report invalid choice for License")
	}
}

func TestValidateVarsValidInput(t *testing.T) {
	s := NewScaffolder()
	tmpl := s.Templates["go-cli"]

	vars := map[string]string{
		"ProjectName": "test",
		"Module":      "github.com/test/test",
		"License":     "MIT",
	}

	errors := s.ValidateVars(tmpl, vars)
	if len(errors) != 0 {
		t.Errorf("ValidateVars should report no errors for valid input, got: %v", errors)
	}
}

func TestListTemplates(t *testing.T) {
	s := NewScaffolder()
	templates := s.ListTemplates()

	if len(templates) != 6 {
		t.Errorf("expected 6 built-in templates, got %d", len(templates))
	}

	// Should be sorted
	for i := 1; i < len(templates); i++ {
		if templates[i].Name < templates[i-1].Name {
			t.Error("ListTemplates should return templates sorted by name")
			break
		}
	}
}

func TestRegisterTemplate(t *testing.T) {
	s := NewScaffolder()

	custom := &Template{
		Name:        "custom",
		Description: "Custom template",
		Language:    "rust",
		Files: []TemplateFile{
			{Path: "{{.ProjectName}}/main.rs", Content: "fn main() {}", Mode: 0o644},
		},
		Variables: []TemplateVariable{
			{Name: "ProjectName", Required: true, Type: "string"},
		},
	}

	s.RegisterTemplate(custom)

	if _, ok := s.Templates["custom"]; !ok {
		t.Error("RegisterTemplate should add template")
	}

	// Generate should work with custom template
	outputDir := t.TempDir()
	err := s.Generate("custom", map[string]string{"ProjectName": "myrust"}, outputDir)
	if err != nil {
		t.Fatalf("Generate with custom template failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "myrust/main.rs"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if string(content) != "fn main() {}" {
		t.Errorf("unexpected content: %s", string(content))
	}
}

func TestLoadTemplateFromJSON(t *testing.T) {
	s := NewScaffolder()

	tmpl := Template{
		Name:        "json-test",
		Description: "Test template from JSON",
		Language:    "go",
		Framework:   "custom",
		Files: []TemplateFile{
			{Path: "{{.ProjectName}}/main.go", Content: "package main\n", Mode: 0o644},
		},
		Variables: []TemplateVariable{
			{Name: "ProjectName", Description: "Project name", Required: true, Type: "string"},
		},
		PostCreate: []string{"echo done"},
	}

	// Write template to a temp JSON file
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "template.json")
	data, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		t.Fatalf("marshaling template: %v", err)
	}
	if writeErr := os.WriteFile(jsonPath, data, 0o644); writeErr != nil {
		t.Fatalf("writing template file: %v", writeErr)
	}

	// Load it
	loaded, err := s.LoadTemplate(jsonPath)
	if err != nil {
		t.Fatalf("LoadTemplate failed: %v", err)
	}

	if loaded.Name != "json-test" {
		t.Errorf("expected name %q, got %q", "json-test", loaded.Name)
	}
	if loaded.Description != "Test template from JSON" {
		t.Errorf("unexpected description: %s", loaded.Description)
	}
	if len(loaded.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(loaded.Files))
	}
	if len(loaded.Variables) != 1 {
		t.Errorf("expected 1 variable, got %d", len(loaded.Variables))
	}

	// Should be registered
	if _, ok := s.Templates["json-test"]; !ok {
		t.Error("LoadTemplate should register the template")
	}

	// Generate should work
	outputDir := t.TempDir()
	err = s.Generate("json-test", map[string]string{"ProjectName": "loaded"}, outputDir)
	if err != nil {
		t.Fatalf("Generate with loaded template failed: %v", err)
	}
}

func TestLoadTemplateInvalidJSON(t *testing.T) {
	s := NewScaffolder()

	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(jsonPath, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := s.LoadTemplate(jsonPath)
	if err == nil {
		t.Error("LoadTemplate should fail for invalid JSON")
	}
}

func TestLoadTemplateMissingName(t *testing.T) {
	s := NewScaffolder()

	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "noname.json")
	data := `{"description": "no name template"}`
	if err := os.WriteFile(jsonPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := s.LoadTemplate(jsonPath)
	if err == nil {
		t.Error("LoadTemplate should fail when name is empty")
	}
}

func TestLoadTemplateFileNotFound(t *testing.T) {
	s := NewScaffolder()

	_, err := s.LoadTemplate("/nonexistent/path/template.json")
	if err == nil {
		t.Error("LoadTemplate should fail for non-existent file")
	}
}

func TestGenerateGoAPITemplate(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	vars := map[string]string{
		"ProjectName": "svc",
		"Module":      "github.com/test/svc",
		"Port":        "9090",
		"WithDocker":  "true",
	}

	err := s.Generate("go-api", vars, outputDir)
	if err != nil {
		t.Fatalf("Generate go-api failed: %v", err)
	}

	// Check handler file
	content, err := os.ReadFile(filepath.Join(outputDir, "svc/internal/handler/handler.go"))
	if err != nil {
		t.Fatalf("reading handler.go: %v", err)
	}
	if !strings.Contains(string(content), "package handler") {
		t.Error("handler.go should have correct package")
	}

	// Check Dockerfile has correct port
	content, err = os.ReadFile(filepath.Join(outputDir, "svc/Dockerfile"))
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	if !strings.Contains(string(content), "9090") {
		t.Error("Dockerfile should contain port 9090")
	}
}

func TestGenerateTSAPITemplate(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	vars := map[string]string{
		"ProjectName": "tsapp",
		"Port":        "4000",
		"WithDocker":  "true",
	}

	err := s.Generate("ts-api", vars, outputDir)
	if err != nil {
		t.Fatalf("Generate ts-api failed: %v", err)
	}

	// Check package.json
	content, err := os.ReadFile(filepath.Join(outputDir, "tsapp/package.json"))
	if err != nil {
		t.Fatalf("reading package.json: %v", err)
	}
	if !strings.Contains(string(content), `"name": "tsapp"`) {
		t.Error("package.json should contain project name")
	}

	// Check tsconfig.json exists
	if _, err := os.Stat(filepath.Join(outputDir, "tsapp/tsconfig.json")); os.IsNotExist(err) {
		t.Error("tsconfig.json should be created")
	}
}

func TestGeneratePythonAPITemplate(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	vars := map[string]string{
		"ProjectName": "pyapi",
		"Port":        "5000",
		"WithDocker":  "true",
	}

	err := s.Generate("python-api", vars, outputDir)
	if err != nil {
		t.Fatalf("Generate python-api failed: %v", err)
	}

	// Check main.py
	content, err := os.ReadFile(filepath.Join(outputDir, "pyapi/app/main.py"))
	if err != nil {
		t.Fatalf("reading main.py: %v", err)
	}
	if !strings.Contains(string(content), "FastAPI") {
		t.Error("main.py should contain FastAPI")
	}
	if !strings.Contains(string(content), "pyapi") {
		t.Error("main.py should contain project name")
	}
}

func TestGeneratePythonCLITemplate(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	vars := map[string]string{
		"ProjectName": "mycli",
		"Author":      "Test Author",
		"WithTests":   "true",
	}

	err := s.Generate("python-cli", vars, outputDir)
	if err != nil {
		t.Fatalf("Generate python-cli failed: %v", err)
	}

	// Check cli.py
	content, err := os.ReadFile(filepath.Join(outputDir, "mycli/mycli/cli.py"))
	if err != nil {
		t.Fatalf("reading cli.py: %v", err)
	}
	if !strings.Contains(string(content), "click") {
		t.Error("cli.py should contain click")
	}

	// Check tests are created
	if _, statErr := os.Stat(filepath.Join(outputDir, "mycli/tests/test_cli.py")); os.IsNotExist(statErr) {
		t.Error("test_cli.py should be created when WithTests is true")
	}

	// Check setup.py has author
	content, err = os.ReadFile(filepath.Join(outputDir, "mycli/setup.py"))
	if err != nil {
		t.Fatalf("reading setup.py: %v", err)
	}
	if !strings.Contains(string(content), "Test Author") {
		t.Error("setup.py should contain author")
	}
}

func TestGeneratePythonCLIWithoutTests(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	vars := map[string]string{
		"ProjectName": "mycli",
		"WithTests":   "false",
	}

	err := s.Generate("python-cli", vars, outputDir)
	if err != nil {
		t.Fatalf("Generate python-cli failed: %v", err)
	}

	// Tests should not be created
	if _, err := os.Stat(filepath.Join(outputDir, "mycli/tests/test_cli.py")); !os.IsNotExist(err) {
		t.Error("test_cli.py should NOT be created when WithTests is false")
	}
}

func TestGenerateGoLibTemplate(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	vars := map[string]string{
		"ProjectName": "mylib",
		"Module":      "github.com/test/mylib",
		"PackageName": "mylib",
		"WithCI":      "true",
	}

	err := s.Generate("go-lib", vars, outputDir)
	if err != nil {
		t.Fatalf("Generate go-lib failed: %v", err)
	}

	// Check library file
	content, err := os.ReadFile(filepath.Join(outputDir, "mylib/mylib.go"))
	if err != nil {
		t.Fatalf("reading mylib.go: %v", err)
	}
	if !strings.Contains(string(content), "package mylib") {
		t.Error("library file should have correct package name")
	}

	// Check test file
	if _, err := os.Stat(filepath.Join(outputDir, "mylib/mylib_test.go")); os.IsNotExist(err) {
		t.Error("test file should be created")
	}

	// Check CI file
	if _, err := os.Stat(filepath.Join(outputDir, "mylib/.github/workflows/ci.yml")); os.IsNotExist(err) {
		t.Error("CI config should be created when WithCI is true")
	}
}

func TestGenerateDefaultValues(t *testing.T) {
	s := NewScaffolder()
	outputDir := t.TempDir()

	// Only provide required vars, let defaults fill in
	vars := map[string]string{
		"ProjectName": "myapp",
		"Module":      "github.com/test/myapp",
	}

	err := s.Generate("go-cli", vars, outputDir)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Author should default to "Developer"
	content, err := os.ReadFile(filepath.Join(outputDir, "myapp/README.md"))
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	if !strings.Contains(string(content), "Developer") {
		t.Error("README should contain default author 'Developer'")
	}
}

func TestScaffoldEvalCondition(t *testing.T) {
	tests := []struct {
		condition string
		vars      map[string]string
		expected  bool
	}{
		{"{{.WithDocker}}", map[string]string{"WithDocker": "true"}, true},
		{"{{.WithDocker}}", map[string]string{"WithDocker": "false"}, false},
		{"{{.WithDocker}}", map[string]string{"WithDocker": "yes"}, true},
		{"{{.WithDocker}}", map[string]string{"WithDocker": "1"}, true},
		{"{{.WithDocker}}", map[string]string{"WithDocker": "0"}, false},
		{"{{.WithDocker}}", map[string]string{"WithDocker": "TRUE"}, true},
	}

	for _, tc := range tests {
		result, err := evalCondition(tc.condition, tc.vars)
		if err != nil {
			t.Errorf("evalCondition(%q, %v) error: %v", tc.condition, tc.vars, err)
			continue
		}
		if result != tc.expected {
			t.Errorf("evalCondition(%q, %v) = %v, want %v", tc.condition, tc.vars, result, tc.expected)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		tmpl     string
		vars     map[string]string
		expected string
	}{
		{"Hello {{.Name}}", map[string]string{"Name": "World"}, "Hello World"},
		{"{{.A}}/{{.B}}", map[string]string{"A": "x", "B": "y"}, "x/y"},
		{"no vars", map[string]string{}, "no vars"},
	}

	for _, tc := range tests {
		result, err := renderTemplate(tc.tmpl, tc.vars)
		if err != nil {
			t.Errorf("renderTemplate(%q) error: %v", tc.tmpl, err)
			continue
		}
		if result != tc.expected {
			t.Errorf("renderTemplate(%q) = %q, want %q", tc.tmpl, result, tc.expected)
		}
	}
}

func TestRenderTemplateError(t *testing.T) {
	_, err := renderTemplate("{{.Missing}}", map[string]string{})
	if err == nil {
		t.Error("renderTemplate should fail for missing variable")
	}
}

func TestFileMode(t *testing.T) {
	s := NewScaffolder()

	// Register a template with specific mode
	s.RegisterTemplate(&Template{
		Name:     "mode-test",
		Language: "bash",
		Files: []TemplateFile{
			{Path: "{{.ProjectName}}/script.sh", Content: "#!/bin/bash\necho hello\n", Mode: 0o755},
		},
		Variables: []TemplateVariable{
			{Name: "ProjectName", Required: true, Type: "string"},
		},
	})

	outputDir := t.TempDir()
	err := s.Generate("mode-test", map[string]string{"ProjectName": "test"}, outputDir)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(outputDir, "test/script.sh"))
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	// Check executable bit is set (at minimum)
	if info.Mode()&0o100 == 0 {
		t.Errorf("script.sh should be executable, got mode %o", info.Mode())
	}
}

func TestScaffoldConcurrentAccess(t *testing.T) {
	s := NewScaffolder()

	done := make(chan bool, 10)

	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func() {
			_ = s.ListTemplates()
			done <- true
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func(n int) {
			s.RegisterTemplate(&Template{
				Name:     fmt.Sprintf("concurrent-%d", n),
				Language: "test",
			})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestPreviewWithConditions(t *testing.T) {
	s := NewScaffolder()

	// With docker
	vars1 := map[string]string{
		"ProjectName": "myapi",
		"Module":      "github.com/test/myapi",
		"WithDocker":  "true",
	}
	result1 := s.Preview("go-api", vars1)
	if !strings.Contains(result1, "Dockerfile") {
		t.Error("Preview with WithDocker=true should show Dockerfile")
	}

	// Without docker
	vars2 := map[string]string{
		"ProjectName": "myapi",
		"Module":      "github.com/test/myapi",
		"WithDocker":  "false",
	}
	result2 := s.Preview("go-api", vars2)
	if strings.Contains(result2, "Dockerfile") {
		t.Error("Preview with WithDocker=false should NOT show Dockerfile")
	}
}
