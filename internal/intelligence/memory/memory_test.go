package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	m := &Memory{
		Content: "Important decision about architecture",
		Tags:    []string{"architecture", "decision"},
		Source:  "session-123",
	}
	if err := Save(m); err != nil {
		t.Fatal(err)
	}
	if m.ID == "" {
		t.Fatal("ID should be set after save")
	}

	loaded, err := Load(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Content != m.Content {
		t.Fatalf("content mismatch: %q vs %q", loaded.Content, m.Content)
	}
	if loaded.Source != m.Source {
		t.Fatalf("source mismatch: %q vs %q", loaded.Source, m.Source)
	}
	if len(loaded.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(loaded.Tags))
	}
}

func TestSave_SetsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	m := &Memory{Content: "test"}
	if err := Save(m); err != nil {
		t.Fatal(err)
	}
	if m.ID == "" {
		t.Error("Save should generate ID")
	}
	if m.CreatedAt.IsZero() {
		t.Error("Save should set CreatedAt")
	}
}

func TestSave_PreservesExistingID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	m := &Memory{ID: "custom-id", Content: "test"}
	if err := Save(m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "custom-id" {
		t.Errorf("ID = %q, want custom-id", m.ID)
	}
}

func TestLoad_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	_, err := Load("nonexistent")
	if err == nil {
		t.Error("Load should return error for missing memory")
	}
}

func TestList_Empty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	memories, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 0 {
		t.Errorf("List() = %d memories, want 0", len(memories))
	}
}

func TestList_Multiple(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	for i := 0; i < 5; i++ {
		m := &Memory{ID: fmt.Sprintf("mem_%d", i), Content: "memory content"}
		if err := Save(m); err != nil {
			t.Fatal(err)
		}
	}

	memories, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 5 {
		t.Errorf("List() = %d memories, want 5", len(memories))
	}
}

func TestSearch_ByContent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	memories := []*Memory{
		{Content: "Architecture decision: use Go for the backend"},
		{Content: "Python is good for scripting tasks"},
		{Content: "Remember to use Go interfaces"},
	}
	for _, m := range memories {
		if err := Save(m); err != nil {
			t.Fatal(err)
		}
	}

	results, err := Search("go")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 1 {
		t.Errorf("Search('go') = %d results, want at least 1", len(results))
	}
}

func TestSearch_ByTag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	m := &Memory{Content: "some content", Tags: []string{"golang", "backend"}}
	if err := Save(m); err != nil {
		t.Fatal(err)
	}

	results, err := Search("golang")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("Search('golang') by tag = %d results, want 1", len(results))
	}
}

func TestSearch_NoMatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	m := &Memory{Content: "architecture decision"}
	if err := Save(m); err != nil {
		t.Fatal(err)
	}

	results, err := Search("kubernetes")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("Search('kubernetes') = %d results, want 0", len(results))
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	m := &Memory{Content: "IMPORTANT architecture DECISION"}
	if err := Save(m); err != nil {
		t.Fatal(err)
	}

	results, err := Search("important")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("case-insensitive search = %d results, want 1", len(results))
	}
}

func TestExtractFromSession(t *testing.T) {
	messages := []string{
		"Important: we decided to use Redis for caching",
		"Note the API key is in .env",
		"Just a regular message with no indicators",
		"Remember to update the README",
		"Another regular message",
		"Critical bug found in auth module",
	}
	memories := ExtractFromSession("test-session", messages)
	if len(memories) != 4 {
		t.Fatalf("expected 4 memories, got %d", len(memories))
	}
	for _, m := range memories {
		if m.Source != "test-session" {
			t.Errorf("Source = %q, want test-session", m.Source)
		}
	}
}

func TestExtractFromSession_Empty(t *testing.T) {
	memories := ExtractFromSession("empty", nil)
	if len(memories) != 0 {
		t.Errorf("expected 0 memories from empty input, got %d", len(memories))
	}
}

func TestIsMemoryWorthy(t *testing.T) {
	tests := []struct {
		msg   string
		worth bool
	}{
		{"Important: use context everywhere", true},
		{"Remember to add tests", true},
		{"Note: the API changed in v2", true},
		{"Key insight about performance", true},
		{"Critical fix needed", true},
		{"Decision: use SQLite for local storage", true},
		{"just a regular chat message", false},
		{"hello world", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			got := isMemoryWorthy(tt.msg)
			if got != tt.worth {
				t.Errorf("isMemoryWorthy(%q) = %v, want %v", tt.msg, got, tt.worth)
			}
		})
	}
}

func TestConsolidate(t *testing.T) {
	memories := []*Memory{
		{Content: "Decision A about architecture"},
		{Content: "Decision A about architecture"},
		{Content: "Decision B about testing"},
		{Content: "Decision A about architecture"}, // third duplicate
	}
	consolidated := Consolidate(memories)
	if len(consolidated) != 2 {
		t.Fatalf("expected 2 consolidated memories, got %d", len(consolidated))
	}
}

func TestConsolidate_Empty(t *testing.T) {
	consolidated := Consolidate(nil)
	if len(consolidated) != 0 {
		t.Errorf("Consolidate(nil) = %d, want 0", len(consolidated))
	}
}

func TestConsolidate_AllUnique(t *testing.T) {
	memories := []*Memory{
		{Content: "Memory one about X"},
		{Content: "Memory two about Y"},
		{Content: "Memory three about Z"},
	}
	consolidated := Consolidate(memories)
	if len(consolidated) != 3 {
		t.Fatalf("expected 3 unique memories, got %d", len(consolidated))
	}
}

func TestMemoryDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	stateDir := filepath.Join(dir, "state")
	t.Setenv("HAWK_STATE_DIR", stateDir)

	d := memoryDir()
	if !strings.HasPrefix(d, stateDir) {
		t.Errorf("memoryDir() = %q, should be under %q", d, stateDir)
	}
	if !strings.Contains(d, "memories") {
		t.Errorf("memoryDir() = %q, should contain memories", d)
	}
}

func TestSaveAndLoad_WithNoHome(t *testing.T) {
	t.Setenv("HOME", "/nonexistent-path-12345")
	m := &Memory{Content: "test"}
	err := Save(m)
	if err == nil {
		_ = os.Remove("/nonexistent-path-12345/.hawk/memories/" + m.ID + ".json")
	}
}
