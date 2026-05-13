package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewMemoryConsolidator(t *testing.T) {
	mc := NewMemoryConsolidator("/tmp/test-mem")
	if mc == nil {
		t.Fatal("expected non-nil consolidator")
	}
	if mc.Dir != "/tmp/test-mem" {
		t.Errorf("expected dir /tmp/test-mem, got %s", mc.Dir)
	}
	if len(mc.RawMemories) != 0 {
		t.Errorf("expected empty raw memories, got %d", len(mc.RawMemories))
	}
	if len(mc.ConsolidatedMemories) != 0 {
		t.Errorf("expected empty consolidated memories, got %d", len(mc.ConsolidatedMemories))
	}
}

func TestIngest(t *testing.T) {
	mc := NewMemoryConsolidator("")

	mc.Ingest("the project uses Go modules", "session", "sess-1")
	mc.Ingest("always run tests with -race", "user_feedback", "sess-1")
	mc.Ingest("file created: main.go", "tool_output", "sess-2")

	if len(mc.RawMemories) != 3 {
		t.Fatalf("expected 3 raw memories, got %d", len(mc.RawMemories))
	}

	rm := mc.RawMemories[0]
	if rm.Content != "the project uses Go modules" {
		t.Errorf("unexpected content: %s", rm.Content)
	}
	if rm.Source != "session" {
		t.Errorf("unexpected source: %s", rm.Source)
	}
	if rm.SessionID != "sess-1" {
		t.Errorf("unexpected session id: %s", rm.SessionID)
	}
	if rm.Processed {
		t.Error("expected processed=false")
	}
}

func TestExtractFacts(t *testing.T) {
	mc := NewMemoryConsolidator("")

	raw := []RawMemory{
		{Content: "the project uses Go modules", Source: "session", SessionID: "s1"},
		{Content: "the server is running on port 8080", Source: "session", SessionID: "s1"},
		{Content: "hello world", Source: "session", SessionID: "s1"},
		{Content: "the app has a REST API", Source: "tool_output", SessionID: "s2"},
	}

	facts := mc.ExtractFacts(raw)

	if len(facts) != 3 {
		t.Fatalf("expected 3 facts, got %d", len(facts))
	}

	for _, f := range facts {
		if f.Category != "fact" {
			t.Errorf("expected category 'fact', got %s", f.Category)
		}
		if f.Confidence <= 0 {
			t.Error("expected positive confidence")
		}
	}
}

func TestExtractConventions(t *testing.T) {
	mc := NewMemoryConsolidator("")

	raw := []RawMemory{
		{Content: "always run tests with -race flag", Source: "user_feedback", SessionID: "s1"},
		{Content: "never commit .env files", Source: "user_feedback", SessionID: "s1"},
		{Content: "you should use interfaces for mocking", Source: "session", SessionID: "s1"},
		{Content: "the sky is blue", Source: "session", SessionID: "s1"},
	}

	conventions := mc.ExtractConventions(raw)

	if len(conventions) != 3 {
		t.Fatalf("expected 3 conventions, got %d", len(conventions))
	}

	for _, c := range conventions {
		if c.Category != "convention" {
			t.Errorf("expected category 'convention', got %s", c.Category)
		}
	}
}

func TestMemoryConsolidatorExtractDecisions(t *testing.T) {
	mc := NewMemoryConsolidator("")

	raw := []RawMemory{
		{Content: "we decided to use PostgreSQL for the database", Source: "session", SessionID: "s1"},
		{Content: "chose gRPC because of performance requirements", Source: "session", SessionID: "s1"},
		{Content: "went with microservices architecture", Source: "session", SessionID: "s1"},
		{Content: "unrelated statement here", Source: "session", SessionID: "s1"},
	}

	decisions := mc.ExtractDecisions(raw)

	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}

	for _, d := range decisions {
		if d.Category != "decision" {
			t.Errorf("expected category 'decision', got %s", d.Category)
		}
	}
}

func TestExtractWarnings(t *testing.T) {
	raw := []RawMemory{
		{Content: "don't use global variables, it causes race conditions", Source: "user_feedback", SessionID: "s1"},
		{Content: "avoid using sleep in tests", Source: "session", SessionID: "s1"},
		{Content: "this is a normal statement", Source: "session", SessionID: "s1"},
	}

	warnings := extractWarnings(raw)

	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(warnings))
	}

	for _, w := range warnings {
		if w.Category != "warning" {
			t.Errorf("expected category 'warning', got %s", w.Category)
		}
	}
}

