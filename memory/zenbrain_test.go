package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestZenBrain_StoreAndRetrieve(t *testing.T) {
	zb := &ZenBrain{
		path: filepath.Join(t.TempDir(), "zenbrain.json"),
	}

	zb.Store(LayerShortTerm, "user prefers Go over Python", []string{"preference", "language"})
	zb.Store(LayerSemantic, "the API uses REST endpoints", []string{"api", "rest"})
	zb.Store(LayerProcedural, "run go test before committing", []string{"testing", "workflow"})

	if zb.TotalSize() != 3 {
		t.Fatalf("expected 3 total entries, got %d", zb.TotalSize())
	}

	results := zb.Retrieve("Go language preference", nil, 5)
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'Go language preference'")
	}

	// The preference entry should be among results.
	found := false
	for _, r := range results {
		if r.Content == "user prefers Go over Python" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'user prefers Go over Python' in results")
	}
}

func TestZenBrain_RetrieveByLayer(t *testing.T) {
	zb := &ZenBrain{
		path: filepath.Join(t.TempDir(), "zenbrain.json"),
	}

	zb.Store(LayerWorking, "current task is fixing the bug in auth", []string{"auth", "bug"})
	zb.Store(LayerCore, "always use descriptive variable names", []string{"coding", "style"})

	// Search only Core layer.
	results := zb.Retrieve("coding style", []MemoryLayer{LayerCore}, 5)
	if len(results) != 1 {
		t.Fatalf("expected 1 result from Core layer, got %d", len(results))
	}
	if results[0].Layer != LayerCore {
		t.Errorf("expected result from Core layer, got %s", results[0].Layer)
	}
}

func TestZenBrain_Consolidate(t *testing.T) {
	zb := &ZenBrain{
		path: filepath.Join(t.TempDir(), "zenbrain.json"),
	}

	zb.Store(LayerShortTerm, "the user likes dark mode", []string{"preference"})

	// Simulate 3 accesses to trigger promotion.
	zb.mu.Lock()
	if len(zb.layers[LayerShortTerm]) > 0 {
		zb.layers[LayerShortTerm][0].AccessCount = 3
	}
	zb.mu.Unlock()

	zb.Consolidate()

	if zb.LayerSize(LayerShortTerm) != 0 {
		t.Errorf("expected 0 entries in ShortTerm after consolidation, got %d", zb.LayerSize(LayerShortTerm))
	}
	if zb.LayerSize(LayerEpisodic) != 1 {
		t.Errorf("expected 1 entry in Episodic after consolidation, got %d", zb.LayerSize(LayerEpisodic))
	}
}

func TestZenBrain_Sleep(t *testing.T) {
	zb := &ZenBrain{
		path: filepath.Join(t.TempDir(), "zenbrain.json"),
	}

	// Store in Core (should never expire).
	zb.Store(LayerCore, "fundamental preference", []string{"core"})
	// Store in Working (will decay).
	zb.Store(LayerWorking, "temporary context", []string{"temp"})

	// Force low priority on working entry to test decay removal.
	zb.mu.Lock()
	if len(zb.layers[LayerWorking]) > 0 {
		zb.layers[LayerWorking][0].Priority = 0.15
		zb.layers[LayerWorking][0].AccessedAt = zb.layers[LayerWorking][0].AccessedAt.AddDate(0, 0, -30)
	}
	zb.mu.Unlock()

	zb.Sleep()

	// Core entry should survive.
	if zb.LayerSize(LayerCore) != 1 {
		t.Errorf("expected Core entry to survive Sleep, got %d", zb.LayerSize(LayerCore))
	}
}

func TestZenBrain_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zenbrain.json")

	zb := &ZenBrain{path: path}
	zb.Store(LayerSemantic, "Go is statically typed", []string{"go", "types"})
	zb.Store(LayerCore, "prefer simplicity", []string{"principles"})

	if err := zb.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("zenbrain file not created: %v", err)
	}

	zb2 := &ZenBrain{path: path}
	if err := zb2.Load(); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if zb2.TotalSize() != 2 {
		t.Fatalf("expected 2 total entries after load, got %d", zb2.TotalSize())
	}
	if zb2.LayerSize(LayerSemantic) != 1 {
		t.Errorf("expected 1 Semantic entry after load, got %d", zb2.LayerSize(LayerSemantic))
	}
	if zb2.LayerSize(LayerCore) != 1 {
		t.Errorf("expected 1 Core entry after load, got %d", zb2.LayerSize(LayerCore))
	}
}
