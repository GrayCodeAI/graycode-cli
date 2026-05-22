package intelligence

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// LanguageConfig describes a programming language's tooling and conventions.
type LanguageConfig struct {
	Name            string
	Extensions      []string
	TestCommand     string
	LintCommand     string
	FormatCommand   string
	BuildCommand    string
	PackageManager  string
	PackageFile     string
	ImportPattern   *regexp.Regexp
	FunctionPattern *regexp.Regexp
	CommentStyle    string // "//", "#", "--"
}

// LanguageRegistry manages a collection of language configurations.
type LanguageRegistry struct {
	Languages map[string]*LanguageConfig
	mu        sync.RWMutex
}

// NewLanguageRegistry creates a registry pre-populated with common languages.
func NewLanguageRegistry() *LanguageRegistry {
	r := &LanguageRegistry{
		Languages: make(map[string]*LanguageConfig),
	}

	defaults := []*LanguageConfig{
		{
			Name:            "Go",
			Extensions:      []string{".go"},
			TestCommand:     "go test ./...",
			LintCommand:     "golangci-lint run",
			FormatCommand:   "gofmt -w",
			BuildCommand:    "go build",
			PackageManager:  "go",
			PackageFile:     "go.mod",
			ImportPattern:   regexp.MustCompile(`^\s*"([^"]+)"\s*$`),
			FunctionPattern: regexp.MustCompile(`^func\s+(\w+)`),
			CommentStyle:    "//",
		},
		{
			Name:            "Python",
			Extensions:      []string{".py"},
			TestCommand:     "pytest",
			LintCommand:     "ruff check",
			FormatCommand:   "black",
			BuildCommand:    "",
			PackageManager:  "pip",
			PackageFile:     "requirements.txt",
			ImportPattern:   regexp.MustCompile(`^\s*(?:import|from)\s+(\S+)`),
			FunctionPattern: regexp.MustCompile(`^\s*def\s+(\w+)`),
			CommentStyle:    "#",
		},
		{
			Name:            "TypeScript",
			Extensions:      []string{".ts", ".tsx"},
			TestCommand:     "npm test",
			LintCommand:     "eslint",
			FormatCommand:   "prettier --write",
			BuildCommand:    "tsc",
			PackageManager:  "npm",
			PackageFile:     "package.json",
			ImportPattern:   regexp.MustCompile(`^\s*import\s+.*from\s+['"]([^'"]+)['"]`),
			FunctionPattern: regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)`),
			CommentStyle:    "//",
		},
		{
			Name:            "JavaScript",
			Extensions:      []string{".js", ".jsx"},
			TestCommand:     "npm test",
			LintCommand:     "eslint",
			FormatCommand:   "prettier --write",
			BuildCommand:    "",
			PackageManager:  "npm",
			PackageFile:     "package.json",
			ImportPattern:   regexp.MustCompile(`^\s*(?:import\s+.*from\s+['"]([^'"]+)['"]|require\(['"]([^'"]+)['"]\))`),
			FunctionPattern: regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+(\w+)`),
			CommentStyle:    "//",
		},
		{
			Name:            "Rust",
			Extensions:      []string{".rs"},
			TestCommand:     "cargo test",
			LintCommand:     "cargo clippy",
			FormatCommand:   "cargo fmt",
			BuildCommand:    "cargo build",
			PackageManager:  "cargo",
			PackageFile:     "Cargo.toml",
			ImportPattern:   regexp.MustCompile(`^\s*use\s+([^;]+);`),
			FunctionPattern: regexp.MustCompile(`^\s*(?:pub\s+)?fn\s+(\w+)`),
			CommentStyle:    "//",
		},
		{
			Name:            "Ruby",
			Extensions:      []string{".rb"},
			TestCommand:     "bundle exec rspec",
			LintCommand:     "rubocop",
			FormatCommand:   "rubocop -a",
			BuildCommand:    "",
			PackageManager:  "bundler",
			PackageFile:     "Gemfile",
			ImportPattern:   regexp.MustCompile(`^\s*require\s+['"]([^'"]+)['"]`),
			FunctionPattern: regexp.MustCompile(`^\s*def\s+(\w+)`),
			CommentStyle:    "#",
		},
		{
			Name:            "Java",
			Extensions:      []string{".java"},
			TestCommand:     "./gradlew test",
			LintCommand:     "checkstyle",
			FormatCommand:   "",
			BuildCommand:    "./gradlew build",
			PackageManager:  "gradle",
			PackageFile:     "build.gradle",
			ImportPattern:   regexp.MustCompile(`^\s*import\s+([^;]+);`),
			FunctionPattern: regexp.MustCompile(`^\s*(?:public|private|protected)?\s*(?:static\s+)?\w+\s+(\w+)\s*\(`),
			CommentStyle:    "//",
		},
		{
			Name:            "C#",
			Extensions:      []string{".cs"},
			TestCommand:     "dotnet test",
			LintCommand:     "dotnet format --verify-no-changes",
			FormatCommand:   "dotnet format",
			BuildCommand:    "dotnet build",
			PackageManager:  "dotnet",
			PackageFile:     "*.csproj",
			ImportPattern:   regexp.MustCompile(`^\s*using\s+([^;]+);`),
			FunctionPattern: regexp.MustCompile(`^\s*(?:public|private|protected|internal)?\s*(?:static\s+)?\w+\s+(\w+)\s*\(`),
			CommentStyle:    "//",
		},
		{
			Name:            "Shell",
			Extensions:      []string{".sh", ".bash"},
			TestCommand:     "shellcheck",
			LintCommand:     "shellcheck",
			FormatCommand:   "shfmt -w",
			BuildCommand:    "",
			PackageManager:  "",
			PackageFile:     "",
			ImportPattern:   regexp.MustCompile(`^\s*(?:source|\.)\s+(.+)`),
			FunctionPattern: regexp.MustCompile(`^\s*(?:function\s+)?(\w+)\s*\(\)`),
			CommentStyle:    "#",
		},
		{
			Name:            "SQL",
			Extensions:      []string{".sql"},
			TestCommand:     "",
			LintCommand:     "sqlfluff lint",
			FormatCommand:   "sqlfluff fix",
			BuildCommand:    "",
			PackageManager:  "",
			PackageFile:     "",
			ImportPattern:   nil,
			FunctionPattern: regexp.MustCompile(`(?i)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?(?:FUNCTION|PROCEDURE)\s+(\w+)`),
			CommentStyle:    "--",
		},
		{
			Name:            "Kotlin",
			Extensions:      []string{".kt", ".kts"},
			TestCommand:     "./gradlew test",
			LintCommand:     "ktlint",
			FormatCommand:   "ktlint -F",
			BuildCommand:    "./gradlew build",
			PackageManager:  "gradle",
			PackageFile:     "build.gradle.kts",
			ImportPattern:   regexp.MustCompile(`^\s*import\s+(\S+)`),
			FunctionPattern: regexp.MustCompile(`^\s*(?:(?:public|private|internal|protected)\s+)?fun\s+(\w+)`),
			CommentStyle:    "//",
		},
		{
			Name:            "Swift",
			Extensions:      []string{".swift"},
			TestCommand:     "swift test",
			LintCommand:     "swiftlint",
			FormatCommand:   "swiftformat",
			BuildCommand:    "swift build",
			PackageManager:  "spm",
			PackageFile:     "Package.swift",
			ImportPattern:   regexp.MustCompile(`^\s*import\s+(\w+)`),
			FunctionPattern: regexp.MustCompile(`^\s*(?:(?:public|private|internal|open)\s+)?func\s+(\w+)`),
			CommentStyle:    "//",
		},
	}

	for _, cfg := range defaults {
		r.Languages[strings.ToLower(cfg.Name)] = cfg
	}

	return r
}

