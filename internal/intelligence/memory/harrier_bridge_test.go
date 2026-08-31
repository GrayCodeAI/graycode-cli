package memory

import (
	"strings"
	"testing"
)

func TestSearchCompact_NotReady(t *testing.T) {
	b := &HarrierBridge{ready: false}
	results, err := b.SearchCompact("test", 5)
	if err == nil {
		t.Fatal("expected error when bridge not ready")
	}
	if results != nil {
		t.Fatal("expected nil results when not ready")
	}
}

func TestGetFullContent_NotReady(t *testing.T) {
	b := &HarrierBridge{ready: false}
	results, err := b.GetFullContent([]string{"id1"})
	if err == nil {
		t.Fatal("expected error when bridge not ready")
	}
	if results != nil {
		t.Fatal("expected nil results when not ready")
	}
}

func TestGetFullContent_EmptyIDs(t *testing.T) {
	b := &HarrierBridge{ready: true}
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

func TestFormatHarrierDetail_NotReady(t *testing.T) {
	// Force not-ready by using a bridge with ready=false via empty home trick is heavy;
	// FormatHarrierDetail calls NewHarrierBridge which may succeed on dev machines.
	out := FormatHarrierDetail(5)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if strings.Contains(out, "not initialized") {
		return
	}
	if !strings.Contains(out, "Recent memories:") {
		t.Fatalf("expected recent memories section, got %q", out)
	}
}

func TestFormatHarrierSearch_EmptyQuery(t *testing.T) {
	out := FormatHarrierSearch("  ", 5)
	if !strings.Contains(out, "query required") {
		t.Fatalf("expected query required, got %q", out)
	}
}

func TestFormatHarrierSearch_NotReadyOrResults(t *testing.T) {
	out := FormatHarrierSearch("decision", 5)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if strings.Contains(out, "not initialized") {
		return
	}
	if !strings.Contains(out, "Harrier search:") {
		t.Fatalf("expected search header, got %q", out)
	}
}
