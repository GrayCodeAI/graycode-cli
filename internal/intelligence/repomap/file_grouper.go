package repomap

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FileGroup represents a set of files that logically belong together.
type FileGroup struct {
	Name       string
	Files      []string
	Reason     string
	Confidence float64
	Type       string // "package", "feature", "layer", "test_pair", "config"
}

// FileGrouper identifies which files belong together and should be edited as a unit.
type FileGrouper struct {
	ProjectDir string
	Groups     []FileGroup
	mu         sync.RWMutex
}

// NewFileGrouper creates a new FileGrouper for the given project directory.
func NewFileGrouper(projectDir string) *FileGrouper {
	return &FileGrouper{
		ProjectDir: projectDir,
	}
}

// AnalyzeGroups runs multiple strategies to identify file groups in the project.
func (fg *FileGrouper) AnalyzeGroups() ([]FileGroup, error) {
	fg.mu.Lock()
	defer fg.mu.Unlock()

	var allGroups []FileGroup

	// Collect all source files
	files, err := fg.collectFiles()
	if err != nil {
		return nil, err
	}

	// Strategy 1: Package-based grouping (same Go package)
	pkgGroups := fg.analyzePackageGroups(files)
	allGroups = append(allGroups, pkgGroups...)

	// Strategy 2: Test pair grouping
	testPairGroups := fg.analyzeTestPairs(files)
	allGroups = append(allGroups, testPairGroups...)

	// Strategy 3: Feature-based grouping (shared prefix/naming convention)
	featureGroups := fg.analyzeFeatureGroups(files)
	allGroups = append(allGroups, featureGroups...)

	// Strategy 4: Import-based grouping (files that import each other)
	importGroups := fg.analyzeImportGroups(files)
	allGroups = append(allGroups, importGroups...)

	// Strategy 5: Config groups (related config files)
	configGroups := fg.analyzeConfigGroups(files)
	allGroups = append(allGroups, configGroups...)

	fg.Groups = allGroups
	return allGroups, nil
}

// FindRelated returns all files related to the given file.
func (fg *FileGrouper) FindRelated(file string) []string {
	fg.mu.RLock()
	defer fg.mu.RUnlock()

	relatedSet := make(map[string]bool)
	relPath := fg.relativePath(file)

	// Check all groups for this file
	for _, group := range fg.Groups {
		for _, f := range group.Files {
			if f == relPath || f == file {
				// Add all files in this group
				for _, gf := range group.Files {
					if gf != relPath && gf != file {
						relatedSet[gf] = true
					}
				}
				break
			}
		}
	}

	// Find test pair
	if testPair := fg.FindTestPair(file); testPair != "" {
		relatedSet[testPair] = true
	}

	// Find files with similar base names
	baseName := fileBaseName(relPath)
	files, _ := fg.collectFiles()
	for _, f := range files {
		if f == relPath {
			continue
		}
		if strings.Contains(fileBaseName(f), baseName) || strings.Contains(baseName, fileBaseName(f)) {
			if fileBaseName(f) != "" && baseName != "" {
				relatedSet[f] = true
			}
		}
	}

	result := make([]string, 0, len(relatedSet))
	for f := range relatedSet {
		result = append(result, f)
	}
	sort.Strings(result)
	return result
}

// FindTestPair finds the corresponding test file for a source file or vice versa.
func (fg *FileGrouper) FindTestPair(file string) string {
	relPath := fg.relativePath(file)
	ext := filepath.Ext(relPath)
	base := strings.TrimSuffix(relPath, ext)

	switch {
	// Go: foo.go <-> foo_test.go
	case ext == ".go":
		if strings.HasSuffix(base, "_test") {
			// Test file -> source file
			candidate := strings.TrimSuffix(base, "_test") + ext
			if fg.fileExists(candidate) {
				return candidate
			}
		} else {
			// Source file -> test file
			candidate := base + "_test" + ext
			if fg.fileExists(candidate) {
				return candidate
			}
		}

	// Python: handler.py <-> test_handler.py
	case ext == ".py":
		dir := filepath.Dir(relPath)
		name := filepath.Base(base)
		if strings.HasPrefix(name, "test_") {
			// Test file -> source file
			candidate := filepath.Join(dir, strings.TrimPrefix(name, "test_")+ext)
			if fg.fileExists(candidate) {
				return candidate
			}
		} else {
			// Source file -> test file
			candidate := filepath.Join(dir, "test_"+name+ext)
			if fg.fileExists(candidate) {
				return candidate
			}
		}

	// TypeScript/JavaScript: component.tsx <-> component.test.tsx / component.spec.tsx
	case ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx":
		if strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".spec") {
			// Test file -> source file
			srcBase := strings.TrimSuffix(strings.TrimSuffix(base, ".test"), ".spec")
			candidate := srcBase + ext
			if fg.fileExists(candidate) {
				return candidate
			}
		} else {
			// Source file -> test file
			candidate := base + ".test" + ext
			if fg.fileExists(candidate) {
				return candidate
			}
			candidate = base + ".spec" + ext
			if fg.fileExists(candidate) {
				return candidate
			}
		}
	}

	return ""
}

