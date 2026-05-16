package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewKnowledgeBase(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")
	if kb == nil {
		t.Fatal("expected non-nil KnowledgeBase")
	}
	if kb.Dir != "/tmp/test-kb" {
		t.Errorf("expected Dir=/tmp/test-kb, got %s", kb.Dir)
	}
	if kb.Entries == nil {
		t.Error("expected non-nil Entries map")
	}
	if kb.Categories == nil {
		t.Error("expected non-nil Categories map")
	}
}

func TestKnowledgeAdd(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")

	// Test nil entry
	if err := kb.Add(nil); err == nil {
		t.Error("expected error for nil entry")
	}

	// Test missing ID
	if err := kb.Add(&KnowledgeEntry{Category: "pattern"}); err == nil {
		t.Error("expected error for empty ID")
	}

	// Test missing category
	if err := kb.Add(&KnowledgeEntry{ID: "test-1"}); err == nil {
		t.Error("expected error for empty category")
	}

	// Test invalid category
	if err := kb.Add(&KnowledgeEntry{ID: "test-1", Category: "invalid"}); err == nil {
		t.Error("expected error for invalid category")
	}

	// Test valid entry
	entry := &KnowledgeEntry{
		ID:         "test-1",
		Title:      "Test Pattern",
		Content:    "Always check error returns in Go",
		Category:   "pattern",
		Language:   "go",
		Tags:       []string{"error-handling"},
		Confidence: 0.9,
	}
	if err := kb.Add(entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, exists := kb.Entries["test-1"]; !exists {
		t.Error("entry not found in Entries map")
	}

	ids := kb.Categories["pattern"]
	if len(ids) != 1 || ids[0] != "test-1" {
		t.Errorf("expected category index to contain test-1, got %v", ids)
	}

	// Test that CreatedAt is set
	if kb.Entries["test-1"].CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Test adding same ID again (should overwrite)
	entry2 := &KnowledgeEntry{
		ID:       "test-1",
		Title:    "Updated Pattern",
		Content:  "Updated content",
		Category: "pattern",
	}
	if err := kb.Add(entry2); err != nil {
		t.Fatalf("unexpected error on overwrite: %v", err)
	}
	if kb.Entries["test-1"].Title != "Updated Pattern" {
		t.Error("expected entry to be overwritten")
	}

	// Test all valid categories
	validCats := []string{"pattern", "anti-pattern", "convention", "shortcut", "gotcha"}
	for i, cat := range validCats {
		e := &KnowledgeEntry{
			ID:         fmt.Sprintf("cat-%d", i),
			Category:   cat,
			Confidence: 0.5,
		}
		if err := kb.Add(e); err != nil {
			t.Errorf("unexpected error for category %s: %v", cat, err)
		}
	}
}

func TestKnowledgeSearch(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")

	entries := []*KnowledgeEntry{
		{
			ID:         "go-err",
			Title:      "Go error handling pattern",
			Content:    "Always check error returns in Go functions",
			Category:   "pattern",
			Language:   "go",
			Tags:       []string{"error", "golang"},
			Confidence: 0.9,
			UsageCount: 5,
			LastUsed:   time.Now().Add(-1 * time.Hour),
		},
		{
			ID:         "py-except",
			Title:      "Python exception handling",
			Content:    "Use specific exception types rather than bare except",
			Category:   "pattern",
			Language:   "python",
			Tags:       []string{"error", "python"},
			Confidence: 0.85,
			UsageCount: 3,
		},
		{
			ID:         "go-defer",
			Title:      "Go defer for cleanup",
			Content:    "Use defer for resource cleanup in Go",
			Category:   "convention",
			Language:   "go",
			Tags:       []string{"cleanup", "golang"},
			Confidence: 0.95,
			UsageCount: 10,
			LastUsed:   time.Now().Add(-30 * time.Minute),
		},
	}

	for _, e := range entries {
		if err := kb.Add(e); err != nil {
			t.Fatalf("failed to add entry: %v", err)
		}
	}

	// Test empty query
	results := kb.Search("", 5)
	if len(results) != 0 {
		t.Error("expected no results for empty query")
	}

	// Test zero limit
	results = kb.Search("go", 0)
	if len(results) != 0 {
		t.Error("expected no results for zero limit")
	}

	// Test query matching
	results = kb.Search("go error", 5)
	if len(results) == 0 {
		t.Fatal("expected results for 'go error'")
	}
	// Go error handling should be first (matches title, content, language, tags)
	if results[0].ID != "go-err" {
		t.Errorf("expected go-err first, got %s", results[0].ID)
	}

	// Test limit
	results = kb.Search("go", 1)
	if len(results) != 1 {
		t.Errorf("expected 1 result with limit=1, got %d", len(results))
	}

	// Test query that matches nothing
	results = kb.Search("javascript react hooks", 5)
	if len(results) != 0 {
		t.Errorf("expected no results for unrelated query, got %d", len(results))
	}
}

func TestKnowledgeGetByCategory(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")

	_ = kb.Add(&KnowledgeEntry{ID: "p1", Category: "pattern", Title: "Pattern 1", Confidence: 0.8})
	_ = kb.Add(&KnowledgeEntry{ID: "p2", Category: "pattern", Title: "Pattern 2", Confidence: 0.8})
	_ = kb.Add(&KnowledgeEntry{ID: "c1", Category: "convention", Title: "Convention 1", Confidence: 0.8})

	patterns := kb.GetByCategory("pattern")
	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(patterns))
	}

	conventions := kb.GetByCategory("convention")
	if len(conventions) != 1 {
		t.Errorf("expected 1 convention, got %d", len(conventions))
	}

	empty := kb.GetByCategory("nonexistent")
	if len(empty) != 0 {
		t.Errorf("expected 0 results for nonexistent category, got %d", len(empty))
	}
}

