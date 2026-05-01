package cmd

import (
	"os"
	"path/filepath"
	"strings"
)

const maxHistoryEntries = 1000

// historyFilePath returns the path to the persistent input history file.
func historyFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "history")
}

// loadInputHistory loads input history from ~/.hawk/history.
// Returns an empty slice if the file does not exist.
func loadInputHistory() []string {
	data, err := os.ReadFile(historyFilePath())
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	var entries []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			entries = append(entries, line)
		}
	}
	return entries
}

// saveInputHistory writes the history list to ~/.hawk/history.
// Deduplicates entries (keeping the last occurrence) and caps at maxHistoryEntries.
func saveInputHistory(history []string) {
	// Deduplicate: keep the last occurrence of each entry
	seen := make(map[string]bool)
	var deduped []string
	for i := len(history) - 1; i >= 0; i-- {
		entry := strings.TrimSpace(history[i])
		if entry == "" || seen[entry] {
			continue
		}
		seen[entry] = true
		deduped = append(deduped, entry)
	}
	// Reverse to restore chronological order
	for i, j := 0, len(deduped)-1; i < j; i, j = i+1, j-1 {
		deduped[i], deduped[j] = deduped[j], deduped[i]
	}
	// Cap at max entries
	if len(deduped) > maxHistoryEntries {
		deduped = deduped[len(deduped)-maxHistoryEntries:]
	}

	path := historyFilePath()
	os.MkdirAll(filepath.Dir(path), 0o755)
	content := strings.Join(deduped, "\n") + "\n"
	os.WriteFile(path, []byte(content), 0o644)
}

// appendToHistory appends a single entry to the history file.
// It loads existing history, appends, deduplicates, and saves.
func appendToHistory(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}
	history := loadInputHistory()
	history = append(history, entry)
	saveInputHistory(history)
}