// FindByFeature finds all files related to a feature by name.
func (fg *FileGrouper) FindByFeature(featureName string) []string {
	fg.mu.RLock()
	defer fg.mu.RUnlock()

	featureLower := strings.ToLower(featureName)
	var result []string

	files, _ := fg.collectFiles()
	for _, f := range files {
		name := strings.ToLower(filepath.Base(f))
		nameNoExt := strings.TrimSuffix(name, filepath.Ext(name))

		// Match files that contain the feature name as a component
		if nameNoExt == featureLower ||
			strings.HasPrefix(nameNoExt, featureLower+"_") ||
			strings.HasPrefix(nameNoExt, featureLower+".") ||
			strings.HasSuffix(nameNoExt, "_"+featureLower) ||
			strings.Contains(nameNoExt, "_"+featureLower+"_") ||
			strings.HasPrefix(nameNoExt, "test_"+featureLower) {
			result = append(result, f)
		}

		// Also match directory-based features
		dir := filepath.Dir(f)
		dirBase := strings.ToLower(filepath.Base(dir))
		if dirBase == featureLower {
			result = append(result, f)
		}
	}

	sort.Strings(result)
	return dedupe(result)
}

// SuggestEditGroup suggests related files that likely need updates when editing a target file.
func (fg *FileGrouper) SuggestEditGroup(targetFile string) []string {
	fg.mu.RLock()
	defer fg.mu.RUnlock()

	relPath := fg.relativePath(targetFile)
	suggestions := make(map[string]bool)

	// Add test pair
	if testPair := fg.FindTestPair(targetFile); testPair != "" {
		suggestions[testPair] = true
	}

	// Find from groups with high confidence
	for _, group := range fg.Groups {
		contains := false
		for _, f := range group.Files {
			if f == relPath || f == targetFile {
				contains = true
				break
			}
		}
		if contains && group.Confidence >= 0.7 {
			for _, f := range group.Files {
				if f != relPath && f != targetFile {
					suggestions[f] = true
				}
			}
		}
	}

	// Add co-change suggestions
	cochangeMap := CoChangeAnalysisMap(fg.ProjectDir)
	if related, ok := cochangeMap[relPath]; ok {
		for _, r := range related {
			suggestions[r] = true
		}
	}

	result := make([]string, 0, len(suggestions))
	for f := range suggestions {
		result = append(result, f)
	}
	sort.Strings(result)
	return result
}

