package session

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AutoSaver periodically saves sessions and tracks session metadata.
type AutoSaver struct {
	mu       sync.Mutex
	interval time.Duration
	timer    *time.Timer
	saveFn   func()
	stopped  bool
}

// NewAutoSaver creates an auto-saver that triggers saveFn at the given interval.
func NewAutoSaver(interval time.Duration, saveFn func()) *AutoSaver {
	as := &AutoSaver{
		interval: interval,
		saveFn:   saveFn,
	}
	as.Reset()
	return as
}

// Reset restarts the auto-save timer.
func (as *AutoSaver) Reset() {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.stopped {
		return
	}
	if as.timer != nil {
		as.timer.Stop()
	}
	as.timer = time.AfterFunc(as.interval, func() {
		as.saveFn()
		as.Reset()
	})
}

// Stop stops the auto-saver.
func (as *AutoSaver) Stop() {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.stopped = true
	if as.timer != nil {
		as.timer.Stop()
	}
}

// Touch resets the timer (called on activity to delay the save).
func (as *AutoSaver) Touch() {
	as.Reset()
}

// SessionMeta holds lightweight metadata for listing without full parse.
type SessionMeta struct {
	ID         string    `json:"id"`
	Name       string    `json:"name,omitempty"`
	CWD        string    `json:"cwd,omitempty"`
	Model      string    `json:"model,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	GitBranch  string    `json:"git_branch,omitempty"`
	MsgCount   int       `json:"message_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Tags       []string  `json:"tags,omitempty"`
	TokenCount int       `json:"token_count,omitempty"`
}

// LockFile prevents double-opening a session.
type LockFile struct {
	path string
}

// AcquireLock creates a lock file for the session. Returns error if already locked.
func AcquireLock(sessionID string) (*LockFile, error) {
	dir := sessionsDir()
	path := filepath.Join(dir, sessionID+".lock")

	// Check if lock exists and is stale (>5 min old)
	if info, err := os.Stat(path); err == nil {
		if time.Since(info.ModTime()) > 5*time.Minute {
			os.Remove(path) // stale lock
		} else {
			return nil, &SessionLockedError{ID: sessionID}
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, &SessionLockedError{ID: sessionID}
	}
	f.Write([]byte(time.Now().Format(time.RFC3339)))
	f.Close()

	return &LockFile{path: path}, nil
}

// Release removes the lock file.
func (l *LockFile) Release() {
	if l != nil && l.path != "" {
		os.Remove(l.path)
	}
}

// Refresh updates the lock file timestamp to prevent it from going stale.
func (l *LockFile) Refresh() {
	if l != nil && l.path != "" {
		os.Chtimes(l.path, time.Now(), time.Now())
	}
}

// SessionLockedError indicates a session is already open.
type SessionLockedError struct {
	ID string
}

func (e *SessionLockedError) Error() string {
	return "session " + e.ID + " is already open in another instance"
}

// AddTag adds a tag to a session.
func AddTag(sess *Session, tag string) {
	// Tags stored in name field as comma-separated for simplicity
	if sess.Name == "" {
		sess.Name = "#" + tag
	} else if !containsTag(sess.Name, tag) {
		sess.Name += " #" + tag
	}
}

// RemoveTag removes a tag from a session.
func RemoveTag(sess *Session, tag string) {
	// Simple implementation
	sess.Name = replaceTag(sess.Name, tag)
}

func containsTag(name, tag string) bool {
	target := "#" + tag
	for _, part := range splitWords(name) {
		if part == target {
			return true
		}
	}
	return false
}

func replaceTag(name, tag string) string {
	target := "#" + tag
	result := ""
	for _, part := range splitWords(name) {
		if part != target {
			if result != "" {
				result += " "
			}
			result += part
		}
	}
	return result
}

func splitWords(s string) []string {
	var words []string
	word := ""
	for _, r := range s {
		if r == ' ' {
			if word != "" {
				words = append(words, word)
				word = ""
			}
		} else {
			word += string(r)
		}
	}
	if word != "" {
		words = append(words, word)
	}
	return words
}

// CleanOldSessions removes sessions older than the given duration.
func CleanOldSessions(maxAge time.Duration) (int, error) {
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	removed := 0
	cutoff := time.Now().Add(-maxAge)

	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext != ".jsonl" && ext != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}

// ExportToMarkdown exports a session as readable markdown.
func ExportToMarkdown(sess *Session) string {
	var md string
	md += "# Session: " + sess.ID + "\n\n"
	md += "**Model:** " + sess.Model + " | **Provider:** " + sess.Provider + "\n"
	md += "**Created:** " + sess.CreatedAt.Format("2006-01-02 15:04") + "\n\n"
	md += "---\n\n"

	for _, msg := range sess.Messages {
		switch msg.Role {
		case "user":
			if msg.ToolResult != nil {
				continue // skip tool results in markdown export
			}
			md += "## User\n\n" + msg.Content + "\n\n"
		case "assistant":
			if len(msg.ToolUse) > 0 {
				for _, tc := range msg.ToolUse {
					md += "**Tool:** `" + tc.Name + "`\n\n"
				}
			}
			if msg.Content != "" {
				md += "## Assistant\n\n" + msg.Content + "\n\n"
			}
		}
	}
	return md
}

// SearchSessions searches across all sessions for content.
func SearchSessions(query string, maxResults int) ([]SearchResult, error) {
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, e := range entries {
		if len(results) >= maxResults {
			break
		}
		ext := filepath.Ext(e.Name())
		if ext != ".jsonl" && ext != ".json" {
			continue
		}

		id := e.Name()[:len(e.Name())-len(ext)]
		sess, err := Load(id)
		if err != nil {
			continue
		}

		for i, msg := range sess.Messages {
			if containsIgnoreCase(msg.Content, query) {
				preview := extractContext(msg.Content, query, 100)
				results = append(results, SearchResult{
					SessionID: id,
					MsgIndex:  i,
					Role:      msg.Role,
					Preview:   preview,
				})
				if len(results) >= maxResults {
					break
				}
			}
		}
	}
	return results, nil
}

// SearchResult represents a match found in session search.
type SearchResult struct {
	SessionID string
	MsgIndex  int
	Role      string
	Preview   string
}

func containsIgnoreCase(s, substr string) bool {
	sl := toLower(s)
	ql := toLower(substr)
	return indexOf(sl, ql) >= 0
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func extractContext(content, query string, maxLen int) string {
	idx := indexOf(toLower(content), toLower(query))
	if idx < 0 {
		if len(content) > maxLen {
			return content[:maxLen] + "..."
		}
		return content
	}

	start := idx - 30
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 50
	if end > len(content) {
		end = len(content)
	}

	result := content[start:end]
	if start > 0 {
		result = "..." + result
	}
	if end < len(content) {
		result += "..."
	}
	return result
}
