package repomap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// CodebaseSummary holds the high-level overview of a repository.
type CodebaseSummary struct {
	ProjectName  string
	Description  string
	Language     string
	Packages     []SummaryPackageInfo
	EntryPoints  []string
	KeyFiles     []string
	Architecture string
	TotalLOC     int
	TotalFiles   int
	GeneratedAt  time.Time
}

// SummaryPackageInfo holds the overview of a single package/module for summaries.
type SummaryPackageInfo struct {
	Name          string
	Path          string
	Purpose       string
	PublicSymbols int
	Files         int
	LOC           int
	Dependencies  []string
	Dependents    []string
}

// SummaryGenerator produces concise codebase overviews for context injection.
type SummaryGenerator struct {
	ProjectDir string
	MaxTokens  int
	mu         sync.RWMutex
}

// NewSummaryGenerator creates a new SummaryGenerator for the given project directory.
func NewSummaryGenerator(projectDir string, maxTokens int) *SummaryGenerator {
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	return &SummaryGenerator{
		ProjectDir: projectDir,
		MaxTokens:  maxTokens,
	}
}

// Generate walks the project directory, analyzes packages, identifies entry points
// and key files, infers architecture, and produces a CodebaseSummary.
func (sg *SummaryGenerator) Generate() (*CodebaseSummary, error) {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	projectName := filepath.Base(sg.ProjectDir)
	lang := summaryDetectLanguage(sg.ProjectDir)

	packages, totalLOC, totalFiles, err := sg.analyzePackages()
	if err != nil {
		return nil, fmt.Errorf("analyzing packages: %w", err)
	}

	entryPoints := FindEntryPoints(sg.ProjectDir)
	keyFiles := FindKeyFiles(sg.ProjectDir, 10)
	arch := InferArchitecture(packages)

	summary := &CodebaseSummary{
		ProjectName:  projectName,
		Description:  inferProjectDescription(projectName, packages, lang),
		Language:     lang,
		Packages:     packages,
		EntryPoints:  entryPoints,
		KeyFiles:     keyFiles,
		Architecture: arch,
		TotalLOC:     totalLOC,
		TotalFiles:   totalFiles,
		GeneratedAt:  time.Now(),
	}

	return summary, nil
}

// analyzePackages walks the project tree and collects per-package statistics.
func (sg *SummaryGenerator) analyzePackages() ([]SummaryPackageInfo, int, int, error) {
	pkgMap := make(map[string]*SummaryPackageInfo)
	importMap := make(map[string][]string) // pkg -> imports
	totalLOC := 0
	totalFiles := 0

	err := filepath.Walk(sg.ProjectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if summarySkipDir(base) {
				return filepath.SkipDir
			}
			return nil
		}

		if !summaryIsSupportedFile(path) {
			return nil
		}

		totalFiles++

		rel, _ := filepath.Rel(sg.ProjectDir, path)
		pkgPath := filepath.Dir(rel)
		if pkgPath == "." {
			pkgPath = summaryProjectRoot
		}

		loc := summaryCountFileLines(path)
		totalLOC += loc

		symbols := summaryExtractSymbols(path)
		publicCount := summaryCountPublicSymbols(symbols, summaryDetectLanguage(sg.ProjectDir))

		pkg, exists := pkgMap[pkgPath]
		if !exists {
			pkg = &SummaryPackageInfo{
				Name: filepath.Base(pkgPath),
				Path: pkgPath,
			}
			pkgMap[pkgPath] = pkg
		}
		pkg.Files++
		pkg.LOC += loc
		pkg.PublicSymbols += publicCount

		// Track imports for dependency analysis
		imports := summaryExtractImports(path)
		importMap[pkgPath] = append(importMap[pkgPath], imports...)

		return nil
	})
	if err != nil {
		return nil, 0, 0, err
	}

	// Resolve dependencies between packages
	pkgPaths := make(map[string]bool)
	for p := range pkgMap {
		pkgPaths[p] = true
	}

	for pkgPath, imports := range importMap {
		pkg := pkgMap[pkgPath]
		depSet := make(map[string]bool)
		for _, imp := range imports {
			// Try to match imports to local packages
			for candidate := range pkgPaths {
				if candidate != pkgPath && strings.HasSuffix(imp, candidate) {
					depSet[candidate] = true
				}
			}
		}
		for dep := range depSet {
			pkg.Dependencies = append(pkg.Dependencies, dep)
		}
		sort.Strings(pkg.Dependencies)
	}

	// Build dependents (reverse edges)
	for _, pkg := range pkgMap {
		for _, dep := range pkg.Dependencies {
			if target, ok := pkgMap[dep]; ok {
				target.Dependents = append(target.Dependents, pkg.Path)
			}
		}
	}

	// Infer purpose for each package
	for _, pkg := range pkgMap {
		symbols := summaryCollectPackageSymbols(sg.ProjectDir, pkg.Path)
		pkg.Purpose = InferPurpose(pkg.Path, symbols)
	}

	// Sort dependents
	for _, pkg := range pkgMap {
		sort.Strings(pkg.Dependents)
	}

	// Convert to sorted slice
	packages := make([]SummaryPackageInfo, 0, len(pkgMap))
	for _, pkg := range pkgMap {
		packages = append(packages, *pkg)
	}
	sort.Slice(packages, func(i, j int) bool {
		// Sort by LOC descending to surface important packages first
		return packages[i].LOC > packages[j].LOC
	})

	return packages, totalLOC, totalFiles, nil
}

