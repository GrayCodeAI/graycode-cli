package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewTemplateRegistry(t *testing.T) {
	r := NewTemplateRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}

	expectedTemplates := []string{
		"go-default", "go-api", "ts-react", "ts-api",
		"python-api", "python-ml", "rust-cli", "monorepo",
	}

	for _, name := range expectedTemplates {
		if _, ok := r.Templates[name]; !ok {
			t.Errorf("expected built-in template %q", name)
		}
	}
}

func TestTemplateRegistry_List(t *testing.T) {
	r := NewTemplateRegistry()
	templates := r.List()

	if len(templates) != 8 {
		t.Fatalf("expected 8 templates, got %d", len(templates))
	}

	// Verify sorted order
	for i := 1; i < len(templates); i++ {
		if templates[i].Name < templates[i-1].Name {
			t.Errorf("templates not sorted: %q before %q", templates[i-1].Name, templates[i].Name)
		}
	}
}

func TestTemplateRegistry_Register(t *testing.T) {
	r := NewTemplateRegistry()
	custom := &ConfigTemplate{
		Name:        "custom-project",
		Description: "Custom template",
		Language:    "zig",
		Framework:   "",
		Content:     map[string]interface{}{"model": "opus"},
		Files: map[string]string{
			".hawk/settings.json": `{"model": "{{model}}"}`,
		},
		Tags: []string{"zig", "custom"},
	}

	r.Register(custom)

	if _, ok := r.Templates["custom-project"]; !ok {
		t.Fatal("expected custom template to be registered")
	}

	templates := r.List()
	if len(templates) != 9 {
		t.Fatalf("expected 9 templates after register, got %d", len(templates))
	}
}

func TestTemplateRegistry_Generate(t *testing.T) {
	r := NewTemplateRegistry()
	vars := map[string]string{
		"project_name": "myapp",
		"model":        "opus",
		"framework":    "gin",
	}

	files, err := r.Generate("go-api", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("expected generated files")
	}

	// Check variable substitution
	settings, ok := files[".hawk/settings.json"]
	if !ok {
		t.Fatal("expected .hawk/settings.json in output")
	}
	if !strings.Contains(settings, "opus") {
		t.Error("expected model variable to be substituted")
	}
	if strings.Contains(settings, "{{model}}") {
		t.Error("expected {{model}} placeholder to be replaced")
	}

	agents, ok := files["AGENTS.md"]
	if !ok {
		t.Fatal("expected AGENTS.md in output")
	}
	if !strings.Contains(agents, "myapp") {
		t.Error("expected project_name to be substituted in AGENTS.md")
	}
	if !strings.Contains(agents, "gin") {
		t.Error("expected framework to be substituted in AGENTS.md")
	}
}

func TestTemplateRegistry_Generate_NotFound(t *testing.T) {
	r := NewTemplateRegistry()
	_, err := r.Generate("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestTemplateRegistry_DetectAndGenerate_Go(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644)

	r := NewTemplateRegistry()
	files, err := r.DetectAndGenerate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := files[".hawk/settings.json"]; !ok {
		t.Error("expected .hawk/settings.json")
	}
	if _, ok := files["AGENTS.md"]; !ok {
		t.Error("expected AGENTS.md")
	}
}

func TestTemplateRegistry_DetectAndGenerate_GoAPI(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/api\n\ngo 1.21\n\nrequire github.com/go-chi/chi v5.0.0\n"), 0o644)

	r := NewTemplateRegistry()
	files, err := r.DetectAndGenerate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agents := files["AGENTS.md"]
	if !strings.Contains(agents, "API") {
		t.Error("expected API-specific AGENTS.md for go-api template")
	}
}

func TestTemplateRegistry_DetectAndGenerate_TypeScriptReact(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"^18.0.0"}}`), 0o644)

	r := NewTemplateRegistry()
	files, err := r.DetectAndGenerate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agents := files["AGENTS.md"]
	if !strings.Contains(agents, "React") {
		t.Error("expected React-specific AGENTS.md")
	}
}

func TestTemplateRegistry_DetectAndGenerate_TypeScriptAPI(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"express":"^4.18.0"}}`), 0o644)

	r := NewTemplateRegistry()
	files, err := r.DetectAndGenerate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agents := files["AGENTS.md"]
	if !strings.Contains(agents, "API") {
		t.Error("expected API-specific AGENTS.md for ts-api template")
	}
}