func TestExtractFromSession(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")

	// Test empty messages
	results := kb.ExtractFromSession(nil, "success")
	if results != nil {
		t.Error("expected nil for empty messages")
	}

	// Test correction extraction
	messages := []string{
		"Let me try this approach",
		"Actually, that was wrong. You should use defer instead of manual cleanup.",
		"Got it, thanks",
	}
	results = kb.ExtractFromSession(messages, "completed")
	found := false
	for _, r := range results {
		if r.Category == "gotcha" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to extract a 'gotcha' entry from correction")
	}

	// Test discovery extraction
	messages = []string{
		"Investigating the issue",
		"I found that the API requires authentication headers",
		"Fixing now",
	}
	results = kb.ExtractFromSession(messages, "success")
	found = false
	for _, r := range results {
		if r.Category == "pattern" {
			found = true
			if r.Confidence <= 0.5 {
				t.Error("expected confidence boost from successful outcome")
			}
			break
		}
	}
	if !found {
		t.Error("expected to extract a 'pattern' entry from discovery")
	}

	// Test convention extraction
	messages = []string{
		"Convention: always use snake_case for database columns",
	}
	results = kb.ExtractFromSession(messages, "neutral")
	found = false
	for _, r := range results {
		if r.Category == "convention" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to extract a 'convention' entry")
	}

	// Test shortcut extraction
	messages = []string{
		"Here's a shortcut for running tests faster: use -short flag",
	}
	results = kb.ExtractFromSession(messages, "neutral")
	found = false
	for _, r := range results {
		if r.Category == "shortcut" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to extract a 'shortcut' entry")
	}
}

