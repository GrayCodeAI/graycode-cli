package repomap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestNewIncrementalMap_CreatesEmptyCache(t *testing.T) {
	cacheDir := t.TempDir()

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatalf("NewIncrementalMap failed: %v", err)
	}

	allSymbols := im.AllSymbols()
	if len(allSymbols) != 0 {
		t.Errorf("expected empty symbols map, got %d entries", len(allSymbols))
	}
}

func TestNewIncrementalMap_LoadsExistingCache(t *testing.T) {
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "repomap-cache.json")

	existing := map[string]FileCache{
		"main.go": {
			Hash:    "abc123",
			Mtime:   1000,
			Symbols: []string{"func main"},
		},
	}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatalf("NewIncrementalMap failed: %v", err)
	}

	syms := im.Symbols("main.go")
	if len(syms) != 1 || syms[0] != "func main" {
		t.Errorf("expected cached symbols, got %v", syms)
	}
}

func TestNewIncrementalMap_HandlesCorruptedCache(t *testing.T) {
	cacheDir := t.TempDir()
	cacheFile := filepath.Join(cacheDir, "repomap-cache.json")

	if err := os.WriteFile(cacheFile, []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatalf("NewIncrementalMap failed on corrupted cache: %v", err)
	}

	allSymbols := im.AllSymbols()
	if len(allSymbols) != 0 {
		t.Errorf("expected empty symbols for corrupted cache, got %d entries", len(allSymbols))
	}
}

