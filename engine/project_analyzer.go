package engine

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ProjectAnalysis holds the full analysis of a project's architecture, patterns, and conventions.
type ProjectAnalysis struct {
	Name         string
	Language     string
	Framework    string
	Architecture string
	EntryPoints  []string
	KeyModules   []ModuleInfo
	Patterns     []Pattern
	Conventions  []string
	Dependencies int
	TestCoverage string
	LOC          int
	Complexity   string
}

// ModuleInfo describes a single module/package within the project.
type ModuleInfo struct {
	Name         string
	Path         string
	Purpose      string
	PublicAPI    []string
	Dependencies []string
	Size         int
}

// Pattern represents an architectural or design pattern detected in the codebase.
type Pattern struct {
	Name        string
	Description string
	Files       []string
	Confidence  float64
}

// ProjectAnalyzer performs deep analysis of a codebase to understand its architecture,
// patterns, and conventions. Inspired by gpt-pilot's importer agent.
type ProjectAnalyzer struct {
	Dir string
	mu  sync.Mutex
}

// NewProjectAnalyzer creates a new ProjectAnalyzer for the given directory.
func NewProjectAnalyzer(dir string) *ProjectAnalyzer {
	return &ProjectAnalyzer{Dir: dir}
}

// Analyze performs a full project scan: detects language/framework, maps architecture,
// identifies entry points, detects patterns, and extracts conventions.
func (pa *ProjectAnalyzer) Analyze() (*ProjectAnalysis, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	if _, err := os.Stat(pa.Dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("project directory does not exist: %s", pa.Dir)
	}

	analysis := &ProjectAnalysis{}

	// Detect project name from go.mod or directory name.
	analysis.Name = pa.detectProjectName()

	// Detect language and framework.
	analysis.Language = pa.detectLanguage()
	analysis.Framework = pa.detectFramework()

	// Detect architecture.
	analysis.Architecture = DetectArchitecture(pa.Dir)

	// Find entry points.
	analysis.EntryPoints = pa.detectEntryPoints()

	// Analyze key modules.
	analysis.KeyModules = pa.analyzeKeyModules()

	// Detect patterns.
	analysis.Patterns = DetectPatterns(pa.Dir)

	// Extract conventions.
	analysis.Conventions = pa.detectConventions()

	// Count dependencies.
	analysis.Dependencies = pa.countDependencies()

	// Calculate LOC.
	analysis.LOC = pa.countLOC()

	// Assess test coverage.
	analysis.TestCoverage = pa.assessTestCoverage()

	// Assess complexity.
	analysis.Complexity = pa.assessComplexity()

	return analysis, nil
}

// DetectArchitecture determines the architectural style of a project by examining
// its directory structure.
func DetectArchitecture(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "unknown"
	}

	dirs := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirs[entry.Name()] = true
		}
	}

	// Hexagonal: domain/ + ports/ + adapters/
	if dirs["domain"] && dirs["ports"] && dirs["adapters"] {
		return "hexagonal"
	}

	// Microservices: multiple service directories.
	serviceCount := 0
	for name := range dirs {
		if strings.HasSuffix(name, "-service") || strings.HasSuffix(name, "-svc") {
			serviceCount++
		}
	}
	if dirs["services"] || serviceCount >= 2 {
		return "microservices"
	}

	// Layered: cmd/ -> service/ or internal/ -> repository/ or repo/
	if dirs["cmd"] && (dirs["service"] || dirs["internal"] || dirs["engine"]) {
		if dirs["repo"] || dirs["repository"] || dirs["store"] || dirs["tool"] {
			return "layered"
		}
		return "layered"
	}

	// Modular: feature-based directories (more than 4 sibling directories with similar structure).
	featureDirs := 0
	for name := range dirs {
		subPath := filepath.Join(dir, name)
		if hasGoFiles(subPath) {
			featureDirs++
		}
	}
	if featureDirs >= 5 && !dirs["cmd"] {
		return "modular"
	}

	// Monolith: single main package.
	if hasMainPackage(dir) && featureDirs <= 2 {
		return "monolith"
	}

	// Default to modular if there are many subdirectories.
	if featureDirs >= 4 {
		return "modular"
	}

	return "monolith"
}

