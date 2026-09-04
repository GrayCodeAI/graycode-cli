package memory

import (
	"os"
	"testing"
)

func TestEvolvingMemory_FullLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.graycode", 0o755)

	em := NewEvolvingMemory()
	if em == nil {
		t.Fatal("nil")
	}

	em.Learn("error handling", "always wrap with context", "session-1")
	em.Learn("testing", "use table-driven tests", "session-2")
	em.Learn("error handling", "use fmt.Errorf with %w", "session-3")

	guidelines := em.Guidelines()
	if len(guidelines) == 0 {
		t.Error("should have guidelines after learning")
	}
}

func TestEvolvingMemory_Retrieve(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.graycode", 0o755)

	em := NewEvolvingMemory()
	em.Learn("concurrency", "use errgroup for parallel ops", "s1")
	em.Learn("logging", "use slog not fmt.Println", "s2")

	results := em.Retrieve("goroutine concurrency", 5)
	_ = results
}

func TestEvolvingMemory_StrengthenGuideline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.graycode", 0o755)

	em := NewEvolvingMemory()
	em.Learn("pattern", "lesson", "src")

	guidelines := em.Guidelines()
	if len(guidelines) > 0 {
		em.Strengthen(guidelines[0].ID)
	}
}

func TestEvolvingMemory_DecayAll(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.graycode", 0o755)

	em := NewEvolvingMemory()
	em.Learn("old pattern", "old lesson", "old-session")
	em.Decay()
}

func TestEvolvingMemory_Format(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.graycode", 0o755)

	em := NewEvolvingMemory()
	em.Learn("format test", "should appear in output", "src")

	formatted := em.Format(5)
	_ = formatted
}

func TestEvolvingMemory_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.graycode", 0o755)

	em := NewEvolvingMemory()
	em.Learn("persist", "this should persist", "src")
	if err := em.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	em2 := NewEvolvingMemory()
	if err := em2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestKeywordOverlap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b string
		pos  bool
	}{
		{"error handling golang", "golang error wrap", true},
		{"completely different", "nothing in common xyz", false},
		{"", "", false},
		{"same same same", "same same same", true},
	}
	for _, tt := range tests {
		score := keywordOverlap(tt.a, tt.b)
		if tt.pos && score <= 0 {
			t.Errorf("keywordOverlap(%q, %q) = %f, want > 0", tt.a, tt.b, score)
		}
		if !tt.pos && score > 0 {
			t.Errorf("keywordOverlap(%q, %q) = %f, want 0", tt.a, tt.b, score)
		}
	}
}

func TestProactiveContext_TrackFiles(t *testing.T) {
	pc := NewProactiveContext(nil)
	if pc == nil {
		t.Fatal("nil")
	}
	pc.TrackFile("main.go")
	pc.TrackFiles([]string{"config.go", "handler.go"})
	pc.Reset()
}