func TestTemplateRegistry_DetectAndGenerate_Python(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`[project]\nname = "myapp"\n[project.dependencies]\nfastapi = ">=0.100"\n`), 0o644)

	r := NewTemplateRegistry()
	files, err := r.DetectAndGenerate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agents := files["AGENTS.md"]
	if !strings.Contains(agents, "Python") {
		t.Error("expected Python AGENTS.md")
	}
}

func TestTemplateRegistry_DetectAndGenerate_PythonML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("torch>=2.0\nnumpy\npandas\n"), 0o644)

	r := NewTemplateRegistry()
	files, err := r.DetectAndGenerate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agents := files["AGENTS.md"]
	if !strings.Contains(agents, "ML") {
		t.Error("expected ML-specific AGENTS.md")
	}
}

func TestTemplateRegistry_DetectAndGenerate_Rust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"mycli\"\nversion = \"0.1.0\"\n"), 0o644)

	r := NewTemplateRegistry()
	files, err := r.DetectAndGenerate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agents := files["AGENTS.md"]
	if !strings.Contains(agents, "Rust") {
		t.Error("expected Rust AGENTS.md")
	}
}

func TestTemplateRegistry_DetectAndGenerate_Monorepo(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "packages", "core"), 0o755)
	os.MkdirAll(filepath.Join(dir, "packages", "web"), 0o755)
	os.WriteFile(filepath.Join(dir, "pnpm-workspace.yaml"), []byte("packages:\n  - packages/*\n"), 0o644)

	r := NewTemplateRegistry()
	files, err := r.DetectAndGenerate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agents := files["AGENTS.md"]
	if !strings.Contains(agents, "packages/core") {
		t.Error("expected detected packages in AGENTS.md")
	}
	if !strings.Contains(agents, "packages/web") {
		t.Error("expected detected packages in AGENTS.md")
	}
}

func TestTemplateRegistry_DetectAndGenerate_Unknown(t *testing.T) {
	dir := t.TempDir()
	// Empty directory - no recognizable project files

	r := NewTemplateRegistry()
	_, err := r.DetectAndGenerate(dir)
	if err == nil {
		t.Fatal("expected error for undetectable project")
	}
	if !strings.Contains(err.Error(), "could not detect") {
		t.Errorf("expected detection error, got: %v", err)
	}
}

func TestGenerateHawkConfig(t *testing.T) {
	r := NewTemplateRegistry()
	tmpl := r.Templates["go-default"]
	vars := map[string]string{"model": "haiku"}

	config := GenerateHawkConfig(tmpl, vars)
	if !strings.Contains(config, "haiku") {
		t.Error("expected model variable substituted")
	}
	if strings.Contains(config, "{{model}}") {
		t.Error("expected placeholder replaced")
	}
}

func TestGenerateHawkConfig_FallbackToContent(t *testing.T) {
	tmpl := &ConfigTemplate{
		Name:     "no-files",
		Language: "go",
		Content:  map[string]interface{}{"model": "sonnet", "sandbox": "workspace"},
		Files:    map[string]string{}, // no settings.json in Files
	}
	vars := map[string]string{"model": "opus"}

	config := GenerateHawkConfig(tmpl, vars)
	if config == "{}" {
		t.Error("expected non-empty fallback config")
	}
	if !strings.Contains(config, "sonnet") {
		t.Error("expected content from Content map")
	}
}

func TestGenerateAgentsmd(t *testing.T) {
	r := NewTemplateRegistry()
	tmpl := r.Templates["ts-react"]
	vars := map[string]string{
		"project_name":     "my-frontend",
		"state_management": "zustand",
	}

	md := GenerateAgentsmd(tmpl, vars)
	if !strings.Contains(md, "my-frontend") {
		t.Error("expected project name in AGENTS.md")
	}
	if !strings.Contains(md, "zustand") {
		t.Error("expected state_management variable")
	}
}

func TestGenerateAgentsmd_Fallback(t *testing.T) {
	tmpl := &ConfigTemplate{
		Name:     "bare",
		Language: "haskell",
		Files:    map[string]string{},
	}
	vars := map[string]string{"project_name": "myproj"}

	md := GenerateAgentsmd(tmpl, vars)
	if !strings.Contains(md, "myproj") {
		t.Error("expected project name in fallback")
	}
	if !strings.Contains(md, "haskell") {
		t.Error("expected language in fallback")
	}
}

