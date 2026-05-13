package fingerprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScan_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	fp, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan(%q): %v", dir, err)
	}

	if fp.Language != "" {
		t.Errorf("expected empty language, got %q", fp.Language)
	}
	if len(fp.Languages) != 0 {
		t.Errorf("expected no languages, got %d", len(fp.Languages))
	}
	if fp.ProjectSize != "tiny" {
		t.Errorf("expected 'tiny' project size, got %q", fp.ProjectSize)
	}
	if fp.Framework != "" {
		t.Errorf("expected empty framework, got %q", fp.Framework)
	}
	if fp.Docker {
		t.Error("expected Docker=false for empty dir")
	}
	if fp.Monorepo {
		t.Error("expected Monorepo=false for empty dir")
	}
}

func TestScan_InvalidPath(t *testing.T) {
	_, err := Scan("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestDetectLanguages_Extensions(t *testing.T) {
	dir := t.TempDir()

	// Create files with different extensions.
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(dir, "util.go"), "package main\n\nfunc util() {}\n")
	writeTestFile(t, filepath.Join(dir, "handler.go"), "package main\n\nfunc handler() {}\n")
	writeTestFile(t, filepath.Join(dir, "app.ts"), "const x = 1;\n")
	writeTestFile(t, filepath.Join(dir, "style.css"), "body { color: red; }\n")

	langs := detectLanguages(dir)

	if len(langs) == 0 {
		t.Fatal("expected at least one language")
	}

	// Go should be first (3 files).
	if langs[0].Name != "Go" {
		t.Errorf("expected Go as primary language, got %q", langs[0].Name)
	}
	if langs[0].FileCount != 3 {
		t.Errorf("expected 3 Go files, got %d", langs[0].FileCount)
	}

	// Check percentage.
	expectedPct := 3.0 / 5.0 * 100
	if langs[0].Percentage < expectedPct-0.1 || langs[0].Percentage > expectedPct+0.1 {
		t.Errorf("expected ~%.1f%% for Go, got %.1f%%", expectedPct, langs[0].Percentage)
	}

	// Check that all languages are detected.
	langMap := make(map[string]int)
	for _, l := range langs {
		langMap[l.Name] = l.FileCount
	}
	if langMap["TypeScript"] != 1 {
		t.Errorf("expected 1 TypeScript file, got %d", langMap["TypeScript"])
	}
	if langMap["CSS"] != 1 {
		t.Errorf("expected 1 CSS file, got %d", langMap["CSS"])
	}
}

func TestDetectLanguages_SortedByFileCount(t *testing.T) {
	dir := t.TempDir()

	// Create 5 Python files, 3 JS files, 1 Go file.
	for i := 0; i < 5; i++ {
		writeTestFile(t, filepath.Join(dir, strings.Repeat("a", i+1)+".py"), "# python\n")
	}
	for i := 0; i < 3; i++ {
		writeTestFile(t, filepath.Join(dir, strings.Repeat("b", i+1)+".js"), "// js\n")
	}
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n")

	langs := detectLanguages(dir)

	if len(langs) < 3 {
		t.Fatalf("expected at least 3 languages, got %d", len(langs))
	}

	// Should be sorted: Python (5), JavaScript (3), Go (1).
	if langs[0].Name != "Python" {
		t.Errorf("expected Python first, got %q", langs[0].Name)
	}
	if langs[1].Name != "JavaScript" {
		t.Errorf("expected JavaScript second, got %q", langs[1].Name)
	}
	if langs[2].Name != "Go" {
		t.Errorf("expected Go third, got %q", langs[2].Name)
	}
}