func TestConsolidate(t *testing.T) {
	mc := NewMemoryConsolidator("")

	mc.Ingest("the project uses Go modules", "session", "sess-1")
	mc.Ingest("always run tests with -race", "user_feedback", "sess-1")
	mc.Ingest("we decided to use PostgreSQL", "session", "sess-2")
	mc.Ingest("don't use global state, it causes bugs", "user_feedback", "sess-2")

	results, err := mc.Consolidate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected some consolidated memories")
	}

	// All raw memories should be marked as processed
	for _, rm := range mc.RawMemories {
		if !rm.Processed {
			t.Errorf("expected all raw memories to be processed, found unprocessed: %s", rm.Content)
		}
	}

	// LastConsolidation should be set
	if mc.LastConsolidation.IsZero() {
		t.Error("expected LastConsolidation to be set")
	}

	// Consolidating again should return nothing (all processed)
	results2, err := mc.Consolidate()
	if err != nil {
		t.Fatalf("unexpected error on second consolidate: %v", err)
	}
	if results2 != nil {
		t.Errorf("expected nil on second consolidate, got %d results", len(results2))
	}
}

func TestConsolidateDeduplication(t *testing.T) {
	mc := NewMemoryConsolidator("")

	// Ingest similar content
	mc.Ingest("the project uses Go modules", "session", "sess-1")

	_, err := mc.Consolidate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	initialCount := len(mc.ConsolidatedMemories)

	// Ingest something very similar
	mc.Ingest("the project uses Go modules for dependency management", "session", "sess-2")
	_, err = mc.Consolidate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not add a duplicate
	if len(mc.ConsolidatedMemories) != initialCount {
		t.Errorf("expected %d consolidated memories after dedup, got %d", initialCount, len(mc.ConsolidatedMemories))
	}
}

func TestRecall(t *testing.T) {
	mc := NewMemoryConsolidator("")

	mc.ConsolidatedMemories = []ConsolidatedMemory{
		{ID: "f1", Category: "fact", Content: "the project uses Go modules", Confidence: 0.8},
		{ID: "f2", Category: "fact", Content: "the server runs on port 8080", Confidence: 0.7},
		{ID: "c1", Category: "convention", Content: "always run tests with race detector", Confidence: 0.9},
		{ID: "d1", Category: "decision", Content: "chose PostgreSQL for the database", Confidence: 0.7},
	}

	// Search for "Go"
	results := mc.Recall("Go modules", 10)
	if len(results) == 0 {
		t.Fatal("expected results for 'Go modules' query")
	}
	if results[0].Content != "the project uses Go modules" {
		t.Errorf("expected top result to be about Go modules, got: %s", results[0].Content)
	}

	// Search with limit
	results = mc.Recall("the", 2)
	if len(results) > 2 {
		t.Errorf("expected at most 2 results, got %d", len(results))
	}

	// Search for something not present
	results = mc.Recall("kubernetes deployment", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results for unrelated query, got %d", len(results))
	}
}

func TestRecallSkipsExpired(t *testing.T) {
	mc := NewMemoryConsolidator("")

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	mc.ConsolidatedMemories = []ConsolidatedMemory{
		{ID: "f1", Category: "fact", Content: "expired memory about Go", Confidence: 0.8, ExpiresAt: &past},
		{ID: "f2", Category: "fact", Content: "valid memory about Go", Confidence: 0.7, ExpiresAt: &future},
	}

	results := mc.Recall("Go", 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (skipping expired), got %d", len(results))
	}
	if results[0].ID != "f2" {
		t.Errorf("expected f2, got %s", results[0].ID)
	}
}

func TestExpire(t *testing.T) {
	mc := NewMemoryConsolidator("")

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	mc.ConsolidatedMemories = []ConsolidatedMemory{
		{ID: "f1", Category: "fact", Content: "expired", Confidence: 0.8, ExpiresAt: &past},
		{ID: "f2", Category: "fact", Content: "still valid", Confidence: 0.7, ExpiresAt: &future},
		{ID: "f3", Category: "fact", Content: "no expiry", Confidence: 0.6},
	}

	mc.Expire()

	if len(mc.ConsolidatedMemories) != 2 {
		t.Fatalf("expected 2 memories after expiry, got %d", len(mc.ConsolidatedMemories))
	}

	for _, m := range mc.ConsolidatedMemories {
		if m.ID == "f1" {
			t.Error("expired memory f1 should have been removed")
		}
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()

	mc := NewMemoryConsolidator(dir)
	mc.Ingest("the project uses Go", "session", "sess-1")
	mc.Ingest("always use gofmt", "user_feedback", "sess-1")

	_, err := mc.Consolidate()
	if err != nil {
		t.Fatalf("consolidate error: %v", err)
	}

	err = mc.Save()
	if err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "consolidated_memory.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected consolidated_memory.json to exist")
	}

	// Load into a new consolidator
	mc2 := NewMemoryConsolidator(dir)
	err = mc2.Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if len(mc2.RawMemories) != len(mc.RawMemories) {
		t.Errorf("raw memory count mismatch: %d vs %d", len(mc2.RawMemories), len(mc.RawMemories))
	}
	if len(mc2.ConsolidatedMemories) != len(mc.ConsolidatedMemories) {
		t.Errorf("consolidated memory count mismatch: %d vs %d", len(mc2.ConsolidatedMemories), len(mc.ConsolidatedMemories))
	}
}

func TestLoadNonexistent(t *testing.T) {
	mc := NewMemoryConsolidator(t.TempDir())
	err := mc.Load()
	if err != nil {
		t.Fatalf("loading from nonexistent file should not error, got: %v", err)
	}
}

