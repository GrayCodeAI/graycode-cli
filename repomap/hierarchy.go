package repomap

import (
	"os"
	"path/filepath"
	"strings"
)

// HierarchicalSummary provides 3-level code summarization:
// Level 1: Project (all packages as one-liners)
// Level 2: Package (file list with exported symbols)
// Level 3: File (function signatures, no bodies)
type HierarchicalSummary struct {
	Root     string
	Packages []PackageSummary
}

// PackageSummary is a level-2 summary of a Go package.
type PackageSummary struct {
	Path    string
	Name    string
	Files   []FileSummary
	Symbols int
}

// FileSummary is a level-3 summary of a single file.
type FileSummary struct {
	Path       string
	Functions  []string // exported function signatures
	Types      []string // exported type names
	LineCount  int
}

// BuildHierarchy scans a Go project and builds a 3-level summary.
func BuildHierarchy(root string) (*HierarchicalSummary, error) {
	h := &HierarchicalSummary{Root: root}
	pkgs := make(map[string]*PackageSummary)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if shouldSkipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		dir := filepath.Dir(path)
		relDir, _ := filepath.Rel(root, dir)
		if relDir == "" {
			relDir = "."
		}

		pkg, ok := pkgs[relDir]
		if !ok {
			pkg = &PackageSummary{Path: relDir, Name: filepath.Base(dir)}
			pkgs[relDir] = pkg
		}

		fs := summarizeFile(path)
		pkg.Files = append(pkg.Files, fs)
		pkg.Symbols += len(fs.Functions) + len(fs.Types)

		return nil
	})

	for _, pkg := range pkgs {
		h.Packages = append(h.Packages, *pkg)
	}
	return h, err
}

// FormatLevel1 returns the project-level summary (one line per package).
func (h *HierarchicalSummary) FormatLevel1(maxTokens int) string {
	var b strings.Builder
	tokens := 0
	for _, pkg := range h.Packages {
		line := pkg.Path + "/ (" + itoa(pkg.Symbols) + " symbols, " + itoa(len(pkg.Files)) + " files)\n"
		est := len(strings.Fields(line))
		if maxTokens > 0 && tokens+est > maxTokens {
			break
		}
		b.WriteString(line)
		tokens += est
	}
	return b.String()
}

// FormatLevel2 returns the package-level summary (files + exported symbols).
func (h *HierarchicalSummary) FormatLevel2(pkgPath string, maxTokens int) string {
	for _, pkg := range h.Packages {
		if pkg.Path == pkgPath {
			var b strings.Builder
			tokens := 0
			for _, f := range pkg.Files {
				line := "  " + filepath.Base(f.Path)
				if len(f.Functions) > 0 {
					line += ": " + strings.Join(f.Functions, ", ")
				}
				line += "\n"
				est := len(strings.Fields(line))
				if maxTokens > 0 && tokens+est > maxTokens {
					break
				}
				b.WriteString(line)
				tokens += est
			}
			return b.String()
		}
	}
	return ""
}

// FormatLevel3 returns a file-level summary (full function signatures).
func (h *HierarchicalSummary) FormatLevel3(filePath string) string {
	for _, pkg := range h.Packages {
		for _, f := range pkg.Files {
			if f.Path == filePath || strings.HasSuffix(f.Path, filePath) {
				var b strings.Builder
				for _, fn := range f.Functions {
					b.WriteString(fn + "\n")
				}
				for _, t := range f.Types {
					b.WriteString("type " + t + "\n")
				}
				return b.String()
			}
		}
	}
	return ""
}

func summarizeFile(path string) FileSummary {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileSummary{Path: path}
	}

	lines := strings.Split(string(data), "\n")
	fs := FileSummary{
		Path:      path,
		LineCount: len(lines),
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Exported functions
		if strings.HasPrefix(trimmed, "func ") && isExported(trimmed) {
			sig := extractSignature(trimmed)
			if sig != "" {
				fs.Functions = append(fs.Functions, sig)
			}
		}
		// Exported types
		if strings.HasPrefix(trimmed, "type ") && len(trimmed) > 5 {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 && isExportedName(parts[1]) {
				fs.Types = append(fs.Types, parts[1])
			}
		}
	}
	return fs
}

func extractSignature(line string) string {
	// Extract up to the opening brace
	if idx := strings.Index(line, "{"); idx > 0 {
		return strings.TrimSpace(line[:idx])
	}
	return line
}

func isExported(funcLine string) bool {
	// "func Name" or "func (r *T) Name"
	parts := strings.Fields(funcLine)
	for i, p := range parts {
		if p == "func" && i+1 < len(parts) {
			next := parts[i+1]
			if strings.HasPrefix(next, "(") {
				// method: find the name after the receiver
				for j := i + 2; j < len(parts); j++ {
					if !strings.HasPrefix(parts[j], "(") && !strings.HasPrefix(parts[j], "*") && !strings.HasSuffix(parts[j], ")") {
						return isExportedName(parts[j])
					}
				}
			}
			return isExportedName(next)
		}
	}
	return false
}

func isExportedName(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}
