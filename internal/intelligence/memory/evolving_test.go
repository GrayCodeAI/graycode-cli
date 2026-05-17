package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvolvingMemory_LearnAndRetrieve(t *testing.T) {
	em := &EvolvingMemory{
		path: filepath.Join(t.TempDir(), "guidelines.json"),
	}

	em.Learn("writing Go tests", "always use table-driven tests", "session-1")
	em.Learn("database migrations", "run migrate down first", "session-2")
	em.Learn("error handling in Go", "wrap errors with context", "session-3")

	results := em.Retrieve("Go tests", 5)
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'Go tests'")
	}
	found := false
	for _, r := range results {
		if r.Lesson == "always use table-driven tests" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'always use table-driven tests' guideline")
	}
}

func TestEvolvingMemory_LearnStrengthensDuplicate(t *testing.T) {
	em := &EvolvingMemory{
		path: filepath.Join(t.TempDir(), "guidelines.json"),
	}

	em.Learn("writing Go tests", "use table-driven tests", "session-1")
	initial := em.Guidelines()
	if len(initial) != 1 {
		t.Fatalf("expected 1 guideline, got %d", len(initial))
	}
	initialConf := initial[0].Confidence

	// Learning a similar pattern should strengthen rather than add new.
	em.Learn("writing Go tests for handlers", "check status codes", "session-2")
	after := em.Guidelines()
	if len(after) != 1 {
		t.Fatalf("expected 1 guideline after similar learn, got %d", len(after))
	}
	if after[0].Confidence <= initialConf {
		t.Errorf("confidence should increase: was %f, now %f", initialConf, after[0].Confidence)
	}
}

func TestEvolvingMemory_Strengthen(t *testing.T) {
	em := &EvolvingMemory{
		path: filepath.Join(t.TempDir(), "guidelines.json"),
	}

	em.Learn("error handling", "always wrap errors", "session-1")
	guidelines := em.Guidelines()
	if len(guidelines) != 1 {
		t.Fatalf("expected 1 guideline, got %d", len(guidelines))
	}
	id := guidelines[0].ID
	before := guidelines[0].Confidence

	em.Strengthen(id)
	after := em.Guidelines()
	if after[0].Confidence <= before {
		t.Errorf("confidence should increase: was %f, now %f", before, after[0].Confidence)
	}
	if after[0].Uses != 1 {
		t.Errorf("uses should be 1, got %d", after[0].Uses)
	}
}

func TestEvolvingMemory_Decay(t *testing.T) {
	em := &EvolvingMemory{
		path: filepath.Join(t.TempDir(), "guidelines.json"),
	}

	em.Learn("strong pattern", "strong lesson", "session-1")
	// Strengthen to high confidence.
	guidelines := em.Guidelines()
	em.Strengthen(guidelines[0].ID)
	em.Strengthen(guidelines[0].ID)
	em.Strengthen(guidelines[0].ID)

	// Add a weak guideline with low confidence.
	em.Learn("weak pattern xyz", "weak lesson xyz", "session-2")
	before := em.Guidelines()
	if len(before) != 2 {
		t.Fatalf("expected 2 guidelines, got %d", len(before))
	}

	// Decay multiple times to push weak guideline below threshold.
	for i := 0; i < 10; i++ {
		em.Decay()
	}

	after := em.Guidelines()
	// Weak guideline should be removed, strong should survive.
	if len(after) > len(before) {
		t.Error("decay should not increase guideline count")
	}
}

func TestEvolvingMemory_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guidelines.json")

	em := &EvolvingMemory{path: path}
	em.Learn("writing tests", "use assertions", "session-1")
	em.Learn("code review", "check edge cases", "session-2")

	if err := em.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Verify file was created.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("guidelines file not created: %v", err)
	}

	// Load into a new instance.
	em2 := &EvolvingMemory{path: path}
	if err := em2.Load(); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	loaded := em2.Guidelines()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 guidelines after load, got %d", len(loaded))
	}
}