func TestDetectFramework_GoMod(t *testing.T) {
	tests := []struct {
		name     string
		gomod    string
		expected string
	}{
		{
			name: "chi",
			gomod: `module example.com/app

go 1.21

require (
	github.com/go-chi/chi/v5 v5.0.10
)
`,
			expected: "chi",
		},
		{
			name: "gin",
			gomod: `module example.com/app

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
)
`,
			expected: "gin",
		},
		{
			name: "echo",
			gomod: `module example.com/app

go 1.21

require (
	github.com/labstack/echo/v4 v4.11.0
)
`,
			expected: "echo",
		},
		{
			name: "fiber",
			gomod: `module example.com/app

go 1.21

require (
	github.com/gofiber/fiber/v2 v2.50.0
)
`,
			expected: "fiber",
		},
		{
			name: "gorilla",
			gomod: `module example.com/app

go 1.21

require (
	github.com/gorilla/mux v1.8.0
)
`,
			expected: "gorilla",
		},
		{
			name:     "no framework",
			gomod:    "module example.com/app\n\ngo 1.21\n",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "go.mod"), tt.gomod)

			result := detectFramework(dir, "Go")
			if result != tt.expected {
				t.Errorf("expected framework %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectFramework_PackageJSON(t *testing.T) {
	tests := []struct {
		name     string
		pkg      string
		expected string
	}{
		{
			name:     "next.js",
			pkg:      `{"dependencies": {"next": "^13.0.0", "react": "^18.0.0"}}`,
			expected: "next.js",
		},
		{
			name:     "express",
			pkg:      `{"dependencies": {"express": "^4.18.0"}}`,
			expected: "express",
		},
		{
			name:     "vue",
			pkg:      `{"dependencies": {"vue": "^3.0.0"}}`,
			expected: "vue",
		},
		{
			name:     "angular",
			pkg:      `{"dependencies": {"@angular/core": "^16.0.0"}}`,
			expected: "angular",
		},
		{
			name:     "react only",
			pkg:      `{"dependencies": {"react": "^18.0.0"}}`,
			expected: "react",
		},
		{
			name:     "no framework",
			pkg:      `{"dependencies": {"lodash": "^4.0.0"}}`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "package.json"), tt.pkg)

			result := detectFramework(dir, "JavaScript")
			if result != tt.expected {
				t.Errorf("expected framework %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectFramework_Python(t *testing.T) {
	tests := []struct {
		name         string
		requirements string
		expected     string
	}{
		{
			name:         "django",
			requirements: "django==4.2\ncelery==5.3\n",
			expected:     "django",
		},
		{
			name:         "flask",
			requirements: "flask==2.3\n",
			expected:     "flask",
		},
		{
			name:         "fastapi",
			requirements: "fastapi==0.100.0\nuvicorn==0.23.0\n",
			expected:     "fastapi",
		},
		{
			name:         "no framework",
			requirements: "requests==2.31\nnumpy==1.25\n",
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "requirements.txt"), tt.requirements)

			result := detectFramework(dir, "Python")
			if result != tt.expected {
				t.Errorf("expected framework %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectFramework_Rust(t *testing.T) {
	tests := []struct {
		name     string
		cargo    string
		expected string
	}{
		{
			name: "actix",
			cargo: `[package]
name = "myapp"
version = "0.1.0"

[dependencies]
actix-web = "4"
`,
			expected: "actix",
		},
		{
			name: "axum",
			cargo: `[package]
name = "myapp"
version = "0.1.0"

[dependencies]
axum = "0.6"
tokio = { version = "1", features = ["full"] }
`,
			expected: "axum",
		},
		{
			name: "rocket",
			cargo: `[package]
name = "myapp"
version = "0.1.0"

[dependencies]
rocket = "0.5"
`,
			expected: "rocket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "Cargo.toml"), tt.cargo)

			result := detectFramework(dir, "Rust")
			if result != tt.expected {
				t.Errorf("expected framework %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectCISystem(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(dir string)
		expected string
	}{
		{
			name: "github-actions",
			setup: func(dir string) {
				os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0755)
			},
			expected: "github-actions",
		},
		{
			name: "gitlab-ci",
			setup: func(dir string) {
				writeTestFile2(filepath.Join(dir, ".gitlab-ci.yml"), "stages:\n  - test\n")
			},
			expected: "gitlab-ci",
		},
		{
			name: "circleci",
			setup: func(dir string) {
				os.MkdirAll(filepath.Join(dir, ".circleci"), 0755)
			},
			expected: "circleci",
		},
		{
			name: "jenkins",
			setup: func(dir string) {
				writeTestFile2(filepath.Join(dir, "Jenkinsfile"), "pipeline {}\n")
			},
			expected: "jenkins",
		},
		{
			name: "travis-ci",
			setup: func(dir string) {
				writeTestFile2(filepath.Join(dir, ".travis.yml"), "language: go\n")
			},
			expected: "travis-ci",
		},
		{
			name:     "no CI",
			setup:    func(dir string) {},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(dir)

			result := detectCISystem(dir)
			if result != tt.expected {
				t.Errorf("expected CI %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectBuildSystem(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		content  string
		expected string
	}{
		{"go modules", "go.mod", "module example.com/app\n\ngo 1.21\n", "go modules"},
		{"npm", "package.json", `{"name": "app"}`, "npm"},
		{"cargo", "Cargo.toml", "[package]\nname = \"app\"\n", "cargo"},
		{"maven", "pom.xml", "<project></project>", "maven"},
		{"gradle", "build.gradle", "plugins { id 'java' }", "gradle"},
		{"cmake", "CMakeLists.txt", "cmake_minimum_required(VERSION 3.10)", "cmake"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, tt.file), tt.content)

			result := detectBuildSystem(dir)
			if result != tt.expected {
				t.Errorf("expected build system %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectTestFramework(t *testing.T) {
	t.Run("go test", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.21\n")
		writeTestFile(t, filepath.Join(dir, "main_test.go"), "package main\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {}\n")

		result := detectTestFramework(dir, "Go")
		if result != "go test" {
			t.Errorf("expected 'go test', got %q", result)
		}
	})

	t.Run("go test + testify", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.21\n\nrequire (\n\tgithub.com/stretchr/testify v1.8.0\n)\n")
		writeTestFile(t, filepath.Join(dir, "main_test.go"), "package main\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {}\n")

		result := detectTestFramework(dir, "Go")
		if result != "go test + testify" {
			t.Errorf("expected 'go test + testify', got %q", result)
		}
	})

	t.Run("jest", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "package.json"), `{"devDependencies": {"jest": "^29.0.0"}}`)

		result := detectTestFramework(dir, "JavaScript")
		if result != "jest" {
			t.Errorf("expected 'jest', got %q", result)
		}
	})

	t.Run("vitest", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "package.json"), `{"devDependencies": {"vitest": "^0.34.0"}}`)

		result := detectTestFramework(dir, "TypeScript")
		if result != "vitest" {
			t.Errorf("expected 'vitest', got %q", result)
		}
	})

	t.Run("pytest", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "requirements.txt"), "pytest==7.4.0\nflask==2.3.0\n")

		result := detectTestFramework(dir, "Python")
		if result != "pytest" {
			t.Errorf("expected 'pytest', got %q", result)
		}
	})

	t.Run("pytest from conftest", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "conftest.py"), "import pytest\n")

		result := detectTestFramework(dir, "Python")
		if result != "pytest" {
			t.Errorf("expected 'pytest', got %q", result)
		}
	})

	t.Run("cargo test", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "Cargo.toml"), "[package]\nname = \"app\"\nversion = \"0.1.0\"\n")

		result := detectTestFramework(dir, "Rust")
		if result != "cargo test" {
			t.Errorf("expected 'cargo test', got %q", result)
		}
	})

	t.Run("rspec", func(t *testing.T) {
		dir := t.TempDir()
		os.Mkdir(filepath.Join(dir, "spec"), 0755)
		writeTestFile(t, filepath.Join(dir, "Gemfile"), "gem 'rails'\ngem 'rspec'\n")

		result := detectTestFramework(dir, "Ruby")
		if result != "rspec" {
			t.Errorf("expected 'rspec', got %q", result)
		}
	})
}

