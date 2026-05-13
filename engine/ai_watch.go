package engine

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AIComment represents a detected AI instruction comment in a source file.
type AIComment struct {
	File     string // relative file path
	Line     int    // 1-based line number
	Comment  string // the instruction text after "ai:"
	Language string // detected language (go, python, js, etc.)
	Context  string // surrounding 10 lines of code (5 before, 5 after)
	Marker   string // the full comment line for removal after completion
}

// AIWatcher monitors a directory tree for AI instruction comments and fires
// a callback when new ones are detected.
type AIWatcher struct {
	RootDir   string
	Patterns  []string // file glob patterns to watch (e.g., "*.go", "*.py")
	Debounce  time.Duration
	OnComment func(comment AIComment)

	done chan struct{}
	mu   sync.Mutex
	// known tracks comments by their content hash to detect new/removed ones
	known map[string]AIComment
}

// aiCommentPatterns defines regex patterns that match AI instruction comments
// across multiple programming languages.
var aiCommentPatterns = []*regexp.Regexp{
	// Go/JS/TS/Rust/C single-line: // ai: ..., // AI: ..., // todo-ai: ...
	regexp.MustCompile(`^\s*//\s*(?i:ai|todo-ai)\s*:\s*(.+)$`),
	// Go/JS/TS/Rust/C block comment: /* ai: ... */
	regexp.MustCompile(`^\s*/\*\s*(?i:ai|todo-ai)\s*:\s*(.+?)\s*\*/$`),
	// Python/Ruby/Shell: # ai: ..., # AI: ..., # todo-ai: ...
	regexp.MustCompile(`^\s*#\s*(?i:ai|todo-ai)\s*:\s*(.+)$`),
	// HTML: <!-- ai: ... -->
	regexp.MustCompile(`^\s*<!--\s*(?i:ai|todo-ai)\s*:\s*(.+?)\s*-->$`),
	// CSS block comment: /* ai: ... */  (same as above, already covered)
}

// languageByExt maps file extensions to language names.
var languageByExt = map[string]string{
	".go":   "go",
	".py":   "python",
	".js":   "javascript",
	".ts":   "typescript",
	".tsx":  "typescript",
	".jsx":  "javascript",
	".rs":   "rust",
	".c":    "c",
	".cpp":  "cpp",
	".h":    "c",
	".hpp":  "cpp",
	".rb":   "ruby",
	".sh":   "shell",
	".bash": "shell",
	".zsh":  "shell",
	".html": "html",
	".css":  "css",
	".scss": "css",
	".java": "java",
}

// NewAIWatcher creates a new AIWatcher for the given directory and file patterns.
// If patterns is empty, defaults to common source file patterns.
func NewAIWatcher(rootDir string, patterns []string) *AIWatcher {
	if len(patterns) == 0 {
		patterns = []string{
			"*.go", "*.py", "*.js", "*.ts", "*.tsx", "*.jsx",
			"*.rs", "*.c", "*.cpp", "*.h", "*.hpp",
			"*.rb", "*.sh", "*.bash", "*.zsh",
			"*.html", "*.css", "*.scss", "*.java",
		}
	}
	return &AIWatcher{
		RootDir:  rootDir,
		Patterns: patterns,
		Debounce: 2 * time.Second,
		done:     make(chan struct{}),
		known:    make(map[string]AIComment),
	}
}

// ScanFile scans a single file for AI comments and returns all found.
// The path should be an absolute or relative path to the file.
func ScanFile(path string) []AIComment {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if scanner.Err() != nil {
		return nil
	}

	ext := filepath.Ext(path)
	lang := languageByExt[ext]

	var comments []AIComment
	for i, line := range lines {
		instruction, matched := matchAIComment(line)
		if !matched {
			continue
		}

		// Extract context: 5 lines before and 5 after
		contextStart := i - 5
		if contextStart < 0 {
			contextStart = 0
		}
		contextEnd := i + 6 // exclusive, so 5 after
		if contextEnd > len(lines) {
			contextEnd = len(lines)
		}
		contextLines := lines[contextStart:contextEnd]
		contextStr := strings.Join(contextLines, "\n")

		comments = append(comments, AIComment{
			File:     path,
			Line:     i + 1, // 1-based
			Comment:  instruction,
			Language: lang,
			Context:  contextStr,
			Marker:   line,
		})
	}

	return comments
}

// matchAIComment checks if a line matches any AI comment pattern and returns
// the instruction text if matched.
func matchAIComment(line string) (string, bool) {
	for _, pattern := range aiCommentPatterns {
		matches := pattern.FindStringSubmatch(line)
		if matches != nil {
			return strings.TrimSpace(matches[1]), true
		}
	}
	return "", false
}