const summaryProjectRoot = "(root)"

// InferArchitecture detects architectural patterns from the package layout.
// Returns one of: "layered", "flat", "monorepo", "microservices", "hexagonal".
func InferArchitecture(packages []SummaryPackageInfo) string {
	if len(packages) == 0 {
		return "flat"
	}

	paths := make(map[string]bool)
	for _, p := range packages {
		paths[p.Path] = true
	}

	// Check for monorepo (multiple service directories)
	serviceCount := 0
	for p := range paths {
		parts := strings.Split(p, string(filepath.Separator))
		if len(parts) >= 2 && (parts[0] == "services" || parts[0] == "apps" || parts[0] == "packages") {
			serviceCount++
		}
	}
	if serviceCount >= 3 {
		return "monorepo"
	}

	// Check for microservices pattern
	cmdCount := 0
	for p := range paths {
		parts := strings.Split(p, string(filepath.Separator))
		if len(parts) >= 2 && parts[0] == "cmd" {
			cmdCount++
		}
	}
	if cmdCount >= 3 {
		return "microservices"
	}

	// Check for hexagonal/ports-and-adapters
	hasPort := false
	hasAdapter := false
	hasDomain := false
	for p := range paths {
		lower := strings.ToLower(p)
		if strings.Contains(lower, "port") || strings.Contains(lower, "interface") {
			hasPort = true
		}
		if strings.Contains(lower, "adapter") || strings.Contains(lower, "infra") {
			hasAdapter = true
		}
		if strings.Contains(lower, "domain") || strings.Contains(lower, "entity") {
			hasDomain = true
		}
	}
	if hasPort && hasAdapter && hasDomain {
		return "hexagonal"
	}

	// Check for layered architecture (cmd -> service/engine -> repository/store)
	hasCmd := false
	hasService := false
	hasRepo := false
	for p := range paths {
		lower := strings.ToLower(p)
		if strings.Contains(lower, "cmd") {
			hasCmd = true
		}
		if strings.Contains(lower, "service") || strings.Contains(lower, "engine") || strings.Contains(lower, "handler") {
			hasService = true
		}
		if strings.Contains(lower, "repo") || strings.Contains(lower, "store") || strings.Contains(lower, "dal") || strings.Contains(lower, "database") {
			hasRepo = true
		}
	}
	if hasCmd && hasService {
		return "layered"
	}
	if hasCmd && hasRepo {
		return "layered"
	}

	// Flat: few packages, shallow nesting
	if len(packages) <= 5 {
		return "flat"
	}

	maxDepth := 0
	for p := range paths {
		depth := strings.Count(p, string(filepath.Separator))
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	if maxDepth <= 1 {
		return "flat"
	}

	return "layered"
}

// InferPurpose infers a package's purpose from its path and exported symbols.
func InferPurpose(pkgPath string, symbols []string) string {
	name := strings.ToLower(filepath.Base(pkgPath))
	symbolsLower := make([]string, len(symbols))
	for i, s := range symbols {
		symbolsLower[i] = strings.ToLower(s)
	}
	symbolStr := strings.Join(symbolsLower, " ")

	// Pattern matching from package name + symbols
	patterns := []struct {
		nameContains   string
		symbolContains string
		purpose        string
	}{
		{"auth", "validatetoken", "Authentication and token management"},
		{"auth", "login", "Authentication and authorization"},
		{"auth", "", "Authentication"},
		{"handler", "servehttp", "HTTP request handling"},
		{"handler", "handle", "Request handling"},
		{"handler", "", "Request handlers"},
		{"router", "route", "HTTP routing"},
		{"router", "", "Request routing"},
		{"middleware", "", "HTTP middleware"},
		{"config", "load", "Configuration loading and validation"},
		{"config", "", "Configuration management"},
		{"model", "", "Data models and types"},
		{"store", "query", "Data persistence and querying"},
		{"store", "", "Data storage"},
		{"repo", "find", "Data repository and querying"},
		{"repo", "", "Data repository"},
		{"database", "", "Database access layer"},
		{"db", "connect", "Database connection management"},
		{"db", "", "Database access"},
		{"service", "", "Business logic"},
		{"engine", "run", "Core execution engine"},
		{"engine", "stream", "Agent loop, compaction, streaming"},
		{"engine", "", "Core engine"},
		{"tool", "execute", "Tool execution and management"},
		{"tool", "", "Tool definitions"},
		{"cmd", "main", "CLI entry point"},
		{"cmd", "", "Command-line interface"},
		{"util", "", "Shared utilities"},
		{"helper", "", "Helper functions"},
		{"test", "", "Test utilities"},
		{"mock", "", "Test mocks"},
		{"api", "handler", "API endpoints"},
		{"api", "", "API layer"},
		{"client", "request", "HTTP client"},
		{"client", "", "Client library"},
		{"server", "listen", "Server initialization and lifecycle"},
		{"server", "", "Server components"},
		{"cache", "", "Caching layer"},
		{"queue", "", "Message queue handling"},
		{"event", "", "Event handling"},
		{"log", "", "Logging"},
		{"metric", "", "Metrics and telemetry"},
		{"session", "", "Session management"},
		{"sandbox", "", "Sandboxed execution"},
		{"permission", "", "Permission and access control"},
		{"planner", "", "Planning and task decomposition"},
		{"memory", "", "Persistent memory"},
		{"daemon", "", "Background service"},
		{"hook", "", "Event-driven hooks"},
		{"circuit", "", "Circuit breaker resilience"},
		{"ratelimit", "", "Rate limiting"},
		{"retry", "", "Retry logic with backoff"},
		{"health", "", "Health checks and diagnostics"},
		{"parallel", "", "Parallel execution"},
		{"mission", "", "Multi-agent orchestration"},
		{"repomap", "", "Code intelligence and repository mapping"},
		{"pagerank", "", "PageRank-based file ranking"},
		{"semantic", "", "Semantic analysis"},
		{"predict", "", "Prediction and ranking"},
	}

	for _, p := range patterns {
		if p.nameContains != "" && !strings.Contains(name, p.nameContains) {
			continue
		}
		if p.symbolContains != "" && !strings.Contains(symbolStr, p.symbolContains) {
			continue
		}
		return p.purpose
	}

	// Fallback: generate from package name
	if name == summaryProjectRoot || name == "." || name == "(root)" {
		return "Project root"
	}
	return summaryFormatPackageName(name) + " package"
}

// FindEntryPoints identifies program entry points in the project.
func FindEntryPoints(projectDir string) []string {
	var entryPoints []string
	seen := make(map[string]bool)

	filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if summarySkipDir(filepath.Base(path)) {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(projectDir, path)

		switch {
		case strings.HasSuffix(path, ".go"):
			// Go: look for package main with func main()
			if summaryHasGoMain(path) {
				entry := rel
				if !seen[entry] {
					entryPoints = append(entryPoints, entry)
					seen[entry] = true
				}
			}
		case strings.HasSuffix(path, ".py"):
			// Python: look for if __name__ == "__main__"
			if summaryHasPythonMain(path) {
				entry := rel
				if !seen[entry] {
					entryPoints = append(entryPoints, entry)
					seen[entry] = true
				}
			}
		case filepath.Base(path) == "package.json":
			// JS/TS: look for "main" field in package.json
			mains := summaryFindJSEntryPoints(path, projectDir)
			for _, m := range mains {
				if !seen[m] {
					entryPoints = append(entryPoints, m)
					seen[m] = true
				}
			}
		}
		return nil
	})

	sort.Strings(entryPoints)
	return entryPoints
}