func TestDetectDocker(t *testing.T) {
	t.Run("with Dockerfile", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "Dockerfile"), "FROM golang:1.21\n")

		if !detectDocker(dir) {
			t.Error("expected Docker=true with Dockerfile")
		}
	})

	t.Run("with docker-compose", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "docker-compose.yml"), "version: '3'\n")

		if !detectDocker(dir) {
			t.Error("expected Docker=true with docker-compose.yml")
		}
	})

	t.Run("with compose.yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "compose.yaml"), "services:\n")

		if !detectDocker(dir) {
			t.Error("expected Docker=true with compose.yaml")
		}
	})

	t.Run("no docker", func(t *testing.T) {
		dir := t.TempDir()

		if detectDocker(dir) {
			t.Error("expected Docker=false for empty dir")
		}
	})
}

func TestDetectMonorepo_MultipleGoMod(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "pkg1"), 0755)
	os.MkdirAll(filepath.Join(dir, "pkg2"), 0755)
	writeTestFile(t, filepath.Join(dir, "pkg1", "go.mod"), "module example.com/pkg1\n")
	writeTestFile(t, filepath.Join(dir, "pkg2", "go.mod"), "module example.com/pkg2\n")

	if !detectMonorepo(dir) {
		t.Error("expected Monorepo=true with multiple go.mod files")
	}
}

