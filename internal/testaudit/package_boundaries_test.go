package testaudit

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	hawkModule  = "github.com/GrayCodeAI/hawk"
	eyrieModule = "github.com/GrayCodeAI/eyrie"
)

var supportEngines = []string{"eyrie", "harrier", "shrike", "swift", "kestrel", "merlin"}

type packageImport struct {
	file string
	line int
	path string
}

// TestPackageDependencyGraph checks production imports using the Go parser.
// The shell guards remain useful for fast, cross-repository checks, while this
// test gives us syntax-aware file/line diagnostics and does not depend on the
// sibling repositories being buildable from the parent workspace.
func TestPackageDependencyGraph(t *testing.T) {
	root := repoRoot(t)

	checkHawkEyrieFacade(t, root)
	checkHawkInternalLayers(t, root)
	checkSupportRepositoryBoundaries(t, root)
	checkGoSDKBoundary(t, root)
}

func checkHawkEyrieFacade(t *testing.T, root string) {
	paths := []string{filepath.Join(root, "internal"), filepath.Join(root, "cmd")}
	var violations []string

	for _, path := range paths {
		for _, imp := range productionImports(t, root, path) {
			if !strings.HasPrefix(imp.path, eyrieModule+"/") {
				continue
			}
			if imp.path == eyrieModule+"/engine" || strings.HasPrefix(imp.path, eyrieModule+"/engine/") {
				continue
			}
			// Hawk uses the full vendored Eyrie API surface for provider, graph,
			// and tooling contracts that the engine facade does not re-export.
			switch imp.path {
			case eyrieModule + "/llm", eyrieModule + "/graph", eyrieModule + "/tools":
				continue
			}
			// Hawk's gateway declares the credential service name so existing
			// keychain entries remain compatible. It is the only non-engine
			// production exception.
			relFile, relErr := filepath.Rel(root, imp.file)
			if relErr == nil && filepath.ToSlash(filepath.Dir(relFile)) == "internal/provider/gateway" &&
				imp.path == eyrieModule+"/credentials" {
				continue
			}
			violations = append(violations, formatImportViolation(root, imp, "use the eyrie/engine facade"))
		}
	}

	assertNoPackageViolations(t, "Hawk Eyrie facade", violations)
}

func checkHawkInternalLayers(t *testing.T, root string) {
	rules := map[string][]string{
		"internal/engine":      {"cmd", "internal/daemon", "internal/platform", "internal/bridge"},
		"internal/permissions": {"cmd", "internal/daemon", "internal/engine", "internal/platform", "internal/bridge"},
		"internal/session":     {"cmd", "internal/daemon", "internal/engine", "internal/platform", "internal/bridge"},
		"internal/platform":    {"cmd", "internal/daemon", "internal/engine", "internal/bridge"},
		"internal/bridge":      {"cmd", "internal/daemon", "internal/engine", "internal/platform"},
	}
	var violations []string

	for source, forbidden := range rules {
		for _, imp := range productionImports(t, root, filepath.Join(root, filepath.FromSlash(source))) {
			rel, err := filepath.Rel(root, imp.file)
			if err != nil {
				t.Fatalf("relative path for %s: %v", imp.file, err)
			}
			if !strings.HasPrefix(imp.path, hawkModule+"/") {
				continue
			}
			for _, prefix := range forbidden {
				if strings.HasPrefix(imp.path, hawkModule+"/"+prefix+"/") || imp.path == hawkModule+"/"+prefix {
					violations = append(violations, fmt.Sprintf("%s:%d imports %s (%s); %s must not depend on %s", filepath.ToSlash(rel), imp.line, imp.path, source, source, prefix))
				}
			}
		}
	}

	assertNoPackageViolations(t, "Hawk internal layers", violations)
}

func checkSupportRepositoryBoundaries(t *testing.T, root string) {
	var violations []string

	for _, owner := range supportEngines {
		for _, repoRoot := range repositoryRoots(root, owner) {
			for _, imp := range productionImports(t, root, repoRoot) {
				if strings.HasPrefix(imp.path, hawkModule+"/internal/") || imp.path == hawkModule+"/shared/types" {
					violations = append(violations, formatImportViolation(root, imp, "support engines must not import Hawk internals"))
					continue
				}

				for _, peer := range supportEngines {
					if peer == owner {
						continue
					}
					peerPrefix := "github.com/GrayCodeAI/" + peer
					if imp.path == peerPrefix || strings.HasPrefix(imp.path, peerPrefix+"/") {
						violations = append(violations, formatImportViolation(root, imp, fmt.Sprintf("%s must not import peer engine %s", owner, peer)))
					}
				}
			}
		}
	}

	assertNoPackageViolations(t, "support repository boundaries", violations)
}

func checkGoSDKBoundary(t *testing.T, root string) {
	var violations []string
	for _, sdkRoot := range []string{
		filepath.Join(root, "..", "sparrow"),
	} {
		for _, imp := range productionImports(t, root, sdkRoot) {
			for _, engine := range supportEngines {
				prefix := "github.com/GrayCodeAI/" + engine
				if imp.path == prefix || strings.HasPrefix(imp.path, prefix+"/") {
					violations = append(violations, formatImportViolation(root, imp, "SDKs must consume Hawk public surfaces"))
				}
			}
		}
	}
	assertNoPackageViolations(t, "Go SDK boundary", violations)
}

func repositoryRoots(root, repo string) []string {
	return []string{
		filepath.Join(root, "..", repo),
	}
}

func productionImports(t *testing.T, root, dir string) []packageImport {
	t.Helper()
	var imports []packageImport
	if !pathExists(dir) {
		return imports
	}

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".gocache", ".gomodcache", "vendor", "node_modules", "testdata":
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			// Skip files that fail to parse (e.g. generated files with build tags
			// or encoding issues).
			// This test checks for boundary violations, not syntax correctness;
			// a file that doesn't parse cannot contain import violations.
			return nil
		}
		for _, spec := range file.Imports {
			imports = append(imports, packageImport{
				file: path,
				line: fset.Position(spec.Pos()).Line,
				path: strings.Trim(spec.Path.Value, `"`),
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production imports under %s: %v", filepath.ToSlash(dir), err)
	}

	sort.Slice(imports, func(i, j int) bool {
		if imports[i].file != imports[j].file {
			return imports[i].file < imports[j].file
		}
		return imports[i].line < imports[j].line
	})
	return imports
}

func pathExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func formatImportViolation(root string, imp packageImport, rule string) string {
	rel, err := filepath.Rel(root, imp.file)
	if err != nil {
		rel = imp.file
	}
	return fmt.Sprintf("%s:%d imports %s; %s", filepath.ToSlash(rel), imp.line, imp.path, rule)
}

func assertNoPackageViolations(t *testing.T, name string, violations []string) {
	t.Helper()
	if len(violations) == 0 {
		return
	}
	sort.Strings(violations)
	t.Fatalf("%s failed:\n%s", name, strings.Join(violations, "\n"))
}