// DetectPatterns identifies design patterns used in the codebase.
func DetectPatterns(dir string) []Pattern {
	var patterns []Pattern

	// Repository pattern: *Repository interfaces + implementations.
	repoFiles := findFilesWithPattern(dir, "repository", "repo")
	if len(repoFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Repository",
			Description: "Data access abstracted behind repository interfaces",
			Files:       repoFiles,
			Confidence:  calculateConfidence(repoFiles, 2),
		})
	}

	// Middleware pattern: handler wrappers, interceptors.
	middlewareFiles := findFilesWithPattern(dir, "middleware", "interceptor")
	if len(middlewareFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Middleware",
			Description: "Request/response processing chain with handler wrappers",
			Files:       middlewareFiles,
			Confidence:  calculateConfidence(middlewareFiles, 2),
		})
	}

	// Factory pattern: New* constructors.
	factoryFiles := findFactoryPattern(dir)
	if len(factoryFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Factory",
			Description: "Object creation via New* constructor functions",
			Files:       factoryFiles,
			Confidence:  calculateConfidence(factoryFiles, 5),
		})
	}

	// Observer pattern: event/listener files.
	observerFiles := findFilesWithPattern(dir, "event", "listener", "observer", "hook")
	if len(observerFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Observer",
			Description: "Event-driven communication with listeners/hooks",
			Files:       observerFiles,
			Confidence:  calculateConfidence(observerFiles, 2),
		})
	}

	// Strategy pattern: interface + multiple implementations.
	strategyFiles := findStrategyPattern(dir)
	if len(strategyFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Strategy",
			Description: "Interface with multiple interchangeable implementations",
			Files:       strategyFiles,
			Confidence:  calculateConfidence(strategyFiles, 3),
		})
	}

	// Interface-driven tools pattern.
	toolFiles := findFilesWithPattern(dir, "tool")
	if len(toolFiles) >= 3 {
		patterns = append(patterns, Pattern{
			Name:        "Interface-driven tools",
			Description: "Tool interface with multiple implementations",
			Files:       toolFiles,
			Confidence:  calculateConfidence(toolFiles, 5),
		})
	}

	// Functional options pattern (WithXxx).
	optionFiles := findFunctionalOptionsPattern(dir)
	if len(optionFiles) > 0 {
		patterns = append(patterns, Pattern{
			Name:        "Functional Options",
			Description: "Configuration via WithXxx option functions",
			Files:       optionFiles,
			Confidence:  calculateConfidence(optionFiles, 3),
		})
	}

	return patterns
}

// AnalyzeModule scans a package directory and extracts its public API, line count, and purpose.
func (pa *ProjectAnalyzer) AnalyzeModule(path string) *ModuleInfo {
	info := &ModuleInfo{
		Name: filepath.Base(path),
		Path: path,
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return info
	}

	fset := token.NewFileSet()
	var totalLines int

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		filePath := filepath.Join(path, entry.Name())

		// Count lines.
		totalLines += countFileLines(filePath)

		// Parse for public API.
		f, parseErr := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if parseErr != nil {
			continue
		}

		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() {
					sig := d.Name.Name
					if d.Recv != nil && len(d.Recv.List) > 0 {
						recvType := projAnalyzerExprToString(d.Recv.List[0].Type)
						sig = recvType + "." + sig
					}
					info.PublicAPI = append(info.PublicAPI, sig)
				}
			case *ast.GenDecl:
				if d.Tok == token.TYPE {
					for _, spec := range d.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
							info.PublicAPI = append(info.PublicAPI, ts.Name.Name)
						}
					}
				}
			}
		}

		// Extract imports for dependencies.
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if !strings.Contains(importPath, ".") {
				continue // skip stdlib
			}
			info.Dependencies = projAnalyzerAppendUnique(info.Dependencies, importPath)
		}
	}

	info.Size = totalLines
	info.Purpose = pa.inferPurpose(info)

	return info
}

