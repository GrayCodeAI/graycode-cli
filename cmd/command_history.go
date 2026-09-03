package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

// commandHistory persists recently-used slash commands so the command
// palette can surface a "Recent" section. Entries are stored most-recent-first
// with a cap to keep the file small.
const maxRecentCommands = 12

// commandHistoryEntry is one recorded command use.
type commandHistoryEntry struct {
	Command  string    `json:"command"`
	UsedAt   time.Time `json:"used_at"`
	UseCount int       `json:"use_count"`
}

// commandHistory is the on-disk shape.
type commandHistory struct {
	Entries []commandHistoryEntry `json:"entries"`
}

// commandHistoryPath returns the path to the command history file.
func commandHistoryPath() string {
	return filepath.Join(storage.StateDir(), "command_history.json")
}

// loadCommandHistory reads the persisted command history, or returns an empty
// history if the file doesn't exist yet.
func loadCommandHistory() commandHistory {
	h := commandHistory{Entries: []commandHistoryEntry{}}
	data, err := os.ReadFile(commandHistoryPath())
	if err != nil {
		return h
	}
	_ = json.Unmarshal(data, &h)
	if h.Entries == nil {
		h.Entries = []commandHistoryEntry{}
	}
	return h
}

// saveCommandHistory persists the command history to disk.
func saveCommandHistory(h commandHistory) {
	if len(h.Entries) > maxRecentCommands {
		h.Entries = h.Entries[:maxRecentCommands]
	}
	_ = os.MkdirAll(storage.StateDir(), 0o750) // #nosec G301 -- state dir holds private user data
	data, _ := json.MarshalIndent(h, "", "  ")
	_ = os.WriteFile(commandHistoryPath(), data, 0o600) // #nosec G306 -- command history is private user state
}

// recordCommandUsed notes that a slash command was just run. It moves the
// command to the front of the history (or inserts it if new) and increments
// its use count.
func recordCommandUsed(cmd string) {
	if cmd == "" || !strings.HasPrefix(cmd, "/") {
		return
	}
	h := loadCommandHistory()

	// Find existing entry.
	found := -1
	for i, e := range h.Entries {
		if e.Command == cmd {
			found = i
			break
		}
	}

	if found >= 0 {
		// Move to front, increment count.
		entry := h.Entries[found]
		entry.UsedAt = time.Now()
		entry.UseCount++
		// Remove from current position.
		h.Entries = append(h.Entries[:found], h.Entries[found+1:]...)
		// Prepend.
		h.Entries = append([]commandHistoryEntry{entry}, h.Entries...)
	} else {
		h.Entries = append([]commandHistoryEntry{{Command: cmd, UsedAt: time.Now(), UseCount: 1}}, h.Entries...)
	}

	saveCommandHistory(h)
}

// recentCommands returns up to maxRecentCommands slash commands ordered by
// recency (most recently used first). Used by the command palette to render
// a "Recent" section.
func recentCommands() []string {
	h := loadCommandHistory()
	seen := make(map[string]bool, len(h.Entries))
	result := make([]string, 0, len(h.Entries))
	for _, e := range h.Entries {
		if seen[e.Command] {
			continue
		}
		seen[e.Command] = true
		result = append(result, e.Command)
	}
	return result
}