func TestDetectMonorepo_GoWork(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.work"), "go 1.21\n\nuse (\n\t./pkg1\n\t./pkg2\n)\n")

	if !detectMonorepo(dir) {
		t.Error("expected Monorepo=true with go.work")
	}
}

func TestDetectMonorepo_PackagesDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "packages"), 0755)

	if !detectMonorepo(dir) {
		t.Error("expected Monorepo=true with packages/ directory")
	}
}

func TestDetectMonorepo_Workspaces(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "package.json"), `{"name": "monorepo", "workspaces": ["packages/*"]}`)

	if !detectMonorepo(dir) {
		t.Error("expected Monorepo=true with workspaces in package.json")
	}
}

func TestDetectMonorepo_LernaJSON(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "lerna.json"), `{"version": "1.0.0"}`)

	if !detectMonorepo(dir) {
		t.Error("expected Monorepo=true with lerna.json")
	}
}

func TestDetectMonorepo_NotMonorepo(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n")
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n")

	if detectMonorepo(dir) {
		t.Error("expected Monorepo=false for simple project")
	}
}

func TestDetectConventions_EditorConfig(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".editorconfig"), `root = true

[*]
indent_style = tab
indent_size = 4
`)

	convs := detectConventions(dir, "Go")

	found := false
	for _, c := range convs {
		if c.Name == "indentation" {
			found = true
			if !strings.Contains(c.Description, "Tab") {
				t.Errorf("expected tab indentation, got %q", c.Description)
			}
			if c.Confidence != 1.0 {
				t.Errorf("expected confidence 1.0, got %f", c.Confidence)
			}
		}
	}
	if !found {
		t.Error("expected indentation convention to be detected from .editorconfig")
	}
}

func TestDetectConventions_SpacesEditorConfig(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".editorconfig"), `root = true

[*]
indent_style = space
indent_size = 2
`)

	convs := detectConventions(dir, "TypeScript")

	found := false
	for _, c := range convs {
		if c.Name == "indentation" {
			found = true
			if !strings.Contains(c.Description, "2-space") {
				t.Errorf("expected 2-space indentation, got %q", c.Description)
			}
		}
	}
	if !found {
		t.Error("expected indentation convention from .editorconfig")
	}
}

func TestDetectConventions_GoNaming(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	convs := detectConventions(dir, "Go")

	found := false
	for _, c := range convs {
		if c.Name == "naming" {
			found = true
			if !strings.Contains(c.Description, "camelCase") {
				t.Errorf("expected camelCase/PascalCase for Go, got %q", c.Description)
			}
		}
	}
	if !found {
		t.Error("expected naming convention for Go")
	}
}

func TestDetectConventions_GoErrorWrapping(t *testing.T) {
	dir := t.TempDir()
	content := `package main

import "fmt"

func foo() error {
	err := bar()
	if err != nil {
		return fmt.Errorf("foo: %w", err)
	}
	err2 := baz()
	if err2 != nil {
		return fmt.Errorf("baz failed: %w", err2)
	}
	return nil
}
`
	writeTestFile(t, filepath.Join(dir, "main.go"), content)

	convs := detectConventions(dir, "Go")

	found := false
	for _, c := range convs {
		if c.Name == "error-handling" {
			found = true
			if !strings.Contains(c.Description, "wrapping") {
				t.Errorf("expected error wrapping convention, got %q", c.Description)
			}
		}
	}
	if !found {
		t.Error("expected error-handling convention to be detected")
	}
}