// GenerateOnboardingDoc produces a human-readable onboarding document from the analysis.
func GenerateOnboardingDoc(analysis *ProjectAnalysis) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Project: %s\n\n", analysis.Name))

	// Architecture section.
	b.WriteString(fmt.Sprintf("## Architecture: %s\n", projAnalyzerTitle(analysis.Architecture)))
	if len(analysis.KeyModules) > 0 {
		moduleNames := make([]string, 0, len(analysis.KeyModules))
		for _, m := range analysis.KeyModules {
			moduleNames = append(moduleNames, m.Name)
		}
		b.WriteString(strings.Join(moduleNames, " -> "))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Key modules section.
	b.WriteString("## Key Modules\n")
	for _, m := range analysis.KeyModules {
		locStr := formatLOC(m.Size)
		purpose := m.Purpose
		if purpose == "" {
			purpose = "Core functionality"
		}
		b.WriteString(fmt.Sprintf("- %s (%s): %s\n", m.Name, locStr, purpose))
	}
	b.WriteString("\n")

	// Patterns section.
	if len(analysis.Patterns) > 0 {
		b.WriteString("## Patterns Detected\n")
		for _, p := range analysis.Patterns {
			if p.Confidence >= 0.5 {
				b.WriteString(fmt.Sprintf("- %s (%s)\n", p.Name, p.Description))
			}
		}
		b.WriteString("\n")
	}

	// Conventions section.
	if len(analysis.Conventions) > 0 {
		b.WriteString("## Conventions\n")
		for _, c := range analysis.Conventions {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
		b.WriteString("\n")
	}

	// Stats section.
	b.WriteString("## Stats\n")
	b.WriteString(fmt.Sprintf("- Language: %s\n", analysis.Language))
	if analysis.Framework != "" {
		b.WriteString(fmt.Sprintf("- Framework: %s\n", analysis.Framework))
	}
	b.WriteString(fmt.Sprintf("- Total LOC: %s\n", formatLOC(analysis.LOC)))
	b.WriteString(fmt.Sprintf("- Dependencies: %d\n", analysis.Dependencies))
	b.WriteString(fmt.Sprintf("- Test Coverage: %s\n", analysis.TestCoverage))
	b.WriteString(fmt.Sprintf("- Complexity: %s\n", analysis.Complexity))

	return b.String()
}

// FormatAnalysis produces a concise summary string from a ProjectAnalysis.
func FormatAnalysis(analysis *ProjectAnalysis) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Project: %s (%s", analysis.Name, analysis.Language))
	if analysis.Framework != "" {
		b.WriteString(fmt.Sprintf(" / %s", analysis.Framework))
	}
	b.WriteString(")\n")

	b.WriteString(fmt.Sprintf("Architecture: %s\n", analysis.Architecture))
	b.WriteString(fmt.Sprintf("LOC: %s | Deps: %d | Tests: %s | Complexity: %s\n",
		formatLOC(analysis.LOC), analysis.Dependencies, analysis.TestCoverage, analysis.Complexity))

	if len(analysis.EntryPoints) > 0 {
		b.WriteString(fmt.Sprintf("Entry Points: %s\n", strings.Join(analysis.EntryPoints, ", ")))
	}

	if len(analysis.KeyModules) > 0 {
		b.WriteString(fmt.Sprintf("Modules: %d key modules\n", len(analysis.KeyModules)))
	}

	if len(analysis.Patterns) > 0 {
		patternNames := make([]string, 0, len(analysis.Patterns))
		for _, p := range analysis.Patterns {
			if p.Confidence >= 0.5 {
				patternNames = append(patternNames, p.Name)
			}
		}
		if len(patternNames) > 0 {
			b.WriteString(fmt.Sprintf("Patterns: %s\n", strings.Join(patternNames, ", ")))
		}
	}

	return b.String()
}

// --- Private helper methods ---

func (pa *ProjectAnalyzer) detectProjectName() string {
	// Try go.mod first.
	modPath := filepath.Join(pa.Dir, "go.mod")
	if f, err := os.Open(modPath); err == nil {
		defer func() { _ = f.Close() }()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "module ") {
				mod := strings.TrimSpace(strings.TrimPrefix(line, "module"))
				parts := strings.Split(mod, "/")
				return parts[len(parts)-1]
			}
		}
	}

	// Fall back to directory name.
	return filepath.Base(pa.Dir)
}

func (pa *ProjectAnalyzer) detectLanguage() string {
	counts := map[string]int{
		"Go":         0,
		"Python":     0,
		"JavaScript": 0,
		"TypeScript": 0,
		"Rust":       0,
		"Java":       0,
	}

	_ = filepath.Walk(pa.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == "node_modules" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		switch ext {
		case ".go":
			counts["Go"]++
		case ".py":
			counts["Python"]++
		case ".js":
			counts["JavaScript"]++
		case ".ts", ".tsx":
			counts["TypeScript"]++
		case ".rs":
			counts["Rust"]++
		case ".java":
			counts["Java"]++
		}
		return nil
	})

	maxLang := "Go"
	maxCount := 0
	for lang, count := range counts {
		if count > maxCount {
			maxCount = count
			maxLang = lang
		}
	}

	if maxCount == 0 {
		return "unknown"
	}
	return maxLang
}

