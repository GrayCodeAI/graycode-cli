package memory

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// AutoMemory manages automatic memory extraction and storage for a project.
type AutoMemory struct {
	dir string
}

// NewAutoMemory creates a new AutoMemory rooted in Hawk's project state.
func NewAutoMemory(projectDir string) *AutoMemory {
	h := sha256.Sum256([]byte(projectDir))
	hash := fmt.Sprintf("%x", h[:8])
	dir := filepath.Join(storage.ProjectStateDir(projectDir), "memory", hash)
	return &AutoMemory{dir: dir}
}

// triggerWords indicate content worth remembering when multiple appear.
var triggerWords = []string{
	"don't", "instead", "correction", "actually", "remember",
	"mistake", "should have", "better approach", "wrong", "fix",
}

// strongTriggers fire alone (explicit learning signals).
var strongTriggers = []string{
	"note to self", "decision:", "convention:", "always use", "never use",
	"prefer ", "important:",
}

// ShouldAutoRemember reports whether assistant content should be persisted to yaad.
// Requires two weak triggers or one strong trigger to limit noise.
func ShouldAutoRemember(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}
	for _, t := range strongTriggers {
		if strings.Contains(lower, t) {
			return true
		}
	}
	hits := 0
	for _, w := range triggerWords {
		if strings.Contains(lower, w) {
			hits++
			if hits >= 2 {
				return true
			}
		}
	}
	return false
}

// ShouldRemember returns true if content contains trigger words indicating
// the user is correcting, emphasizing, or asking the assistant to remember something.
func (am *AutoMemory) ShouldRemember(content string) bool {
	return ShouldAutoRemember(content)
}

// Write appends content to a topic-specific markdown file in the memory directory.
func (am *AutoMemory) Write(topic, content string) error {
	if err := os.MkdirAll(am.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(am.dir, topic+".md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintf(f, "- %s\n", content)
	return err
}

// LoadStartup reads the first 200 lines (max 25KB) of MEMORY.md from the memory directory.
func (am *AutoMemory) LoadStartup() string {
	path := filepath.Join(am.dir, "MEMORY.md")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	var b strings.Builder
	scanner := bufio.NewScanner(f)
	lineCount := 0
	const maxLines = 200
	const maxBytes = 25 * 1024

	for scanner.Scan() {
		if lineCount >= maxLines || b.Len() >= maxBytes {
			break
		}
		line := scanner.Text()
		if b.Len()+len(line)+1 > maxBytes {
			break
		}
		b.WriteString(line)
		b.WriteByte('\n')
		lineCount++
	}
	return b.String()
}

// Search greps across all .md files in the memory directory for lines matching query.
func (am *AutoMemory) Search(query string) []string {
	query = strings.ToLower(query)
	var results []string

	entries, err := os.ReadDir(am.dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(am.dir, e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(strings.ToLower(line), query) {
				results = append(results, line)
			}
		}
	}
	return results
}

// Format returns memory content formatted for prompt injection.
func (am *AutoMemory) Format() string {
	startup := am.LoadStartup()
	if startup == "" {
		return ""
	}
	return "## Project Memory\n\n" + startup
}