// Register adds or replaces a language configuration in the registry.
func (r *LanguageRegistry) Register(config *LanguageConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Languages[strings.ToLower(config.Name)] = config
}

// GetByName looks up a language by its name (case-insensitive).
func (r *LanguageRegistry) GetByName(name string) *LanguageConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Languages[strings.ToLower(name)]
}

// GetByExtension finds a language by file extension (including the dot).
func (r *LanguageRegistry) GetByExtension(ext string) *LanguageConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ext = strings.ToLower(ext)
	for _, cfg := range r.Languages {
		for _, e := range cfg.Extensions {
			if e == ext {
				return cfg
			}
		}
	}
	return nil
}

// Detect returns the primary language for a project directory by counting
// source files and checking for package manifest files.
func (r *LanguageRegistry) Detect(projectDir string) *LanguageConfig {
	configs := r.DetectAll(projectDir)
	if len(configs) == 0 {
		return nil
	}
	return configs[0]
}

// DetectAll returns all languages detected in a project, sorted by file count
// (most prevalent first). It walks the directory tree and matches extensions.
func (r *LanguageRegistry) DetectAll(projectDir string) []*LanguageConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[string]int)

	_ = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			// Skip hidden dirs and common vendor/dependency dirs.
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" || base == "target" || base == "bin" || base == "obj" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == "" {
			return nil
		}
		for name, cfg := range r.Languages {
			for _, e := range cfg.Extensions {
				if e == ext {
					counts[name]++
					break
				}
			}
		}
		return nil
	})

	// Also boost languages whose package file exists at the project root.
	for name, cfg := range r.Languages {
		if cfg.PackageFile == "" {
			continue
		}
		// Handle glob patterns like *.csproj.
		if strings.Contains(cfg.PackageFile, "*") {
			matches, _ := filepath.Glob(filepath.Join(projectDir, cfg.PackageFile))
			if len(matches) > 0 {
				counts[name] += 100
			}
		} else {
			if _, err := os.Stat(filepath.Join(projectDir, cfg.PackageFile)); err == nil {
				counts[name] += 100
			}
		}
	}

	if len(counts) == 0 {
		return nil
	}

	type langCount struct {
		name  string
		count int
	}
	sorted := make([]langCount, 0, len(counts))
	for name, count := range counts {
		sorted = append(sorted, langCount{name, count})
	}
	// Sort descending by count, then alphabetically for stability.
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count || (sorted[j].count == sorted[i].count && sorted[j].name < sorted[i].name) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	result := make([]*LanguageConfig, 0, len(sorted))
	for _, lc := range sorted {
		result = append(result, r.Languages[lc.name])
	}
	return result
}