// ScanDirectory walks a directory tree and scans all files matching the given
// patterns for AI comments.
func ScanDirectory(dir string, patterns []string) []AIComment {
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}

	var allComments []AIComment

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			// Skip common non-source directories
			if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		if !matchesAnyPattern(filepath.Base(path), patterns) {
			return nil
		}

		comments := ScanFile(path)
		for i := range comments {
			// Store relative path from the scan root
			rel, relErr := filepath.Rel(dir, path)
			if relErr == nil {
				comments[i].File = rel
			}
		}
		allComments = append(allComments, comments...)
		return nil
	})

	return allComments
}

// matchesAnyPattern checks if a filename matches any of the given glob patterns.
func matchesAnyPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, name)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// BuildPrompt constructs a prompt for the AI agent based on the detected comment.
func BuildPrompt(comment AIComment) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("The file %s at line %d has an AI instruction comment:\n\n", comment.File, comment.Line))

	// Show context with the comment line highlighted
	contextLines := strings.Split(comment.Context, "\n")
	for _, cl := range contextLines {
		if strings.TrimSpace(cl) == strings.TrimSpace(comment.Marker) {
			b.WriteString(fmt.Sprintf(">>> %s <<<\n", cl))
		} else {
			b.WriteString(fmt.Sprintf("    %s\n", cl))
		}
	}

	b.WriteString(fmt.Sprintf("\nInstruction: %s\n\n", comment.Comment))
	b.WriteString("Please implement this change. After making the change, remove the AI comment.\n")

	return b.String()
}

// Start begins watching the directory for AI comments. It uses filesystem polling
// at the configured Debounce interval. It blocks until ctx is cancelled or Stop
// is called.
func (w *AIWatcher) Start(ctx context.Context) error {
	// Do an initial scan
	w.mu.Lock()
	initial := ScanDirectory(w.RootDir, w.Patterns)
	for _, c := range initial {
		hash := commentHash(c)
		w.known[hash] = c
		if w.OnComment != nil {
			w.OnComment(c)
		}
	}
	w.mu.Unlock()

	ticker := time.NewTicker(w.Debounce)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.done:
			return nil
		case <-ticker.C:
			w.poll()
		}
	}
}

// poll scans the directory and detects new or removed AI comments.
func (w *AIWatcher) poll() {
	current := ScanDirectory(w.RootDir, w.Patterns)

	currentMap := make(map[string]AIComment, len(current))
	for _, c := range current {
		hash := commentHash(c)
		currentMap[hash] = c
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Detect new comments
	for hash, c := range currentMap {
		if _, exists := w.known[hash]; !exists {
			w.known[hash] = c
			if w.OnComment != nil {
				w.OnComment(c)
			}
		}
	}

	// Remove resolved comments (no longer present in files)
	for hash := range w.known {
		if _, exists := currentMap[hash]; !exists {
			delete(w.known, hash)
		}
	}
}

// Stop signals the watcher to stop polling.
func (w *AIWatcher) Stop() {
	select {
	case <-w.done:
		// already closed
	default:
		close(w.done)
	}
}

// RemoveComment removes the AI comment from the specified file at the given line.
// It matches the marker string to ensure the correct line is removed.
func RemoveComment(file string, line int, marker string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("reading file %s: %w", file, err)
	}

	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return fmt.Errorf("line %d out of range (file has %d lines)", line, len(lines))
	}

	// Verify the marker matches the line content
	if strings.TrimSpace(lines[line-1]) != strings.TrimSpace(marker) {
		return fmt.Errorf("marker mismatch at line %d: expected %q, got %q", line, strings.TrimSpace(marker), strings.TrimSpace(lines[line-1]))
	}

	// Check if the entire line is the AI comment (remove the line entirely)
	trimmed := strings.TrimSpace(lines[line-1])
	_, isAI := matchAIComment(trimmed)
	if isAI {
		// Remove the entire line
		lines = append(lines[:line-1], lines[line:]...)
	} else {
		// Remove just the AI comment portion (for inline comments)
		for _, pattern := range aiCommentPatterns {
			lines[line-1] = pattern.ReplaceAllString(lines[line-1], "")
		}
		lines[line-1] = strings.TrimRight(lines[line-1], " \t")
	}

	return os.WriteFile(file, []byte(strings.Join(lines, "\n")), 0o644)
}

// commentHash produces a unique hash for a comment based on file, line, and text.
func commentHash(c AIComment) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%d:%s", c.File, c.Line, c.Comment)))
	return fmt.Sprintf("%x", h.Sum(nil))
}
