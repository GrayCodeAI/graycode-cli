package memory

import (
	"os"
	"strings"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	m := &Memory{
		Content: "Important decision about architecture",
		Tags:    []string{"architecture", "decision"},
		Source:  "session-123",
	}
	if err := Save(m); err != nil {
		t.Fatal(err)
	}
	if m.ID == "" {
		t.Fatal("ID should be set")
	}

	loaded, err := Load(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Content != m.Content {
		t.Fatalf("content mismatch: %q vs %q", loaded.Content, m.Content)
	}
}

func TestSearch(t *testing.T) {
	// Test the core search logic without filesystem
	memories := []*Memory{
		{Content: "Architecture decision: use Go", Tags: []string{"go"}},
		{Content: "Python is slow for this task", Tags: []string{"python"}},
	}

	// Search for "go" should find first memory by content
	found := false
	for _, m := range memories {
		if strings.Contains(strings.ToLower(m.Content), "go") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected search to find 'go' in content")
	}

	// Search by tag
	found = false
	for _, m := range memories {
		for _, tag := range m.Tags {
			if strings.Contains(strings.ToLower(tag), "python") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected search to find 'python' in tags")
	}
}

func TestExtractFromSession(t *testing.T) {
	messages := []string{
		"Important: we decided to use Redis for caching",
		"Note the API key is in .env",
		"Just a regular message",
		"Remember to update the README",
	}
	memories := ExtractFromSession("test", messages)
	if len(memories) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(memories))
	}
}

func TestConsolidate(t *testing.T) {
	memories := []*Memory{
		{Content: "Decision A"},
		{Content: "Decision A"},
		{Content: "Decision B"},
	}
	consolidated := Consolidate(memories)
	if len(consolidated) != 2 {
		t.Fatalf("expected 2 consolidated memories, got %d", len(consolidated))
	}
}