func TestGenerateRules(t *testing.T) {
	r := NewTemplateRegistry()
	tmpl := r.Templates["go-default"]

	rules := GenerateRules(tmpl)
	if !strings.Contains(rules, "Go Rules") {
		t.Error("expected Go rules content")
	}
	if !strings.Contains(rules, "gofmt") {
		t.Error("expected gofmt mention in rules")
	}
}

func TestGenerateRules_NoRulesFiles(t *testing.T) {
	tmpl := &ConfigTemplate{
		Name:     "bare",
		Language: "zig",
		Files:    map[string]string{"AGENTS.md": "hello"},
	}

	rules := GenerateRules(tmpl)
	if !strings.Contains(rules, "zig") {
		t.Error("expected language in fallback rules")
	}
}

func TestFormatTemplate(t *testing.T) {
	r := NewTemplateRegistry()
	tmpl := r.Templates["python-api"]

	formatted := FormatTemplate(tmpl)
	if !strings.Contains(formatted, "python-api") {
		t.Error("expected template name")
	}
	if !strings.Contains(formatted, "Python API") {
		t.Error("expected description")
	}
	if !strings.Contains(formatted, "python") {
		t.Error("expected language")
	}
	if !strings.Contains(formatted, "fastapi/django") {
		t.Error("expected framework")
	}
	if !strings.Contains(formatted, "Tags:") {
		t.Error("expected tags section")
	}
	if !strings.Contains(formatted, "Files generated:") {
		t.Error("expected files section")
	}
}

func TestTemplateRegistry_Preview(t *testing.T) {
	r := NewTemplateRegistry()
	vars := map[string]string{
		"project_name": "preview-test",
		"model":        "sonnet",
		"framework":    "gin",
	}

	preview := r.Preview("go-api", vars)
	if !strings.Contains(preview, "Preview for template") {
		t.Error("expected preview header")
	}
	if !strings.Contains(preview, ".hawk/settings.json") {
		t.Error("expected settings file in preview")
	}
	if !strings.Contains(preview, "AGENTS.md") {
		t.Error("expected AGENTS.md in preview")
	}
	if !strings.Contains(preview, "preview-test") {
		t.Error("expected variable substitution in preview")
	}
}

func TestTemplateRegistry_Preview_NotFound(t *testing.T) {
	r := NewTemplateRegistry()
	preview := r.Preview("nonexistent", nil)
	if !strings.Contains(preview, "Error") {
		t.Error("expected error message in preview for nonexistent template")
	}
}

func TestApplyVars(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		vars     map[string]string
		expected string
	}{
		{
			name:     "simple substitution",
			text:     "Hello {{name}}!",
			vars:     map[string]string{"name": "World"},
			expected: "Hello World!",
		},
		{
			name:     "multiple vars",
			text:     "{{greeting}} {{name}}, welcome to {{place}}",
			vars:     map[string]string{"greeting": "Hi", "name": "Alice", "place": "Hawk"},
			expected: "Hi Alice, welcome to Hawk",
		},
		{
			name:     "no vars",
			text:     "no placeholders here",
			vars:     map[string]string{"unused": "value"},
			expected: "no placeholders here",
		},
		{
			name:     "missing var leaves placeholder",
			text:     "Hello {{name}}, your {{role}} is ready",
			vars:     map[string]string{"name": "Bob"},
			expected: "Hello Bob, your {{role}} is ready",
		},
		{
			name:     "nil vars",
			text:     "Hello {{name}}",
			vars:     nil,
			expected: "Hello {{name}}",
		},
		{
			name:     "repeated placeholder",
			text:     "{{x}} and {{x}} again",
			vars:     map[string]string{"x": "Y"},
			expected: "Y and Y again",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyVars(tt.text, tt.vars)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectProject(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(dir string)
		wantLang  string
		wantFrame string
	}{
		{
			name: "go project",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0o644)
			},
			wantLang:  "go",
			wantFrame: "",
		},
		{
			name: "go api project",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n\nrequire github.com/go-chi/chi v5.0.0\n"), 0o644)
			},
			wantLang:  "go",
			wantFrame: "api",
		},
		{
			name: "rust project",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname=\"test\"\n"), 0o644)
			},
			wantLang:  "rust",
			wantFrame: "cli",
		},
		{
			name: "typescript react",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"18"}}`), 0o644)
			},
			wantLang:  "typescript",
			wantFrame: "react",
		},
		{
			name: "typescript express",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"express":"4"}}`), 0o644)
			},
			wantLang:  "typescript",
			wantFrame: "api",
		},
		{
			name: "python fastapi",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\ndependencies=[\"fastapi\"]\n"), 0o644)
			},
			wantLang:  "python",
			wantFrame: "api",
		},
		{
			name: "python ml",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("torch\nnumpy\n"), 0o644)
			},
			wantLang:  "python",
			wantFrame: "ml",
		},
		{
			name: "monorepo pnpm",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "pnpm-workspace.yaml"), []byte("packages:\n  - packages/*\n"), 0o644)
			},
			wantLang:  "multi",
			wantFrame: "monorepo",
		},
		{
			name:      "empty dir",
			setup:     func(dir string) {},
			wantLang:  "",
			wantFrame: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(dir)
			lang, frame := detectProject(dir)
			if lang != tt.wantLang {
				t.Errorf("language: expected %q, got %q", tt.wantLang, lang)
			}
			if frame != tt.wantFrame {
				t.Errorf("framework: expected %q, got %q", tt.wantFrame, frame)
			}
		})
	}
}