func TestBuildContextForTask(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")

	// Test empty task
	result := kb.BuildContextForTask("", 100)
	if result != "" {
		t.Error("expected empty result for empty task")
	}

	// Test zero tokens
	result = kb.BuildContextForTask("something", 0)
	if result != "" {
		t.Error("expected empty result for zero tokens")
	}

	// Add entries
	_ = kb.Add(&KnowledgeEntry{
		ID:         "go-err",
		Title:      "Error handling in Go",
		Content:    "Always check error returns",
		Category:   "pattern",
		Language:   "go",
		Tags:       []string{"error"},
		Confidence: 0.9,
	})

	// Test with matching task
	result = kb.BuildContextForTask("handle errors in go", 500)
	if result == "" {
		t.Error("expected non-empty context for matching task")
	}
	if !strings.Contains(result, "Relevant Knowledge") {
		t.Error("expected header in context")
	}
	if !strings.Contains(result, "Error handling") {
		t.Error("expected entry content in context")
	}

	// Verify usage count was incremented
	if kb.Entries["go-err"].UsageCount != 1 {
		t.Errorf("expected usage count 1, got %d", kb.Entries["go-err"].UsageCount)
	}

	// Test token budget limit (very small budget)
	result = kb.BuildContextForTask("handle errors in go", 5)
	// With only ~20 chars budget, might be empty or very short
	if len(result) > 40 {
		t.Errorf("expected result to respect token budget, got len=%d", len(result))
	}
}

func TestMerge(t *testing.T) {
	kb1 := NewKnowledgeBase("/tmp/test-kb1")
	kb2 := NewKnowledgeBase("/tmp/test-kb2")

	_ = kb1.Add(&KnowledgeEntry{
		ID:         "entry-1",
		Title:      "Pattern One",
		Content:    "First pattern content here",
		Category:   "pattern",
		Confidence: 0.8,
		UsageCount: 5,
	})

	_ = kb2.Add(&KnowledgeEntry{
		ID:         "entry-2",
		Title:      "Pattern Two",
		Content:    "Second pattern content here",
		Category:   "convention",
		Confidence: 0.7,
		UsageCount: 3,
	})

	// Add duplicate with different stats
	_ = kb2.Add(&KnowledgeEntry{
		ID:         "entry-1-dup",
		Title:      "Pattern One",
		Content:    "First pattern content here",
		Category:   "pattern",
		Confidence: 0.9,
		UsageCount: 2,
		LastUsed:   time.Now(),
	})

	kb1.Merge(kb2)

	// entry-2 should be merged in
	if _, exists := kb1.Entries["entry-2"]; !exists {
		t.Error("expected entry-2 to be merged")
	}

	// duplicate should not create new entry but boost stats
	if _, exists := kb1.Entries["entry-1-dup"]; exists {
		t.Error("duplicate should not be added as new entry")
	}

	// Original should have boosted confidence and usage
	original := kb1.Entries["entry-1"]
	if original.Confidence != 0.9 {
		t.Errorf("expected confidence boosted to 0.9, got %f", original.Confidence)
	}
	if original.UsageCount != 7 {
		t.Errorf("expected usage count 7, got %d", original.UsageCount)
	}

	// Test nil merge
	kb1.Merge(nil) // should not panic
}

func TestKnowledgePrune(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")

	now := time.Now()
	_ = kb.Add(&KnowledgeEntry{
		ID:         "high-conf",
		Title:      "High confidence",
		Category:   "pattern",
		Confidence: 0.9,
		CreatedAt:  now,
	})
	_ = kb.Add(&KnowledgeEntry{
		ID:         "low-conf",
		Title:      "Low confidence",
		Category:   "pattern",
		Confidence: 0.2,
		CreatedAt:  now,
	})
	_ = kb.Add(&KnowledgeEntry{
		ID:         "old-entry",
		Title:      "Old entry",
		Category:   "convention",
		Confidence: 0.8,
		CreatedAt:  now.Add(-100 * 24 * time.Hour), // 100 days old
	})

	// Prune low confidence
	kb.Prune(0.5, 0)
	if _, exists := kb.Entries["low-conf"]; exists {
		t.Error("expected low-conf to be pruned")
	}
	if _, exists := kb.Entries["high-conf"]; !exists {
		t.Error("expected high-conf to remain")
	}
	if _, exists := kb.Entries["old-entry"]; !exists {
		t.Error("expected old-entry to remain (no maxAge set)")
	}

	// Prune by age
	kb.Prune(0.0, 30*24*time.Hour) // 30 days
	if _, exists := kb.Entries["old-entry"]; exists {
		t.Error("expected old-entry to be pruned by age")
	}
	if _, exists := kb.Entries["high-conf"]; !exists {
		t.Error("expected high-conf to remain after age prune")
	}

	// Check category index is cleaned up
	patternIDs := kb.Categories["pattern"]
	for _, id := range patternIDs {
		if id == "low-conf" {
			t.Error("expected low-conf removed from category index")
		}
	}
}

func TestKnowledgeSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	kb := NewKnowledgeBase(dir)

	_ = kb.Add(&KnowledgeEntry{
		ID:         "save-test-1",
		Title:      "Save Test",
		Content:    "Testing persistence",
		Category:   "pattern",
		Language:   "go",
		Tags:       []string{"test"},
		Examples:   []string{"example 1"},
		Confidence: 0.85,
		UsageCount: 3,
		LastUsed:   time.Now().Truncate(time.Second),
		CreatedAt:  time.Now().Truncate(time.Second),
		Source:     "test",
	})

	// Test save
	if err := kb.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "knowledge.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected knowledge.json to exist")
	}

	// Test load into new KB
	kb2 := NewKnowledgeBase(dir)
	if err := kb2.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(kb2.Entries) != 1 {
		t.Fatalf("expected 1 entry after load, got %d", len(kb2.Entries))
	}

	loaded := kb2.Entries["save-test-1"]
	if loaded == nil {
		t.Fatal("expected save-test-1 to be loaded")
	}
	if loaded.Title != "Save Test" {
		t.Errorf("expected title 'Save Test', got %q", loaded.Title)
	}
	if loaded.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", loaded.Confidence)
	}
	if loaded.Language != "go" {
		t.Errorf("expected language 'go', got %q", loaded.Language)
	}

	// Test load from nonexistent dir (should not error)
	kb3 := NewKnowledgeBase("/tmp/nonexistent-kb-dir-xyz")
	if err := kb3.Load(); err != nil {
		t.Errorf("load from nonexistent should not error: %v", err)
	}

	// Test save with empty dir
	kb4 := NewKnowledgeBase("")
	if err := kb4.Save(); err == nil {
		t.Error("expected error saving with empty dir")
	}
	if err := kb4.Load(); err == nil {
		t.Error("expected error loading with empty dir")
	}
}

func TestKnowledgeStats(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")

	_ = kb.Add(&KnowledgeEntry{ID: "p1", Category: "pattern", Language: "go", Confidence: 0.8, UsageCount: 10})
	_ = kb.Add(&KnowledgeEntry{ID: "p2", Category: "pattern", Language: "go", Confidence: 0.6, UsageCount: 5})
	_ = kb.Add(&KnowledgeEntry{ID: "c1", Category: "convention", Language: "python", Confidence: 0.9, UsageCount: 0})
	_ = kb.Add(&KnowledgeEntry{ID: "g1", Category: "gotcha", Language: "go", Confidence: 0.7, UsageCount: 3})

	stats := kb.Stats()

	if stats.TotalEntries != 4 {
		t.Errorf("expected 4 total entries, got %d", stats.TotalEntries)
	}

	if stats.ByCategory["pattern"] != 2 {
		t.Errorf("expected 2 patterns, got %d", stats.ByCategory["pattern"])
	}
	if stats.ByCategory["convention"] != 1 {
		t.Errorf("expected 1 convention, got %d", stats.ByCategory["convention"])
	}
	if stats.ByCategory["gotcha"] != 1 {
		t.Errorf("expected 1 gotcha, got %d", stats.ByCategory["gotcha"])
	}

	if stats.ByLanguage["go"] != 3 {
		t.Errorf("expected 3 go entries, got %d", stats.ByLanguage["go"])
	}
	if stats.ByLanguage["python"] != 1 {
		t.Errorf("expected 1 python entry, got %d", stats.ByLanguage["python"])
	}

	expectedAvg := (0.8 + 0.6 + 0.9 + 0.7) / 4.0
	if absFloat(stats.AvgConfidence-expectedAvg) > 0.001 {
		t.Errorf("expected avg confidence %f, got %f", expectedAvg, stats.AvgConfidence)
	}

	// MostUsed should have entries with count > 0
	if len(stats.MostUsed) != 3 {
		t.Errorf("expected 3 entries in MostUsed (those with count>0), got %d", len(stats.MostUsed))
	}
	if stats.MostUsed[0] != "p1" {
		t.Errorf("expected p1 as most used, got %s", stats.MostUsed[0])
	}
}