// FindKeyFiles identifies the most important files in a project.
func FindKeyFiles(projectDir string, limit int) []string {
	if limit <= 0 {
		limit = 10
	}

	type fileScore struct {
		path  string
		score float64
	}

	var files []fileScore
	importCounts := make(map[string]int) // how many files import this one

	// First pass: collect all files and count imports
	filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if summarySkipDir(filepath.Base(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !summaryIsSupportedFile(path) {
			return nil
		}

		rel, _ := filepath.Rel(projectDir, path)
		imports := summaryExtractImports(path)
		for _, imp := range imports {
			// Normalize import to relative path within project
			for _, ext := range []string{".go", ".py", ".ts", ".js", ".tsx", ".jsx"} {
				candidate := imp + ext
				full := filepath.Join(projectDir, candidate)
				if _, err := os.Stat(full); err == nil {
					importCounts[candidate]++
				}
			}
			// Direct match
			if _, err := os.Stat(filepath.Join(projectDir, imp)); err == nil {
				importCounts[imp]++
			}
		}

		symbols := summaryExtractSymbols(path)
		publicCount := summaryCountPublicSymbols(symbols, summaryDetectLanguage(projectDir))
		loc := summaryCountFileLines(path)

		score := float64(publicCount) * 2.0
		score += float64(loc) * 0.01
		if summaryIsConfigFile(rel) {
			score += 5.0
		}

		files = append(files, fileScore{path: rel, score: score})
		return nil
	})

	// Apply import count bonus
	for i := range files {
		if count, ok := importCounts[files[i].path]; ok {
			files[i].score += float64(count) * 3.0
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].score > files[j].score
	})

	result := make([]string, 0, limit)
	for i, f := range files {
		if i >= limit {
			break
		}
		result = append(result, f.path)
	}
	return result
}