func TestDetectConventions_PythonNaming(t *testing.T) {
	dir := t.TempDir()
	content := `def get_user_name():
    pass

def calculate_total_price():
    pass

def handle_request_error():
    pass
`
	writeTestFile(t, filepath.Join(dir, "app.py"), content)

	convs := detectConventions(dir, "Python")

	found := false
	for _, c := range convs {
		if c.Name == "naming" {
			found = true
			if !strings.Contains(c.Description, "snake_case") {
				t.Errorf("expected snake_case for Python, got %q", c.Description)
			}
		}
	}
	if !found {
		t.Error("expected naming convention for Python")
	}
}

func TestGenerateRecommendations_Go(t *testing.T) {
	fp := &ProjectFingerprint{
		Language:      "Go",
		TestFramework: "go test",
		Framework:     "chi",
		LintTools:     nil,
		CI:            "github-actions",
		ProjectSize:   "medium",
	}

	recs := generateRecommendations(fp)

	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}

	// Should recommend test command.
	foundTestRec := false
	for _, r := range recs {
		if strings.Contains(r, "go test") {
			foundTestRec = true
		}
	}
	if !foundTestRec {
		t.Error("expected recommendation about 'go test ./...'")
	}

	// Should recommend golangci-lint.
	foundLintRec := false
	for _, r := range recs {
		if strings.Contains(r, "golangci") {
			foundLintRec = true
		}
	}
	if !foundLintRec {
		t.Error("expected recommendation about golangci-lint")
	}

	// Should mention chi.
	foundChiRec := false
	for _, r := range recs {
		if strings.Contains(r, "chi") {
			foundChiRec = true
		}
	}
	if !foundChiRec {
		t.Error("expected recommendation mentioning chi router")
	}
}

func TestGenerateRecommendations_NoCI(t *testing.T) {
	fp := &ProjectFingerprint{
		Language:    "Go",
		CI:          "",
		ProjectSize: "small",
	}

	recs := generateRecommendations(fp)

	foundCIRec := false
	for _, r := range recs {
		if strings.Contains(r, "CI") || strings.Contains(r, "GitHub Actions") {
			foundCIRec = true
		}
	}
	if !foundCIRec {
		t.Error("expected CI recommendation when no CI is detected")
	}
}

func TestGenerateRecommendations_JS(t *testing.T) {
	fp := &ProjectFingerprint{
		Language:      "JavaScript",
		TestFramework: "jest",
		Framework:     "next.js",
		LintTools:     []string{"eslint"},
		CI:            "github-actions",
		Docker:        true,
		ProjectSize:   "medium",
	}

	recs := generateRecommendations(fp)

	// Should recommend jest test command.
	foundTestRec := false
	for _, r := range recs {
		if strings.Contains(r, "jest") || strings.Contains(r, "npm test") {
			foundTestRec = true
		}
	}
	if !foundTestRec {
		t.Error("expected recommendation about jest/npm test")
	}

	// Should mention Next.js.
	foundNextRec := false
	for _, r := range recs {
		if strings.Contains(r, "Next.js") {
			foundNextRec = true
		}
	}
	if !foundNextRec {
		t.Error("expected recommendation mentioning Next.js")
	}
}

func TestGenerateRecommendations_Python(t *testing.T) {
	fp := &ProjectFingerprint{
		Language:      "Python",
		TestFramework: "pytest",
		LintTools:     nil,
		CI:            "github-actions",
		ProjectSize:   "medium",
	}

	recs := generateRecommendations(fp)

	// Should recommend pytest command.
	foundTestRec := false
	for _, r := range recs {
		if strings.Contains(r, "pytest") {
			foundTestRec = true
		}
	}
	if !foundTestRec {
		t.Error("expected recommendation about pytest")
	}

	// Should recommend linting tool.
	foundLintRec := false
	for _, r := range recs {
		if strings.Contains(r, "ruff") || strings.Contains(r, "flake8") {
			foundLintRec = true
		}
	}
	if !foundLintRec {
		t.Error("expected recommendation about Python linting")
	}
}

