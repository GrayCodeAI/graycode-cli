package fingerprint

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ProjectFingerprint holds a comprehensive analysis of a project's type,
// tech stack, conventions, and recommended configuration.
type ProjectFingerprint struct {
	Language       string         // primary language
	Languages      []ProjectLangInfo // all detected languages
	Framework      string         // e.g., "chi", "gin", "express", "django", "next.js"
	BuildSystem    string         // "go modules", "npm", "cargo", "gradle", "maven"
	TestFramework  string         // "go test", "jest", "pytest", "cargo test"
	LintTools      []string
	PackageManager string
	CI             string // "github-actions", "gitlab-ci", "circleci"
	Docker         bool
	Monorepo       bool
	ProjectSize    string // "tiny" (<10 files), "small", "medium", "large" (>1000)
	Conventions    []Convention
	Recommendations []string

	// Internal tracking fields (unexported).
	totalFiles int
}

// ProjectLangInfo holds detection results for a single language in the project scan.
type ProjectLangInfo struct {
	Name       string
	FileCount  int
	Percentage float64
}

// Convention describes a detected coding convention.
type Convention struct {
	Name        string
	Description string
	Confidence  float64
}

// Scan performs a comprehensive analysis of the project directory, detecting
// languages, frameworks, build systems, CI, conventions, and generating
// recommendations.
func Scan(projectDir string) (*ProjectFingerprint, error) {
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: resolve path: %w", err)
	}

	// Check that the directory exists.
	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: stat directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("fingerprint: %s is not a directory", absDir)
	}

	fp := &ProjectFingerprint{}

	// Detect languages.
	fp.Languages = detectLanguages(absDir)
	if len(fp.Languages) > 0 {
		fp.Language = fp.Languages[0].Name
	}

	// Calculate total files.
	for _, l := range fp.Languages {
		fp.totalFiles += l.FileCount
	}

	// Detect project size.
	fp.ProjectSize = classifyProjectSize(fp.totalFiles)

	// Detect framework.
	fp.Framework = detectFramework(absDir, fp.Language)

	// Detect build system.
	fp.BuildSystem = detectBuildSystem(absDir)

	// Detect package manager.
	fp.PackageManager = detectProjectPackageManager(absDir)

	// Detect test framework.
	fp.TestFramework = detectTestFramework(absDir, fp.Language)

	// Detect lint tools.
	fp.LintTools = detectLintTools(absDir)

	// Detect CI system.
	fp.CI = detectCISystem(absDir)

	// Detect Docker.
	fp.Docker = detectDocker(absDir)

	// Detect monorepo.
	fp.Monorepo = detectMonorepo(absDir)

	// Detect conventions.
	fp.Conventions = detectConventions(absDir, fp.Language)

	// Generate recommendations.
	fp.Recommendations = generateRecommendations(fp)

	return fp, nil
}

// detectLanguages walks the project directory, maps extensions to languages,
// calculates percentages, and returns results sorted by file count descending.
func detectLanguages(dir string) []ProjectLangInfo {
	counts := make(map[string]int)

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		ext := filepath.Ext(path)
		if lang, ok := extToLang[ext]; ok {
			counts[lang]++
		}
		return nil
	})

	if len(counts) == 0 {
		return nil
	}

	total := 0
	for _, c := range counts {
		total += c
	}

	langs := make([]ProjectLangInfo, 0, len(counts))
	for name, count := range counts {
		pct := float64(count) / float64(total) * 100
		langs = append(langs, ProjectLangInfo{
			Name:       name,
			FileCount:  count,
			Percentage: pct,
		})
	}

	sort.Slice(langs, func(i, j int) bool {
		return langs[i].FileCount > langs[j].FileCount
	})

	return langs
}

// detectFramework reads key config files to determine the web/app framework.
func detectFramework(dir string, primaryLang string) string {
	switch primaryLang {
	case "Go":
		return detectGoFramework(dir)
	case "Python":
		return detectPythonFramework(dir)
	case "JavaScript", "TypeScript":
		return detectJSFramework(dir)
	case "Rust":
		return detectRustFramework(dir)
	}
	return ""
}