// FormatGroups produces a human-readable summary of the file groups.
func FormatGroups(groups []FileGroup) string {
	if len(groups) == 0 {
		return "No file groups found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File Groups (%d):\n", len(groups)))
	sb.WriteString(strings.Repeat("─", 17))
	sb.WriteString("\n")

	for _, g := range groups {
		sb.WriteString(fmt.Sprintf("[%s] %s (%d files, confidence: %.2f)\n",
			g.Type, g.Name, len(g.Files), g.Confidence))
		sb.WriteString("  ")
		if len(g.Files) <= 6 {
			sb.WriteString(strings.Join(shortNames(g.Files), ", "))
		} else {
			short := shortNames(g.Files[:5])
			sb.WriteString(strings.Join(short, ", "))
			sb.WriteString(fmt.Sprintf(", ... +%d more", len(g.Files)-5))
		}
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// CoChangeAnalysisMap returns a map of files to their frequently co-changed companions.
func CoChangeAnalysisMap(projectDir string) map[string][]string {
	result := make(map[string][]string)

	cmd := exec.Command("git", "log", "--name-only", "--pretty=format:", "-100")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return result
	}

	// Parse commits (separated by blank lines)
	cooccurrence := make(map[string]map[string]int)
	commits := splitCommitBlocks(string(out))

	for _, commit := range commits {
		files := extractNonEmpty(commit)
		if len(files) < 2 {
			continue
		}
		for i := 0; i < len(files); i++ {
			for j := i + 1; j < len(files); j++ {
				a, b := files[i], files[j]
				if cooccurrence[a] == nil {
					cooccurrence[a] = make(map[string]int)
				}
				if cooccurrence[b] == nil {
					cooccurrence[b] = make(map[string]int)
				}
				cooccurrence[a][b]++
				cooccurrence[b][a]++
			}
		}
	}

	// For each file, pick the top co-changed companions (minimum 2 co-changes)
	for file, peers := range cooccurrence {
		type entry struct {
			path  string
			count int
		}
		var entries []entry
		for p, c := range peers {
			if c >= 2 {
				entries = append(entries, entry{p, c})
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].count > entries[j].count
		})
		topK := 5
		if topK > len(entries) {
			topK = len(entries)
		}
		for i := 0; i < topK; i++ {
			result[file] = append(result[file], entries[i].path)
		}
	}

	return result
}

// ── Internal strategies ──