func TestGenerateRecommendations_Monorepo(t *testing.T) {
	fp := &ProjectFingerprint{
		Language:    "TypeScript",
		Monorepo:    true,
		CI:          "github-actions",
		ProjectSize: "large",
		Docker:      true,
	}

	recs := generateRecommendations(fp)

	foundMonoRec := false
	for _, r := range recs {
		if strings.Contains(r, "Monorepo") || strings.Contains(r, "monorepo") {
			foundMonoRec = true
		}
	}
	if !foundMonoRec {
		t.Error("expected monorepo recommendation")
	}
}

func TestFormatSummary_Complete(t *testing.T) {
	fp := &ProjectFingerprint{
		Language: "Go",
		Languages: []ProjectLangInfo{
			{Name: "Go", FileCount: 140, Percentage: 97.0},
			{Name: "Shell", FileCount: 4, Percentage: 3.0},
		},
		Framework:      "chi",
		BuildSystem:    "go modules",
		TestFramework:  "go test",
		LintTools:      []string{"golangci-lint"},
		PackageManager: "go modules",
		CI:             "github-actions",
		Docker:         true,
		Monorepo:       false,
		ProjectSize:    "medium",
		Conventions: []Convention{
			{Name: "indentation", Description: "Tabs for indentation", Confidence: 1.0},
			{Name: "error-handling", Description: "Error wrapping with %w", Confidence: 0.85},
		},
		Recommendations: []string{
			"Add `go test ./...` as your test command",
			"Your project uses chi router — hawk can help with middleware patterns",
		},
	}

	summary := FormatSummary(fp)

	// Check that key information is present.
	checks := []string{
		"Go (97%)",
		"Shell (3%)",
		"chi",
		"go modules",
		"go test",
		"GitHub Actions",
		"medium",
		"Docker: yes",
		"golangci-lint",
		"Tabs for indentation",
		"100% confidence",
		"Error wrapping with %w",
		"85% confidence",
		"go test ./...",
		"chi router",
	}

	for _, check := range checks {
		if !strings.Contains(summary, check) {
			t.Errorf("FormatSummary missing %q\nGot:\n%s", check, summary)
		}
	}

	t.Logf("FormatSummary output:\n%s", summary)
}

func TestFormatSummary_Minimal(t *testing.T) {
	fp := &ProjectFingerprint{
		Language: "Python",
		Languages: []ProjectLangInfo{
			{Name: "Python", FileCount: 5, Percentage: 100.0},
		},
		ProjectSize: "tiny",
	}

	summary := FormatSummary(fp)

	if !strings.Contains(summary, "Python (100%)") {
		t.Errorf("expected 'Python (100%%)', got:\n%s", summary)
	}
	if !strings.Contains(summary, "tiny") {
		t.Errorf("expected 'tiny' size, got:\n%s", summary)
	}
	// Should not contain sections that are empty.
	if strings.Contains(summary, "Framework:") {
		t.Error("should not show Framework when empty")
	}
	if strings.Contains(summary, "Docker:") {
		t.Error("should not show Docker when false")
	}
	if strings.Contains(summary, "Monorepo:") {
		t.Error("should not show Monorepo when false")
	}
}

func TestFormatSummary_MultipleLanguages(t *testing.T) {
	fp := &ProjectFingerprint{
		Language: "TypeScript",
		Languages: []ProjectLangInfo{
			{Name: "TypeScript", FileCount: 50, Percentage: 60.0},
			{Name: "JavaScript", FileCount: 20, Percentage: 24.0},
			{Name: "CSS", FileCount: 10, Percentage: 12.0},
			{Name: "HTML", FileCount: 3, Percentage: 4.0},
		},
		Framework:   "next.js",
		BuildSystem: "npm",
		ProjectSize: "small",
	}

	summary := FormatSummary(fp)

	if !strings.Contains(summary, "TypeScript (60%)") {
		t.Errorf("expected TypeScript (60%%), got:\n%s", summary)
	}
	if !strings.Contains(summary, "JavaScript (24%)") {
		t.Errorf("expected JavaScript (24%%), got:\n%s", summary)
	}
}

