package repomap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// This file holds the internal helpers for codebase summary generation:
// language/file detection, symbol and import extraction, entry-point detection,
// and formatting. The CodebaseSummary type, SummaryGenerator, and the public
// Render/Infer/Find entry points live in summary.go.

func summaryDetectLanguage(projectDir string) string {
	counts := map[string]int{}

	_ = filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
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
	f, err := os.Open(path) // #nosec G304 -- path is a repo file discovered while scanning the repo being analyzed by this dev CLI
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

func summaryExtractSymbols(path string) []string {
	data, err := os.ReadFile(path) // #nosec G304 -- path is a repo file discovered while scanning the repo being analyzed by this dev CLI
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
	f, err := os.Open(path) // #nosec G304 -- path is a repo file discovered while scanning the repo being analyzed by this dev CLI
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

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

var (
	summaryGoMainRe        = regexp.MustCompile(`^func\s+main\s*\(`)
	summaryGoPackageMainRe = regexp.MustCompile(`^package\s+main\b`)
)

func summaryHasGoMain(path string) bool {
	f, err := os.Open(path) // #nosec G304 -- path is a repo file discovered while scanning the repo being analyzed by this dev CLI
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

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
	f, err := os.Open(path) // #nosec G304 -- path is a repo file discovered while scanning the repo being analyzed by this dev CLI
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if summaryPyMainRe.MatchString(scanner.Text()) {
			return true
		}
	}
	return false
}

func summaryFindJSEntryPoints(packageJSONPath string, projectDir string) []string {
	data, err := os.ReadFile(packageJSONPath) // #nosec G304 -- packageJSONPath is a repo file discovered while scanning the repo being analyzed by this dev CLI
	if err != nil {
		return nil
	}

	var pkg struct {
		Main string `json:"main"`
	}
	if unmarshalErr := json.Unmarshal(data, &pkg); unmarshalErr != nil {
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