func (pa *ProjectAnalyzer) detectFramework() string {
	// Check go.mod for known frameworks.
	modPath := filepath.Join(pa.Dir, "go.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		return ""
	}
	content := string(data)

	frameworks := map[string]string{
		"github.com/gin-gonic/gin":   "Gin",
		"github.com/labstack/echo":   "Echo",
		"github.com/gorilla/mux":     "Gorilla",
		"github.com/go-chi/chi":      "Chi",
		"github.com/gofiber/fiber":   "Fiber",
		"github.com/spf13/cobra":     "Cobra CLI",
		"github.com/urfave/cli":      "urfave/cli",
		"google.golang.org/grpc":     "gRPC",
		"github.com/charmbracelet/bubbletea": "Bubbletea TUI",
	}

	var detected []string
	for dep, name := range frameworks {
		if strings.Contains(content, dep) {
			detected = append(detected, name)
		}
	}

	if len(detected) == 0 {
		return ""
	}
	sort.Strings(detected)
	return strings.Join(detected, " + ")
}

func (pa *ProjectAnalyzer) detectEntryPoints() []string {
	var entryPoints []string

	// Look for main packages in cmd/ directory.
	cmdDir := filepath.Join(pa.Dir, "cmd")
	if entries, err := os.ReadDir(cmdDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				mainFile := filepath.Join(cmdDir, entry.Name(), "main.go")
				if _, statErr := os.Stat(mainFile); statErr == nil {
					entryPoints = append(entryPoints, "cmd/"+entry.Name()+"/main.go")
				}
			}
		}
	}

	// Check root for main.go.
	rootMain := filepath.Join(pa.Dir, "main.go")
	if _, err := os.Stat(rootMain); err == nil {
		entryPoints = append(entryPoints, "main.go")
	}

	// Look for root-level cmd entry (Cobra pattern).
	rootCmd := filepath.Join(pa.Dir, "cmd", "root.go")
	if _, err := os.Stat(rootCmd); err == nil {
		entryPoints = append(entryPoints, "cmd/root.go")
	}

	return entryPoints
}

func (pa *ProjectAnalyzer) analyzeKeyModules() []ModuleInfo {
	var modules []ModuleInfo

	entries, err := os.ReadDir(pa.Dir)
	if err != nil {
		return modules
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" || name == "testdata" {
			continue
		}

		subPath := filepath.Join(pa.Dir, name)
		if !hasGoFiles(subPath) {
			continue
		}

		info := pa.AnalyzeModule(subPath)
		if info.Size > 0 {
			modules = append(modules, *info)
		}
	}

	// Sort by size descending.
	sort.Slice(modules, func(i, j int) bool {
		return modules[i].Size > modules[j].Size
	})

	// Keep top 10 modules.
	if len(modules) > 10 {
		modules = modules[:10]
	}

	return modules
}

func (pa *ProjectAnalyzer) detectConventions() []string {
	var conventions []string

	// Check for error wrapping convention.
	if pa.hasPatternInFiles("%w") {
		conventions = append(conventions, "Error wrapping with %w")
	}

	// Check for table-driven tests.
	if pa.hasPatternInTestFiles("tests := []struct") || pa.hasPatternInTestFiles("testCases := []struct") || pa.hasPatternInTestFiles("cases := []struct") {
		conventions = append(conventions, "Table-driven tests")
	}

	// Check for Cobra CLI.
	if pa.hasPatternInFiles("cobra.Command") {
		conventions = append(conventions, "Cobra for CLI commands")
	}

	// Check for context usage.
	if pa.hasPatternInFiles("context.Context") {
		conventions = append(conventions, "Context propagation")
	}

	// Check for interface-first design.
	interfaceCount := pa.countInterfaces()
	if interfaceCount >= 5 {
		conventions = append(conventions, fmt.Sprintf("Interface-first design (%d interfaces)", interfaceCount))
	}

	// Check for structured logging.
	if pa.hasPatternInFiles("slog.") || pa.hasPatternInFiles("zap.") || pa.hasPatternInFiles("logrus.") {
		conventions = append(conventions, "Structured logging")
	}

	// Check for goroutine + sync patterns.
	if pa.hasPatternInFiles("sync.Mutex") || pa.hasPatternInFiles("sync.RWMutex") {
		conventions = append(conventions, "Mutex-based concurrency")
	}

	// Check for channels.
	if pa.hasPatternInFiles("make(chan ") {
		conventions = append(conventions, "Channel-based communication")
	}

	return conventions
}