// detectGoFramework reads go.mod for known Go web frameworks.
func detectGoFramework(dir string) string {
	goModPath := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}
	content := string(data)

	frameworks := []struct {
		module string
		name   string
	}{
		{"github.com/go-chi/chi", "chi"},
		{"github.com/gin-gonic/gin", "gin"},
		{"github.com/labstack/echo", "echo"},
		{"github.com/gofiber/fiber", "fiber"},
		{"github.com/gorilla/mux", "gorilla"},
		{"github.com/julienschmidt/httprouter", "httprouter"},
		{"github.com/valyala/fasthttp", "fasthttp"},
	}

	for _, fw := range frameworks {
		if strings.Contains(content, fw.module) {
			return fw.name
		}
	}

	// Check for net/http usage (fallback — it's in stdlib so not in go.mod).
	return ""
}

// detectPythonFramework reads requirements.txt, Pipfile, or pyproject.toml.
func detectPythonFramework(dir string) string {
	files := []string{
		filepath.Join(dir, "requirements.txt"),
		filepath.Join(dir, "Pipfile"),
		filepath.Join(dir, "pyproject.toml"),
		filepath.Join(dir, "setup.py"),
	}

	frameworks := []struct {
		keyword string
		name    string
	}{
		{"django", "django"},
		{"Django", "django"},
		{"flask", "flask"},
		{"Flask", "flask"},
		{"fastapi", "fastapi"},
		{"FastAPI", "fastapi"},
		{"tornado", "tornado"},
		{"starlette", "starlette"},
		{"sanic", "sanic"},
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		content := string(data)
		for _, fw := range frameworks {
			if strings.Contains(content, fw.keyword) {
				return fw.name
			}
		}
	}

	return ""
}