func (fg *FileGrouper) analyzePackageGroups(files []string) []FileGroup {
	// Group Go files by their package declaration directory
	packages := make(map[string][]string)
	for _, f := range files {
		if filepath.Ext(f) == ".go" {
			dir := filepath.Dir(f)
			packages[dir] = append(packages[dir], f)
		}
	}

	var groups []FileGroup
	for dir, pkgFiles := range packages {
		if len(pkgFiles) < 2 {
			continue
		}
		name := dir
		if name == "." {
			name = "root"
		}
		groups = append(groups, FileGroup{
			Name:       name,
			Files:      pkgFiles,
			Reason:     "Files in the same Go package directory",
			Confidence: 1.0,
			Type:       "package",
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	return groups
}

func (fg *FileGrouper) analyzeTestPairs(files []string) []FileGroup {
	var groups []FileGroup
	seen := make(map[string]bool)

	for _, f := range files {
		if seen[f] {
			continue
		}
		testPair := fg.FindTestPair(f)
		if testPair == "" {
			continue
		}
		// Ensure we haven't already paired these
		pairKey := f + "|" + testPair
		reversePairKey := testPair + "|" + f
		if seen[pairKey] || seen[reversePairKey] {
			continue
		}
		seen[pairKey] = true
		seen[reversePairKey] = true
		seen[f] = true
		seen[testPair] = true

		baseName := fileBaseName(f)
		if strings.HasSuffix(baseName, "_test") || strings.HasPrefix(baseName, "test_") {
			baseName = fileBaseName(testPair)
		}

		groups = append(groups, FileGroup{
			Name:       baseName,
			Files:      []string{f, testPair},
			Reason:     "Source file and its test counterpart",
			Confidence: 1.0,
			Type:       "test_pair",
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	return groups
}

func (fg *FileGrouper) analyzeFeatureGroups(files []string) []FileGroup {
	// Group files by shared prefix (e.g., auth.go, auth_middleware.go, auth_test.go)
	prefixGroups := make(map[string][]string)

	for _, f := range files {
		base := fileBaseName(f)
		// Strip common suffixes to find the feature root
		root := extractFeatureRoot(base)
		if root == "" {
			continue
		}
		prefixGroups[root] = append(prefixGroups[root], f)
	}

	var groups []FileGroup
	for prefix, prefixFiles := range prefixGroups {
		if len(prefixFiles) < 2 {
			continue
		}
		// Filter out groups that are just a test pair (already captured)
		if len(prefixFiles) == 2 {
			f1Base := fileBaseName(prefixFiles[0])
			f2Base := fileBaseName(prefixFiles[1])
			if isTestVariant(f1Base, f2Base) {
				continue
			}
		}

		groups = append(groups, FileGroup{
			Name:       prefix,
			Files:      prefixFiles,
			Reason:     "Files sharing a common feature prefix: " + prefix,
			Confidence: 0.85,
			Type:       "feature",
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	return groups
}

func (fg *FileGrouper) analyzeImportGroups(files []string) []FileGroup {
	// Build a lightweight import graph for Go files
	goFiles := make(map[string][]string) // file -> imported packages (local paths only)

	for _, f := range files {
		if filepath.Ext(f) != ".go" {
			continue
		}
		imports := fg.extractLocalImports(f)
		if len(imports) > 0 {
			goFiles[f] = imports
		}
	}

	// Find mutual import clusters (files that import each other)
	type importPair struct {
		a, b string
	}
	var mutualPairs []importPair

	for f, imports := range goFiles {
		fDir := filepath.Dir(f)
		for _, imp := range imports {
			// Check if any file in that import target imports back
			for otherFile, otherImports := range goFiles {
				if otherFile == f {
					continue
				}
				otherDir := filepath.Dir(otherFile)
				if otherDir == imp {
					for _, oi := range otherImports {
						if oi == fDir {
							mutualPairs = append(mutualPairs, importPair{f, otherFile})
						}
					}
				}
			}
		}
	}

	// Convert pairs to groups
	var groups []FileGroup
	seen := make(map[string]bool)
	for _, pair := range mutualPairs {
		key := pair.a + "|" + pair.b
		rev := pair.b + "|" + pair.a
		if seen[key] || seen[rev] {
			continue
		}
		seen[key] = true
		seen[rev] = true

		groups = append(groups, FileGroup{
			Name:       filepath.Base(pair.a) + " <-> " + filepath.Base(pair.b),
			Files:      []string{pair.a, pair.b},
			Reason:     "Files with mutual import dependencies",
			Confidence: 0.9,
			Type:       "layer",
		})
	}

	return groups
}

func (fg *FileGrouper) analyzeConfigGroups(files []string) []FileGroup {
	configPatterns := []string{
		".env", ".env.example", ".env.local",
		"config.go", "config.yaml", "config.yml", "config.json", "config.toml",
		"makefile", "dockerfile", "docker-compose.yml",
		"go.mod", "go.sum",
		"package.json", "package-lock.json", "tsconfig.json",
		".gitignore", ".dockerignore",
	}

	var configFiles []string
	for _, f := range files {
		base := filepath.Base(f)
		baseLower := strings.ToLower(base)
		for _, pat := range configPatterns {
			if baseLower == pat || strings.HasPrefix(baseLower, ".env") {
				configFiles = append(configFiles, f)
				break
			}
		}
	}

	if len(configFiles) < 2 {
		return nil
	}

	// Subgroup config files by category
	var groups []FileGroup

	// Environment files
	var envFiles []string
	for _, f := range configFiles {
		base := strings.ToLower(filepath.Base(f))
		if strings.HasPrefix(base, ".env") {
			envFiles = append(envFiles, f)
		}
	}
	if len(envFiles) >= 2 {
		groups = append(groups, FileGroup{
			Name:       "environment",
			Files:      envFiles,
			Reason:     "Environment configuration files",
			Confidence: 0.95,
			Type:       "config",
		})
	}

	// Build/deploy files
	var buildFiles []string
	for _, f := range configFiles {
		base := strings.ToLower(filepath.Base(f))
		if base == "makefile" || base == "dockerfile" || strings.HasPrefix(base, "docker-compose") {
			buildFiles = append(buildFiles, f)
		}
	}
	if len(buildFiles) >= 2 {
		groups = append(groups, FileGroup{
			Name:       "build",
			Files:      buildFiles,
			Reason:     "Build and deployment configuration",
			Confidence: 0.9,
			Type:       "config",
		})
	}

	// Package manager files
	var pkgFiles []string
	for _, f := range configFiles {
		base := strings.ToLower(filepath.Base(f))
		if base == "go.mod" || base == "go.sum" || base == "package.json" || base == "package-lock.json" {
			pkgFiles = append(pkgFiles, f)
		}
	}
	if len(pkgFiles) >= 2 {
		groups = append(groups, FileGroup{
			Name:       "dependencies",
			Files:      pkgFiles,
			Reason:     "Package dependency files",
			Confidence: 0.95,
			Type:       "config",
		})
	}

	return groups
}

// ── Helpers ──

func (fg *FileGrouper) collectFiles() ([]string, error) {
	var files []string
	ignoreSet := make(map[string]bool)
	for _, p := range defaultIgnorePatterns {
		ignoreSet[p] = true
	}

	err := filepath.Walk(fg.ProjectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if ignoreSet[base] {
				return filepath.SkipDir
			}
			return nil
		}
		// Only include source/config files
		ext := filepath.Ext(path)
		base := filepath.Base(path)
		if isSourceExt(ext) || isConfigFile(base) {
			rel, _ := filepath.Rel(fg.ProjectDir, path)
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

func (fg *FileGrouper) relativePath(file string) string {
	if filepath.IsAbs(file) {
		rel, err := filepath.Rel(fg.ProjectDir, file)
		if err == nil {
			return rel
		}
	}
	return file
}

func (fg *FileGrouper) fileExists(relPath string) bool {
	absPath := filepath.Join(fg.ProjectDir, relPath)
	_, err := os.Stat(absPath)
	return err == nil
}

func (fg *FileGrouper) extractLocalImports(relPath string) []string {
	absPath := filepath.Join(fg.ProjectDir, relPath)
	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	// Detect the module path from go.mod
	modPath := detectModulePath(fg.ProjectDir)
	if modPath == "" {
		return nil
	}

	var imports []string
	inBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "import (") {
			inBlock = true
			continue
		}
		if inBlock && trimmed == ")" {
			inBlock = false
			continue
		}

		var importPath string
		if inBlock {
			// Inside import block
			parts := strings.Split(trimmed, "\"")
			if len(parts) >= 2 {
				importPath = parts[1]
			}
		} else if strings.HasPrefix(trimmed, "import \"") {
			parts := strings.Split(trimmed, "\"")
			if len(parts) >= 2 {
				importPath = parts[1]
			}
		}

		if importPath != "" && strings.HasPrefix(importPath, modPath) {
			// Convert to relative path within project
			relImport := strings.TrimPrefix(importPath, modPath+"/")
			imports = append(imports, relImport)
		}
	}
	return imports
}

func isSourceExt(ext string) bool {
	switch ext {
	case ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".rs", ".java",
		".rb", ".c", ".h", ".cpp", ".hpp", ".cs", ".swift", ".kt":
		return true
	}
	return false
}

func isConfigFile(name string) bool {
	lower := strings.ToLower(name)
	configNames := []string{
		"makefile", "dockerfile", "go.mod", "go.sum",
		"package.json", "package-lock.json", "tsconfig.json",
		".gitignore", ".dockerignore",
	}
	for _, cn := range configNames {
		if lower == cn {
			return true
		}
	}
	if strings.HasPrefix(lower, ".env") {
		return true
	}
	ext := filepath.Ext(lower)
	switch ext {
	case ".yaml", ".yml", ".toml", ".json":
		return true
	}
	return false
}

func fileBaseName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	// Handle double extensions like .test.tsx
	if strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec") {
		name = strings.TrimSuffix(strings.TrimSuffix(name, ".test"), ".spec")
	}
	return name
}

func extractFeatureRoot(baseName string) string {
	// Strip common test prefixes/suffixes
	name := baseName
	name = strings.TrimSuffix(name, "_test")
	name = strings.TrimPrefix(name, "test_")

	// Split by underscore and take the first meaningful part
	parts := strings.Split(name, "_")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return parts[0]
}

func isTestVariant(a, b string) bool {
	aClean := strings.TrimSuffix(strings.TrimPrefix(a, "test_"), "_test")
	bClean := strings.TrimSuffix(strings.TrimPrefix(b, "test_"), "_test")
	return aClean == bClean
}

func shortNames(files []string) []string {
	result := make([]string, len(files))
	for i, f := range files {
		result[i] = filepath.Base(f)
	}
	return result
}

func dedupe(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func splitCommitBlocks(s string) []string {
	var chunks []string
	var current strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
		} else {
			current.WriteString(line)
			current.WriteString("\n")
		}
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func extractNonEmpty(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
