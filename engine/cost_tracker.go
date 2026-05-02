package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/analytics"
)

// CostTracker records per-request cost entries for analytics and optimization.
// Data is appended to ~/.hawk/cost.jsonl for cross-session analysis.
type CostTracker struct {
	mu        sync.Mutex
	entries   []analytics.CostEntry
	sessionID string
	filePath  string
}

// NewCostTracker creates a tracker that persists to ~/.hawk/cost.jsonl.
func NewCostTracker(sessionID string) *CostTracker {
	home, _ := os.UserHomeDir()
	return &CostTracker{
		sessionID: sessionID,
		filePath:  filepath.Join(home, ".hawk", "cost.jsonl"),
	}
}

// Record adds a cost entry and persists it.
func (ct *CostTracker) Record(entry analytics.CostEntry) error {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	entry.SessionID = ct.sessionID
	entry.Timestamp = time.Now()
	ct.entries = append(ct.entries, entry)

	return ct.appendToFile(entry)
}

// SessionTotal returns total USD spent in the current session.
func (ct *CostTracker) SessionTotal() float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	var total float64
	for _, e := range ct.entries {
		total += e.CostUSD
	}
	return total
}

// Entries returns all recorded entries for this session.
func (ct *CostTracker) Entries() []analytics.CostEntry {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	out := make([]analytics.CostEntry, len(ct.entries))
	copy(out, ct.entries)
	return out
}

// LoadHistory reads all historical cost entries from the JSONL file.
func LoadCostHistory() ([]analytics.CostEntry, error) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".hawk", "cost.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []analytics.CostEntry
	for _, line := range splitJSONLines(data) {
		if len(line) == 0 {
			continue
		}
		var e analytics.CostEntry
		if json.Unmarshal(line, &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func (ct *CostTracker) appendToFile(entry analytics.CostEntry) error {
	dir := filepath.Dir(ct.filePath)
	os.MkdirAll(dir, 0o755)

	f, err := os.OpenFile(ct.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, _ := json.Marshal(entry)
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

func splitJSONLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