func TestFormatEntry(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")

	// Test nil entry
	result := kb.FormatEntry(nil)
	if result != "" {
		t.Error("expected empty string for nil entry")
	}

	entry := &KnowledgeEntry{
		ID:       "fmt-test",
		Title:    "Error Handling",
		Content:  "Always check errors",
		Category: "pattern",
		Language: "go",
		Tags:     []string{"error", "best-practice"},
		Examples: []string{"if err != nil { return err }"},
	}

	result = kb.FormatEntry(entry)

	if !strings.Contains(result, "## Error Handling [pattern]") {
		t.Error("expected title with category in output")
	}
	if !strings.Contains(result, "Language: go") {
		t.Error("expected language in output")
	}
	if !strings.Contains(result, "Always check errors") {
		t.Error("expected content in output")
	}
	if !strings.Contains(result, "if err != nil") {
		t.Error("expected examples in output")
	}
	if !strings.Contains(result, "error, best-practice") {
		t.Error("expected tags in output")
	}

	// Test entry without optional fields
	minimal := &KnowledgeEntry{
		ID:       "min",
		Title:    "Minimal",
		Content:  "Just content",
		Category: "shortcut",
	}
	result = kb.FormatEntry(minimal)
	if !strings.Contains(result, "## Minimal [shortcut]") {
		t.Error("expected minimal entry formatted correctly")
	}
	if strings.Contains(result, "Language:") {
		t.Error("should not contain Language for empty language")
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		msg      string
		fallback string
		want     string
	}{
		{"", "Fallback", "Fallback"},
		{"Short message", "F", "Short message"},
		{"First sentence. Second sentence.", "F", "First sentence"},
		{"A very long message that exceeds eighty characters in length and should be truncated to fit properly", "F", "A very long message that exceeds eighty characters in length and should be tru..."},
	}

	for _, tt := range tests {
		got := extractTitle(tt.msg, tt.fallback)
		if got != tt.want {
			t.Errorf("extractTitle(%q, %q) = %q, want %q", tt.msg, tt.fallback, got, tt.want)
		}
	}
}

func TestKnowledgeConcurrentAccess(t *testing.T) {
	kb := NewKnowledgeBase("/tmp/test-kb")

	// Add some initial entries
	for i := 0; i < 10; i++ {
		_ = kb.Add(&KnowledgeEntry{
			ID:         fmt.Sprintf("concurrent-%d", i),
			Title:      fmt.Sprintf("Entry %d", i),
			Content:    fmt.Sprintf("Content for entry %d with some words", i),
			Category:   "pattern",
			Confidence: 0.8,
		})
	}

	done := make(chan bool, 4)

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			kb.Search("entry content", 5)
		}
		done <- true
	}()

	// Concurrent writes
	go func() {
		for i := 10; i < 50; i++ {
			_ = kb.Add(&KnowledgeEntry{
				ID:         fmt.Sprintf("concurrent-%d", i),
				Title:      fmt.Sprintf("Entry %d", i),
				Content:    "More content",
				Category:   "convention",
				Confidence: 0.7,
			})
		}
		done <- true
	}()

	// Concurrent stats
	go func() {
		for i := 0; i < 100; i++ {
			kb.Stats()
		}
		done <- true
	}()

	// Concurrent category reads
	go func() {
		for i := 0; i < 100; i++ {
			kb.GetByCategory("pattern")
		}
		done <- true
	}()

	for i := 0; i < 4; i++ {
		<-done
	}
}

// absFloat returns the absolute value of a float64.
func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