func (pa *ProjectAnalyzer) countDependencies() int {
	modPath := filepath.Join(pa.Dir, "go.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		return 0
	}

	count := 0
	inRequire := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		if inRequire && line != "" && !strings.HasPrefix(line, "//") {
			count++
		}
		// Single-line require.
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			count++
		}
	}

	return count
}

func (pa *ProjectAnalyzer) countLOC() int {
	total := 0
	_ = filepath.Walk(pa.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == "node_modules" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			total += countFileLines(path)
		}
		return nil
	})
	return total
}

func (pa *ProjectAnalyzer) assessTestCoverage() string {
	totalPkgs := 0
	testedPkgs := 0

	_ = filepath.Walk(pa.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") || base == "testdata" {
			return filepath.SkipDir
		}
		if hasGoFiles(path) {
			totalPkgs++
			if hasTestFiles(path) {
				testedPkgs++
			}
		}
		return nil
	})

	if totalPkgs == 0 {
		return "unknown"
	}

	pct := float64(testedPkgs) / float64(totalPkgs) * 100
	return fmt.Sprintf("%.0f%% (%d/%d packages have tests)", pct, testedPkgs, totalPkgs)
}

func (pa *ProjectAnalyzer) assessComplexity() string {
	totalFuncs := 0
	longFuncs := 0 // functions > 50 lines

	_ = filepath.Walk(pa.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				totalFuncs++
				startLine := fset.Position(fn.Pos()).Line
				endLine := fset.Position(fn.End()).Line
				if endLine-startLine > 50 {
					longFuncs++
				}
			}
		}
		return nil
	})

	if totalFuncs == 0 {
		return "unknown"
	}

	longPct := float64(longFuncs) / float64(totalFuncs) * 100
	if longPct > 20 {
		return fmt.Sprintf("high (%d/%d functions >50 lines)", longFuncs, totalFuncs)
	}
	if longPct > 10 {
		return fmt.Sprintf("moderate (%d/%d functions >50 lines)", longFuncs, totalFuncs)
	}
	return fmt.Sprintf("low (%d/%d functions >50 lines)", longFuncs, totalFuncs)
}

func (pa *ProjectAnalyzer) inferPurpose(info *ModuleInfo) string {
	name := strings.ToLower(info.Name)

	purposeMap := map[string]string{
		"cmd":        "CLI entry point and command definitions",
		"engine":     "Core agent loop and orchestration",
		"tool":       "Built-in tool implementations",
		"config":     "Configuration and settings management",
		"session":    "Session persistence and state management",
		"daemon":     "Background service and HTTP server",
		"sandbox":    "Command isolation and security",
		"memory":     "Persistent cross-session memory",
		"planner":    "Task decomposition and planning",
		"repomap":    "Code intelligence and file relevance",
		"hooks":      "Event-driven plugin system",
		"mcp":        "Model Context Protocol client",
		"permission": "User approval and access control",
		"circuit":    "Circuit breaker pattern for resilience",
		"ratelimit":  "Rate limiting and throttling",
		"retry":      "Retry logic with backoff",
		"health":     "Diagnostics and health checks",
		"parallel":   "Parallel execution and worktrees",
		"mission":    "Multi-agent orchestration",
		"agents":     "Custom persona definitions",
		"internal":   "Private implementation details",
		"pkg":        "Shared utilities and helpers",
		"api":        "API endpoints and handlers",
		"model":      "Data models and types",
		"store":      "Data persistence layer",
		"service":    "Business logic layer",
		"handler":    "Request handlers",
		"middleware": "Request/response middleware",
		"util":       "Utility functions",
		"common":     "Shared common code",
	}

	if purpose, ok := purposeMap[name]; ok {
		return purpose
	}

	// Try partial matching.
	for key, purpose := range purposeMap {
		if strings.Contains(name, key) {
			return purpose
		}
	}

	// Infer from public API names.
	if len(info.PublicAPI) > 0 {
		sample := strings.Join(info.PublicAPI[:projAnalyzerMin(3, len(info.PublicAPI))], ", ")
		return fmt.Sprintf("Provides: %s", sample)
	}

	return "Module functionality"
}

