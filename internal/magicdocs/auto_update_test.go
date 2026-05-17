package magicdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanForMagicDocs_FindsMarker(t *testing.T) {
	dir := t.TempDir()
	content := `package main

// # MAGIC DOC: API Reference
// This section is auto-generated.
func main() {}
`
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0o644)

	docs := ScanForMagicDocs(dir)
	if len(docs) != 1 {
		t.Fatalf("expected 1 magic doc, got %d", len(docs))
	}
	if docs[0].Title != "API Reference" {
		t.Fatalf("expected title 'API Reference', got %q", docs[0].Title)
	}
}

func TestScanForMagicDocs_NoMarker(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)

	docs := ScanForMagicDocs(dir)
	if len(docs) != 0 {
		t.Fatalf("expected 0 magic docs, got %d", len(docs))
	}
}

func TestScanForMagicDocs_SkipsHidden(t *testing.T) {
	dir := t.TempDir()
	hiddenDir := filepath.Join(dir, ".hidden")
	os.MkdirAll(hiddenDir, 0o755)
	os.WriteFile(filepath.Join(hiddenDir, "secret.go"), []byte("// # MAGIC DOC: Hidden"), 0o644)

	docs := ScanForMagicDocs(dir)
	if len(docs) != 0 {
		t.Fatalf("expected 0 magic docs (hidden dir skipped), got %d", len(docs))
	}
}

func TestMagicDocFile_NeedsUpdate(t *testing.T) {
	fresh := &MagicDocFile{LastUpdated: time.Now()}
	if fresh.NeedsUpdate() {
		t.Fatal("fresh doc should not need update")
	}

	stale := &MagicDocFile{LastUpdated: time.Now().Add(-48 * time.Hour)}
	if !stale.NeedsUpdate() {
		t.Fatal("stale doc should need update")
	}
}

func TestMagicDocFile_GenerateUpdatePrompt(t *testing.T) {
	doc := &MagicDocFile{
		Path:        "/path/to/file.go",
		Title:       "API Reference",
		LastUpdated: time.Now().Add(-48 * time.Hour),
	}

	prompt := doc.GenerateUpdatePrompt()
	if !strings.Contains(prompt, "API Reference") {
		t.Fatalf("expected title in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "/path/to/file.go") {
		t.Fatalf("expected path in prompt, got %q", prompt)
	}
}
