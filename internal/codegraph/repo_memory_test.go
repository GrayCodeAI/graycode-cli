package codegraph

import (
	"testing"
)

// memoryStore is a simple in-memory implementation of MemoryStore for testing.
type memoryStore struct {
	data map[string][]byte
}

func newMemoryStore() *memoryStore {
	return &memoryStore{data: make(map[string][]byte)}
}

func (m *memoryStore) Save(key string, value []byte) error {
	m.data[key] = value
	return nil
}

func (m *memoryStore) Load(key string) ([]byte, error) {
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (m *memoryStore) List(prefix string) ([]string, error) {
	var keys []string
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (m *memoryStore) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func TestNewRepoMemory(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	rm := NewRepoMemory(store)
	if rm == nil {
		t.Fatal("NewRepoMemory returned nil")
	}
}

func TestRepoMemory_SaveIssue(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	rm := NewRepoMemory(store)

	mem := IssueMemory{
		IssueID:     "ISSUE-123",
		Title:       "Database connection timeout",
		Description: "Connection pool exhausted under load",
		RootCause:   "Missing connection pool size configuration",
		FixPattern:  "Add pool size to config",
		Tags:        []string{"bug", "database"},
	}

	err := rm.SaveIssue(mem)
	if err != nil {
		t.Fatalf("SaveIssue failed: %v", err)
	}

	// Verify it was saved
	keys, _ := store.List("issue:")
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
}

func TestRepoMemory_FindSimilarIssues(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	rm := NewRepoMemory(store)

	rm.SaveIssue(IssueMemory{
		IssueID:     "I1",
		Title:       "Database connection timeout error",
		Description: "Connection pool exhausted",
	})
	rm.SaveIssue(IssueMemory{
		IssueID:     "I2",
		Title:       "UI rendering glitch",
		Description: "CSS layout broken on mobile",
	})

	results, err := rm.FindSimilarIssues("database connection error", 5)
	if err != nil {
		t.Fatalf("FindSimilarIssues failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 similar issue")
	}

	// The database issue should match
	found := false
	for _, r := range results {
		if r.IssueID == "I1" {
			found = true
		}
	}
	if !found {
		t.Error("expected database issue to be found as similar")
	}
}

func TestRepoMemory_FindSimilarIssues_Limit(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	rm := NewRepoMemory(store)

	for i := 0; i < 10; i++ {
		rm.SaveIssue(IssueMemory{
			IssueID: "I" + string(rune(i+'0')),
			Title:   "database timeout issue",
		})
	}

	results, err := rm.FindSimilarIssues("database timeout", 3)
	if err != nil {
		t.Fatalf("FindSimilarIssues failed: %v", err)
	}
	if len(results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(results))
	}
}

func TestRepoMemory_SavePattern(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	rm := NewRepoMemory(store)

	pattern := CodePattern{
		PatternID:   "PAT-001",
		Name:        "Error wrapping pattern",
		Description: "Always wrap errors with context",
		WhenToUse:   "When returning errors from functions",
		Tags:        []string{"error", "convention"},
	}

	err := rm.SavePattern(pattern)
	if err != nil {
		t.Fatalf("SavePattern failed: %v", err)
	}

	keys, _ := store.List("pattern:")
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
}

func TestRepoMemory_FindRelevantPatterns(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	rm := NewRepoMemory(store)

	rm.SavePattern(CodePattern{
		PatternID:   "P1",
		Name:        "Error handling pattern",
		Description: "Functions that handle errors",
		WhenToUse:   "When processing operations that can fail",
	})
	rm.SavePattern(CodePattern{
		PatternID:   "P2",
		Name:        "Factory pattern",
		Description: "Constructor functions for creating instances",
		WhenToUse:   "When creating new objects",
	})

	results, err := rm.FindRelevantPatterns("error handling in functions", 5)
	if err != nil {
		t.Fatalf("FindRelevantPatterns failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 relevant pattern")
	}
}

func TestRepoMemory_BuildContextFromMemory(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	rm := NewRepoMemory(store)

	rm.SaveIssue(IssueMemory{
		IssueID:    "I1",
		Title:      "Database connection timeout",
		RootCause:  "Pool size too small",
		FixPattern: "Increase pool size",
	})
	rm.SavePattern(CodePattern{
		PatternID: "P1",
		Name:      "Connection pooling pattern",
		WhenToUse: "When managing database connections",
	})

	ctx := rm.BuildContextFromMemory("database connection handling")
	if ctx == "" {
		t.Error("expected non-empty context")
	}
}

func TestRepoMemory_BuildContextFromMemory_Empty(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	rm := NewRepoMemory(store)

	ctx := rm.BuildContextFromMemory("unrelated query about UI")
	if ctx != "" {
		t.Errorf("expected empty context when no matches, got %q", ctx)
	}
}

func TestExtractPatternsFromCode(t *testing.T) {
	t.Parallel()
	nodes := []Node{
		{Kind: "function", Name: "HandleError", FilePath: "err.go"},
		{Kind: "function", Name: "NewServer", FilePath: "server.go"},
		{Kind: "function", Name: "NewClient", FilePath: "client.go"},
		{Kind: "interface", Name: "Reader", FilePath: "io.go"},
	}

	patterns := ExtractPatternsFromCode(nodes)

	patternMap := make(map[string]*CodePattern)
	for i := range patterns {
		patternMap[patterns[i].PatternID] = &patterns[i]
	}

	if ep, ok := patternMap["error_handling"]; ok {
		if ep.Frequency != 1 {
			t.Errorf("expected 1 error handling function, got %d", ep.Frequency)
		}
	} else {
		t.Error("expected error_handling pattern")
	}

	if fp, ok := patternMap["factory"]; ok {
		if fp.Frequency != 2 {
			t.Errorf("expected 2 factory functions, got %d", fp.Frequency)
		}
	} else {
		t.Error("expected factory pattern")
	}

	if _, ok := patternMap["interface"]; !ok {
		t.Error("expected interface pattern")
	}
}

func TestExtractPatternsFromCode_Empty(t *testing.T) {
	t.Parallel()
	patterns := ExtractPatternsFromCode(nil)
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns for nil input, got %d", len(patterns))
	}
}

func TestIsSimilar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b     string
		expected bool
	}{
		{"database connection timeout", "database connection error", true},
		{"completely different topic", "something unrelated", false},
		{"", "", false},
		{"short", "also short but different", false},
	}

	for _, tt := range tests {
		got := isSimilar(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("isSimilar(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestTokenizeWords(t *testing.T) {
	t.Parallel()
	words := tokenizeWords("Fix the authentication-token bug")
	if len(words) == 0 {
		t.Fatal("expected non-empty tokenization")
	}
	for _, w := range words {
		if len(w) <= 2 {
			t.Errorf("word %q should be > 2 chars", w)
		}
	}
}

func TestContainsString(t *testing.T) {
	t.Parallel()
	if !containsString("HandleError", "Error") {
		t.Error("expected HandleError to contain Error")
	}
	if containsString("Process", "Error") {
		t.Error("Process should not contain Error")
	}
	if !containsString("abc", "abc") {
		t.Error("exact match should return true")
	}
	if containsString("ab", "abc") {
		t.Error("shorter string should not contain longer")
	}
}
