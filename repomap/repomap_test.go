package repomap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	// Create a temp directory with some Go files
	dir := t.TempDir()

	goFile := filepath.Join(dir, "main.go")
	err := os.WriteFile(goFile, []byte(`package main

func main() {}

type Server struct {}

func (s *Server) Start() error { return nil }
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	pyFile := filepath.Join(dir, "app.py")
	err = os.WriteFile(pyFile, []byte(`class App:
    pass

def run():
    pass
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	rm, err := Generate(dir, Options{MaxFiles: 100, MaxTokens: 5000})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if rm == nil {
		t.Fatal("expected non-nil RepoMap")
	}
	if len(rm.Files) != 2 {
		t.Fatalf("expected 2 file maps, got %d", len(rm.Files))
	}

	// Check Go file
	var goMap *FileMap
	var pyMap *FileMap
	for i := range rm.Files {
		if strings.HasSuffix(rm.Files[i].Path, "main.go") {
			goMap = &rm.Files[i]
		}
		if strings.HasSuffix(rm.Files[i].Path, "app.py") {
			pyMap = &rm.Files[i]
		}
	}

	if goMap == nil {
		t.Fatal("expected main.go in file maps")
	}
	if len(goMap.Symbols) != 3 {
		t.Errorf("expected 3 Go symbols, got %d: %+v", len(goMap.Symbols), goMap.Symbols)
	}

	if pyMap == nil {
		t.Fatal("expected app.py in file maps")
	}
	if len(pyMap.Symbols) != 2 {
		t.Errorf("expected 2 Python symbols, got %d: %+v", len(pyMap.Symbols), pyMap.Symbols)
	}
}

func TestGenerateIgnoresGitDir(t *testing.T) {
	dir := t.TempDir()

	// Create .git directory with a Go file
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0o755)
	os.WriteFile(filepath.Join(gitDir, "hooks.go"), []byte(`package hooks
func Run() {}
`), 0o644)

	// Create a normal Go file
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main
func main() {}
`), 0o644)

	rm, err := Generate(dir, Options{})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(rm.Files) != 1 {
		t.Errorf("expected 1 file (should ignore .git), got %d", len(rm.Files))
	}
}

func TestGenerateEmptyDir(t *testing.T) {
	dir := t.TempDir()

	rm, err := Generate(dir, Options{})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(rm.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(rm.Files))
	}
}

func TestGenerateMaxFiles(t *testing.T) {
	dir := t.TempDir()

	// Create 5 Go files
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, strings.Replace("file_N.go", "N", string(rune('a'+i)), 1))
		os.WriteFile(name, []byte("package main\nfunc Func() {}\n"), 0o644)
	}

	rm, err := Generate(dir, Options{MaxFiles: 3})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	// Should have at most 3 files with symbols
	if len(rm.Files) > 3 {
		t.Errorf("expected at most 3 files, got %d", len(rm.Files))
	}
}

func TestFormat(t *testing.T) {
	rm := &RepoMap{
		Files: []FileMap{
			{Path: "main.go", Symbols: []Symbol{
				{Name: "main", Kind: "func", Line: 3},
				{Name: "Config", Kind: "struct", Line: 7},
			}},
			{Path: "server.go", Symbols: []Symbol{
				{Name: "Server", Kind: "struct", Line: 1},
				{Name: "Start", Kind: "func", Line: 5},
			}},
		},
	}

	out := rm.Format(5000)
	if out == "" {
		t.Fatal("expected non-empty formatted output")
	}
	if !strings.Contains(out, "main.go") {
		t.Error("expected main.go in output")
	}
	if !strings.Contains(out, "func main") {
		t.Error("expected func main in output")
	}
	if !strings.Contains(out, "struct Config") {
		t.Error("expected struct Config in output")
	}
	if !strings.Contains(out, "server.go") {
		t.Error("expected server.go in output")
	}
}

func TestFormatNil(t *testing.T) {
	var rm *RepoMap
	if out := rm.Format(1000); out != "" {
		t.Errorf("expected empty string for nil RepoMap, got %q", out)
	}
}

func TestFormatEmpty(t *testing.T) {
	rm := &RepoMap{}
	if out := rm.Format(1000); out != "" {
		t.Errorf("expected empty string for empty RepoMap, got %q", out)
	}
}

func TestFormatTokenTruncation(t *testing.T) {
	// Create a large repo map
	var files []FileMap
	for i := 0; i < 100; i++ {
		files = append(files, FileMap{
			Path: strings.Replace("file_NNN.go", "NNN", string(rune('a'+i%26))+string(rune('0'+i/26)), 1),
			Symbols: []Symbol{
				{Name: "Func", Kind: "func", Line: 1},
				{Name: "Type", Kind: "struct", Line: 5},
				{Name: "Handler", Kind: "interface", Line: 10},
			},
		})
	}
	rm := &RepoMap{Files: files}

	out := rm.Format(50) // very small token budget
	if out == "" {
		t.Fatal("expected some output even with small budget")
	}
	// Should contain truncation notice
	if !strings.Contains(out, "... and") {
		t.Error("expected truncation notice in output")
	}
}

func TestIsSupportedExt(t *testing.T) {
	supported := []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".java",
		".c", ".h", ".cpp", ".cc", ".cs", ".php", ".rb", ".kt", ".swift", ".scala",
		".lua", ".dart", ".ex", ".exs", ".hs"}
	for _, ext := range supported {
		if !isSupportedExt(ext) {
			t.Errorf("expected %s to be supported", ext)
		}
	}

	unsupported := []string{".txt", ".md", ".yaml", ".json", ".xml", ".csv"}
	for _, ext := range unsupported {
		if isSupportedExt(ext) {
			t.Errorf("expected %s to not be supported", ext)
		}
	}
}

func TestCacheIntegration(t *testing.T) {
	CacheClear()
	if CacheSize() != 0 {
		t.Fatal("expected empty cache")
	}

	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	os.WriteFile(goFile, []byte("package main\nfunc main() {}\n"), 0o644)

	// First call populates cache
	rm1, _ := Generate(dir, Options{})
	if CacheSize() != 1 {
		t.Errorf("expected cache size 1, got %d", CacheSize())
	}

	// Second call should use cache
	rm2, _ := Generate(dir, Options{})
	if len(rm1.Files) != len(rm2.Files) {
		t.Error("expected same results from cached generation")
	}

	CacheClear()
	if CacheSize() != 0 {
		t.Error("expected empty cache after clear")
	}
}