// RenderForPrompt renders a CodebaseSummary as a markdown-formatted overview
// suitable for injection into an LLM prompt, respecting the given token budget.
func RenderForPrompt(summary *CodebaseSummary, budget int) string {
	if budget <= 0 {
		budget = 1024
	}

	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("## Codebase: %s\n", summary.ProjectName))
	sb.WriteString(fmt.Sprintf("%s project (%d packages, %s files, %s LOC)\n",
		summary.Language,
		len(summary.Packages),
		summaryFormatNumber(summary.TotalFiles),
		summaryFormatLOC(summary.TotalLOC),
	))

	// Architecture
	if summary.Architecture != "" {
		archDetail := summaryDescribeArchitecture(summary)
		sb.WriteString(fmt.Sprintf("Architecture: %s\n", archDetail))
	}

	sb.WriteString("\n")

	// Key packages (within budget)
	if len(summary.Packages) > 0 {
		sb.WriteString("Key packages:\n")
		maxPkgs := budget / 80 // approximate tokens per package line
		if maxPkgs > 20 {
			maxPkgs = 20
		}
		if maxPkgs > len(summary.Packages) {
			maxPkgs = len(summary.Packages)
		}
		for i := 0; i < maxPkgs; i++ {
			pkg := summary.Packages[i]
			line := fmt.Sprintf("- %s/ — %s (%s LOC)\n", pkg.Path, pkg.Purpose, summaryFormatLOC(pkg.LOC))
			if summaryEstimateTokens(sb.String()+line) > budget {
				break
			}
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}

	// Entry points
	if len(summary.EntryPoints) > 0 {
		entries := summary.EntryPoints
		if len(entries) > 5 {
			entries = entries[:5]
		}
		sb.WriteString(fmt.Sprintf("Entry points: %s\n", strings.Join(entries, ", ")))
	}

	// Key files
	if len(summary.KeyFiles) > 0 {
		keyFiles := summary.KeyFiles
		if len(keyFiles) > 8 {
			keyFiles = keyFiles[:8]
		}
		sb.WriteString(fmt.Sprintf("Key files: %s\n", strings.Join(keyFiles, ", ")))
	}

	return sb.String()
}

// RenderCompact renders the summary as a single paragraph suitable for tight contexts.
func RenderCompact(summary *CodebaseSummary) string {
	pkgCount := len(summary.Packages)
	topPkgs := make([]string, 0, 5)
	for i, pkg := range summary.Packages {
		if i >= 5 {
			break
		}
		topPkgs = append(topPkgs, pkg.Path)
	}

	entries := ""
	if len(summary.EntryPoints) > 0 {
		entries = fmt.Sprintf(" Entry: %s.", strings.Join(summary.EntryPoints, ", "))
	}

	return fmt.Sprintf(
		"%s is a %s %s project with %d packages (%s files, %s LOC). "+
			"Architecture: %s. Key packages: %s.%s",
		summary.ProjectName,
		strings.ToLower(summary.Architecture),
		summary.Language,
		pkgCount,
		summaryFormatNumber(summary.TotalFiles),
		summaryFormatLOC(summary.TotalLOC),
		summary.Architecture,
		strings.Join(topPkgs, ", "),
		entries,
	)
}