func TestSelectTemplate(t *testing.T) {
	tests := []struct {
		lang      string
		framework string
		want      string
	}{
		{"go", "", "go-default"},
		{"go", "api", "go-api"},
		{"typescript", "react", "ts-react"},
		{"typescript", "api", "ts-api"},
		{"typescript", "", "ts-react"},
		{"python", "api", "python-api"},
		{"python", "ml", "python-ml"},
		{"python", "", "python-api"},
		{"rust", "cli", "rust-cli"},
		{"rust", "", "rust-cli"},
		{"multi", "monorepo", "monorepo"},
		{"", "", ""},
		{"unknown", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.lang+"/"+tt.framework, func(t *testing.T) {
			got := selectTemplate(tt.lang, tt.framework)
			if got != tt.want {
				t.Errorf("selectTemplate(%q, %q) = %q, want %q", tt.lang, tt.framework, got, tt.want)
			}
		})
	}
}

func TestDetectPackages(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "packages", "core"), 0o755)
	os.MkdirAll(filepath.Join(dir, "packages", "ui"), 0o755)
	os.MkdirAll(filepath.Join(dir, "apps", "web"), 0o755)

	result := detectPackages(dir)
	if !strings.Contains(result, "packages/core") {
		t.Error("expected packages/core")
	}
	if !strings.Contains(result, "packages/ui") {
		t.Error("expected packages/ui")
	}
	if !strings.Contains(result, "apps/web") {
		t.Error("expected apps/web")
	}
}

func TestDetectPackages_Empty(t *testing.T) {
	dir := t.TempDir()
	result := detectPackages(dir)
	if !strings.Contains(result, "auto-detect") {
		t.Error("expected fallback message for empty dir")
	}
}

func TestConfigTemplateFields(t *testing.T) {
	tmpl := &ConfigTemplate{
		Name:        "test",
		Description: "Test template",
		Language:    "go",
		Framework:   "gin",
		Content:     map[string]interface{}{"key": "value"},
		Files:       map[string]string{"a.txt": "content"},
		Tags:        []string{"tag1", "tag2"},
	}

	if tmpl.Name != "test" {
		t.Error("Name field")
	}
	if tmpl.Description != "Test template" {
		t.Error("Description field")
	}
	if tmpl.Language != "go" {
		t.Error("Language field")
	}
	if tmpl.Framework != "gin" {
		t.Error("Framework field")
	}
	if tmpl.Content["key"] != "value" {
		t.Error("Content field")
	}
	if tmpl.Files["a.txt"] != "content" {
		t.Error("Files field")
	}
	if len(tmpl.Tags) != 2 {
		t.Error("Tags field")
	}
}

func TestConfigTemplateConcurrentAccess(t *testing.T) {
	r := NewTemplateRegistry()
	done := make(chan struct{})

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			r.List()
			r.Generate("go-default", map[string]string{"model": "sonnet", "project_name": "test"})
			r.Preview("go-api", map[string]string{"model": "opus"})
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			r.Register(&ConfigTemplate{
				Name:     fmt.Sprintf("concurrent-%d", n),
				Language: "go",
				Files:    map[string]string{},
			})
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}
}