// TestCommand returns the test command for the primary detected language.
func (r *LanguageRegistry) TestCommand(projectDir string) string {
	cfg := r.Detect(projectDir)
	if cfg == nil {
		return ""
	}
	return cfg.TestCommand
}

// LintCommand returns the lint command for the primary detected language.
func (r *LanguageRegistry) LintCommand(projectDir string) string {
	cfg := r.Detect(projectDir)
	if cfg == nil {
		return ""
	}
	return cfg.LintCommand
}

// FormatCommand returns the format command for the primary detected language.
func (r *LanguageRegistry) FormatCommand(projectDir string) string {
	cfg := r.Detect(projectDir)
	if cfg == nil {
		return ""
	}
	return cfg.FormatCommand
}

// FormatLanguages produces a human-readable summary of detected languages.
// The first language is marked as "(primary)".
func FormatLanguages(configs []*LanguageConfig) string {
	if len(configs) == 0 {
		return "No languages detected."
	}

	var sb strings.Builder
	sb.WriteString("Detected Languages:\n")
	sb.WriteString(strings.Repeat("─", 21))
	sb.WriteString("\n")

	for i, cfg := range configs {
		sb.WriteString(cfg.Name)
		if i == 0 {
			sb.WriteString(" (primary)")
		}
		sb.WriteString(": ")

		parts := make([]string, 0, 3)
		if cfg.TestCommand != "" {
			// Show just the tool name for brevity.
			parts = append(parts, fmt.Sprintf("test=%s", shortCmd(cfg.TestCommand)))
		}
		if cfg.LintCommand != "" {
			parts = append(parts, fmt.Sprintf("lint=%s", shortCmd(cfg.LintCommand)))
		}
		if cfg.FormatCommand != "" {
			parts = append(parts, fmt.Sprintf("fmt=%s", shortCmd(cfg.FormatCommand)))
		}

		if len(parts) == 0 {
			sb.WriteString("(no commands)")
		} else {
			sb.WriteString(strings.Join(parts, ", "))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// shortCmd extracts just the tool/binary name from a command string.
func shortCmd(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return cmd
	}
	// Strip path prefix if any.
	base := filepath.Base(parts[0])
	// Strip leading "./" prefix.
	base = strings.TrimPrefix(base, "./")
	return base
}