// ── Helper functions (prefixed to avoid conflicts with other files in package) ──

func summaryDetectLanguage(projectDir string) string {
	counts := map[string]int{}

	filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if summarySkipDir(filepath.Base(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			counts["Go"]++
		case ".py":
			counts["Python"]++
		case ".ts", ".tsx":
			counts["TypeScript"]++
		case ".js", ".jsx":
			counts["JavaScript"]++
		case ".rs":
			counts["Rust"]++
		case ".java":
			counts["Java"]++
		case ".rb":
			counts["Ruby"]++
		case ".c", ".h":
			counts["C"]++
		case ".cpp", ".cc", ".cxx", ".hpp":
			counts["C++"]++
		case ".cs":
			counts["C#"]++
		}
		return nil
	})

	if len(counts) == 0 {
		return "Unknown"
	}

	best := ""
	bestCount := 0
	for lang, count := range counts {
		if count > bestCount {
			best = lang
			bestCount = count
		}
	}
	return best
}

func summarySkipDir(name string) bool {
	skip := []string{
		".git", "node_modules", "vendor", "__pycache__", ".venv", "venv",
		"dist", "build", ".next", ".nuxt", "target", "bin", "obj",
		".idea", ".vscode", ".DS_Store", ".cache", "coverage",
	}
	for _, s := range skip {
		if name == s {
			return true
		}
	}
	return false
}

func summaryIsSupportedFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	supported := map[string]bool{
		".go": true, ".py": true, ".ts": true, ".tsx": true,
		".js": true, ".jsx": true, ".rs": true, ".java": true,
		".rb": true, ".c": true, ".h": true, ".cpp": true,
		".cc": true, ".cxx": true, ".hpp": true, ".cs": true,
	}
	return supported[ext]
}

func summaryCountFileLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count
}

func summaryExtractSymbols(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	src := string(data)

	var symbols []Symbol
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		symbols = parseGo(src)
	case ".py":
		symbols = parsePython(src)
	case ".ts", ".tsx", ".js", ".jsx":
		symbols = parseTypeScript(src)
	default:
		return nil
	}

	names := make([]string, 0, len(symbols))
	for _, s := range symbols {
		names = append(names, s.Name)
	}
	return names
}

func summaryCountPublicSymbols(symbols []string, lang string) int {
	count := 0
	for _, s := range symbols {
		if summaryIsPublicSymbol(s, lang) {
			count++
		}
	}
	return count
}

func summaryIsPublicSymbol(name string, lang string) bool {
	if name == "" {
		return false
	}
	switch lang {
	case "Go":
		// In Go, public symbols start with uppercase
		return unicode.IsUpper(rune(name[0]))
	case "Python":
		// In Python, public symbols don't start with underscore
		return !strings.HasPrefix(name, "_")
	default:
		// For JS/TS, we consider exported symbols public (parser already filters)
		return unicode.IsUpper(rune(name[0])) || !strings.HasPrefix(name, "_")
	}
}

func summaryExtractImports(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var imports []string
	scanner := bufio.NewScanner(f)
	ext := strings.ToLower(filepath.Ext(path))
	inBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		switch ext {
		case ".go":
			if goImportBlockRe.MatchString(line) {
				inBlock = true
				continue
			}
			if inBlock {
				if goImportBlockEnd.MatchString(line) {
					inBlock = false
					continue
				}
				if m := goImportPathRe.FindStringSubmatch(line); m != nil {
					imports = append(imports, m[1])
				}
			} else if m := goImportSingleRe.FindStringSubmatch(line); m != nil {
				imports = append(imports, m[1])
			}
		case ".py":
			if m := pyImportRe.FindStringSubmatch(line); m != nil {
				imports = append(imports, m[1])
			} else if m := pyFromImportRe.FindStringSubmatch(line); m != nil {
				imports = append(imports, m[1])
			}
		case ".ts", ".tsx", ".js", ".jsx":
			if m := tsImportFromRe.FindStringSubmatch(line); m != nil {
				imports = append(imports, m[1])
			} else if m := tsImportBareRe.FindStringSubmatch(line); m != nil {
				imports = append(imports, m[1])
			}
		}
	}
	return imports
}

var summaryGoMainRe = regexp.MustCompile(`^func\s+main\s*\(`)
var summaryGoPackageMainRe = regexp.MustCompile(`^package\s+main\b`)