func TestProjectSize_Classification(t *testing.T) {
	tests := []struct {
		files    int
		expected string
	}{
		{0, "tiny"},
		{5, "tiny"},
		{9, "tiny"},
		{10, "small"},
		{50, "small"},
		{99, "small"},
		{100, "medium"},
		{500, "medium"},
		{1000, "medium"},
		{1001, "large"},
		{5000, "large"},
	}

	for _, tt := range tests {
		result := classifyProjectSize(tt.files)
		if result != tt.expected {
			t.Errorf("classifyProjectSize(%d) = %q, want %q", tt.files, result, tt.expected)
		}
	}
}

func TestDetectLintTools(t *testing.T) {
	t.Run("golangci-lint", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, ".golangci.yml"), "linters:\n  enable:\n    - errcheck\n")

		tools := detectLintTools(dir)
		if !containsString(tools, "golangci-lint") {
			t.Errorf("expected golangci-lint, got %v", tools)
		}
	})

	t.Run("eslint config", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, ".eslintrc.json"), `{"extends": "eslint:recommended"}`)

		tools := detectLintTools(dir)
		if !containsString(tools, "eslint") {
			t.Errorf("expected eslint, got %v", tools)
		}
	})

	t.Run("prettier", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, ".prettierrc"), `{"semi": true}`)

		tools := detectLintTools(dir)
		if !containsString(tools, "prettier") {
			t.Errorf("expected prettier, got %v", tools)
		}
	})

	t.Run("multiple tools", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, ".eslintrc.json"), `{}`)
		writeTestFile(t, filepath.Join(dir, ".prettierrc"), `{}`)
		writeTestFile(t, filepath.Join(dir, ".editorconfig"), "root = true\n")

		tools := detectLintTools(dir)
		if len(tools) < 3 {
			t.Errorf("expected at least 3 tools, got %v", tools)
		}
	})

	t.Run("no lint tools", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n")

		tools := detectLintTools(dir)
		if len(tools) != 0 {
			t.Errorf("expected no lint tools, got %v", tools)
		}
	})
}

func TestDetectPackageManager(t *testing.T) {
	t.Run("pnpm", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "pnpm-lock.yaml"), "lockfileVersion: 5.4\n")
		writeTestFile(t, filepath.Join(dir, "package.json"), `{"name": "app"}`)

		result := detectProjectPackageManager(dir)
		if result != "pnpm" {
			t.Errorf("expected 'pnpm', got %q", result)
		}
	})

	t.Run("yarn", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "yarn.lock"), "# yarn lock\n")
		writeTestFile(t, filepath.Join(dir, "package.json"), `{"name": "app"}`)

		result := detectProjectPackageManager(dir)
		if result != "yarn" {
			t.Errorf("expected 'yarn', got %q", result)
		}
	})

	t.Run("go modules", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "go.sum"), "github.com/foo/bar v1.0.0 h1:abc=\n")

		result := detectProjectPackageManager(dir)
		if result != "go modules" {
			t.Errorf("expected 'go modules', got %q", result)
		}
	})
}

