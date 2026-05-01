package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInputHistory_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	history := loadInputHistory()
	if len(history) != 0 {
		t.Fatalf("expected empty history, got %d entries", len(history))
	}
}

func TestSaveAndLoadInputHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	entries := []string{"hello", "world", "test"}
	saveInputHistory(entries)

	loaded := loadInputHistory()
	if len(loaded) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(loaded))
	}
	for i, want := range entries {
		if loaded[i] != want {
			t.Errorf("entry %d: got %q, want %q", i, loaded[i], want)
		}
	}
}

func TestSaveInputHistory_Deduplication(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	entries := []string{"hello", "world", "hello", "test", "world"}
	saveInputHistory(entries)

	loaded := loadInputHistory()
	// "hello" and "world" appear twice each; dedup keeps last occurrence.
	// Order should be: hello, test, world
	if len(loaded) != 3 {
		t.Fatalf("expected 3 entries after dedup, got %d: %v", len(loaded), loaded)
	}
	if loaded[0] != "hello" || loaded[1] != "test" || loaded[2] != "world" {
		t.Fatalf("unexpected deduped order: %v", loaded)
	}
}

func TestSaveInputHistory_MaxEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var entries []string
	for i := 0; i < 1500; i++ {
		entries = append(entries, strings.Repeat("x", 5)+string(rune('a'+i%26)))
	}
	saveInputHistory(entries)

	loaded := loadInputHistory()
	if len(loaded) > maxHistoryEntries {
		t.Fatalf("expected at most %d entries, got %d", maxHistoryEntries, len(loaded))
	}
}

func TestAppendToHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	appendToHistory("first command")
	appendToHistory("second command")

	loaded := loadInputHistory()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}
	if loaded[0] != "first command" || loaded[1] != "second command" {
		t.Fatalf("unexpected entries: %v", loaded)
	}
}

func TestAppendToHistory_EmptySkipped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	appendToHistory("")
	appendToHistory("  ")

	loaded := loadInputHistory()
	if len(loaded) != 0 {
		t.Fatalf("expected 0 entries for empty input, got %d", len(loaded))
	}
}

func TestHistoryFilePath(t *testing.T) {
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".hawk", "history")
	if got := historyFilePath(); got != expected {
		t.Fatalf("got %q, want %q", got, expected)
	}
}