func summaryHasGoMain(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	hasPackageMain := false
	hasFuncMain := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if summaryGoPackageMainRe.MatchString(line) {
			hasPackageMain = true
		}
		if summaryGoMainRe.MatchString(line) {
			hasFuncMain = true
		}
	}
	return hasPackageMain && hasFuncMain
}

var summaryPyMainRe = regexp.MustCompile(`^if\s+__name__\s*==\s*['"]__main__['"]`)

func summaryHasPythonMain(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if summaryPyMainRe.MatchString(scanner.Text()) {
			return true
		}
	}
	return false
}

func summaryFindJSEntryPoints(packageJSONPath string, projectDir string) []string {
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Main string `json:"main"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	if pkg.Main == "" {
		return nil
	}

	dir := filepath.Dir(packageJSONPath)
	rel, err := filepath.Rel(projectDir, filepath.Join(dir, pkg.Main))
	if err != nil {
		return nil
	}
	return []string{rel}
}

func summaryCollectPackageSymbols(projectDir, pkgPath string) []string {
	dir := filepath.Join(projectDir, pkgPath)
	if pkgPath == summaryProjectRoot {
		dir = projectDir
	}

	var symbols []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if summaryIsSupportedFile(path) {
			symbols = append(symbols, summaryExtractSymbols(path)...)
		}
	}
	return symbols
}

func summaryIsConfigFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	configPatterns := []string{
		"config", "settings", "conf", ".env", "yaml", "yml", "toml",
		"makefile", "dockerfile", "docker-compose",
	}
	for _, p := range configPatterns {
		if strings.Contains(base, p) {
			return true
		}
	}
	return false
}

func inferProjectDescription(name string, packages []SummaryPackageInfo, lang string) string {
	if len(packages) == 0 {
		return fmt.Sprintf("A %s project", lang)
	}

	// Look for notable package names to infer purpose
	hasAPI := false
	hasCLI := false
	hasWeb := false
	hasEngine := false

	for _, pkg := range packages {
		lower := strings.ToLower(pkg.Path)
		if strings.Contains(lower, "api") || strings.Contains(lower, "handler") {
			hasAPI = true
		}
		if strings.Contains(lower, "cmd") || strings.Contains(lower, "cli") {
			hasCLI = true
		}
		if strings.Contains(lower, "web") || strings.Contains(lower, "frontend") {
			hasWeb = true
		}
		if strings.Contains(lower, "engine") || strings.Contains(lower, "core") {
			hasEngine = true
		}
	}

	switch {
	case hasCLI && hasEngine:
		return fmt.Sprintf("A %s CLI application with core engine", lang)
	case hasCLI:
		return fmt.Sprintf("A %s command-line application", lang)
	case hasAPI && hasWeb:
		return fmt.Sprintf("A %s full-stack web application", lang)
	case hasAPI:
		return fmt.Sprintf("A %s API service", lang)
	case hasWeb:
		return fmt.Sprintf("A %s web application", lang)
	default:
		return fmt.Sprintf("A %s project with %d packages", lang, len(packages))
	}
}

func summaryDescribeArchitecture(summary *CodebaseSummary) string {
	arch := summary.Architecture

	// Try to describe the layer flow for layered architectures
	if arch == "layered" && len(summary.Packages) > 0 {
		layers := make([]string, 0, 4)
		layerNames := []string{"cmd", "engine", "service", "handler", "tool", "config", "store", "repo"}
		for _, ln := range layerNames {
			for _, pkg := range summary.Packages {
				if strings.Contains(strings.ToLower(pkg.Path), ln) {
					layers = append(layers, pkg.Path)
					break
				}
			}
			if len(layers) >= 4 {
				break
			}
		}
		if len(layers) >= 2 {
			return fmt.Sprintf("layered (%s)", strings.Join(layers, " -> "))
		}
	}

	return arch
}

func summaryFormatNumber(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d", n)
}

func summaryFormatLOC(loc int) string {
	if loc >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(loc)/1000000.0)
	}
	if loc >= 1000 {
		return fmt.Sprintf("%dK", loc/1000)
	}
	return fmt.Sprintf("%d", loc)
}

func summaryFormatPackageName(name string) string {
	if name == "" {
		return "Unknown"
	}
	// Capitalize first letter
	return strings.ToUpper(name[:1]) + name[1:]
}

func summaryEstimateTokens(text string) int {
	// Rough estimate: 1 token per 4 characters
	return len(text) / 4
}