func TestIncrementalMap_UpdateDetectsNewFiles(t *testing.T) {
	cacheDir := t.TempDir()
	rootDir := t.TempDir()

	// Create a Go file
	goFile := filepath.Join(rootDir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n\ntype Server struct {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatal(err)
	}

	changed, err := im.Update(rootDir)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if len(changed) != 1 || changed[0] != "main.go" {
		t.Errorf("expected [main.go] changed, got %v", changed)
	}

	syms := im.Symbols("main.go")
	if len(syms) != 2 {
		t.Errorf("expected 2 symbols (main, Server), got %d: %v", len(syms), syms)
	}
}

func TestIncrementalMap_SkipsUnchangedFiles(t *testing.T) {
	cacheDir := t.TempDir()
	rootDir := t.TempDir()

	goFile := filepath.Join(rootDir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatal(err)
	}

	// First update: file is new
	changed1, err := im.Update(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed1) != 1 {
		t.Fatalf("expected 1 changed file on first run, got %d", len(changed1))
	}

	// Second update: file unchanged, should return no changes
	changed2, err := im.Update(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed2) != 0 {
		t.Errorf("expected 0 changed files on second run, got %v", changed2)
	}
}

func TestIncrementalMap_DetectsModifiedFiles(t *testing.T) {
	cacheDir := t.TempDir()
	rootDir := t.TempDir()

	goFile := filepath.Join(rootDir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatal(err)
	}

	// First update
	_, err = im.Update(rootDir)
	if err != nil {
		t.Fatal(err)
	}

	syms1 := im.Symbols("main.go")
	if len(syms1) != 1 {
		t.Fatalf("expected 1 symbol after first update, got %d: %v", len(syms1), syms1)
	}

	// Modify the file (add a new function)
	if err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n\nfunc helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Second update: should detect the change
	changed2, err := im.Update(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed2) != 1 || changed2[0] != "main.go" {
		t.Errorf("expected [main.go] changed, got %v", changed2)
	}

	syms2 := im.Symbols("main.go")
	if len(syms2) != 2 {
		t.Errorf("expected 2 symbols after modification, got %d: %v", len(syms2), syms2)
	}
}

func TestIncrementalMap_RemovesDeletedFiles(t *testing.T) {
	cacheDir := t.TempDir()
	rootDir := t.TempDir()

	goFile := filepath.Join(rootDir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	helperFile := filepath.Join(rootDir, "helper.go")
	if err := os.WriteFile(helperFile, []byte("package main\n\nfunc helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatal(err)
	}

	// First update: both files indexed
	_, err = im.Update(rootDir)
	if err != nil {
		t.Fatal(err)
	}

	allSyms := im.AllSymbols()
	if len(allSyms) != 2 {
		t.Fatalf("expected 2 files in cache, got %d", len(allSyms))
	}

	// Delete helper.go
	if err := os.Remove(helperFile); err != nil {
		t.Fatal(err)
	}

	// Second update: should detect deletion
	changed, err := im.Update(rootDir)
	if err != nil {
		t.Fatal(err)
	}

	// helper.go should appear in changed (deleted)
	found := false
	for _, c := range changed {
		if c == "helper.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected helper.go in changed list (deleted), got %v", changed)
	}

	// helper.go should no longer have symbols
	syms := im.Symbols("helper.go")
	if syms != nil {
		t.Errorf("expected nil symbols for deleted file, got %v", syms)
	}

	// main.go should still have symbols
	mainSyms := im.Symbols("main.go")
	if len(mainSyms) != 1 {
		t.Errorf("expected 1 symbol for main.go, got %d", len(mainSyms))
	}
}

func TestIncrementalMap_SymbolPreservation(t *testing.T) {
	cacheDir := t.TempDir()
	rootDir := t.TempDir()

	// Create two files
	if err := os.WriteFile(filepath.Join(rootDir, "a.go"), []byte("package main\n\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "b.go"), []byte("package main\n\nfunc Beta() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = im.Update(rootDir)
	if err != nil {
		t.Fatal(err)
	}

	// Modify only b.go
	if err := os.WriteFile(filepath.Join(rootDir, "b.go"), []byte("package main\n\nfunc BetaV2() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := im.Update(rootDir)
	if err != nil {
		t.Fatal(err)
	}

	// Only b.go should be in changed
	if len(changed) != 1 || changed[0] != "b.go" {
		t.Errorf("expected only [b.go] changed, got %v", changed)
	}

	// a.go symbols should be preserved
	aSyms := im.Symbols("a.go")
	if len(aSyms) != 1 || aSyms[0] != "func Alpha" {
		t.Errorf("expected a.go symbols preserved, got %v", aSyms)
	}

	// b.go symbols should be updated
	bSyms := im.Symbols("b.go")
	if len(bSyms) != 1 || bSyms[0] != "func BetaV2" {
		t.Errorf("expected b.go symbols updated, got %v", bSyms)
	}
}

func TestIncrementalMap_SaveAndReload(t *testing.T) {
	cacheDir := t.TempDir()
	rootDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(rootDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create, update, and save
	im1, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := im1.Update(rootDir); err != nil {
		t.Fatal(err)
	}
	if err := im1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload from cache
	im2, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	// Should have the same symbols
	syms := im2.Symbols("main.go")
	if len(syms) != 1 || syms[0] != "func main" {
		t.Errorf("expected saved symbols to be reloaded, got %v", syms)
	}
}

func TestIncrementalMap_AllSymbolsReturnsAllFiles(t *testing.T) {
	cacheDir := t.TempDir()
	rootDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(rootDir, "a.go"), []byte("package main\n\nfunc Alpha() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "b.py"), []byte("def beta():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := im.Update(rootDir); err != nil {
		t.Fatal(err)
	}

	allSyms := im.AllSymbols()
	if len(allSyms) != 2 {
		t.Fatalf("expected 2 files, got %d", len(allSyms))
	}

	paths := make([]string, 0, len(allSyms))
	for p := range allSyms {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	if paths[0] != "a.go" || paths[1] != "b.py" {
		t.Errorf("unexpected paths: %v", paths)
	}
}

func TestIncrementalMap_IgnoresDefaultPatterns(t *testing.T) {
	cacheDir := t.TempDir()
	rootDir := t.TempDir()

	// Create a file inside node_modules (should be ignored)
	nmDir := filepath.Join(rootDir, "node_modules")
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "lib.js"), []byte("function lib() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a normal file
	if err := os.WriteFile(filepath.Join(rootDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := im.Update(rootDir); err != nil {
		t.Fatal(err)
	}

	allSyms := im.AllSymbols()
	if len(allSyms) != 1 {
		t.Errorf("expected 1 file (node_modules should be ignored), got %d", len(allSyms))
	}
	if _, ok := allSyms["main.go"]; !ok {
		t.Error("expected main.go in symbols")
	}
}

func TestIncrementalMap_SaveCreatesDirectory(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "nested", ".hawk")

	im, err := NewIncrementalMap(cacheDir)
	if err != nil {
		t.Fatal(err)
	}

	if err := im.Save(); err != nil {
		t.Fatalf("Save should create parent directories: %v", err)
	}

	// Verify file was created
	cacheFile := filepath.Join(cacheDir, "repomap-cache.json")
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		t.Error("expected cache file to be created")
	}
}
