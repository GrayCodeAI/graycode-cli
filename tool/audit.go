package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEntry records a file modification event.
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Tool      string    `json:"tool"`
	Action    string    `json:"action"` // "create", "edit", "write", "delete"
	Path      string    `json:"path"`
	BackupRef string    `json:"backup_ref,omitempty"`
	LinesChanged int    `json:"lines_changed,omitempty"`
	BytesWritten int    `json:"bytes_written,omitempty"`
}

// AuditLog tracks all file modifications for accountability.
type AuditLog struct {
	mu   sync.Mutex
	f    *os.File
	path string
}

var globalAudit *AuditLog
var auditOnce sync.Once

// GetAuditLog returns the global audit log instance.
func GetAuditLog() *AuditLog {
	auditOnce.Do(func() {
		home, _ := os.UserHomeDir()
		dir := filepath.Join(home, ".hawk", "audit")
		os.MkdirAll(dir, 0o755)

		today := time.Now().Format("2006-01-02")
		path := filepath.Join(dir, today+".jsonl")

		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			globalAudit = &AuditLog{} // no-op audit
			return
		}
		globalAudit = &AuditLog{f: f, path: path}
	})
	return globalAudit
}

// Record logs a file modification event.
func (a *AuditLog) Record(entry AuditEntry) {
	if a == nil || a.f == nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	entry.Timestamp = time.Now()
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	a.f.Write(data)
	a.f.Write([]byte("\n"))
}

// Close closes the audit log.
func (a *AuditLog) Close() {
	if a != nil && a.f != nil {
		a.f.Close()
	}
}

// RecordFileWrite logs a file write/create operation.
func RecordFileWrite(toolName, path string, bytes int) {
	GetAuditLog().Record(AuditEntry{
		Tool:         toolName,
		Action:       "write",
		Path:         path,
		BytesWritten: bytes,
	})
}

// RecordFileEdit logs a file edit operation.
func RecordFileEdit(toolName, path string, linesChanged int) {
	GetAuditLog().Record(AuditEntry{
		Tool:         toolName,
		Action:       "edit",
		Path:         path,
		LinesChanged: linesChanged,
	})
}

// RecordFileDelete logs a file deletion.
func RecordFileDelete(toolName, path string) {
	GetAuditLog().Record(AuditEntry{
		Tool:   toolName,
		Action: "delete",
		Path:   path,
	})
}

// TodayEntries reads today's audit entries.
func TodayEntries() ([]AuditEntry, error) {
	home, _ := os.UserHomeDir()
	today := time.Now().Format("2006-01-02")
	path := filepath.Join(home, ".hawk", "audit", today+".jsonl")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entries []AuditEntry
	for _, line := range splitByNewline(data) {
		if len(line) == 0 {
			continue
		}
		var entry AuditEntry
		if json.Unmarshal(line, &entry) == nil {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// FormatAuditSummary produces a human-readable summary of today's modifications.
func FormatAuditSummary() string {
	entries, err := TodayEntries()
	if err != nil || len(entries) == 0 {
		return "No file modifications recorded today."
	}

	var result string
	result = fmt.Sprintf("File modifications today (%d total):\n", len(entries))
	for _, e := range entries {
		ts := e.Timestamp.Format("15:04:05")
		result += fmt.Sprintf("  [%s] %s %s %s", ts, e.Tool, e.Action, e.Path)
		if e.LinesChanged > 0 {
			result += fmt.Sprintf(" (%d lines)", e.LinesChanged)
		}
		if e.BytesWritten > 0 {
			result += fmt.Sprintf(" (%d bytes)", e.BytesWritten)
		}
		result += "\n"
	}
	return result
}

func splitByNewline(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