func TestMemoryConsolidatorLoadNoDir(t *testing.T) {
	mc := NewMemoryConsolidator("")
	err := mc.Load()
	if err == nil {
		t.Fatal("expected error when dir is empty")
	}
}

func TestMemoryConsolidatorSaveNoDir(t *testing.T) {
	mc := NewMemoryConsolidator("")
	err := mc.Save()
	if err == nil {
		t.Fatal("expected error when dir is empty")
	}
}

func TestFormatMemories(t *testing.T) {
	mc := NewMemoryConsolidator("")

	memories := []ConsolidatedMemory{
		{ID: "f1", Category: "fact", Content: "project uses Go", Confidence: 0.8},
		{ID: "c1", Category: "convention", Content: "always use gofmt", Confidence: 0.9},
		{ID: "d1", Category: "decision", Content: "chose PostgreSQL", Confidence: 0.7},
		{ID: "w1", Category: "warning", Content: "avoid global state", Confidence: 0.85},
	}

	formatted := mc.FormatMemories(memories)

	if formatted == "" {
		t.Fatal("expected non-empty formatted output")
	}
	if !strings.Contains(formatted, "## Consolidated Memories") {
		t.Error("expected header in formatted output")
	}
	if !strings.Contains(formatted, "### Facts") {
		t.Error("expected Facts section")
	}
	if !strings.Contains(formatted, "### Conventions") {
		t.Error("expected Conventions section")
	}
	if !strings.Contains(formatted, "### Decisions") {
		t.Error("expected Decisions section")
	}
	if !strings.Contains(formatted, "### Warnings") {
		t.Error("expected Warnings section")
	}

	// Empty memories
	empty := mc.FormatMemories(nil)
	if empty != "" {
		t.Error("expected empty string for nil memories")
	}
}

func TestMemoryConsolidatorStats(t *testing.T) {
	mc := NewMemoryConsolidator("")

	mc.RawMemories = []RawMemory{
		{Content: "a", Processed: true},
		{Content: "b", Processed: true},
		{Content: "c", Processed: false},
	}
	mc.ConsolidatedMemories = []ConsolidatedMemory{
		{Category: "fact", Confidence: 0.8},
		{Category: "fact", Confidence: 0.6},
		{Category: "convention", Confidence: 0.9},
		{Category: "decision", Confidence: 0.7},
		{Category: "warning", Confidence: 0.85},
	}

	stats := mc.Stats()

	if stats.RawCount != 3 {
		t.Errorf("expected raw count 3, got %d", stats.RawCount)
	}
	if stats.ProcessedCount != 2 {
		t.Errorf("expected processed count 2, got %d", stats.ProcessedCount)
	}
	if stats.ConsolidatedCount != 5 {
		t.Errorf("expected consolidated count 5, got %d", stats.ConsolidatedCount)
	}
	if stats.FactCount != 2 {
		t.Errorf("expected fact count 2, got %d", stats.FactCount)
	}
	if stats.ConventionCount != 1 {
		t.Errorf("expected convention count 1, got %d", stats.ConventionCount)
	}
	if stats.DecisionCount != 1 {
		t.Errorf("expected decision count 1, got %d", stats.DecisionCount)
	}
	if stats.WarningCount != 1 {
		t.Errorf("expected warning count 1, got %d", stats.WarningCount)
	}
	if stats.AvgConfidence < 0.76 || stats.AvgConfidence > 0.78 {
		t.Errorf("expected avg confidence ~0.77, got %f", stats.AvgConfidence)
	}
}

func TestConfidenceFromSource(t *testing.T) {
	tests := []struct {
		source   string
		expected float64
	}{
		{"user_feedback", 0.9},
		{"session", 0.7},
		{"tool_output", 0.6},
		{"unknown", 0.5},
	}

	for _, tt := range tests {
		got := confidenceFromSource(tt.source)
		if got != tt.expected {
			t.Errorf("confidenceFromSource(%q) = %f, want %f", tt.source, got, tt.expected)
		}
	}
}

func TestMemorySimilar(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		{"the project uses Go", "the project uses Go", true},
		{"the project uses Go", "The Project Uses Go", true},
		{"the project uses Go modules", "the project uses Go", true},
		{"completely different", "nothing in common here at all whatsoever", false},
	}

	for _, tt := range tests {
		got := memorySimilar(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("memorySimilar(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestClampConfidenceVal(t *testing.T) {
	if clampConfidenceVal(-0.5) != 0 {
		t.Error("expected 0 for negative input")
	}
	if clampConfidenceVal(1.5) != 1.0 {
		t.Error("expected 1.0 for input > 1")
	}
	if clampConfidenceVal(0.5) != 0.5 {
		t.Error("expected 0.5 for 0.5 input")
	}
}

func TestMemSliceContains(t *testing.T) {
	slice := []string{"a", "b", "c"}
	if !memSliceContains(slice, "b") {
		t.Error("expected true for 'b'")
	}
	if memSliceContains(slice, "d") {
		t.Error("expected false for 'd'")
	}
}