func TestScan_FullGoProject(t *testing.T) {
	dir := t.TempDir()

	// Set up a realistic Go project.
	writeTestFile(t, filepath.Join(dir, "go.mod"), `module example.com/myapp

go 1.21

require (
	github.com/go-chi/chi/v5 v5.0.10
	github.com/stretchr/testify v1.8.0
)
`)
	writeTestFile(t, filepath.Join(dir, "go.sum"), "github.com/go-chi/chi/v5 v5.0.10 h1:abc=\n")
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")
	writeTestFile(t, filepath.Join(dir, "handler.go"), "package main\n\nimport \"net/http\"\n\nfunc handler(w http.ResponseWriter, r *http.Request) {}\n")
	writeTestFile(t, filepath.Join(dir, "main_test.go"), "package main\n\nimport \"testing\"\n\nfunc TestMain(t *testing.T) {}\n")

	os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0755)
	writeTestFile(t, filepath.Join(dir, ".github", "workflows", "ci.yml"), "name: CI\non: push\n")
	writeTestFile(t, filepath.Join(dir, "Dockerfile"), "FROM golang:1.21\nCOPY . .\nRUN go build .\n")
	writeTestFile(t, filepath.Join(dir, ".golangci.yml"), "linters:\n  enable:\n    - errcheck\n")

	fp, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if fp.Language != "Go" {
		t.Errorf("expected language 'Go', got %q", fp.Language)
	}
	if fp.Framework != "chi" {
		t.Errorf("expected framework 'chi', got %q", fp.Framework)
	}
	if fp.BuildSystem != "go modules" {
		t.Errorf("expected build system 'go modules', got %q", fp.BuildSystem)
	}
	if fp.TestFramework != "go test + testify" {
		t.Errorf("expected test framework 'go test + testify', got %q", fp.TestFramework)
	}
	if fp.CI != "github-actions" {
		t.Errorf("expected CI 'github-actions', got %q", fp.CI)
	}
	if !fp.Docker {
		t.Error("expected Docker=true")
	}
	if fp.Monorepo {
		t.Error("expected Monorepo=false")
	}
	if !containsString(fp.LintTools, "golangci-lint") {
		t.Errorf("expected golangci-lint in LintTools, got %v", fp.LintTools)
	}
	if fp.ProjectSize != "tiny" {
		t.Errorf("expected 'tiny' project size (few files), got %q", fp.ProjectSize)
	}
	if len(fp.Recommendations) == 0 {
		t.Error("expected at least one recommendation")
	}

	// Check FormatSummary doesn't panic.
	summary := FormatSummary(fp)
	if summary == "" {
		t.Error("FormatSummary returned empty string")
	}
	t.Logf("Full project summary:\n%s", summary)
}

func TestScan_FullJSProject(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "package.json"), `{
  "name": "my-nextjs-app",
  "dependencies": {
    "next": "^13.0.0",
    "react": "^18.0.0",
    "react-dom": "^18.0.0"
  },
  "devDependencies": {
    "jest": "^29.0.0",
    "eslint": "^8.0.0",
    "@types/react": "^18.0.0"
  }
}`)
	writeTestFile(t, filepath.Join(dir, "package-lock.json"), `{"lockfileVersion": 3}`)
	writeTestFile(t, filepath.Join(dir, ".eslintrc.json"), `{"extends": "next/core-web-vitals"}`)

	os.MkdirAll(filepath.Join(dir, "pages"), 0755)
	writeTestFile(t, filepath.Join(dir, "pages", "index.tsx"), "export default function Home() { return <div>Hello</div>; }\n")
	writeTestFile(t, filepath.Join(dir, "pages", "about.tsx"), "export default function About() { return <div>About</div>; }\n")
	writeTestFile(t, filepath.Join(dir, "pages", "contact.tsx"), "export default function Contact() { return <div>Contact</div>; }\n")
	writeTestFile(t, filepath.Join(dir, "pages", "blog.tsx"), "export default function Blog() { return <div>Blog</div>; }\n")
	writeTestFile(t, filepath.Join(dir, "lib", "utils.ts"), "export function cn(...args: string[]) { return args.join(' '); }\n")

	os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0755)
	writeTestFile(t, filepath.Join(dir, ".github", "workflows", "ci.yml"), "name: CI\n")

	fp, err := Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Primary language should be TypeScript (from .tsx/.ts files).
	if fp.Language != "TypeScript" {
		t.Errorf("expected language 'TypeScript', got %q", fp.Language)
	}
	if fp.Framework != "next.js" {
		t.Errorf("expected framework 'next.js', got %q", fp.Framework)
	}
	if fp.TestFramework != "jest" {
		t.Errorf("expected test framework 'jest', got %q", fp.TestFramework)
	}
	if fp.CI != "github-actions" {
		t.Errorf("expected CI 'github-actions', got %q", fp.CI)
	}
	if !containsString(fp.LintTools, "eslint") {
		t.Errorf("expected eslint in lint tools, got %v", fp.LintTools)
	}
}

// writeTestFile is a helper for creating test files.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeTestFile2 is a non-testing.T helper used in table-driven test setup functions.
func writeTestFile2(path, content string) {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	os.WriteFile(path, []byte(content), 0644)
}
