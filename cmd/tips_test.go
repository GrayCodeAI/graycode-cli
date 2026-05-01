package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllTips_HasEntries(t *testing.T) {
	tips := allTips()
	if len(tips) < 20 {
		t.Fatalf("expected at least 20 tips, got %d", len(tips))
	}
	for _, tip := range tips {
		if tip.ID == "" || tip.Text == "" || tip.Category == "" {
			t.Fatalf("tip has empty field: %+v", tip)
		}
	}
}

func TestAllTips_UniqueIDs(t *testing.T) {
	tips := allTips()
	seen := make(map[string]bool)
	for _, tip := range tips {
		if seen[tip.ID] {
			t.Fatalf("duplicate tip ID: %s", tip.ID)
		}
		seen[tip.ID] = true
	}
}

func TestNextTip_ReturnsNonEmpty(t *testing.T) {
	// Use a temp dir for tip history to avoid polluting real config
	orig := os.Getenv("HOME")
	tmp := t.TempDir()
	os.Setenv("HOME", tmp)
	os.MkdirAll(filepath.Join(tmp, ".hawk"), 0o755)
	defer os.Setenv("HOME", orig)

	tip := nextTip()
	if tip == "" {
		t.Fatal("nextTip returned empty string")
	}
}

func TestRecordTipShown(t *testing.T) {
	orig := os.Getenv("HOME")
	tmp := t.TempDir()
	os.Setenv("HOME", tmp)
	os.MkdirAll(filepath.Join(tmp, ".hawk"), 0o755)
	defer os.Setenv("HOME", orig)

	recordTipShown("slash-help")

	h := loadTipHistory()
	if _, ok := h.Shown["slash-help"]; !ok {
		t.Fatal("expected slash-help to be recorded in history")
	}
}
