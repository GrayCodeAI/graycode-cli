package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHintsLoader_LoadHints(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".graycodehints"), []byte("Use Go idioms\nPrefer table-driven tests"), 0o644)

	h := NewHintsLoader()
	hints := h.LoadHints(dir)
	if hints == "" {
		t.Error("expected hints content")
	}
	if !hasSubstr(hints, "Go idioms") {
		t.Error("expected hint content in output")
	}

	// Second call should return empty (already loaded)
	hints2 := h.LoadHints(dir)
	if hints2 != "" {
		t.Error("expected empty on second load (already loaded)")
	}
}

func TestHintsLoader_Reset(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".graycodehints"), []byte("hint"), 0o644)

	h := NewHintsLoader()
	h.LoadHints(dir)
	h.Reset()

	hints := h.LoadHints(dir)
	if hints == "" {
		t.Error("expected hints after reset")
	}
}

func TestSourceRoots(t *testing.T) {
	sr := NewSourceRoots()
	sr.Mark("/project/src", 42)

	if !sr.IsExplored("/project/src") {
		t.Error("expected explored")
	}
	if sr.IsExplored("/project/tests") {
		t.Error("expected not explored")
	}
	if len(sr.List()) != 1 {
		t.Errorf("expected 1 root, got %d", len(sr.List()))
	}

	sr.Invalidate("/project/src")
	if sr.IsExplored("/project/src") {
		t.Error("expected not explored after invalidate")
	}
}

func TestSourceRoots_Stale(t *testing.T) {
	sr := NewSourceRoots()
	sr.Mark("/old", 10)
	// Manually backdate
	sr.roots["/old"].ExploredAt = time.Now().Add(-2 * time.Hour)

	stale := sr.Stale(1 * time.Hour)
	if len(stale) != 1 {
		t.Errorf("expected 1 stale root, got %d", len(stale))
	}
}

func TestToolInspector(t *testing.T) {
	ti := NewToolInspector()

	// Safe tool
	r := ti.Inspect("Read", nil)
	if !r.ShouldExecute() {
		t.Error("Read should auto-execute")
	}
	if r.Confidence < 0.9 {
		t.Errorf("expected high confidence for Read, got %f", r.Confidence)
	}

	// Dangerous bash
	r = ti.Inspect("Bash", map[string]interface{}{"command": "rm -rf /"})
	if r.ShouldExecute() {
		t.Error("rm -rf should NOT auto-execute")
	}
	if r.Action != ActionRequireApproval {
		t.Errorf("expected RequireApproval, got %d", r.Action)
	}

	// Safe bash
	r = ti.Inspect("Bash", map[string]interface{}{"command": "go test ./..."})
	if !r.ShouldExecute() {
		t.Error("go test should auto-execute")
	}
}
