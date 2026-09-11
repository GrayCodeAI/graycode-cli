package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/hooks"
)

func TestWalkUpDiscoverer_FindsAgentsMd(t *testing.T) {
	dir := t.TempDir()
	// Create AGENTS.md at project root
	agentsMd := "# Project Rules\nUse gofmt."
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMd), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create a subdirectory
	sub := filepath.Join(dir, "src", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// Read a file deep in the tree
	target := filepath.Join(sub, "main.go")
	if err := os.WriteFile(target, []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewWalkUpDiscoverer(dir)
	discovered := d.Discover(target)
	if len(discovered) == 0 {
		t.Fatal("expected to discover AGENTS.md")
	}
	found := false
	for _, cf := range discovered {
		if cf.FileName == "AGENTS.md" {
			found = true
			if cf.Content != agentsMd {
				t.Errorf("content mismatch: got %q", cf.Content)
			}
		}
	}
	if !found {
		t.Error("AGENTS.md not found in discovered files")
	}
}

func TestWalkUpDiscoverer_Deduplication(t *testing.T) {
	dir := t.TempDir()
	agentsMd := "# Rules"
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsMd), 0o644)
	sub := filepath.Join(dir, "a", "b")
	os.MkdirAll(sub, 0o755)
	f1 := filepath.Join(sub, "f1.go")
	f2 := filepath.Join(sub, "f2.go")
	os.WriteFile(f1, []byte("package a"), 0o644)
	os.WriteFile(f2, []byte("package b"), 0o644)

	d := NewWalkUpDiscoverer(dir)

	// First discover
	d1 := d.Discover(f1)
	if len(d1) == 0 {
		t.Fatal("expected to discover AGENTS.md on first call")
	}

	// Second discover — should be empty (already cached)
	d2 := d.Discover(f2)
	if len(d2) != 0 {
		t.Errorf("expected 0 on second call (dedup), got %d", len(d2))
	}
}

func TestWalkUpDiscoverer_MultipleLevels(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Root"), 0o644)
	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte("# Src Rules"), 0o644)
	deep := filepath.Join(sub, "pkg")
	os.MkdirAll(deep, 0o755)
	target := filepath.Join(deep, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	d := NewWalkUpDiscoverer(dir)
	discovered := d.Discover(target)

	if len(discovered) < 2 {
		t.Fatalf("expected at least 2 discoveries (root + src), got %d", len(discovered))
	}
}

func TestWalkUpDiscoverer_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	d := NewWalkUpDiscoverer(dir)
	discovered := d.Discover(filepath.Join(dir, "nonexistent.go"))
	if len(discovered) != 0 {
		t.Errorf("expected 0 discoveries for nonexistent file, got %d", len(discovered))
	}
}

func TestWalkUpDiscoverer_Truncation(t *testing.T) {
	dir := t.TempDir()
	bigContent := make([]byte, 100*1024) // 100KB
	for i := range bigContent {
		bigContent[i] = 'x'
	}
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), bigContent, 0o644)
	target := filepath.Join(dir, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	d := NewWalkUpDiscoverer(dir).WithMaxFileKB(10) // 10KB cap
	discovered := d.Discover(target)
	if len(discovered) == 0 {
		t.Fatal("expected to discover AGENTS.md")
	}
	if len(discovered[0].Content) > 10*1024+50 { // 10KB + truncation message
		t.Errorf("content too large: %d bytes", len(discovered[0].Content))
	}
}

func TestWalkUpDiscoverer_HandlePostTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Rules"), 0o644)
	target := filepath.Join(dir, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	d := NewWalkUpDiscoverer(dir)
	envelope := hooks.EventEnvelope{
		Payload: map[string]interface{}{
			"tool": "Read",
			"path": target,
		},
	}
	err := d.HandlePostTool(nil, envelope)
	if err != nil {
		t.Fatalf("HandlePostTool: %v", err)
	}
	pending := d.FlushPending()
	if pending == "" {
		t.Error("expected pending conventions after HandlePostTool")
	}
}

func TestWalkUpDiscoverer_HandlePostTool_IgnoresNonRead(t *testing.T) {
	dir := t.TempDir()
	d := NewWalkUpDiscoverer(dir)
	envelope := hooks.EventEnvelope{
		Payload: map[string]interface{}{
			"tool": "Bash",
			"cmd":  "ls",
		},
	}
	err := d.HandlePostTool(nil, envelope)
	if err != nil {
		t.Fatalf("HandlePostTool: %v", err)
	}
	if pending := d.FlushPending(); pending != "" {
		t.Error("expected no pending for non-read tools")
	}
}

func TestWalkUpDiscoverer_CompactClearsCache(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Rules"), 0o644)
	target := filepath.Join(dir, "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)

	d := NewWalkUpDiscoverer(dir)
	d.Discover(target)
	if d.Cache().Len() == 0 {
		t.Fatal("expected cache to have entries")
	}

	d.HandleCompact(nil, hooks.EventEnvelope{})
	if d.Cache().Len() != 0 {
		t.Error("expected cache to be cleared after compaction")
	}
}

func TestInjectionCache(t *testing.T) {
	c := NewInjectionCache()
	if c.IsSeen("/foo", "hash1") {
		t.Error("should not be seen initially")
	}
	c.Mark("/foo", "hash1")
	if !c.IsSeen("/foo", "hash1") {
		t.Error("should be seen after mark")
	}
	if c.IsSeen("/foo", "hash2") {
		t.Error("different hash should not be seen")
	}
	c.Clear()
	if c.IsSeen("/foo", "hash1") {
		t.Error("should not be seen after clear")
	}
}

func TestDirLevel(t *testing.T) {
	tests := []struct {
		root, dir string
		want      int
	}{
		{"/project", "/project", 0},
		{"/project", "/project/src", 1},
		{"/project", "/project/src/pkg", 2},
	}
	for _, tt := range tests {
		if got := dirLevel(tt.root, tt.dir); got != tt.want {
			t.Errorf("dirLevel(%q, %q) = %d, want %d", tt.root, tt.dir, got, tt.want)
		}
	}
}
