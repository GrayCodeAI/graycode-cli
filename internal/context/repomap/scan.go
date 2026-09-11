package repomap

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// skipDirs are directory names that are never descended into during scanning.
var skipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	".hawk":        {},
	"dist":         {},
	"build":        {},
	"target":       {},
	".venv":        {},
	"__pycache__":  {},
	".idea":        {},
	".vscode":      {},
}

// maxFileBytes caps the size of a file we will read for extraction.
const maxFileBytes = 1 << 20 // 1 MiB

// scan walks root, extracting symbols and imports for each supported source file.
func (g *Graph) scan(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries rather than aborting the whole walk.
			return nil //nolint:nilerr
		}
		if d.IsDir() {
			if path != root {
				if _, skip := skipDirs[d.Name()]; skip {
					return filepath.SkipDir
				}
				if strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
					return filepath.SkipDir
				}
			}
			return nil
		}
		lang := detectLang(path)
		if lang == "" {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil || info.Size() > maxFileBytes {
			return nil
		}
		data, rerr := os.ReadFile(path) // #nosec G304,G122 -- read-only repository scan
		if rerr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		rel = normalizePath(rel)

		node := &FileNode{Path: rel, Lang: lang}
		node.Symbols, node.Imports = extract(lang, rel, data)
		node.Imports = dedupeStrings(node.Imports)
		g.Nodes[rel] = node
		return nil
	})
}

// detectLang maps a file path to a language id, or "" if unsupported.
func detectLang(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".cxx", ".hpp":
		return "cpp"
	default:
		return ""
	}
}

// extract dispatches to the language-appropriate extractor.
func extract(lang, path string, data []byte) ([]Symbol, []string) {
	if lang == "go" {
		syms, imps, ok := extractGo(path, data)
		if ok {
			return syms, imps
		}
		// Fall through to regex if the Go source failed to parse (e.g. partial
		// file): better a degraded map than none.
	}
	return extractRegex(lang, data)
}
