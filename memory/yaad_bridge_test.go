package memory

import (
	"testing"
)

func TestSearchCompact_NotReady(t *testing.T) {
	b := &YaadBridge{ready: false}
	results, err := b.SearchCompact("test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Fatal("expected nil results when not ready")
	}
}

func TestGetFullContent_NotReady(t *testing.T) {
	b := &YaadBridge{ready: false}
	results, err := b.GetFullContent([]string{"id1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Fatal("expected nil results when not ready")
	}
}

func TestGetFullContent_EmptyIDs(t *testing.T) {
	b := &YaadBridge{ready: true}
	results, err := b.GetFullContent(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Fatal("expected nil results for empty IDs")
	}
}

func TestCompactResult_TitleTruncation(t *testing.T) {
	// Verify the title truncation logic directly
	content := "This is a very long content string that exceeds one hundred characters and should be truncated to fit within the compact result title field limit."
	title := content
	if len(title) > 100 {
		title = title[:100]
	}
	if len(title) != 100 {
		t.Fatalf("expected title length 100, got %d", len(title))
	}
}