func (pa *ProjectAnalyzer) hasPatternInFiles(pattern string) bool {
	found := false
	_ = filepath.Walk(pa.Dir, func(path string, info os.FileInfo, err error) error {
		if found || err != nil {
			return filepath.SkipAll
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), pattern) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func (pa *ProjectAnalyzer) hasPatternInTestFiles(pattern string) bool {
	found := false
	_ = filepath.Walk(pa.Dir, func(path string, info os.FileInfo, err error) error {
		if found || err != nil {
			return filepath.SkipAll
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(data), pattern) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func (pa *ProjectAnalyzer) countInterfaces() int {
	count := 0
	_ = filepath.Walk(pa.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if _, isIface := ts.Type.(*ast.InterfaceType); isIface && ts.Name.IsExported() {
							count++
						}
					}
				}
			}
		}
		return nil
	})
	return count
}

// --- Package-level helper functions ---

func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			return true
		}
	}
	return false
}

func hasMainPackage(dir string) bool {
	mainFile := filepath.Join(dir, "main.go")
	_, err := os.Stat(mainFile)
	return err == nil
}

func countFileLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}

func findFilesWithPattern(dir string, patterns ...string) []string {
	var files []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		lower := strings.ToLower(filepath.Base(path))
		for _, pattern := range patterns {
			if strings.Contains(lower, pattern) {
				relPath, relErr := filepath.Rel(dir, path)
				if relErr == nil {
					files = append(files, relPath)
				}
				break
			}
		}
		return nil
	})
	return files
}

func findFactoryPattern(dir string) []string {
	var files []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		newCount := 0
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if strings.HasPrefix(fn.Name.Name, "New") && fn.Name.IsExported() {
					newCount++
				}
			}
		}

		if newCount >= 2 {
			relPath, relErr := filepath.Rel(dir, path)
			if relErr == nil {
				files = append(files, relPath)
			}
		}
		return nil
	})

	// Limit results.
	if len(files) > 10 {
		files = files[:10]
	}
	return files
}

func findStrategyPattern(dir string) []string {
	// Look for files that define an interface and have sibling files implementing it.
	var files []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		// Check if the file defines interfaces with multiple methods.
		for _, decl := range f.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if iface, isIface := ts.Type.(*ast.InterfaceType); isIface {
							if iface.Methods != nil && len(iface.Methods.List) >= 2 {
								relPath, relErr := filepath.Rel(dir, path)
								if relErr == nil {
									files = append(files, relPath)
								}
							}
						}
					}
				}
			}
		}
		return nil
	})

	if len(files) > 10 {
		files = files[:10]
	}
	return files
}

func findFunctionalOptionsPattern(dir string) []string {
	var files []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "vendor" || base == ".git" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		withCount := 0
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if strings.HasPrefix(fn.Name.Name, "With") && fn.Name.IsExported() {
					withCount++
				}
			}
		}

		if withCount >= 3 {
			relPath, relErr := filepath.Rel(dir, path)
			if relErr == nil {
				files = append(files, relPath)
			}
		}
		return nil
	})

	if len(files) > 10 {
		files = files[:10]
	}
	return files
}

func calculateConfidence(files []string, threshold int) float64 {
	count := len(files)
	if count >= threshold*2 {
		return 0.95
	}
	if count >= threshold {
		return 0.8
	}
	if count >= 1 {
		return 0.5 + float64(count)/float64(threshold)*0.3
	}
	return 0.0
}

func projAnalyzerExprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return projAnalyzerExprToString(t.X)
	case *ast.SelectorExpr:
		return projAnalyzerExprToString(t.X) + "." + t.Sel.Name
	default:
		return "T"
	}
}

func formatLOC(loc int) string {
	if loc >= 1000 {
		return fmt.Sprintf("%dK LOC", loc/1000)
	}
	return fmt.Sprintf("%d LOC", loc)
}

func projAnalyzerAppendUnique(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}

func projAnalyzerMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func projAnalyzerTitle(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