// detectJSFramework reads package.json for known JS/TS frameworks.
func detectJSFramework(dir string) string {
	pkgPath := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return ""
	}

	var pkg struct {
		Dependencies    map[string]interface{} `json:"dependencies"`
		DevDependencies map[string]interface{} `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}

	// Merge deps for lookup.
	allDeps := make(map[string]bool)
	for k := range pkg.Dependencies {
		allDeps[k] = true
	}
	for k := range pkg.DevDependencies {
		allDeps[k] = true
	}

	frameworks := []struct {
		pkg  string
		name string
	}{
		{"next", "next.js"},
		{"nuxt", "nuxt"},
		{"@angular/core", "angular"},
		{"vue", "vue"},
		{"svelte", "svelte"},
		{"express", "express"},
		{"fastify", "fastify"},
		{"koa", "koa"},
		{"hapi", "hapi"},
		{"react", "react"},
		{"gatsby", "gatsby"},
		{"remix", "remix"},
	}

	for _, fw := range frameworks {
		if allDeps[fw.pkg] {
			return fw.name
		}
	}

	return ""
}

// detectRustFramework reads Cargo.toml for known Rust web frameworks.
func detectRustFramework(dir string) string {
	cargoPath := filepath.Join(dir, "Cargo.toml")
	data, err := os.ReadFile(cargoPath)
	if err != nil {
		return ""
	}
	content := string(data)

	frameworks := []struct {
		crate string
		name  string
	}{
		{"actix-web", "actix"},
		{"rocket", "rocket"},
		{"axum", "axum"},
		{"warp", "warp"},
		{"tide", "tide"},
	}

	for _, fw := range frameworks {
		if strings.Contains(content, fw.crate) {
			return fw.name
		}
	}

	return ""
}

// detectBuildSystem determines the project's build system from manifest files.
func detectBuildSystem(dir string) string {
	buildSystems := []struct {
		file   string
		system string
	}{
		{"go.mod", "go modules"},
		{"package.json", "npm"},
		{"Cargo.toml", "cargo"},
		{"pom.xml", "maven"},
		{"build.gradle", "gradle"},
		{"build.gradle.kts", "gradle"},
		{"CMakeLists.txt", "cmake"},
		{"Makefile", "make"},
		{"meson.build", "meson"},
		{"BUILD", "bazel"},
		{"WORKSPACE", "bazel"},
		{"mix.exs", "mix"},
		{"pubspec.yaml", "pub"},
		{"Package.swift", "swift package manager"},
		{"Rakefile", "rake"},
	}

	for _, bs := range buildSystems {
		path := filepath.Join(dir, bs.file)
		if _, err := os.Stat(path); err == nil {
			return bs.system
		}
	}

	return ""
}

// detectProjectPackageManager determines the project's package manager.
func detectProjectPackageManager(dir string) string {
	managers := []struct {
		file    string
		manager string
	}{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"package-lock.json", "npm"},
		{"bun.lockb", "bun"},
		{"go.sum", "go modules"},
		{"Cargo.lock", "cargo"},
		{"Pipfile.lock", "pipenv"},
		{"poetry.lock", "poetry"},
		{"Gemfile.lock", "bundler"},
		{"composer.lock", "composer"},
		{"pubspec.lock", "pub"},
		{"mix.lock", "mix"},
	}

	for _, m := range managers {
		path := filepath.Join(dir, m.file)
		if _, err := os.Stat(path); err == nil {
			return m.manager
		}
	}

	// Fallback: check manifest files.
	fallbacks := []struct {
		file    string
		manager string
	}{
		{"go.mod", "go modules"},
		{"package.json", "npm"},
		{"Cargo.toml", "cargo"},
		{"requirements.txt", "pip"},
		{"pyproject.toml", "pip"},
		{"Gemfile", "bundler"},
		{"composer.json", "composer"},
	}

	for _, f := range fallbacks {
		path := filepath.Join(dir, f.file)
		if _, err := os.Stat(path); err == nil {
			return f.manager
		}
	}

	return ""
}

// detectTestFramework determines the test framework used.
func detectTestFramework(dir string, lang string) string {
	switch lang {
	case "Go":
		// Go has a built-in test framework.
		// Check for testify or other test libs in go.mod.
		goModPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(goModPath); err == nil {
			content := string(data)
			if strings.Contains(content, "github.com/stretchr/testify") {
				return "go test + testify"
			}
		}
		// Check if there are any _test.go files.
		hasTests := false
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || hasTests {
				return filepath.SkipAll
			}
			if d.IsDir() && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(d.Name(), "_test.go") {
				hasTests = true
				return filepath.SkipAll
			}
			return nil
		})
		if hasTests {
			return "go test"
		}
		return ""

	case "JavaScript", "TypeScript":
		pkgPath := filepath.Join(dir, "package.json")
		data, err := os.ReadFile(pkgPath)
		if err != nil {
			return ""
		}
		var pkg struct {
			Dependencies    map[string]interface{} `json:"dependencies"`
			DevDependencies map[string]interface{} `json:"devDependencies"`
			Scripts         map[string]string      `json:"scripts"`
		}
		if err := json.Unmarshal(data, &pkg); err != nil {
			return ""
		}

		allDeps := make(map[string]bool)
		for k := range pkg.Dependencies {
			allDeps[k] = true
		}
		for k := range pkg.DevDependencies {
			allDeps[k] = true
		}

		if allDeps["vitest"] {
			return "vitest"
		}
		if allDeps["jest"] || allDeps["@jest/core"] || allDeps["ts-jest"] {
			return "jest"
		}
		if allDeps["mocha"] {
			return "mocha"
		}
		if allDeps["ava"] {
			return "ava"
		}
		if allDeps["cypress"] {
			return "cypress"
		}
		if allDeps["playwright"] || allDeps["@playwright/test"] {
			return "playwright"
		}
		return ""

	case "Python":
		// Check for pytest in requirements or installed.
		files := []string{
			filepath.Join(dir, "requirements.txt"),
			filepath.Join(dir, "requirements-dev.txt"),
			filepath.Join(dir, "pyproject.toml"),
			filepath.Join(dir, "setup.cfg"),
			filepath.Join(dir, "Pipfile"),
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			content := string(data)
			if strings.Contains(content, "pytest") {
				return "pytest"
			}
		}
		// Check for pytest.ini or conftest.py.
		if _, err := os.Stat(filepath.Join(dir, "pytest.ini")); err == nil {
			return "pytest"
		}
		if _, err := os.Stat(filepath.Join(dir, "conftest.py")); err == nil {
			return "pytest"
		}
		// Check for test files with unittest patterns.
		hasUnittest := false
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || hasUnittest {
				return filepath.SkipAll
			}
			if d.IsDir() && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if !d.IsDir() && (strings.HasPrefix(d.Name(), "test_") || strings.HasSuffix(d.Name(), "_test.py")) {
				hasUnittest = true
				return filepath.SkipAll
			}
			return nil
		})
		if hasUnittest {
			return "unittest"
		}
		return ""

	case "Rust":
		// Rust uses built-in test framework via cargo test.
		cargoPath := filepath.Join(dir, "Cargo.toml")
		if _, err := os.Stat(cargoPath); err == nil {
			return "cargo test"
		}
		return ""

	case "Java":
		pomPath := filepath.Join(dir, "pom.xml")
		if data, err := os.ReadFile(pomPath); err == nil {
			content := string(data)
			if strings.Contains(content, "junit") || strings.Contains(content, "JUnit") {
				return "junit"
			}
			if strings.Contains(content, "testng") {
				return "testng"
			}
		}
		gradlePath := filepath.Join(dir, "build.gradle")
		if data, err := os.ReadFile(gradlePath); err == nil {
			content := string(data)
			if strings.Contains(content, "junit") || strings.Contains(content, "JUnit") {
				return "junit"
			}
			if strings.Contains(content, "testng") {
				return "testng"
			}
		}
		return ""

	case "Ruby":
		if _, err := os.Stat(filepath.Join(dir, "spec")); err == nil {
			return "rspec"
		}
		gemPath := filepath.Join(dir, "Gemfile")
		if data, err := os.ReadFile(gemPath); err == nil {
			content := string(data)
			if strings.Contains(content, "rspec") {
				return "rspec"
			}
			if strings.Contains(content, "minitest") {
				return "minitest"
			}
		}
		return ""
	}

	return ""
}

// detectLintTools detects configured linting tools from config files.
func detectLintTools(dir string) []string {
	var tools []string

	lintConfigs := []struct {
		file string
		tool string
	}{
		{".golangci.yml", "golangci-lint"},
		{".golangci.yaml", "golangci-lint"},
		{".golangci.toml", "golangci-lint"},
		{".eslintrc", "eslint"},
		{".eslintrc.js", "eslint"},
		{".eslintrc.json", "eslint"},
		{".eslintrc.yml", "eslint"},
		{"eslint.config.js", "eslint"},
		{"eslint.config.mjs", "eslint"},
		{".prettierrc", "prettier"},
		{".prettierrc.js", "prettier"},
		{".prettierrc.json", "prettier"},
		{"prettier.config.js", "prettier"},
		{".stylelintrc", "stylelint"},
		{".stylelintrc.json", "stylelint"},
		{"stylelint.config.js", "stylelint"},
		{".flake8", "flake8"},
		{"setup.cfg", "flake8"}, // might contain flake8 config
		{".pylintrc", "pylint"},
		{"pyproject.toml", "ruff"}, // might contain ruff config
		{".rubocop.yml", "rubocop"},
		{"clippy.toml", "clippy"},
		{".clippy.toml", "clippy"},
		{".editorconfig", "editorconfig"},
		{"biome.json", "biome"},
		{"deno.json", "deno lint"},
		{".hadolint.yaml", "hadolint"},
		{".shellcheckrc", "shellcheck"},
	}

	seen := make(map[string]bool)
	for _, lc := range lintConfigs {
		path := filepath.Join(dir, lc.file)
		if _, err := os.Stat(path); err == nil {
			// Special case: setup.cfg / pyproject.toml may or may not contain lint config.
			if lc.file == "setup.cfg" {
				if data, err := os.ReadFile(path); err == nil {
					if !strings.Contains(string(data), "[flake8]") {
						continue
					}
				}
			}
			if lc.file == "pyproject.toml" {
				if data, err := os.ReadFile(path); err == nil {
					if !strings.Contains(string(data), "[tool.ruff]") && !strings.Contains(string(data), "ruff") {
						continue
					}
				}
			}
			if !seen[lc.tool] {
				seen[lc.tool] = true
				tools = append(tools, lc.tool)
			}
		}
	}

	// Check package.json for lint-related devDependencies.
	pkgPath := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		var pkg struct {
			DevDependencies map[string]interface{} `json:"devDependencies"`
		}
		if err := json.Unmarshal(data, &pkg); err == nil {
			jsDeps := []struct {
				pkg  string
				tool string
			}{
				{"eslint", "eslint"},
				{"prettier", "prettier"},
				{"stylelint", "stylelint"},
				{"biome", "biome"},
				{"@biomejs/biome", "biome"},
			}
			for _, jd := range jsDeps {
				if _, ok := pkg.DevDependencies[jd.pkg]; ok && !seen[jd.tool] {
					seen[jd.tool] = true
					tools = append(tools, jd.tool)
				}
			}
		}
	}

	return tools
}

// detectCISystem identifies the CI/CD system in use and returns its name.
func detectCISystem(dir string) string {
	ciSystems := []struct {
		path   string
		isDir  bool
		system string
	}{
		{filepath.Join(".github", "workflows"), true, "github-actions"},
		{".gitlab-ci.yml", false, "gitlab-ci"},
		{".circleci", true, "circleci"},
		{"Jenkinsfile", false, "jenkins"},
		{".travis.yml", false, "travis-ci"},
		{"azure-pipelines.yml", false, "azure-pipelines"},
		{"bitbucket-pipelines.yml", false, "bitbucket-pipelines"},
		{".drone.yml", false, "drone"},
		{".buildkite", true, "buildkite"},
		{"cloudbuild.yaml", false, "google-cloud-build"},
		{"cloudbuild.yml", false, "google-cloud-build"},
		{".tekton", true, "tekton"},
	}

	for _, ci := range ciSystems {
		full := filepath.Join(dir, ci.path)
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		if ci.isDir && info.IsDir() {
			return ci.system
		}
		if !ci.isDir && !info.IsDir() {
			return ci.system
		}
	}

	return ""
}

// detectDocker checks for Dockerfile or docker-compose files.
func detectDocker(dir string) bool {
	dockerFiles := []string{
		"Dockerfile",
		"dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
		".dockerignore",
	}

	for _, f := range dockerFiles {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}

// detectMonorepo checks for indicators of a monorepo structure.
func detectMonorepo(dir string) bool {
	// Check for multiple go.mod files.
	goModCount := 0
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "go.mod" {
			goModCount++
			if goModCount > 1 {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if goModCount > 1 {
		return true
	}

	// Check for go.work file.
	if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
		return true
	}

	// Check for packages/ or apps/ directory (JS monorepos).
	monoDirs := []string{"packages", "apps", "modules", "services", "libs"}
	for _, md := range monoDirs {
		path := filepath.Join(dir, md)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return true
		}
	}

	// Check for lerna.json, pnpm-workspace.yaml, or turbo.json.
	monoFiles := []string{"lerna.json", "pnpm-workspace.yaml", "turbo.json", "nx.json"}
	for _, mf := range monoFiles {
		path := filepath.Join(dir, mf)
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	// Check package.json for workspaces field.
	pkgPath := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		var pkg map[string]interface{}
		if err := json.Unmarshal(data, &pkg); err == nil {
			if _, ok := pkg["workspaces"]; ok {
				return true
			}
		}
	}

	return false
}

// classifyProjectSize returns a size classification based on file count.
func classifyProjectSize(fileCount int) string {
	switch {
	case fileCount < 10:
		return "tiny"
	case fileCount < 100:
		return "small"
	case fileCount <= 1000:
		return "medium"
	default:
		return "large"
	}
}

// detectConventions analyzes the project to identify coding conventions.
func detectConventions(dir string, lang string) []Convention {
	var conventions []Convention

	// Detect indentation from .editorconfig.
	if conv := detectIndentationConvention(dir); conv != nil {
		conventions = append(conventions, *conv)
	}

	// Detect naming convention by sampling source files.
	if conv := detectNamingConvention(dir, lang); conv != nil {
		conventions = append(conventions, *conv)
	}

	// Detect error handling style (Go-specific).
	if lang == "Go" {
		if conv := detectGoErrorHandling(dir); conv != nil {
			conventions = append(conventions, *conv)
		}
	}

	// Detect import organization.
	if conv := detectImportOrganization(dir, lang); conv != nil {
		conventions = append(conventions, *conv)
	}

	// Detect test naming convention.
	if conv := detectTestNaming(dir, lang); conv != nil {
		conventions = append(conventions, *conv)
	}

	// Detect commit message style.
	if conv := detectCommitStyle(dir); conv != nil {
		conventions = append(conventions, *conv)
	}

	return conventions
}

// detectIndentationConvention reads .editorconfig or samples files.
func detectIndentationConvention(dir string) *Convention {
	// Check .editorconfig first.
	editorConfigPath := filepath.Join(dir, ".editorconfig")
	if data, err := os.ReadFile(editorConfigPath); err == nil {
		content := strings.ToLower(string(data))
		if strings.Contains(content, "indent_style = tab") {
			return &Convention{
				Name:        "indentation",
				Description: "Tabs for indentation",
				Confidence:  1.0,
			}
		}
		if strings.Contains(content, "indent_style = space") {
			// Try to find indent_size.
			size := "unknown"
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "indent_size") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						size = strings.TrimSpace(parts[1])
					}
				}
			}
			desc := "Spaces for indentation"
			if size != "unknown" {
				desc = fmt.Sprintf("%s-space indentation", size)
			}
			return &Convention{
				Name:        "indentation",
				Description: desc,
				Confidence:  1.0,
			}
		}
	}

	// Sample source files to detect indentation.
	tabCount := 0
	spaceCount := 0
	sampled := 0
	maxSamples := 20

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || sampled >= maxSamples {
			return filepath.SkipAll
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if _, ok := extToLang[ext]; !ok {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()

		scanner := bufio.NewScanner(f)
		lineCount := 0
		for scanner.Scan() && lineCount < 50 {
			line := scanner.Text()
			if len(line) > 0 {
				if line[0] == '\t' {
					tabCount++
				} else if line[0] == ' ' && len(line) > 1 && line[1] == ' ' {
					spaceCount++
				}
			}
			lineCount++
		}
		sampled++
		return nil
	})

	total := tabCount + spaceCount
	if total == 0 {
		return nil
	}

	if tabCount > spaceCount {
		confidence := float64(tabCount) / float64(total)
		return &Convention{
			Name:        "indentation",
			Description: "Tabs for indentation",
			Confidence:  confidence,
		}
	}
	confidence := float64(spaceCount) / float64(total)
	return &Convention{
		Name:        "indentation",
		Description: "Spaces for indentation",
		Confidence:  confidence,
	}
}

// detectNamingConvention samples identifiers to determine naming style.
func detectNamingConvention(dir string, lang string) *Convention {
	// For Go, the convention is well-known: exported = PascalCase, local = camelCase.
	if lang == "Go" {
		return &Convention{
			Name:        "naming",
			Description: "camelCase/PascalCase (Go standard)",
			Confidence:  1.0,
		}
	}

	// For Python, sample for snake_case vs camelCase.
	if lang == "Python" {
		snakeCount := 0
		camelCount := 0
		sampled := 0

		snakeRe := regexp.MustCompile(`\bdef ([a-z][a-z0-9]*_[a-z0-9_]+)\b`)
		camelRe := regexp.MustCompile(`\bdef ([a-z][a-zA-Z0-9]+[A-Z][a-zA-Z0-9]*)\b`)

		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || sampled >= 10 {
				return filepath.SkipAll
			}
			if d.IsDir() && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if d.IsDir() || filepath.Ext(path) != ".py" {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			content := string(data)
			snakeCount += len(snakeRe.FindAllString(content, -1))
			camelCount += len(camelRe.FindAllString(content, -1))
			sampled++
			return nil
		})

		total := snakeCount + camelCount
		if total == 0 {
			return nil
		}
		if snakeCount > camelCount {
			return &Convention{
				Name:        "naming",
				Description: "snake_case (Python standard)",
				Confidence:  float64(snakeCount) / float64(total),
			}
		}
		return &Convention{
			Name:        "naming",
			Description: "camelCase",
			Confidence:  float64(camelCount) / float64(total),
		}
	}

	return nil
}

// detectGoErrorHandling checks error handling patterns in Go source files.
func detectGoErrorHandling(dir string) *Convention {
	wrapCount := 0  // fmt.Errorf("...: %w", err)
	bareCount := 0  // return err (without wrapping)
	sampled := 0

	wrapRe := regexp.MustCompile(`fmt\.Errorf\([^)]*%w`)
	bareRe := regexp.MustCompile(`return\s+err\b`)

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || sampled >= 20 {
			return filepath.SkipAll
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		wrapCount += len(wrapRe.FindAllString(content, -1))
		bareCount += len(bareRe.FindAllString(content, -1))
		sampled++
		return nil
	})

	total := wrapCount + bareCount
	if total == 0 {
		return nil
	}

	if wrapCount > bareCount {
		return &Convention{
			Name:        "error-handling",
			Description: "Error wrapping with %w",
			Confidence:  float64(wrapCount) / float64(total),
		}
	}
	return &Convention{
		Name:        "error-handling",
		Description: "Bare error returns",
		Confidence:  float64(bareCount) / float64(total),
	}
}

// detectImportOrganization checks if imports are grouped (stdlib vs third-party).
func detectImportOrganization(dir string, lang string) *Convention {
	if lang != "Go" {
		return nil
	}

	groupedCount := 0
	ungroupedCount := 0
	sampled := 0

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || sampled >= 15 {
			return filepath.SkipAll
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)

		// Find import blocks.
		importStart := strings.Index(content, "import (")
		if importStart == -1 {
			return nil
		}
		importEnd := strings.Index(content[importStart:], ")")
		if importEnd == -1 {
			return nil
		}
		importBlock := content[importStart : importStart+importEnd]

		// Check for blank lines within the import block (indicating grouping).
		if strings.Contains(importBlock, "\n\n") {
			groupedCount++
		} else {
			// Only count as ungrouped if there are multiple imports.
			lines := strings.Split(importBlock, "\n")
			importLines := 0
			for _, l := range lines {
				l = strings.TrimSpace(l)
				if l != "" && l != "import (" && l != ")" && !strings.HasPrefix(l, "//") {
					importLines++
				}
			}
			if importLines > 1 {
				ungroupedCount++
			}
		}
		sampled++
		return nil
	})

	total := groupedCount + ungroupedCount
	if total == 0 {
		return nil
	}

	if groupedCount > ungroupedCount {
		return &Convention{
			Name:        "imports",
			Description: "Grouped imports (stdlib separated from third-party)",
			Confidence:  float64(groupedCount) / float64(total),
		}
	}
	return &Convention{
		Name:        "imports",
		Description: "Ungrouped imports",
		Confidence:  float64(ungroupedCount) / float64(total),
	}
}

// detectTestNaming checks test naming conventions.
func detectTestNaming(dir string, lang string) *Convention {
	if lang != "Go" {
		return nil
	}

	// Check for table-driven tests vs simple tests.
	tableDrivenCount := 0
	simpleCount := 0
	sampled := 0

	tableDrivenRe := regexp.MustCompile(`(tests|cases|testCases|tt)\s*:?=\s*\[\]struct`)
	simpleFuncRe := regexp.MustCompile(`func Test[A-Z]\w+\(t \*testing\.T\)`)

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || sampled >= 15 {
			return filepath.SkipAll
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		tableDrivenCount += len(tableDrivenRe.FindAllString(content, -1))
		simpleCount += len(simpleFuncRe.FindAllString(content, -1))
		sampled++
		return nil
	})

	if tableDrivenCount > 0 && simpleCount > 0 {
		total := tableDrivenCount + simpleCount
		if tableDrivenCount > simpleCount/2 {
			return &Convention{
				Name:        "test-style",
				Description: "Table-driven tests",
				Confidence:  float64(tableDrivenCount) / float64(total),
			}
		}
	}

	return nil
}

// detectCommitStyle checks git log for conventional commits or other patterns.
func detectCommitStyle(dir string) *Convention {
	cmd := exec.Command("git", "log", "--oneline", "-20", "--format=%s")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return nil
	}

	// Check for conventional commits (feat:, fix:, chore:, etc.).
	conventionalRe := regexp.MustCompile(`^(feat|fix|chore|docs|style|refactor|perf|test|build|ci|revert)(\(.+\))?:`)
	conventionalCount := 0

	for _, line := range lines {
		if conventionalRe.MatchString(line) {
			conventionalCount++
		}
	}

	if conventionalCount > 0 {
		confidence := float64(conventionalCount) / float64(len(lines))
		if confidence >= 0.3 {
			return &Convention{
				Name:        "commit-style",
				Description: "Conventional commits (feat:, fix:, etc.)",
				Confidence:  confidence,
			}
		}
	}

	return nil
}

// generateRecommendations produces hawk configuration suggestions based on the
// detected project fingerprint.
func generateRecommendations(fp *ProjectFingerprint) []string {
	var recs []string

	// Language-specific recommendations.
	switch fp.Language {
	case "Go":
		if fp.TestFramework == "go test" || fp.TestFramework == "go test + testify" {
			recs = append(recs, "Add `go test ./...` as your test command")
		}
		if !containsString(fp.LintTools, "golangci-lint") {
			recs = append(recs, "Consider adding .golangci.yml for consistent linting")
		}
		if fp.Framework == "chi" {
			recs = append(recs, "Your project uses chi router — hawk can help with middleware patterns")
		}
		if fp.Framework == "gin" {
			recs = append(recs, "Your project uses gin — hawk can help with handler patterns")
		}
		if fp.Framework == "echo" {
			recs = append(recs, "Your project uses echo — hawk can help with middleware and routing")
		}

	case "JavaScript", "TypeScript":
		if fp.TestFramework == "jest" {
			recs = append(recs, "Add `npm test` or `npx jest` as your test command")
		}
		if fp.TestFramework == "vitest" {
			recs = append(recs, "Add `npx vitest` as your test command")
		}
		if !containsString(fp.LintTools, "eslint") && !containsString(fp.LintTools, "biome") {
			recs = append(recs, "Consider adding ESLint or Biome for consistent linting")
		}
		if fp.Framework == "next.js" {
			recs = append(recs, "Your project uses Next.js — hawk can help with App Router patterns")
		}

	case "Python":
		if fp.TestFramework == "pytest" {
			recs = append(recs, "Add `pytest` as your test command")
		}
		if !containsString(fp.LintTools, "ruff") && !containsString(fp.LintTools, "flake8") {
			recs = append(recs, "Consider adding ruff or flake8 for Python linting")
		}

	case "Rust":
		if fp.TestFramework == "cargo test" {
			recs = append(recs, "Add `cargo test` as your test command")
		}
		if !containsString(fp.LintTools, "clippy") {
			recs = append(recs, "Consider adding clippy for Rust linting")
		}
	}

	// CI recommendations.
	if fp.CI == "" {
		recs = append(recs, "No CI detected — consider adding GitHub Actions for automated testing")
	}

	// Docker recommendations.
	if !fp.Docker && fp.ProjectSize != "tiny" {
		recs = append(recs, "Consider adding a Dockerfile for reproducible builds")
	}

	// Monorepo recommendations.
	if fp.Monorepo {
		recs = append(recs, "Monorepo detected — configure hawk to scope analysis to relevant packages")
	}

	// Convention-based recommendations.
	hasEditorConfig := false
	for _, c := range fp.Conventions {
		if c.Name == "indentation" && c.Confidence < 0.8 {
			recs = append(recs, "Inconsistent indentation detected — consider adding .editorconfig")
		}
	}
	for _, tool := range fp.LintTools {
		if tool == "editorconfig" {
			hasEditorConfig = true
		}
	}
	_ = hasEditorConfig

	return recs
}

// FormatSummary produces a human-readable summary of the project fingerprint.
func FormatSummary(fp *ProjectFingerprint) string {
	var b strings.Builder

	// Project language line.
	if len(fp.Languages) > 0 {
		b.WriteString("Project: ")
		parts := make([]string, 0, len(fp.Languages))
		for _, l := range fp.Languages {
			parts = append(parts, fmt.Sprintf("%s (%.0f%%)", l.Name, l.Percentage))
		}
		// Show top languages (limit to 5).
		limit := len(parts)
		if limit > 5 {
			limit = 5
		}
		if limit == 1 {
			b.WriteString(parts[0])
		} else {
			b.WriteString(parts[0])
			for i := 1; i < limit; i++ {
				b.WriteString(" with some ")
				b.WriteString(parts[i])
			}
		}
		b.WriteByte('\n')
	}

	// Framework.
	if fp.Framework != "" {
		b.WriteString(fmt.Sprintf("Framework: %s\n", fp.Framework))
	}

	// Build system.
	if fp.BuildSystem != "" {
		b.WriteString(fmt.Sprintf("Build: %s\n", fp.BuildSystem))
	}

	// Tests.
	if fp.TestFramework != "" {
		b.WriteString(fmt.Sprintf("Tests: %s\n", fp.TestFramework))
	}

	// CI.
	if fp.CI != "" {
		// Format CI name nicely.
		ciName := fp.CI
		switch ciName {
		case "github-actions":
			ciName = "GitHub Actions"
		case "gitlab-ci":
			ciName = "GitLab CI"
		case "circleci":
			ciName = "CircleCI"
		case "jenkins":
			ciName = "Jenkins"
		case "travis-ci":
			ciName = "Travis CI"
		}
		b.WriteString(fmt.Sprintf("CI: %s\n", ciName))
	}

	// Size.
	if fp.ProjectSize != "" {
		totalFiles := 0
		for _, l := range fp.Languages {
			totalFiles += l.FileCount
		}
		b.WriteString(fmt.Sprintf("Size: %s (%d files)\n", fp.ProjectSize, totalFiles))
	}

	// Docker.
	if fp.Docker {
		b.WriteString("Docker: yes\n")
	}

	// Monorepo.
	if fp.Monorepo {
		b.WriteString("Monorepo: yes\n")
	}

	// Package manager.
	if fp.PackageManager != "" {
		b.WriteString(fmt.Sprintf("Package Manager: %s\n", fp.PackageManager))
	}

	// Lint tools.
	if len(fp.LintTools) > 0 {
		b.WriteString(fmt.Sprintf("Lint: %s\n", strings.Join(fp.LintTools, ", ")))
	}

	// Conventions.
	if len(fp.Conventions) > 0 {
		b.WriteString("\nConventions:\n")
		for _, c := range fp.Conventions {
			b.WriteString(fmt.Sprintf("  - %s (%.0f%% confidence)\n", c.Description, c.Confidence*100))
		}
	}

	// Recommendations.
	if len(fp.Recommendations) > 0 {
		b.WriteString("\nRecommendations:\n")
		for _, r := range fp.Recommendations {
			b.WriteString(fmt.Sprintf("  - %s\n", r))
		}
	}

	return b.String()
}

// containsString checks if a slice contains a given string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
